package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func issueFileTransport(t *testing.T, fn func(*http.Request) (*http.Response, error)) {
	t.Helper()
	t.Setenv("SHIMO_NET_ORIGIN", "https://shimo.net")
	t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic-test")
	t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	oldClient, oldTransport := httpClient, http.DefaultTransport
	transport := phase3RoundTripFunc(fn)
	httpClient = &http.Client{Transport: transport}
	http.DefaultTransport = transport
	t.Cleanup(func() { httpClient, http.DefaultTransport = oldClient, oldTransport })
}

func issueDocx(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	f, err := z.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.WriteString(f, `<w:document xmlns:w="urn:test"><w:body><w:p><w:r><w:t>真实导出正文</w:t></w:r></w:p></w:body></w:document>`)
	if err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestIssueA10ReadDocumentFallsBackAndParsesActualDocx(t *testing.T) {
	payload := issueDocx(t)
	var attempts []string
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/files/abcdefgh":
			return phase3Response(200, `{"guid":"abcdefgh","name":"文档","type":"document"}`), nil
		case "/api/v1/files/export/abcdefgh":
			var data map[string]string
			if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
				t.Fatal(err)
			}
			attempts = append(attempts, data["type"])
			if data["type"] == "md" {
				return phase3Response(400, `{"error":"not supported"}`), nil
			}
			return phase3Response(200, `{"data":{"taskId":"task"}}`), nil
		case "/api/v1/files/export/progress":
			return phase3Response(200, `{"data":{"progress":100,"downloadUrl":"https://download.invalid/no-extension"}}`), nil
		case "/no-extension":
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(payload)), Header: http.Header{}}, nil
		default:
			t.Fatalf("unexpected request: %s", r.URL)
			return nil, errors.New("unexpected request")
		}
	})
	a := newApp([]string{"read", "abcdefgh"})
	result, err := a.readText("abcdefgh", 12000)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(attempts, []string{"md", "docx"}) || result["text"] != "真实导出正文" || result["via"] != "export docx" {
		t.Fatalf("wrong fallback result: attempts=%v result=%v", attempts, result)
	}
}

func TestIssueA10GetUsesActualFallbackExtension(t *testing.T) {
	var attempts []string
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/files/export/abcdefgh":
			var data map[string]string
			_ = json.NewDecoder(r.Body).Decode(&data)
			attempts = append(attempts, data["type"])
			if data["type"] != "pdf" {
				return phase3Response(400, `{}`), nil
			}
			return phase3Response(200, `{"data":{"taskId":"task"}}`), nil
		case "/api/v1/files/export/progress":
			return phase3Response(200, `{"data":{"progress":100,"downloadUrl":"https://download.invalid/content"}}`), nil
		case "/content":
			return phase3Response(200, "%PDF-test"), nil
		default:
			t.Fatalf("unexpected request: %s", r.URL)
			return nil, errors.New("unexpected request")
		}
	})
	a := newApp([]string{"get"})
	result, err := a.downloadOne(map[string]any{"guid": "abcdefgh", "name": "文档", "type": "document"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(attempts, []string{"md", "docx", "pdf"}) || !strings.HasSuffix(str(result, "file"), ".pdf") || result["exportType"] != "pdf" {
		t.Fatalf("wrong fallback: attempts=%v result=%v", attempts, result)
	}
}

func TestIssueA10ReadPDFDoesNotReportBinaryAsText(t *testing.T) {
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/files/abcdefgh":
			return phase3Response(200, `{"guid":"abcdefgh","name":"文档","type":"documentPro"}`), nil
		case "/api/v1/files/export/abcdefgh":
			var data map[string]string
			_ = json.NewDecoder(r.Body).Decode(&data)
			if data["type"] != "pdf" {
				return phase3Response(400, `{}`), nil
			}
			return phase3Response(200, `{"data":{"taskId":"task"}}`), nil
		case "/api/v1/files/export/progress":
			return phase3Response(200, `{"data":{"progress":100,"downloadUrl":"https://download.invalid/content"}}`), nil
		case "/content":
			return phase3Response(200, "%PDF-test"), nil
		default:
			t.Fatalf("unexpected request: %s", r.URL)
			return nil, errors.New("unexpected request")
		}
	})
	result, err := newApp([]string{"read"}).readText("abcdefgh", 12000)
	if err == nil || result != nil || !strings.Contains(strings.ToLower(err.Error()), "pdf") || !strings.Contains(err.Error(), "get") {
		t.Fatalf("PDF must give explicit unsupported extraction error: result=%v err=%v", result, err)
	}
}

func TestIssueA11ImportRenameFailureKeepsCreatedIdentity(t *testing.T) {
	uploads, imports, renames := 0, 0, 0
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/files/upload":
			uploads++
			return phase3Response(200, `{"guid":"cloudguid","name":"original.md"}`), nil
		case r.URL.Path == "/api/v1/files/import/cloudguid":
			imports++
			return phase3Response(200, `{"data":{"taskId":"task"}}`), nil
		case r.URL.Path == "/api/v1/files/import/progress":
			return phase3Response(200, `{"data":{"progress":100,"file":{"guid":"abcdefgh"}}}`), nil
		case r.URL.Path == "/api/v1/files/abcdefgh" && r.Method == "GET":
			return phase3Response(200, `{"guid":"abcdefgh","name":"original","type":"document"}`), nil
		case r.URL.Path == "/api/v1/files/abcdefgh" && r.Method == "PATCH":
			renames++
			return phase3Response(401, `{}`), nil
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
			return nil, errors.New("unexpected request")
		}
	})
	local := filepath.Join(t.TempDir(), "original.md")
	if err := os.WriteFile(local, []byte("text"), 0600); err != nil {
		t.Fatal(err)
	}
	a := newApp([]string{"import", local, "--to", "desktop", "--title", "requested", "--json", "--relogin"})
	err := a.Run()
	var partial *commandError
	if !errors.As(err, &partial) {
		t.Fatalf("must return structured partial failure: %v", err)
	}
	if !partial.NoReplay || partial.Result["guid"] != "abcdefgh" || partial.Result["name"] != "original" || partial.Result["renamed"] != false || partial.Result["imported"] != true || uploads != 1 || imports != 1 || renames != 1 {
		t.Fatalf("misleading/replayed partial import: %v requests=%d/%d/%d", partial, uploads, imports, renames)
	}
}

func TestIssueB8PullFailureAndB9SkippedDirectoryAreRecorded(t *testing.T) {
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/v1/files" {
			return phase3Response(200, `{"list":[{"guid":"folderid","name":"子目录","isFolder":true,"type":"folder"},{"guid":"fileguid","name":"失败文件","type":"spreadsheet"}]}`), nil
		}
		if r.URL.Path == "/api/v1/files/export/fileguid" {
			return phase3Response(403, `{}`), nil
		}
		t.Fatalf("unexpected request: %s", r.URL)
		return nil, errors.New("unexpected request")
	})
	out := t.TempDir()
	a := newApp([]string{"pull", "desktop", "--out", out, "--json"})
	err := a.cmdPull()
	var partial *commandError
	if !errors.As(err, &partial) {
		t.Fatalf("download failures require nonzero structured error: %v", err)
	}
	raw, err := os.ReadFile(issuePullManifestPath(t, out))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	files := asSlice(manifest["files"])
	if len(files) != 2 || asMap(files[0])["skipped"] != true || asMap(files[0])["guid"] != "folderid" || asMap(files[1])["error"] == nil || manifest["recursive"] != false || manifest["failed"] != float64(1) || manifest["skipped"] != float64(1) {
		t.Fatalf("incomplete manifest: %s", raw)
	}
}

func TestIssueB1CSVRejectedBeforeNetworkAndB4TypeLabel(t *testing.T) {
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		t.Fatalf("must reject before network: %s", r.URL)
		return nil, errors.New("unexpected request")
	})
	if err := newApp([]string{"get", "abcdefgh", "--format", "csv"}).cmdGet(); err == nil || !strings.Contains(err.Error(), "不支持") {
		t.Fatalf("CSV must explicitly fail: %v", err)
	}
	if err := newApp([]string{"new", "test", "--type", "bad"}).cmdNew(); err == nil || !strings.Contains(err.Error(), "table=应用表格") {
		t.Fatalf("table label ambiguous: %v", err)
	}
}

func TestIssueA10ExactExportDoesNotFallback(t *testing.T) {
	var attempts []string
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/files/export/abcdefgh" {
			t.Fatalf("unexpected request: %s", r.URL)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		attempts = append(attempts, body["type"])
		return phase3Response(400, `{}`), nil
	})
	_, err := newApp([]string{"phase3"}).exportFile("abcdefgh", "pdf")
	if err == nil || !reflect.DeepEqual(attempts, []string{"pdf"}) {
		t.Fatalf("exact export must not silently change format: attempts=%v err=%v", attempts, err)
	}
}

func TestIssueB8PullPreservesSuccessfulFileOnPartialFailure(t *testing.T) {
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/files":
			return phase3Response(200, `{"list":[{"guid":"goodfile","name":"good.txt","isCloudFile":true},{"guid":"badfile1","name":"bad","type":"spreadsheet"}]}`), nil
		case "/api/v1/files/goodfile/download":
			return phase3Response(200, `{"resourceURL":"https://download.invalid/good.txt"}`), nil
		case "/good.txt":
			return phase3Response(200, "preserved contents"), nil
		case "/api/v1/files/export/badfile1":
			return phase3Response(403, `{}`), nil
		default:
			t.Fatalf("unexpected request: %s", r.URL)
			return nil, errors.New("unexpected request")
		}
	})
	out := t.TempDir()
	err := newApp([]string{"pull", "desktop", "--out", out, "--json"}).Run()
	var partial *commandError
	if !errors.As(err, &partial) || partial.Result["downloaded"] != 1 || partial.Result["failed"] != 1 {
		t.Fatalf("must report partial failure: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(out, "good.txt"))
	if err != nil || string(data) != "preserved contents" {
		t.Fatalf("successful download lost: %q %v", data, err)
	}
}

func TestIssueB9DirectoryOnlyPullReportsScopeAndSkipped(t *testing.T) {
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/v1/files" {
			t.Fatalf("must not descend into subdirectory: %s", r.URL)
		}
		return phase3Response(200, `{"list":[{"guid":"folderid","name":"子目录","isFolder":true,"updatedAt":0}]}`), nil
	})
	out := t.TempDir()
	// A since filter applies to files; the skipped subdirectory must still be recorded.
	err := newApp([]string{"pull", "desktop", "--out", out, "--since", "7d"}).Run()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(issuePullManifestPath(t, out))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result["ok"] != true || result["recursive"] != false || result["downloaded"] != float64(0) || result["skipped"] != float64(1) {
		t.Fatalf("misleading directory-only result: %s", data)
	}
}
