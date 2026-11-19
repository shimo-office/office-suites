package main

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type releaseBrokenDownload struct{ sent bool }

func (b *releaseBrokenDownload) Read(p []byte) (int, error) {
	if !b.sent {
		b.sent = true
		return copy(p, "partial"), nil
	}
	return 0, errors.New("synthetic stream interrupted")
}
func (b *releaseBrokenDownload) Close() error { return nil }

func TestReleaseFileFailedStreamPreservesExistingDestination(t *testing.T) {
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: &releaseBrokenDownload{}}, nil
	})
	dest := filepath.Join(t.TempDir(), "existing.txt")
	if err := os.WriteFile(dest, []byte("原始完整内容"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := fetchToFile("https://download.invalid/content", dest)
	if err == nil {
		t.Fatal("interrupted stream reported success")
	}
	got, readErr := os.ReadFile(dest)
	if readErr != nil || string(got) != "原始完整内容" {
		t.Fatalf("failed download destroyed original: content=%q read=%v", got, readErr)
	}
}

func TestReleaseFileReadRejectsNegativeLimitWithoutPanic(t *testing.T) {
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/files/abcdefgh":
			return phase3Response(200, `{"guid":"abcdefgh","name":"文本.txt","isCloudFile":true}`), nil
		case "/api/v1/files/abcdefgh/download":
			return phase3Response(200, `{"resourceURL":"https://download.invalid/content.txt"}`), nil
		case "/content.txt":
			return phase3Response(200, "中文"), nil
		}
		return nil, errors.New("unexpected request " + r.URL.String())
	})
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("negative --max-chars panicked: %v", r)
		}
	}()
	out, err := issueCapture(t, newApp([]string{"read", "abcdefgh", "--max-chars", "-1", "--json"}).Run)
	if err == nil || strings.Contains(out, `"ok": true`) {
		t.Fatalf("negative limit must fail: out=%s err=%v", out, err)
	}
}

var _ io.ReadCloser = (*releaseBrokenDownload)(nil)
