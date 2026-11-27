package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestPullUnsupportedIsNotReclassifiedAsFailed(t *testing.T) {
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/v1/files" {
			return phase3Response(200, `{"list":[{"guid":"folderid","name":"子目录","isFolder":true},{"guid":"formguid","name":"表单","type":"form"}]}`), nil
		}
		if r.URL.Path == "/api/v1/files/export/formguid" {
			return phase3Response(400, `{"code":3613,"msg":"暂不支持该导出类型"}`), nil
		}
		t.Fatalf("unexpected request %s", r.URL)
		return nil, errors.New("unexpected")
	})
	out := t.TempDir()
	a := newApp([]string{"pull", "desktop", "--out", out, "--json"})
	if err := a.cmdPull(); err != nil {
		t.Fatalf("unsupported is not a failed download: %v", err)
	}
	raw, err := os.ReadFile(issuePullManifestPath(t, out))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err = json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["complete"] != false || num(m, "downloaded") != 0 || num(m, "failed") != 0 || num(m, "unsupported") != 1 || num(m, "skipped") != 1 {
		t.Fatalf("nonexclusive counters: %s", raw)
	}
	for _, v := range asSlice(m["files"]) {
		f := asMap(v)
		if str(f, "status") == "unsupported" && str(f, "error") != "" {
			t.Fatalf("unsupported checked as file: %v", f)
		}
	}
}

func TestPptxTextExcludesTimingMetadata(t *testing.T) {
	p := releasePhase2ZIP(t, map[string]string{"ppt/slides/slide1.xml": `<p:sld xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>Visible title</a:t></a:r><a:fld><a:t>2</a:t></a:fld></a:p></p:txBody></p:sp></p:spTree></p:cSld><p:timing><p:attrName>style.visibility</p:attrName><p:strVal>hidden</p:strVal></p:timing></p:sld>`}, "")
	got, err := extractPptx(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != "## 幻灯片 1\nVisible title 2" || strings.Contains(got, "style.visibility") {
		t.Fatalf("metadata leaked into body: %q", got)
	}
}
