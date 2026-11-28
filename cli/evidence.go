package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type phase3EvidenceRun struct {
	dirs       []string
	output     []byte
	jsonOutput bool
}

func (a *App) phase3EvidenceDir(parent string) (string, error) {
	// PDF/CSV are command deliverables, not disposable verification evidence.
	action := strings.TrimSpace(a.opt("--action"))
	if parent != "" {
		if err := validateEvidenceOutput(parent); err != nil {
			return "", err
		}
	}
	if parent != "" || a.evidence == nil || action == "slide-print-pdf" || action == "sheet-export-pdf" || action == "form-results" {
		return phase3EvidenceDir(parent)
	}
	dir, err := newManagedEvidenceDir("shimo-phase3-")
	if err == nil {
		a.evidence.dirs = append(a.evidence.dirs, dir)
	}
	return dir, err
}

func (a *App) withPhase3Evidence(fn func() error) error {
	// Artifact inspection may deliberately read a previous failure's exports.
	if len(a.Pos) > 0 && a.Pos[0] == "artifact" {
		return fn()
	}
	if out := a.opt("--out"); out != "" {
		if err := validateEvidenceOutput(out); err != nil {
			return err
		}
	}
	// Prune once before any current-command evidence is written. A later browser
	// allocation must not collect earlier diagnostics from this same command.
	if err := pruneManagedEvidence(time.Now(), a.evidenceInputs()); err != nil {
		fmt.Fprintf(os.Stderr, "警告：旧临时证据清理失败：%s\n", terminalText(err.Error()))
	}
	run := &phase3EvidenceRun{}
	a.evidence = run
	defer func() { a.evidence = nil }()
	if err := fn(); err != nil {
		return run.keepFailure(err)
	}
	out := run.output
	if run.jsonOutput && len(run.dirs) > 0 {
		var err error
		out, err = evidenceResultWithoutPaths(out, run.dirs)
		if err != nil {
			return run.keepFailure(err)
		}
	}
	if len(out) > 0 {
		if _, err := os.Stdout.Write(out); err != nil {
			return run.keepFailure(fmt.Errorf("结果输出失败，操作可能已完成；禁止自动重放：%w", err))
		}
	}
	// Handler defers (browser close and final logs) have already finished.
	for _, dir := range run.dirs {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintf(os.Stderr, "警告：操作已完成，临时证据未能清理（%s）：%s\n", terminalText(dir), terminalText(err.Error()))
			if err := retainCleanupRemainder(dir); err != nil {
				fmt.Fprintf(os.Stderr, "警告：无法标记临时证据保留期限：%s\n", terminalText(err.Error()))
			}
		}
	}
	return nil
}

func (a *App) evidenceInputs() []string {
	paths := []string{}
	if out := a.opt("--out"); out != "" {
		paths = append(paths, out)
	}
	action := strings.TrimSpace(a.opt("--action"))
	if action == "doc-image" || action == "slide-media-insert" {
		if args, err := phase2Args(a.opt("--payload")); err == nil {
			if payload, err := phase2ObjectArg(args); err == nil {
				if file := strings.TrimSpace(str(payload, "file")); file != "" {
					paths = append(paths, file)
				}
			}
		}
	}
	return paths
}

func (r *phase3EvidenceRun) keepFailure(err error) error {
	for _, dir := range r.dirs {
		if markErr := retainManagedEvidence(dir); markErr != nil {
			err = fmt.Errorf("%w；无法标记证据保留期限：%v", err, markErr)
		}
	}
	if len(r.dirs) > 0 {
		return fmt.Errorf("%w；临时证据保留（后续运行按 7 天/256 MiB 清理）：%s", err, strings.Join(r.dirs, ", "))
	}
	return err
}

func (a *App) printPhase3Result(result map[string]any, summary string) error {
	if a.JSON {
		return a.printJSON(result)
	}
	message := summary + "\n"
	if a.evidence == nil || len(a.evidence.dirs) == 0 {
		if dir := str(result, "evidenceDir"); dir != "" {
			message += "证据：" + terminalText(dir) + "\n"
		}
	}
	if limitation := str(result, "exportLimitation"); limitation != "" {
		message += limitation + "\n"
	}
	if a.evidence != nil {
		a.evidence.output = []byte(message)
		return nil
	}
	_, err := fmt.Print(message)
	return err
}

// Change only the command's evidence schema. User content (including nested
// fields named path/before/after) and artifact verification metadata stay intact.
func evidenceResultWithoutPaths(raw []byte, dirs []string) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	result, ok := value.(map[string]any)
	if !ok {
		return raw, nil
	}
	strip := func(m map[string]any, key string) {
		path, ok := m[key].(string)
		if !ok || !filepath.IsAbs(path) {
			return
		}
		for _, dir := range dirs {
			rel, err := filepath.Rel(dir, path)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				delete(m, key)
				return
			}
		}
	}
	for _, key := range []string{"evidenceDir", "before", "after", "screenshot"} {
		strip(result, key)
	}
	for _, key := range []string{"artifact", "beforeArtifact", "afterArtifact", "reloadArtifact", "reloadedArtifact"} {
		if artifact, ok := result[key].(map[string]any); ok {
			strip(artifact, "path")
		} else {
			strip(result, key)
		}
	}
	if steps, ok := result["completedSteps"].([]any); ok {
		for _, step := range steps {
			if item, ok := step.(map[string]any); ok {
				strip(item, "evidenceDir")
			}
		}
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
