package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestReleaseFileReadProductionPipeline(t *testing.T) {
	for _, kind := range []string{"cloud-json", "cloud-text", "cloud-full", "cloud-unsupported", "cloud-download-error", "cloud-address-error", "unknown-type", "info-error", "document", "documentPro", "spreadsheet", "presentation", "bad-package", "export-error", "export-download-error"} {
		t.Run(kind, func(t *testing.T) {
			typ := ""
			name := "文本.txt"
			payload := "  中文😀边界\r\n第二行  "
			switch kind {
			case "unknown-type":
				typ = "other"
			case "document", "export-error", "export-download-error":
				typ = "document"
			case "documentPro", "bad-package":
				typ = "documentPro"
			case "spreadsheet":
				typ = "spreadsheet"
			case "presentation":
				typ = "presentation"
			}
			if typ == "documentPro" && kind != "bad-package" {
				payload = string(issueDocx(t))
			}
			if typ == "spreadsheet" {
				payload = string(releaseFileWorkbook(t, `<row r="1"><c r="A1" t="inlineStr"><is><t>中文单元格</t></is></c></row>`))
			}
			if typ == "presentation" {
				p := artifactTestZip(t, map[string]string{"ppt/slides/slide1.xml": `<p:sld xmlns:p="urn:p" xmlns:a="urn:a"><a:p><a:r><a:t>中文幻灯片</a:t></a:r></a:p></p:sld>`})
				b, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				payload = string(b)
			}
			issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/api/v1/files/abcdefgh":
					if kind == "info-error" {
						return phase3Response(403, `{}`), nil
					}
					b, _ := json.Marshal(map[string]any{"guid": "abcdefgh", "name": name, "type": typ, "isCloudFile": strings.HasPrefix(kind, "cloud")})
					return phase3Response(200, string(b)), nil
				case "/api/v1/files/abcdefgh/download":
					if kind == "cloud-address-error" {
						return phase3Response(403, `{}`), nil
					}
					ext := "txt"
					if kind == "cloud-unsupported" {
						ext = "pdf"
					}
					return phase3Response(200, `{"resourceURL":"https://download.invalid/content.`+ext+`"}`), nil
				case "/api/v1/files/export/abcdefgh":
					if kind == "export-error" {
						return phase3Response(403, `{}`), nil
					}
					return phase3Response(200, `{"data":{"taskId":"task"}}`), nil
				case "/api/v1/files/export/progress":
					return phase3Response(200, `{"data":{"progress":100,"downloadUrl":"https://download.invalid/content"}}`), nil
				case "/content", "/content.txt", "/content.pdf":
					if kind == "cloud-download-error" || kind == "export-download-error" {
						return phase3Response(502, "failure"), nil
					}
					return phase3Response(200, payload), nil
				}
				return nil, errors.New("unexpected request " + r.URL.String())
			})
			args := []string{"read", "abcdefgh", "--max-chars", "3"}
			if kind != "cloud-text" {
				args = append(args, "--json")
			}
			if kind == "cloud-full" || kind == "spreadsheet" || kind == "presentation" {
				args = []string{"read", "abcdefgh", "--json"}
			}
			out, err := issueCapture(t, newApp(args).Run)
			failed := strings.Contains(kind, "error") || kind == "unknown-type" || kind == "cloud-unsupported" || kind == "bad-package"
			if failed {
				if err == nil || strings.Contains(out, `"ok": true`) {
					t.Fatalf("false success: %s %v", out, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !utf8.ValidString(out) || strings.Contains(out, "�") {
				t.Fatalf("invalid UTF-8: %q", out)
			}
			if kind == "cloud-text" {
				if !strings.Contains(out, "中文😀") || !strings.Contains(out, "已截断") {
					t.Fatalf("bad readable truncation: %s", out)
				}
				return
			}
			var got map[string]any
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatal(err)
			}
			if got["ok"] != true {
				t.Fatalf("missing success: %s", out)
			}
			if kind == "cloud-json" || kind == "document" {
				if got["text"] != "中文😀" || got["truncated"] != true || got["chars"] != float64(9) {
					t.Fatalf("incorrect rune truncation: %s", out)
				}
			}
			if expected := map[string]string{"documentPro": "真实导", "spreadsheet": "## Sheet 1\n中文单元格", "presentation": "## 幻灯片 1\n中文幻灯片"}[kind]; expected != "" && got["text"] != expected {
				t.Fatalf("wrong extracted text: %s", out)
			}
			if kind == "cloud-full" && got["text"] != "中文😀边界\n第二行" {
				t.Fatalf("incorrect normalization: %s", out)
			}
		})
	}
}

func TestReleaseFileGetAndPullProductionBoundaries(t *testing.T) {
	for _, kind := range []string{"get-json", "get-text", "get-folder", "get-info-error", "get-download-error", "get-url-fallback", "get-bin", "get-directory-error", "pull-text", "pull-json", "pull-empty", "pull-manifest-error", "pull-directory-error", "pull-list-error", "pull-bad-since"} {
		t.Run(kind, func(t *testing.T) {
			outDir := t.TempDir()
			dest := filepath.Join(outDir, "中文.txt")
			if strings.HasSuffix(kind, "directory-error") {
				outDir = filepath.Join(outDir, "blocked")
				if err := os.WriteFile(outDir, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "pull-manifest-error" {
				if err := os.Mkdir(filepath.Join(outDir, "manifest.json"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(dest, []byte("原内容"), 0600); err != nil {
				t.Fatal(err)
			}
			issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/api/v1/files/abcdefgh":
					if kind == "get-info-error" {
						return phase3Response(403, `{}`), nil
					}
					if kind == "get-folder" {
						return phase3Response(200, `{"guid":"abcdefgh","name":"目录","isFolder":true}`), nil
					}
					name := "中文.txt"
					if kind == "get-bin" {
						name = "中文"
					}
					return phase3Response(200, `{"guid":"abcdefgh","name":"`+name+`","isCloudFile":true}`), nil
				case "/api/v1/files":
					if kind == "pull-list-error" {
						return phase3Response(403, `{}`), nil
					}
					if kind == "pull-empty" || kind == "pull-manifest-error" {
						return phase3Response(200, `{"list":[]}`), nil
					}
					return phase3Response(200, `{"list":[{"guid":"abcdefgh","name":"中文.txt","isCloudFile":true},{"guid":"folderid","name":"目录","isFolder":true}]}`), nil
				case "/api/v1/files/abcdefgh/download":
					if kind == "get-url-fallback" {
						if r.URL.RawQuery != "" {
							return phase3Response(404, `{}`), nil
						}
						res := phase3Response(302, "")
						res.Header.Set("Location", "https://download.invalid/content.txt")
						return res, nil
					}
					path := "content.txt"
					if kind == "get-bin" {
						path = "content"
					}
					return phase3Response(200, `{"resourceURL":"https://download.invalid/`+path+`"}`), nil
				case "/content.txt", "/content":
					if kind == "get-download-error" {
						return phase3Response(503, "failure"), nil
					}
					return phase3Response(200, "下载中文😀"), nil
				}
				return nil, errors.New("unexpected request " + r.URL.String())
			})
			cmd := "get"
			target := "abcdefgh"
			if strings.HasPrefix(kind, "pull") {
				cmd = "pull"
				target = "desktop"
			}
			args := []string{cmd, target, "--out", outDir}
			if !strings.HasSuffix(kind, "text") {
				args = append(args, "--json")
			}
			if kind == "pull-bad-since" {
				args = append(args, "--since", "not-a-date")
			}
			out, err := issueCapture(t, newApp(args).Run)
			failed := strings.Contains(kind, "error") || kind == "get-folder" || kind == "pull-bad-since"
			if failed {
				if err == nil {
					t.Fatalf("false success: %s", out)
				}
				if kind == "get-download-error" {
					b, _ := os.ReadFile(dest)
					if string(b) != "原内容" {
						t.Fatalf("failed download changed file: %q", b)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if kind != "pull-empty" {
				if kind == "get-bin" {
					dest = filepath.Join(outDir, "中文.bin")
				}
				b, e := os.ReadFile(dest)
				if e != nil || string(b) != "下载中文😀" {
					t.Fatalf("download mismatch %q %v", b, e)
				}
			}
			if strings.HasPrefix(kind, "pull") {
				b, e := os.ReadFile(filepath.Join(outDir, "manifest.json"))
				if e != nil {
					t.Fatal(e)
				}
				var m map[string]any
				if e = json.Unmarshal(b, &m); e != nil {
					t.Fatal(e)
				}
				if m["ok"] != true || m["recursive"] != false {
					t.Fatalf("wrong manifest: %s", b)
				}
			}
		})
	}
}

func TestReleaseFileInvalidArgumentsDoNotFetch(t *testing.T) {
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/v1/files/spaces" {
			return phase3Response(200, `{"spaces":[]}`), nil
		}
		t.Errorf("invalid arguments fetched %s", r.URL)
		return nil, errors.New("unexpected network")
	})
	for _, args := range [][]string{{"get"}, {"pull"}, {"read"}, {"get", "@missing"}, {"read", "@missing"}, {"get", "abcdefgh", "--format", "csv"}, {"read", "abcdefgh", "--max-chars", "0"}, {"read", "abcdefgh", "--max-chars", "1oops"}, {"read", "abcdefgh", "--max-chars", ""}} {
		_, err := issueCapture(t, newApp(args).Run)
		if err == nil {
			t.Errorf("accepted %v", args)
		}
	}
	if _, err := newApp(nil).readText("abcdefgh", -1); err == nil {
		t.Error("internal negative limit accepted")
	}
}

func TestReleaseFileFetchBoundaryFailures(t *testing.T) {
	for _, kind := range []string{"mkdir", "network", "status", "rename", "stream-new", "success-mode"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			dest := filepath.Join(dir, "output.txt")
			if kind == "mkdir" {
				if err := os.WriteFile(dest, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
				dest = filepath.Join(dest, "nested.txt")
			}
			if kind == "rename" {
				if err := os.Mkdir(dest, 0700); err != nil {
					t.Fatal(err)
				}
			}
			var beforeMode os.FileMode
			if kind == "success-mode" {
				if err := os.WriteFile(dest, []byte("old"), 0640); err != nil {
					t.Fatal(err)
				}
				before, err := os.Stat(dest)
				if err != nil || !before.Mode().IsRegular() {
					t.Fatalf("initial destination is not a regular file: %v %v", before, err)
				}
				beforeMode = before.Mode().Perm()
			}
			issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
				if kind == "network" {
					return nil, errors.New("synthetic network failure")
				}
				if kind == "status" {
					return phase3Response(503, "unavailable"), nil
				}
				if kind == "stream-new" {
					return &http.Response{StatusCode: 200, Header: http.Header{}, Body: &releaseBrokenDownload{}}, nil
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("complete"))}, nil
			})
			n, err := fetchToFile("https://download.invalid/content", dest)
			if kind == "success-mode" {
				if err != nil || n != 8 {
					t.Fatalf("success failed n=%d err=%v", n, err)
				}
				info, e := os.Stat(dest)
				if e != nil || !info.Mode().IsRegular() || info.Mode().Perm() != beforeMode {
					t.Fatalf("changed permissions from %v: %v %v", beforeMode, info, e)
				}
			} else if err == nil {
				t.Fatal("false success")
			}
			if kind == "stream-new" {
				if _, e := os.Stat(dest); !os.IsNotExist(e) {
					t.Errorf("partial destination left: %v", e)
				}
			}
			temps, e := filepath.Glob(filepath.Join(dir, ".shimo-download-*"))
			if e != nil || len(temps) != 0 {
				t.Errorf("temporary files retained: %v %v", temps, e)
			}
		})
	}
}
