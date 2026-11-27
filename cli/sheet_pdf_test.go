package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSheetPDFDownloadCompletionRequiresMatchingEvent(t *testing.T) {
	d := sheetPDFDownload{}
	d.handle(map[string]any{"method": "Browser.downloadProgress", "params": map[string]any{"guid": "other", "state": "completed"}})
	if d.completed {
		t.Fatal("unrelated completion accepted")
	}
	d.handle(map[string]any{"method": "Browser.downloadWillBegin", "params": map[string]any{"guid": "12345678-abcd-1234-abcd-123456789012", "suggestedFilename": "example.pdf"}})
	d.handle(map[string]any{"method": "Browser.downloadProgress", "params": map[string]any{"guid": "12345678-abcd-1234-abcd-123456789012", "state": "inProgress"}})
	if d.completed {
		t.Fatal("partial download accepted")
	}
	d.handle(map[string]any{"method": "Browser.downloadProgress", "params": map[string]any{"guid": "12345678-abcd-1234-abcd-123456789012", "state": "completed"}})
	if !d.completed || d.err != nil {
		t.Fatalf("completion not recognized: %+v", d)
	}
}

func TestSheetPDFRejectsPartialAndWrongContent(t *testing.T) {
	for _, body := range []string{"<html>login</html>", "%PDF-1.7\npartial", ""} {
		p := filepath.Join(t.TempDir(), "result")
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := verifySheetPDF(p); err == nil {
			t.Fatalf("accepted incomplete PDF %q", body)
		}
	}
	body := "%PDF-1.7\n" + strings.Repeat("% test-only placeholder\n", 8) + "%%EOF\n"
	p := filepath.Join(t.TempDir(), "result")
	os.WriteFile(p, []byte(body), 0600)
	n, sha, err := verifySheetPDF(p)
	if err != nil || n != int64(len(body)) || len(sha) != 64 {
		t.Fatalf("envelope verification failed: %d %s %v", n, sha, err)
	}
}

func TestSheetPDFUniqueDestinationNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "download")
	os.WriteFile(src, []byte("pdf"), 0600)
	existing := filepath.Join(dir, "report.pdf")
	os.WriteFile(existing, []byte("keep"), 0600)
	got, err := publishSheetPDF(src, dir, "../report.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(got) != dir || got == existing {
		t.Fatalf("unsafe destination %s", got)
	}
	data, _ := os.ReadFile(existing)
	if string(data) != "keep" {
		t.Fatal("existing file overwritten")
	}
}
