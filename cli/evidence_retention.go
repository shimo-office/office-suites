package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	evidenceMarker         = ".shimo-evidence.json"
	evidenceMaxAge         = 7 * 24 * time.Hour
	evidenceMaxBytes int64 = 256 << 20
)

type evidenceRetention struct {
	Owner       string    `json:"owner"`
	CompletedAt time.Time `json:"completedAt"`
}

// Only newly created, explicitly marked automatic evidence is collected.
// Unmarked legacy files, requested exports and active directories are untouched.
func newManagedEvidenceDir(prefix string) (string, error) {
	if prefix != "shimo-phase3-" && prefix != "shimo-phase3-startup-" && prefix != "shimo-phase3-browser-failure-" {
		return "", errors.New("无效的临时证据目录类型")
	}
	root, err := filepath.Abs(os.TempDir())
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(root, prefix)
	if err != nil {
		return "", err
	}
	if err := phase3WriteEvidence(filepath.Join(dir, evidenceMarker), evidenceRetention{Owner: "shimo-net/evidence-v1"}); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

func readEvidenceRetention(dir string) (evidenceRetention, error) {
	var record evidenceRetention
	info, err := os.Lstat(dir)
	if err != nil {
		return record, err
	}
	if !info.IsDir() {
		return record, errors.New("证据路径不是目录")
	}
	path := filepath.Join(dir, evidenceMarker)
	info, err = os.Lstat(path)
	if err != nil {
		return record, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1024 {
		return record, errors.New("无效的证据保留标记")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		return record, err
	}
	if record.Owner != "shimo-net/evidence-v1" {
		return record, errors.New("证据目录不受自动清理管理")
	}
	return record, nil
}

// Call only when all writers have stopped. The current result remains available;
// the next managed operation applies age/capacity limits to completed evidence.
func retainManagedEvidence(dir string) error {
	record, err := readEvidenceRetention(dir)
	if err != nil {
		return err
	}
	record.CompletedAt = time.Now().UTC()
	return phase3WriteEvidence(filepath.Join(dir, evidenceMarker), record)
}

// Only call for a known-owned directory after RemoveAll partially failed.
// RemoveAll may already have removed the marker before encountering a locked
// child. Recreate it exclusively so the remainder can be collected next time.
func retainCleanupRemainder(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("清理残留不再是普通目录")
	}
	path := filepath.Join(dir, evidenceMarker)
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return retainManagedEvidence(dir)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	writeErr := json.NewEncoder(f).Encode(evidenceRetention{Owner: "shimo-net/evidence-v1", CompletedAt: time.Now().UTC()})
	return errors.Join(writeErr, f.Close())
}

// Resolve existing ancestors too: --out may name a new child of a symlink.
func resolveEvidencePath(path string) (string, error) {
	abs, err := filepath.Abs(expandHome(path))
	if err != nil {
		return "", err
	}
	probe := abs
	for {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			tail, err := filepath.Rel(probe, abs)
			if err != nil {
				return "", err
			}
			return filepath.Join(resolved, tail), nil
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return abs, nil
		}
		probe = parent
	}
}

func evidenceContains(dir, path string) bool {
	roots, err := evidencePathVariants(dir)
	if err != nil {
		return false
	}
	targets, err := evidencePathVariants(path)
	if err != nil {
		return false
	}
	for _, root := range roots {
		for _, target := range targets {
			rel, err := filepath.Rel(root, target)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return true
			}
		}
	}
	return false
}

func evidencePathVariants(path string) ([]string, error) {
	abs, err := filepath.Abs(expandHome(path))
	if err != nil {
		return nil, err
	}
	resolved, err := resolveEvidencePath(abs)
	if err != nil {
		return nil, err
	}
	return []string{abs, resolved}, nil
}

// Automatic evidence is a reserved, collectable namespace. Reject explicit
// output there even if its marker is missing: a concurrent collector may already
// have selected the directory. Check lexical and resolved paths so symlinks
// cannot place output inside it or make its public path depend on it.
func validateEvidenceOutput(parent string) error {
	paths, err := evidencePathVariants(parent)
	if err != nil {
		return err
	}
	roots, err := evidencePathVariants(os.TempDir())
	if err != nil {
		return err
	}
	for _, base := range roots {
		for _, target := range paths {
			rel, err := filepath.Rel(base, target)
			if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			name := strings.Split(rel, string(filepath.Separator))[0]
			if strings.HasPrefix(strings.ToLower(name), "shimo-phase3-") {
				return fmt.Errorf("--out 不能放在自动清理的临时证据目录内，请选择独立目录：%s", parent)
			}
		}
	}
	return nil
}

func pruneManagedEvidence(now time.Time, protected []string) error {
	return pruneEvidenceAt(os.TempDir(), now, evidenceMaxAge, evidenceMaxBytes, protected)
}

func pruneEvidenceAt(root string, now time.Time, maxAge time.Duration, maxBytes int64, protected []string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	type candidate struct {
		path      string
		completed time.Time
		bytes     int64
	}
	var candidates []candidate
	var total int64
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "shimo-phase3-") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		record, err := readEvidenceRetention(path)
		if err != nil || record.CompletedAt.IsZero() || record.CompletedAt.After(now) {
			continue
		}
		var size int64
		err = filepath.WalkDir(path, func(_ string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !info.IsDir() {
				size += info.Size()
			}
			return nil
		})
		if err != nil {
			continue
		}
		total += size
		candidates = append(candidates, candidate{path, record.CompletedAt, size})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].completed.Before(candidates[j].completed) })
	var failures []error
	for _, item := range candidates {
		keep := false
		for _, path := range protected {
			if evidenceContains(item.path, path) {
				keep = true
				break
			}
		}
		if keep || (now.Sub(item.completed) <= maxAge && total <= maxBytes) {
			continue
		}
		if err := os.RemoveAll(item.path); err != nil {
			failures = append(failures, err)
			if markErr := retainCleanupRemainder(item.path); markErr != nil {
				failures = append(failures, markErr)
			}
		} else {
			total -= item.bytes
		}
	}
	return errors.Join(failures...)
}
