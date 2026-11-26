package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

const editorReadRuntimeFixture = `const envs = [{"__RUNTIME_ENV__":{"SDK_V2_PATH_PREFIX":"/sdk/v2/api","SDK_V2_SIGNATURE":"synthetic-signature","SDK_V2_TOKEN":"synthetic-token"}}];`

func editorReadFixture(t *testing.T, action, body string, status int) (*App, *[]string) {
	t.Helper()
	t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	t.Setenv("SHIMO_NET_SESSION_DIR", t.TempDir())
	t.Setenv("SHIMO_NET_NO_BROWSER", "")
	t.Setenv("SHIMO_NET_VERIFY", "")
	oldHTTP, oldBrowser := httpClient, phase2RunBrowser
	t.Cleanup(func() { httpClient, phase2RunBrowser = oldHTTP, oldBrowser })
	phase2RunBrowser = func(string, string, []any, string) (any, error) {
		t.Error("read command launched browser")
		return nil, errors.New("unexpected browser")
	}
	paths := []string{}
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		if r.Method != http.MethodGet || r.Header.Get("Cookie") != "sid=synthetic" {
			t.Errorf("unexpected method or missing session: %s", r.Method)
		}
		if len(paths) == 1 {
			if r.URL.Fragment != "" || r.URL.Query().Has("mentionAt") {
				t.Error("page request retained stale location")
			}
			return phase3Response(200, editorReadRuntimeFixture), nil
		}
		if r.Header.Get("x-weboffice-signature") != "synthetic-signature" || r.Header.Get("x-weboffice-token") != "synthetic-token" {
			t.Error("missing runtime credentials")
		}
		if r.Header.Get("Referer") != "https://phase3.test/docs/abcdefgh?lang=zh-CN" {
			t.Error("missing page referer")
		}
		return phase3Response(status, body), nil
	})}
	a := newApp([]string{"editor", "https://phase3.test/docs/abcdefgh?mentionAt=old&lang=zh-CN#old", "--action", action, "--json"})
	a.Origin = "https://phase3.test"
	return a, &paths
}

func TestEditorReadHTTPAndLocationOutput(t *testing.T) {
	for _, tc := range []struct{ action, endpoint, body, anchor string }{
		{"mentions-list", "mention-at-list", `{"mentionAtList":[{"atGuid":"sAT79S8a8JoieBCj","userId":"94148834"}]}`, "sAT79S8a8JoieBCj"},
		{"comments-list", "comment-list", `{"list":[{"commentGuid":"different-comment-id","selectionGuid":"comment-zb9WsyY5dC9zIB1S","content":"first"},{"commentGuid":"reply-id","selectionGuid":"comment-zb9WsyY5dC9zIB1S","content":"reply"}]}`, "comment-zb9WsyY5dC9zIB1S"},
	} {
		t.Run(tc.action, func(t *testing.T) {
			a, paths := editorReadFixture(t, tc.action, tc.body, 200)
			// Neither verification nor window retention can turn a read into UI work.
			a.Bool["--no-browser"], a.Bool["--verify"], a.Bool["--keep-open"] = true, true, true
			stdout, err := issueCapture(t, a.Run)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*paths, []string{"/docs/abcdefgh", "/sdk/v2/collab-files/abcdefgh/" + tc.endpoint}) {
				t.Fatal("wrong request sequence", *paths)
			}
			var got, original map[string]any
			if err := json.Unmarshal([]byte(stdout), &got); err != nil {
				t.Fatal(err)
			}
			_ = json.Unmarshal([]byte(tc.body), &original)
			if !reflect.DeepEqual(got["result"], map[string]any{"ok": true, "value": original}) {
				t.Fatal("raw result changed", got["result"])
			}
			want := []any{"https://phase3.test/docs/abcdefgh?lang=zh-CN#" + tc.anchor}
			if !reflect.DeepEqual(got["links"], want) || got["unlocatedCount"] != float64(0) {
				t.Fatal("wrong links", got)
			}
			count := float64(1)
			if tc.action == "comments-list" {
				count = 2
			}
			if got["count"] != count || got["verifyWith"] != "http" || got["verified"] != true || got["screenshot"] != nil || got["browserKept"] != nil {
				t.Fatal("misleading read verification", got)
			}
			if strings.Contains(stdout, "synthetic-signature") || strings.Contains(stdout, "synthetic-token") {
				t.Fatal("runtime credentials leaked")
			}
		})
	}
}

func TestEditorReadRejectsFailedOrMalformedResponse(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"denied", `"synthetic-secret"`, 403},
		{"expired", `"synthetic-secret"`, 401},
		{"redirect", `"synthetic-secret"`, 302},
		{"server-error", `"synthetic-secret"`, 500},
		{"html", `<html>synthetic-secret</html>`, 200},
		{"missing-list", `{"error":"synthetic-secret"}`, 200},
		{"wrong-list-type", `{"mentionAtList":{}}`, 200},
		{"null-list", `{"mentionAtList":null}`, 200},
		{"wrong-record-type", `{"mentionAtList":["synthetic-secret"]}`, 200},
		{"business-error", `{"code":"synthetic-secret","mentionAtList":[]}`, 200},
		{"failed-envelope", `{"ok":false,"mentionAtList":[]}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := editorReadFixture(t, "mentions-list", tc.body, tc.status)
			stdout, err := issueCapture(t, a.Run)
			if err == nil || stdout != "" {
				t.Fatalf("failure returned success: %v %s", err, stdout)
			}
			if strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatal("raw backend error leaked")
			}
		})
	}
}

func TestEditorReadWithoutStoredLoginNeverHarvests(t *testing.T) {
	for _, target := range []string{"https://phase3.test/docs/abcdefgh", "abcdefgh", "@document"} {
		t.Run(target, func(t *testing.T) {
			a, paths := editorReadFixture(t, "mentions-list", `{"mentionAtList":[]}`, 200)
			t.Setenv("SHIMO_NET_COOKIE", "")
			// Accidental harvest is blocked as a second line of defense. The
			// expected error proves we never reach that fallback in the first place.
			t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
			if err := saveFav(map[string]map[string]any{"document": {"url": "https://phase3.test/docs/abcdefgh"}}); err != nil {
				t.Fatal(err)
			}
			a.Pos = []string{target}
			stdout, err := issueCapture(t, a.Run)
			if !errors.Is(err, errNoStoredCredential) || stdout != "" || len(*paths) != 0 {
				t.Fatalf("missing login did not stop before lookup: %v %s %v", err, stdout, *paths)
			}
			if a.has("--no-browser") {
				t.Fatal("read command mutated caller flags")
			}
		})
	}
}

func TestEditorReadRejectsCrossOriginBeforeRequests(t *testing.T) {
	a, paths := editorReadFixture(t, "comments-list", `{"list":[]}`, 200)
	a.Pos = []string{"https://elsewhere.test/docs/abcdefgh"}
	stdout, err := issueCapture(t, a.Run)
	if err == nil || !strings.Contains(err.Error(), "origin 不一致") || stdout != "" || len(*paths) != 0 {
		t.Fatalf("cross-origin target accepted: %v %s %v", err, stdout, *paths)
	}
}

func TestEditorReadPageAndAPIRedirectsAndMissingRuntime(t *testing.T) {
	for _, mode := range []string{"page-redirect", "api-redirect", "missing-runtime", "missing-signature", "page-transport"} {
		t.Run(mode, func(t *testing.T) {
			a, _ := editorReadFixture(t, "mentions-list", `{"mentionAtList":[]}`, 200)
			calls := 0
			httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host != "phase3.test" || calls > 2 {
					t.Fatal("followed redirect with credentials")
				}
				if mode == "page-transport" {
					return nil, errors.New("connection unavailable")
				}
				if mode == "missing-runtime" {
					return phase3Response(200, `<html>synthetic-secret</html>`), nil
				}
				if mode == "missing-signature" {
					return phase3Response(200, strings.Replace(editorReadRuntimeFixture, `"synthetic-signature"`, `""`, 1)), nil
				}
				if mode == "api-redirect" && calls == 1 {
					return phase3Response(200, editorReadRuntimeFixture), nil
				}
				response := phase3Response(302, "synthetic-secret")
				response.Header.Set("Location", "https://elsewhere.test/steal")
				return response, nil
			})}
			stdout, err := issueCapture(t, a.Run)
			if err == nil || stdout != "" || strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatalf("failed page accepted or leaked: %v %s", err, stdout)
			}
			wantCalls := 1
			if mode == "api-redirect" {
				wantCalls = 2
			}
			if calls != wantCalls {
				t.Fatalf("unexpected requests: %d", calls)
			}
		})
	}
}

func TestEditorReadLinksOnlyUseSupportedLocations(t *testing.T) {
	for _, tc := range []struct {
		name, path, action, anchor string
		wantCount                  int
	}{
		{"mention", "/docs/abcdefgh", "mentions-list", "valid-id", 1},
		{"comment", "/docs/abcdefgh", "comments-list", "comment-valid-id", 1},
		{"comment-prefix-required", "/docs/abcdefgh", "comments-list", "valid-id", 0},
		{"empty-comment-id", "/docs/abcdefgh", "comments-list", "comment-", 0},
		{"invalid-mention", "/docs/abcdefgh", "mentions-list", "bad#id", 0},
		{"sheet-not-verified", "/sheets/abcdefgh", "mentions-list", "valid-id", 0},
		{"docx-not-verified", "/docx/abcdefgh", "comments-list", "comment-valid-id", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, _ := url.Parse("https://phase3.test" + tc.path)
			row := map[string]any{"atGuid": tc.anchor, "selectionGuid": tc.anchor, "commentGuid": "not-a-location"}
			links, missing := editorReadLinks(page, tc.action, []any{row, row})
			if len(links) != tc.wantCount || missing != 2*(1-tc.wantCount) {
				t.Fatal(links, missing)
			}
			if tc.wantCount == 1 && links[0] != page.String()+"#"+tc.anchor {
				t.Fatal("wrong anchor", links)
			}
		})
	}
}

func TestEditorReadTextEmptyAndMissingLocations(t *testing.T) {
	for _, tc := range []struct{ action, body, want string }{
		{"mentions-list", `{"mentionAtList":[]}`, "暂无提及"},
		{"comments-list", `{"list":[]}`, "暂无评论"},
		{"mentions-list", `{"mentionAtList":[{"atGuid":"valid-id"},{"atGuid":"invalid#id"},{}]}`, "2 条记录暂无可用定位链接"},
		{"comments-list", `{"list":[{"commentGuid":"not-a-location"}]}`, "1 条记录暂无可用定位链接"},
	} {
		t.Run(tc.action+tc.want, func(t *testing.T) {
			a, _ := editorReadFixture(t, tc.action, tc.body, 200)
			a.JSON = false
			stdout, err := issueCapture(t, a.Run)
			if err != nil || !strings.Contains(stdout, tc.want) || strings.Contains(stdout, "返回：{") {
				t.Fatalf("bad human output: %v %s", err, stdout)
			}
			if strings.Contains(tc.body, "valid-id") && !strings.Contains(stdout, "https://phase3.test/docs/abcdefgh?lang=zh-CN#valid-id") {
				t.Fatal("human output missing URL", stdout)
			}
		})
	}
}
