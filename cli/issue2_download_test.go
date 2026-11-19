package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func issue2CloudFiles(t *testing.T, names, contents []string, beforeBody func(int)) {
	t.Helper()
	files := make([]map[string]any, len(names))
	for i, name := range names {
		files[i] = map[string]any{"guid": fmt.Sprintf("fixture%02d", i), "name": name, "isCloudFile": true}
	}
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/v1/files" {
			b, _ := json.Marshal(map[string]any{"list": files})
			return phase3Response(200, string(b)), nil
		}
		for i, f := range files {
			guid := str(f, "guid")
			switch r.URL.Path {
			case "/api/v1/files/" + guid:
				b, _ := json.Marshal(f)
				return phase3Response(200, string(b)), nil
			case "/api/v1/files/" + guid + "/download":
				return phase3Response(200, `{"resourceURL":"https://download.invalid/data/`+guid+`"}`), nil
			case "/data/" + guid:
				if beforeBody != nil {
					beforeBody(i)
				}
				return phase3Response(200, contents[i]), nil
			}
		}
		t.Fatalf("unexpected path %s", r.URL.Path)
		return nil, errors.New("unexpected request")
	})
}

func issue2ReadManifest(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err = json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestIssue2PullPreservesManifestDuplicatesAndPreviousRuns(t *testing.T) {
	names := []string{"manifest.json", "same.txt", "same.txt", "a/b.txt", "a?b.txt", " .shimo-pull-manifest-remote.json"}
	contents := []string{strings.Repeat("m", 6989), "same first", "same second", "slash", "question", "hidden-looking remote file"}
	for len(names) < 33 {
		i := len(names)
		names = append(names, fmt.Sprintf("file%02d.txt", i))
		contents = append(contents, fmt.Sprintf("contents %d", i))
	}
	issue2CloudFiles(t, names, contents, nil)
	out := t.TempDir()
	old := filepath.Join(out, "same.txt")
	if err := os.WriteFile(old, []byte("existing local file"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshots := map[string]string{old: "existing local file"}
	for run := 0; run < 2; run++ {
		raw, err := issueCapture(t, newApp([]string{"pull", "desktop", "--out", out, "--json"}).Run)
		if err != nil {
			t.Fatal(err)
		}
		var result map[string]any
		if err = json.Unmarshal([]byte(raw), &result); err != nil {
			t.Fatal(err)
		}
		if result["ok"] != true || result["downloaded"] != float64(33) || result["failed"] != float64(0) {
			t.Fatalf("bad result: %s", raw)
		}
		manifestPath := str(result, "manifest")
		if !strings.HasPrefix(filepath.Base(manifestPath), ".shimo-pull-manifest-") {
			t.Errorf("metadata path is not separate: %s", manifestPath)
		}
		if _, exists := snapshots[manifestPath]; exists {
			t.Errorf("reused existing manifest %s", manifestPath)
		}
		seen := map[string]bool{}
		for i, item := range asSlice(result["files"]) {
			f := asMap(item)
			name := str(f, "file")
			b, e := os.ReadFile(name)
			if e != nil || string(b) != contents[i] || num(f, "bytes") != float64(len(b)) {
				t.Errorf("item %d corrupted: file=%s got bytes=%d expected=%d err=%v", i, name, len(b), len(contents[i]), e)
			}
			if name == manifestPath || seen[name] {
				t.Errorf("output path collision: %s", name)
			}
			seen[name] = true
			if _, exists := snapshots[name]; exists {
				t.Errorf("overwrote previous output: %s", name)
			}
			snapshots[name] = contents[i]
		}
		m := issue2ReadManifest(t, manifestPath)
		if m["downloaded"] != float64(33) || m["ok"] != true {
			t.Fatalf("wrong persisted result: %v", m)
		}
		b, e := os.ReadFile(manifestPath)
		if e != nil {
			t.Fatal(e)
		}
		snapshots[manifestPath] = string(b)
		for name, want := range snapshots {
			b, e := os.ReadFile(name)
			if e != nil || string(b) != want {
				t.Errorf("prior file changed: %s %v", name, e)
			}
		}
	}
}

func TestIssue2PullDetectsFilesChangedAfterDownload(t *testing.T) {
	for _, fault := range []string{"truncate", "delete", "directory"} {
		t.Run(fault, func(t *testing.T) {
			out := t.TempDir()
			first := filepath.Join(out, "first.txt")
			issue2CloudFiles(t, []string{"first.txt", "second.txt"}, []string{"first complete content", "second content"}, func(i int) {
				if i != 1 {
					return
				}
				switch fault {
				case "truncate":
					if e := os.WriteFile(first, []byte("x"), 0600); e != nil {
						t.Fatal(e)
					}
				case "delete":
					if e := os.Remove(first); e != nil {
						t.Fatal(e)
					}
				case "directory":
					if e := os.Remove(first); e != nil {
						t.Fatal(e)
					}
					if e := os.Mkdir(first, 0700); e != nil {
						t.Fatal(e)
					}
				}
			})
			err := newApp([]string{"pull", "desktop", "--out", out, "--json"}).Run()
			var partial *commandError
			if !errors.As(err, &partial) {
				t.Fatalf("changed output reported success: %v", err)
			}
			if partial.Result["ok"] != false || partial.Result["downloaded"] != 1 || partial.Result["failed"] != 1 || !partial.NoReplay {
				t.Fatalf("wrong final counts: %#v", partial)
			}
			m := issue2ReadManifest(t, str(partial.Result, "manifest"))
			files := asSlice(m["files"])
			if m["ok"] != false || m["failed"] != float64(1) || str(asMap(files[0]), "error") == "" {
				t.Fatalf("manifest hides invalid output: %#v", m)
			}
		})
	}
}

func TestIssue2GetAvoidsDirectoryAndSymlinkCollisions(t *testing.T) {
	for _, kind := range []string{"directory", "symlink", "case-alias"} {
		t.Run(kind, func(t *testing.T) {
			out := t.TempDir()
			target := filepath.Join(out, "same.txt")
			if kind == "directory" {
				if e := os.Mkdir(target, 0700); e != nil {
					t.Fatal(e)
				}
			} else if kind == "symlink" {
				outside := filepath.Join(t.TempDir(), "outside")
				if e := os.WriteFile(outside, []byte("outside"), 0600); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(outside, target); e != nil {
					t.Skipf("symlink unavailable: %v", e)
				}
			} else {
				if e := os.WriteFile(filepath.Join(out, "SAME.txt"), []byte("case alias"), 0600); e != nil {
					t.Fatal(e)
				}
				if _, e := os.Lstat(target); os.IsNotExist(e) {
					t.Skip("filesystem is case sensitive")
				}
			}
			issue2CloudFiles(t, []string{"same.txt"}, []string{"fresh"}, nil)
			raw, e := issueCapture(t, newApp([]string{"get", "fixture00", "--out", out, "--json"}).Run)
			if e != nil {
				t.Fatal(e)
			}
			var result map[string]any
			if e = json.Unmarshal([]byte(raw), &result); e != nil {
				t.Fatal(e)
			}
			if str(result, "file") == target {
				t.Fatal("reused occupied path")
			}
			b, e := os.ReadFile(str(result, "file"))
			if e != nil || string(b) != "fresh" {
				t.Fatalf("new download missing %q %v", b, e)
			}
			if kind == "directory" {
				s, e := os.Stat(target)
				if e != nil || !s.IsDir() {
					t.Fatal("directory replaced")
				}
			} else {
				want := "outside"
				if kind == "case-alias" {
					want = "case alias"
				}
				b, e := os.ReadFile(target)
				if e != nil || string(b) != want {
					t.Fatalf("existing target changed: %q %v", b, e)
				}
			}
		})
	}
}

func TestIssue2FailedDownloadCleansOnlyOwnReservation(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(fmt.Sprint(replace), func(t *testing.T) {
			out := t.TempDir()
			existing := filepath.Join(out, "same.txt")
			if e := os.WriteFile(existing, []byte("keep"), 0600); e != nil {
				t.Fatal(e)
			}
			var occupied string
			issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
				if strings.Contains(r.URL.Path, "/download") {
					return phase3Response(200, `{"resourceURL":"https://download.invalid/same.txt"}`), nil
				}
				if r.URL.Path == "/same.txt" {
					entries, e := os.ReadDir(out)
					if e != nil {
						t.Fatal(e)
					}
					for _, entry := range entries {
						if entry.Name() != "same.txt" {
							occupied = filepath.Join(out, entry.Name())
						}
					}
					if replace && occupied != "" {
						if e := os.Remove(occupied); e != nil {
							t.Fatal(e)
						}
						if e := os.WriteFile(occupied, []byte("other writer"), 0600); e != nil {
							t.Fatal(e)
						}
					}
					return phase3Response(503, "failure"), nil
				}
				t.Fatalf("unexpected path %s", r.URL.Path)
				return nil, errors.New("unexpected request")
			})
			_, e := newApp([]string{"get"}).downloadOne(map[string]any{"guid": "fixture00", "name": "same.txt", "isCloudFile": true}, out)
			if e == nil {
				t.Fatal("expected failed download")
			}
			b, e := os.ReadFile(existing)
			if e != nil || string(b) != "keep" {
				t.Fatalf("existing output changed %q %v", b, e)
			}
			if occupied == "" {
				t.Fatal("download did not exclusively reserve an unused name")
			}
			if replace {
				b, e := os.ReadFile(occupied)
				if e != nil || string(b) != "other writer" {
					t.Fatalf("removed replacement %q %v", b, e)
				}
			} else if _, e := os.Stat(occupied); !os.IsNotExist(e) {
				t.Fatalf("left failed reservation: %v", e)
			}
		})
	}
}

func issuePullManifestPath(t *testing.T, outDir string) string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(outDir, ".shimo-pull-manifest-*.json"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("expected one independent manifest: %v %v", paths, err)
	}
	return paths[0]
}

func TestIssue2OnlineExportsAlsoAvoidExistingFiles(t *testing.T) {
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/files/export/fixture00":
			return phase3Response(200, `{"data":{"taskId":"task"}}`), nil
		case "/api/v1/files/export/progress":
			return phase3Response(200, `{"data":{"progress":100,"downloadUrl":"https://download.invalid/data"}}`), nil
		case "/data":
			return phase3Response(200, "export bytes"), nil
		}
		t.Fatalf("unexpected path %s", r.URL.Path)
		return nil, errors.New("unexpected request")
	})
	out := t.TempDir()
	existing := filepath.Join(out, "presentation.pptx")
	if err := os.WriteFile(existing, []byte("existing presentation"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := newApp([]string{"get"}).downloadOne(map[string]any{"guid": "fixture00", "name": "presentation", "type": "presentation"}, out)
	if err != nil {
		t.Fatal(err)
	}
	if str(result, "file") == existing || result["exportType"] != "pptx" {
		t.Fatalf("bad export result: %v", result)
	}
	b, err := os.ReadFile(existing)
	if err != nil || string(b) != "existing presentation" {
		t.Fatalf("existing export changed: %q %v", b, err)
	}
	b, err = os.ReadFile(str(result, "file"))
	if err != nil || string(b) != "export bytes" {
		t.Fatalf("new export missing: %q %v", b, err)
	}
}
