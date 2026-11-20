package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestIssue3HelpNeverRunsCommand(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SHIMO_NET_STATE_DIR", dir)
	t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
	before := []byte("preserve credentials and config")
	path := filepath.Join(dir, "sentinel")
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	old := httpClient
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("help performed a network operation")
		return nil, errors.New("unexpected request")
	})}
	t.Cleanup(func() { httpClient = old })
	for _, args := range [][]string{
		{"init", "--help"}, {"logout", "-h"}, {"login", "--new", "--help"},
		{"session", "harvest", "--help"}, {"new", "title", "--to", "--help"},
		{"--json", "editor", "file", "--payload", "--help"},
	} {
		out, err := issueCapture(t, newApp(args).Run)
		if err != nil || !strings.Contains(out, "shimo-net") {
			t.Fatalf("help %v: %q %v", args, out, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("help changed state: %v %v", entries, err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("help changed existing state")
	}
}

func TestIssue3UnknownAndMissingFlagsFailBeforeExecution(t *testing.T) {
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
	for _, args := range [][]string{
		{"version", "--typo"}, {"schema", "--originn", "https://example.invalid"},
		{"new", "title", "--to"}, {"whoami", "--payload", "{}"}, {"logout", "-x"},
	} {
		err := newApp(args).Run()
		var detail *commandError
		if !errors.As(err, &detail) || detail.ExitCode != 2 {
			t.Fatalf("%v: expected usage error, got %v", args, err)
		}
	}
}

func TestIssue3FlagsPreserveValuesAndPositionals(t *testing.T) {
	a := newApp([]string{"init", "--login", "--origin=https://example.invalid", "--json"})
	if !a.has("--login") || a.opt("--origin") != "https://example.invalid" || !a.JSON {
		t.Fatalf("%+v", a)
	}
	a = newApp([]string{"read", "file", "--max-chars", "-1"})
	if a.opt("--max-chars") != "-1" || len(a.Pos) != 1 {
		t.Fatalf("%+v", a)
	}
	a = newApp([]string{"search", "--", "--help"})
	if a.Cmd != "search" || len(a.Pos) != 1 || a.Pos[0] != "--help" {
		t.Fatalf("%+v", a)
	}
}

func TestIssue3Phase3VerifyKeepsDocumentedCompatibility(t *testing.T) {
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
	baseline, err := issueCapture(t, newApp([]string{"phase3", "artifact", "examples/接口验证.xlsx", "--json"}).Run)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := issueCapture(t, newApp([]string{"phase3", "artifact", "examples/接口验证.xlsx", "--verify", "--json"}).Run)
	if err != nil || actual != baseline {
		t.Fatalf("documented --verify changed artifact execution: %v\n%s\n%s", err, baseline, actual)
	}
}

func TestIssue3EmptySubscriptionTextReportsScanBoundary(t *testing.T) {
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/files/spaces":
			return phase3Response(200, `{"spaces":[]}`), nil
		case "/api/v1/files":
			return phase3Response(200, `{"list":[{"guid":"file0001","type":"presentation"},{"guid":"file0002","type":"presentation"}]}`), nil
		default:
			t.Fatalf("unexpected request: %s", r.URL)
			return nil, errors.New("unexpected request")
		}
	})
	out, err := issueCapture(t, newApp([]string{"subscriptions", "--limit", "1"}).Run)
	if err != nil || !strings.Contains(out, "已扫描 1 个文件") || !strings.Contains(out, "截断=true") {
		t.Fatalf("empty scan hid its incomplete scope: %q %v", out, err)
	}
}

func TestIssue3SubscriptionsCountFilesAcrossPages(t *testing.T) {
	var requested []string
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		requested = append(requested, r.URL.RequestURI())
		switch r.URL.Path {
		case "/api/v1/files/spaces":
			return phase3Response(200, `{"spaces":[]}`), nil
		case "/api/v1/files":
			if r.URL.Query().Get("page") == "2" {
				return phase3Response(200, `{"list":[{"guid":"file0001","type":"document","name":"one"},{"guid":"file0002","type":"document","name":"two"},{"guid":"file0003","type":"document","name":"three"}]}`), nil
			}
			return phase3Response(200, `{"list":[{"guid":"folder01","isFolder":true},{"guid":"folder02","isFolder":true}],"nextURL":"?folder=Desktop&page=2"}`), nil
		case "/api/v1/files/file0001/update_subscription", "/api/v1/files/file0002/update_subscription":
			return phase3Response(200, `{"subscribed":true}`), nil
		default:
			t.Fatalf("unexpected request: %s", r.URL)
			return nil, errors.New("unexpected request")
		}
	})
	out, err := issueCapture(t, newApp([]string{"subscriptions", "--limit", "2", "--json"}).Run)
	var result map[string]any
	if err != nil || json.Unmarshal([]byte(out), &result) != nil {
		t.Fatalf("%s %v", out, err)
	}
	if result["count"] != float64(2) || result["scanned"] != float64(2) || result["truncated"] != true || result["partial"] != false {
		t.Fatalf("wrong scan: %s (%v)", out, requested)
	}
}

func TestIssue3SubscriptionsExposePartialFailure(t *testing.T) {
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/files/spaces":
			return phase3Response(200, `{"spaces":[]}`), nil
		case "/api/v1/files":
			return phase3Response(200, `{"list":[{"guid":"file0001","type":"document"},{"guid":"file0002","type":"document"}]}`), nil
		case "/api/v1/files/file0001/update_subscription":
			return phase3Response(200, `{"subscribed":true}`), nil
		case "/api/v1/files/file0002/update_subscription":
			return phase3Response(503, `{"sensitive":"do not echo"}`), nil
		default:
			t.Fatalf("unexpected request: %s", r.URL)
			return nil, errors.New("unexpected request")
		}
	})
	out, err := issueCapture(t, newApp([]string{"subscriptions", "--json"}).Run)
	var detail *commandError
	if out != "" || !errors.As(err, &detail) || detail.ExitCode == 0 {
		t.Fatalf("expected one nonzero structured failure: %s %v", out, err)
	}
	if detail.Result["partial"] != true || detail.Result["scanned"] != 2 || detail.Result["count"] != 1 || detail.Result["complete"] != false {
		t.Fatalf("lost partial result: %+v", detail.Result)
	}
	if strings.Contains(err.Error(), "do not echo") {
		t.Fatal("raw server body leaked")
	}
	if !strings.Contains(err.Error(), "subscription") || !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("text failure lost the failing stage or HTTP status: %v", err)
	}
}

func TestIssue3SubscriptionsRetainResultsWhenNextPageFails(t *testing.T) {
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/files/spaces":
			return phase3Response(200, `{"spaces":[]}`), nil
		case "/api/v1/files":
			if r.URL.Query().Get("page") == "2" {
				return phase3Response(503, `{"sensitive":"do not echo"}`), nil
			}
			return phase3Response(200, `{"list":[{"guid":"file0001","type":"document"}],"nextURL":{"url":"?folder=Desktop&page=2"}}`), nil
		case "/api/v1/files/file0001/update_subscription":
			return phase3Response(200, `{"subscribed":true}`), nil
		default:
			t.Fatalf("unexpected request: %s", r.URL)
			return nil, errors.New("unexpected request")
		}
	})
	out, err := issueCapture(t, newApp([]string{"subscriptions", "--open", "1", "--json", "--no-browser"}).Run)
	var detail *commandError
	if out != "" || !errors.As(err, &detail) || detail.Result["count"] != 1 || detail.Result["partial"] != true || detail.Result["truncated"] != true {
		t.Fatalf("lost page results or attempted open: %q %v", out, err)
	}
	failures := detail.Result["failures"].([]map[string]any)
	if len(failures) != 1 || failures[0]["stage"] != "folder" || strings.Contains(failures[0]["error"].(string), "do not echo") {
		t.Fatalf("incorrect failure details: %+v", failures)
	}
}

func TestIssue3SubscriptionsCompleteAndDepthBoundary(t *testing.T) {
	for _, withFolder := range []bool{false, true} {
		t.Run(strconv.FormatBool(withFolder), func(t *testing.T) {
			issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/api/v1/files/spaces":
					return phase3Response(200, `{"spaces":[]}`), nil
				case "/api/v1/files":
					if withFolder {
						return phase3Response(200, `{"list":[{"guid":"folder01","isFolder":true},{"guid":"file0001","type":"presentation"}]}`), nil
					}
					return phase3Response(200, `{"list":[{"guid":"file0001","type":"presentation"}]}`), nil
				default:
					t.Fatalf("unexpected request: %s", r.URL)
					return nil, errors.New("unexpected request")
				}
			})
			out, err := issueCapture(t, newApp([]string{"subscriptions", "--limit", "1", "--depth", "0", "--json"}).Run)
			var result map[string]any
			if err != nil || json.Unmarshal([]byte(out), &result) != nil || result["scanned"] != float64(1) || result["truncated"] != withFolder || result["complete"] != !withFolder {
				t.Fatalf("wrong completion boundary: %s %v", out, err)
			}
		})
	}
}
