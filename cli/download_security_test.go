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

const signedURLFixture = "https://download.invalid/private-object?signature=SYNTHETIC_SECRET"

type downloadFailureBody struct{}

func (downloadFailureBody) Read([]byte) (int, error) {
	return 0, errors.New("read failed: " + signedURLFixture)
}
func (downloadFailureBody) Close() error { return nil }

func assertNoSignedDownloadDetails(t *testing.T, text string) {
	t.Helper()
	for _, secret := range []string{"SYNTHETIC_SECRET", "signature", "private-object", "download.invalid"} {
		if strings.Contains(text, secret) {
			t.Errorf("download details leaked (%s): %s", secret, text)
		}
	}
}

func TestDownloadFailureDoesNotExposeSignedURL(t *testing.T) {
	for _, scenario := range []string{"transport", "parse", "redirect", "body"} {
		t.Run(scenario, func(t *testing.T) {
			old := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = old })
			http.DefaultTransport = phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch scenario {
				case "redirect":
					return &http.Response{StatusCode: 302, Header: http.Header{"Location": {signedURLFixture}}, Body: io.NopCloser(strings.NewReader(""))}, nil
				case "body":
					return &http.Response{StatusCode: 200, Header: http.Header{}, Body: downloadFailureBody{}}, nil
				default:
					return nil, errors.New("transport refused " + signedURLFixture)
				}
			})
			u := signedURLFixture
			if scenario == "parse" {
				u = "https://download.invalid/%zz?signature=SYNTHETIC_SECRET"
			}
			dest := filepath.Join(t.TempDir(), "document.pdf")
			if err := os.WriteFile(dest, []byte("keep existing file"), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := fetchToFile(u, dest)
			if err == nil {
				t.Fatal("expected download failure")
			}
			assertNoSignedDownloadDetails(t, err.Error())
			if data, e := os.ReadFile(dest); e != nil || string(data) != "keep existing file" {
				t.Fatalf("failed download changed destination: %q %v", data, e)
			}
			for _, jsonOutput := range []bool{false, true} {
				f, e := os.CreateTemp(t.TempDir(), "stderr-")
				if e != nil {
					t.Fatal(e)
				}
				oldErr := os.Stderr
				os.Stderr = f
				code := (&App{JSON: jsonOutput}).writeFailure(err)
				os.Stderr = oldErr
				_ = f.Close()
				data, e := os.ReadFile(f.Name())
				if e != nil || code != 1 {
					t.Fatalf("failure output: code %d, %v", code, e)
				}
				assertNoSignedDownloadDetails(t, string(data))
			}
		})
	}
}

func TestSlidePrintDownloadFailureEvidenceOmitsSignedURL(t *testing.T) {
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/files/abcdefgh":
			return phase3Response(200, `{"type":"presentation","name":"signed URL fixture"}`), nil
		case "/api/v1/files/export/abcdefgh":
			return phase3Response(200, `{"data":{"taskId":"synthetic-export"}}`), nil
		case "/api/v1/files/export/progress":
			return phase3Response(200, `{"data":{"progress":100,"downloadUrl":"`+signedURLFixture+`"}}`), nil
		case "/private-object":
			return nil, errors.New("transport refused " + signedURLFixture)
		default:
			t.Fatalf("unexpected API path %s", r.URL.Path)
			return nil, errors.New("unexpected request")
		}
	})
	out := t.TempDir()
	a := newApp([]string{"phase3", "https://shimo.net/presentation/abcdefgh", "--action", "slide-print-pdf", "--out", out, "--json"})
	if err := a.cmdPhase3SlidesPrint(); err == nil {
		t.Fatal("expected failed export")
	} else {
		assertNoSignedDownloadDetails(t, err.Error())
	}
	files, err := filepath.Glob(filepath.Join(out, "*", "failure.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("missing failure evidence: %v %v", files, err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	assertNoSignedDownloadDetails(t, string(data))
}
