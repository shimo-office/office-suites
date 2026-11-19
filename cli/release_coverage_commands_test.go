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
)

// Run the real command dispatcher against an in-memory HTTP transport. Unknown
// requests fail rather than reaching a network or consulting an actual account.
func releaseCommandTransport(t *testing.T, handler func(*http.Request) (*http.Response, error)) {
	t.Helper()
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	t.Setenv("SHIMO_NET_ORIGIN", "https://release.invalid")
	t.Setenv("SHIMO_NET_COOKIE", "sid=release-synthetic")
	t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
	old := httpClient
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "release.invalid" || r.Header.Get("Cookie") != "sid=release-synthetic" {
			t.Errorf("unexpected destination or credential: %s", r.URL)
			return nil, errors.New("invalid test request")
		}
		return handler(r)
	})}
	t.Cleanup(func() { httpClient = old })
}
func releaseRun(t *testing.T, args ...string) map[string]any {
	t.Helper()
	out, err := issueCapture(t, newApp(append(args, "--json")).Run)
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	var result map[string]any
	if err = json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("%v produced invalid JSON: %s", args, out)
	}
	return result
}
func releaseResponse(v any) *http.Response {
	b, _ := json.Marshal(v)
	return phase3Response(200, string(b))
}

func TestReleaseCoverageFileLifecycleAndAliases(t *testing.T) {
	const guid = "abcdefgh12345678"
	name := "合成测试文档"
	trashed := false
	creates, deletes, restores, renames := 0, 0, 0, 0
	releaseCommandTransport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/files/create/document":
			if r.Method != "GET" || r.URL.Query().Get("parentGuid") != "Desktop" || r.URL.Query().Get("name") != name {
				t.Errorf("incorrect creation request: %s %s", r.Method, r.URL)
			}
			creates++
			resp := phase3Response(302, "")
			resp.Header.Set("Location", "/docs/"+guid)
			return resp, nil
		case "/api/v1/files/" + guid:
			switch r.Method {
			case "PATCH":
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body["name"] != "改名后的合成测试" {
					t.Errorf("incorrect rename: %v", body)
				}
				name = str(body, "name")
				renames++
			case "DELETE":
				trashed = true
				deletes++
				return phase3Response(204, ""), nil
			case "GET":
				if trashed {
					return phase3Response(404, `{}`), nil
				}
			default:
				t.Errorf("unexpected method: %s", r.Method)
			}
			return releaseResponse(map[string]any{"guid": guid, "name": name, "type": "document", "shareMode": "private", "updatedAt": 1700000000}), nil
		case "/api/v1/files/" + guid + "/ancestors":
			return releaseResponse(map[string]any{"ancestors": []any{map[string]any{"guid": "folder1234567890", "name": "合成目录", "isFolder": true, "url": "/folder/folder1234567890"}}}), nil
		case "/api/v1/files/trashes":
			if !trashed {
				t.Error("trash read before deletion")
			}
			return releaseResponse(map[string]any{"list": []any{map[string]any{"guid": guid, "name": name, "type": "document", "url": "/docs/" + guid, "deletedAt": 1700000001}}}), nil
		case "/api/v1/files/trashes/" + guid:
			if r.Method != "PATCH" || !trashed {
				t.Errorf("incorrect restore %s trashed=%t", r.Method, trashed)
			}
			trashed = false
			restores++
			return releaseResponse(map[string]any{}), nil
		}
		return nil, errors.New("unexpected request " + r.Method + " " + r.URL.Path)
	})
	created := releaseRun(t, "new", name, "--type", "doc", "--to", "desktop")
	if created["guid"] != guid || created["shareMode"] != "private" {
		t.Fatalf("created: %v", created)
	}
	releaseRun(t, "fav", "add", "@验收", guid, "--note", "仅合成内容")
	saved := releaseRun(t, "fav", "show", "验收")
	if saved["name"] != name || saved["note"] != "仅合成内容" || saved["kind"] != "document" {
		t.Fatalf("saved alias: %v", saved)
	}
	if got := releaseRun(t, "fav", "list"); got["count"] != float64(1) {
		t.Fatalf("list aliases: %v", got)
	}
	if st, err := os.Stat(favPath()); err != nil || st.Mode().Perm() != 0600 {
		t.Fatalf("alias storage permissions: %v %v", st, err)
	}
	info := releaseRun(t, "info", "@验收")
	if info["guid"] != guid || info["path"] != "合成目录" {
		t.Fatalf("alias resolution/ancestry: %v", info)
	}
	renamed := releaseRun(t, "rename", "@验收", "改名后的合成测试")
	if renamed["name"] != "改名后的合成测试" {
		t.Fatalf("rename readback: %v", renamed)
	}
	releaseRun(t, "delete", "@验收")
	trash := releaseRun(t, "trash")
	if trash["count"] != float64(1) || !trashed {
		t.Fatalf("trash: %v", trash)
	}
	restored := releaseRun(t, "restore", guid)
	if restored["name"] != name || trashed {
		t.Fatalf("restore: %v", restored)
	}
	releaseRun(t, "fav", "rm", "@验收")
	if got := releaseRun(t, "fav", "ls"); got["count"] != float64(0) {
		t.Fatalf("remaining alias: %v", got)
	}
	if creates != 1 || renames != 1 || deletes != 1 || restores != 1 {
		t.Fatalf("unexpected replay create=%d rename=%d delete=%d restore=%d", creates, renames, deletes, restores)
	}
	for _, args := range [][]string{{"fav", "show", "验收"}, {"fav", "rm", "验收"}, {"fav", "add", "invalid alias", guid}} {
		if _, err := issueCapture(t, newApp(args).Run); err == nil {
			t.Fatalf("invalid alias operation accepted: %v", args)
		}
	}
}

func TestReleaseCoverageFolderPaginationSearchAndSubscriptions(t *testing.T) {
	const first = "first12345678901"
	const second = "second1234567890"
	const nested = "nested1234567890"
	const folder = "folder1234567890"
	const space = "space12345678901"
	subscribedCalls := []string{}
	releaseCommandTransport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/files/spaces":
			if r.URL.Query().Get("page") == "2" {
				return releaseResponse(map[string]any{"spaces": []any{}}), nil
			}
			return releaseResponse(map[string]any{"spaces": []any{map[string]any{"guid": space, "name": "合成空间", "role": "editor", "updatedAt": 1700000000}}, "next": "page=2"}), nil
		case "/api/v1/files":
			switch r.URL.Query().Get("folder") {
			case "Desktop":
				if r.URL.Query().Get("page") == "2" {
					return releaseResponse(map[string]any{"list": []any{map[string]any{"guid": second, "name": "较新文件", "type": "documentPro", "updatedAt": 1800000000}}}), nil
				}
				return releaseResponse(map[string]any{"list": []any{map[string]any{"guid": first, "name": "旧文件", "type": "spreadsheet", "updatedAt": 1700000000}, map[string]any{"guid": folder, "name": "子目录", "isFolder": true}}, "nextURL": map[string]any{"nextURL": "https://release.invalid/api/v1/files?folder=Desktop&page=2"}}), nil
			case folder:
				return releaseResponse(map[string]any{"list": []any{map[string]any{"guid": nested, "name": "嵌套文件", "type": "document", "updatedAt": 1900000000}, map[string]any{"guid": first, "type": "document"}}}), nil
			case space:
				return releaseResponse(map[string]any{"list": []any{map[string]any{"guid": first, "type": "spreadsheet"}, map[string]any{"guid": "slide12345678901", "type": "presentation"}}}), nil
			}
		case "/api/v1/search/files":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if r.Method != "POST" || body["keyword"] != "合成" {
				t.Errorf("search payload: %v", body)
			}
			return releaseResponse(map[string]any{"results": []any{map[string]any{"source": map[string]any{"guid": first, "name": "合成结果", "type": "document", "ancestors": []any{map[string]any{"name": "路径"}}}}, map[string]any{"source": map[string]any{"guid": second, "name": "第二结果", "type": "folder"}}}}), nil
		}
		if strings.HasSuffix(r.URL.Path, "/update_subscription") {
			g := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/files/"), "/update_subscription")
			subscribedCalls = append(subscribedCalls, g)
			return releaseResponse(map[string]any{"subscribed": g != second}), nil
		}
		return nil, errors.New("unexpected request " + r.URL.String())
	})
	if got := releaseRun(t, "spaces"); got["count"] != float64(1) {
		t.Fatalf("spaces: %v", got)
	}
	files := releaseRun(t, "ls", "desktop")
	if files["count"] != float64(3) {
		t.Fatalf("pagination lost files: %v", files)
	}
	if got := releaseRun(t, "ls", "desktop", "--limit", "1"); got["count"] != float64(1) {
		t.Fatalf("limit: %v", got)
	}
	if got := releaseRun(t, "ls", "desktop", "--since", "2026-01-01"); got["count"] != float64(1) {
		t.Fatalf("date filter: %v", got)
	}
	result := releaseRun(t, "search", "合成", "--limit", "1")
	if result["count"] != float64(1) || asMap(asSlice(result["results"])[0])["path"] != "路径" {
		t.Fatalf("search result: %v", result)
	}
	result = releaseRun(t, "subscriptions", "--limit", "10", "--depth", "1")
	list := asSlice(result["files"])
	if len(list) != 2 || asMap(list[0])["guid"] != nested || asMap(list[1])["guid"] != first {
		t.Fatalf("subscription ordering/filtering: %v", result)
	}
	if strings.Join(subscribedCalls, ",") != first+","+second+","+nested {
		t.Fatalf("duplicate/unsupported scans: %v", subscribedCalls)
	}
	subscribedCalls = nil
	result = releaseRun(t, "subscriptions", "--limit", "1", "--depth", "0")
	if result["count"] != float64(1) || len(subscribedCalls) != 1 || result["limitScope"] != "scanned-files" {
		t.Fatalf("subscription budget: %v calls=%v", result, subscribedCalls)
	}
	if got := releaseRun(t, "subscriptions", "--limit", "0", "--depth", "99"); got["limit"] != float64(200) || got["depth"] != float64(8) {
		t.Fatalf("empty/clamped scan: %v", got)
	}
	if _, err := issueCapture(t, newApp([]string{"subscriptions", "--limit", "1", "--open", "99"}).Run); err == nil {
		t.Fatal("empty subscription opened")
	}
	if g, err := newApp(nil).resolveTarget("合成空间"); err != nil || g != space {
		t.Fatalf("space-name resolution: %s %v", g, err)
	}
}

func TestReleaseCoverageUploadImportAndFolder(t *testing.T) {
	const guid = "import1234567890"
	const folder = "folder1234567890"
	title := "原始文件"
	uploads := 0
	imports := 0
	releaseCommandTransport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/files/create/folder":
			if r.URL.Query().Get("parentGuid") != "Desktop" || r.URL.Query().Get("name") != "合成目录" {
				t.Errorf("mkdir query: %s", r.URL)
			}
			res := phase3Response(302, "")
			res.Header.Set("Location", "/folder/"+folder)
			return res, nil
		case "/api/v1/files/" + folder:
			return releaseResponse(map[string]any{"guid": folder, "name": "合成目录", "isFolder": true}), nil
		case "/api/v1/files/upload":
			bytes, _ := io.ReadAll(r.Body)
			if string(bytes) != "# 合成文档" || r.Header.Get("X-File-Parent-Guid") != folder || r.Header.Get("X-File-Type") != "md" || r.Method != "POST" {
				t.Errorf("upload metadata/body: %s %v %s", r.Method, r.Header, bytes)
			}
			uploads++
			return releaseResponse(map[string]any{"guid": "cloud12345678901", "name": "fixture.md", "type": "file", "isCloudFile": true}), nil
		case "/api/v1/files/import/cloud12345678901":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["type"] != "document" {
				t.Errorf("import type: %v", body)
			}
			imports++
			return phase3Response(200, `{"data":{"taskId":"synthetic-task"}}`), nil
		case "/api/v1/files/import/progress":
			return phase3Response(200, `{"data":{"progress":100,"file":{"guid":"`+guid+`"}}}`), nil
		case "/api/v1/files/" + guid:
			if r.Method == "PATCH" {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				title = str(body, "name")
			}
			return releaseResponse(map[string]any{"guid": guid, "name": title, "type": "document", "shareMode": "private"}), nil
		}
		return nil, errors.New("unexpected request " + r.URL.String())
	})
	if got := releaseRun(t, "mkdir", "合成目录", "--to", "desktop"); got["guid"] != folder || got["parent"] != "Desktop" {
		t.Fatalf("mkdir: %v", got)
	}
	releaseRun(t, "fav", "add", "导入目录", folder)
	path := filepath.Join(t.TempDir(), "fixture.md")
	if err := os.WriteFile(path, []byte("# 合成文档"), 0600); err != nil {
		t.Fatal(err)
	}
	uploaded := releaseRun(t, "upload", path, "--to", "@导入目录")
	if uploaded["isCloudFile"] != true || asMap(uploaded["parent"])["kind"] != "folder" {
		t.Fatalf("upload result: %v", uploaded)
	}
	imported := releaseRun(t, "import", path, "--to", "@导入目录", "--title", "导入验收")
	if imported["guid"] != guid || imported["name"] != "导入验收" || imports != 1 || uploads != 2 {
		t.Fatalf("import: %v imports=%d uploads=%d", imported, imports, uploads)
	}
}

func TestReleaseCoverageHTTPFailuresNeverReportSuccess(t *testing.T) {
	requests := 0
	releaseCommandTransport(t, func(r *http.Request) (*http.Response, error) {
		requests++
		return phase3Response(503, `{"error":"synthetic outage"}`), nil
	})
	for _, args := range [][]string{{"whoami"}, {"spaces"}, {"search", "x"}, {"ls", "desktop"}, {"info", "abcdefgh12345678"}, {"mkdir", "folder"}, {"new", "doc"}, {"rename", "abcdefgh12345678", "new"}, {"delete", "abcdefgh12345678"}, {"restore", "abcdefgh12345678"}, {"trash"}, {"subscriptions"}, {"fav", "add", "demo", "abcdefgh12345678"}} {
		t.Run(args[0], func(t *testing.T) {
			before := requests
			out, err := issueCapture(t, newApp(append(args, "--json")).Run)
			if err == nil || out != "" || requests-before != 1 {
				t.Fatalf("command=%v error=%v stdout=%s requests=%d", args, err, out, requests-before)
			}
		})
	}
}

func TestReleaseCoveragePaginationDestinations(t *testing.T) {
	for _, tc := range []struct {
		next    string
		allowed bool
	}{
		{"https://release.invalid/api/v1/files?page=2", true},
		{"https://release.invalid/files?page=2", true},
		{"/api/v1/files?page=2", true}, {"/files?page=2", true},
		{"?page=2", true}, {"files?page=2", true},
		{"https://foreign.invalid/api/v1/files?page=2", false},
		{"//foreign.invalid/files?page=2", false},
		{"https://release.invalid:444/files?page=2", false},
		{"http://release.invalid/files?page=2", false},
		{"https://user:password@release.invalid/files?page=2", false},
		{"/api/v1/users/me", false}, {"/files#fragment", false}, {"%zz", false},
	} {
		t.Run(tc.next, func(t *testing.T) {
			calls := 0
			releaseCommandTransport(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return releaseResponse(map[string]any{"list": []any{map[string]any{"guid": "first12345678901"}}, "nextURL": tc.next}), nil
				}
				if r.URL.Path != "/api/v1/files" || r.URL.Query().Get("page") != "2" {
					t.Errorf("wrong pagination request %s", r.URL)
				}
				return phase3Response(200, `{"list":[{"guid":"second1234567890"}]}`), nil
			})
			rows, err := newApp(nil).listFolder("desktop", 10)
			if tc.allowed {
				if err != nil || len(rows) != 2 || calls != 2 {
					t.Fatalf("valid pagination failed: rows=%v calls=%d err=%v", rows, calls, err)
				}
			} else if err == nil || calls != 1 {
				t.Fatalf("unsafe pagination sent: calls=%d error=%v", calls, err)
			}
		})
	}
}

func TestReleaseCoverageReadableCommandOutput(t *testing.T) {
	const guid = "abcdefgh12345678"
	releaseCommandTransport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/users/me":
			return phase3Response(200, `{"id":123,"name":"合成用户","email":"test@example.invalid"}`), nil
		case "/api/v1/files/spaces":
			return phase3Response(200, `{"spaces":[{"guid":"space12345678901","name":"测试空间","role":"editor","updatedAt":1700000000}]}`), nil
		case "/api/v1/search/files":
			return phase3Response(200, `{"results":[{"source":{"guid":"abcdefgh12345678","name":"合成文件","type":"document","updatedAt":1700000000}}]}`), nil
		case "/api/v1/files":
			return phase3Response(200, `{"list":[{"guid":"abcdefgh12345678","name":"合成文件","type":"document","updatedAt":1700000000}]}`), nil
		case "/api/v1/files/" + guid + "/ancestors":
			return phase3Response(200, `{"ancestors":[{"guid":"folder1234567890","name":"测试目录","isFolder":true}]}`), nil
		case "/api/v1/files/" + guid + "/update_subscription":
			return phase3Response(200, `{"subscribed":true}`), nil
		case "/api/v1/files/trashes":
			return phase3Response(200, `[{"guid":"abcdefgh12345678","name":"合成文件","type":"document","updatedAt":1700000000}]`), nil
		case "/api/v1/files/trashes/" + guid:
			return phase3Response(200, `{}`), nil
		case "/api/v1/files/create/document", "/api/v1/files/create/folder":
			res := phase3Response(302, "")
			route := "docs"
			if strings.HasSuffix(r.URL.Path, "folder") {
				route = "folder"
			}
			res.Header.Set("Location", "/"+route+"/"+guid)
			return res, nil
		case "/api/v1/files/" + guid:
			return phase3Response(200, `{"guid":"abcdefgh12345678","name":"合成文件","type":"document","shareMode":"readonly","updatedAt":1700000000}`), nil
		}
		return nil, errors.New("unexpected request " + r.URL.String())
	})
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"whoami"}, "合成用户 <test@example.invalid>"},
		{[]string{"spaces"}, "测试空间"}, {[]string{"search", "合成"}, "合成文件"},
		{[]string{"ls", "desktop", "--since", "2020-01-01"}, "合成文件"},
		{[]string{"info", guid}, "测试目录"}, {[]string{"info", "desktop"}, "我的桌面"},
		{[]string{"new", "合成文件"}, "新建完成"}, {[]string{"mkdir", "合成目录"}, "已新建文件夹"},
		{[]string{"rename", guid, "合成文件"}, "已改名"}, {[]string{"delete", guid}, "可恢复"},
		{[]string{"trash"}, "合成文件"}, {[]string{"restore", guid}, "已恢复"},
		{[]string{"subscriptions", "--limit", "10"}, "仅在扫描范围内排序"},
		{[]string{"audit", "desktop"}, "公开可访问"},
		{[]string{"fav"}, "还没有常用位置"},
		{[]string{"fav", "add", "合成", guid, "--note", "合成备注"}, "合成备注"},
		{[]string{"fav", "show", "合成"}, "合成文件"}, {[]string{"fav", "list"}, "@合成"},
		{[]string{"fav", "remove", "合成"}, "已删除 @合成"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			out, err := issueCapture(t, newApp(tc.args).Run)
			if err != nil || !strings.Contains(out, tc.want) {
				t.Fatalf("%v error=%v output=%s want=%s", tc.args, err, out, tc.want)
			}
		})
	}
}

func TestReleaseCoverageInputFailuresDoNotReachService(t *testing.T) {
	calls := 0
	releaseCommandTransport(t, func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("must not make a request")
	})
	for _, args := range [][]string{{"search"}, {"ls"}, {"info"}, {"audit"}, {"new"}, {"mkdir"}, {"rename"}, {"delete"}, {"restore"}, {"upload"}, {"import"}, {"fav", "add"}, {"fav", "show"}, {"fav", "rm"}, {"fav", "unsupported"}, {"new", "name", "--type", "unsupported"}, {"ls", "desktop", "--since", "not-a-date"}, {"upload", "/missing/release-fixture"}, {"import", "/missing/release-fixture"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out, err := issueCapture(t, newApp(append(args, "--json")).Run)
			if err == nil || out != "" {
				t.Fatalf("invalid input succeeded: %v %s", err, out)
			}
		})
	}
	if calls != 0 {
		t.Fatalf("invalid input reached service %d times", calls)
	}
	local := filepath.Join(t.TempDir(), "test.xyz")
	if err := os.WriteFile(local, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := issueCapture(t, newApp([]string{"import", local, "--to", "desktop"}).Run); err == nil || !strings.Contains(err.Error(), "暂不支持") {
		t.Fatalf("unknown import type: %v", err)
	}
	if err := os.Rename(local, local+".md"); err != nil {
		t.Fatal(err)
	}
	if _, err := issueCapture(t, newApp([]string{"import", local + ".md"}).Run); err == nil || !strings.Contains(err.Error(), "必须指定导入位置") {
		t.Fatalf("missing destination: %v", err)
	}
	if calls != 0 {
		t.Fatalf("unsupported import reached service %d times", calls)
	}
}

func TestReleaseCoverageCreatedResourceReadbackFailureNotRetried(t *testing.T) {
	for _, command := range []string{"new", "mkdir"} {
		for _, stage := range []string{"redirect", "readback"} {
			t.Run(command+"/"+stage, func(t *testing.T) {
				mutations := 0
				reads := 0
				releaseCommandTransport(t, func(r *http.Request) (*http.Response, error) {
					if strings.Contains(r.URL.Path, "/create/") {
						mutations++
						res := phase3Response(302, "")
						location := "/unexpected"
						if stage == "readback" {
							route := "docs"
							if command == "mkdir" {
								route = "folder"
							}
							location = "/" + route + "/abcdefgh12345678"
						}
						res.Header.Set("Location", location)
						return res, nil
					}
					reads++
					return phase3Response(403, `{}`), nil
				})
				out, err := issueCapture(t, newApp([]string{command, "合成资源", "--json", "--relogin"}).Run)
				if err == nil || out != "" || mutations != 1 {
					t.Fatalf("unsafe success/replay: %s %v mutations=%d", out, err, mutations)
				}
				if stage == "readback" && reads != 1 {
					t.Fatalf("unexpected read count: %d", reads)
				}
			})
		}
	}
}

func TestReleaseCoverageTransportFailuresPreserveNoSuccess(t *testing.T) {
	releaseCommandTransport(t, func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("synthetic transport disconnected")
	})
	for _, args := range [][]string{{"whoami"}, {"spaces"}, {"search", "fixture"}, {"ls", "desktop"}, {"info", "abcdefgh12345678"}, {"new", "fixture"}, {"mkdir", "fixture"}, {"rename", "abcdefgh12345678", "new"}, {"delete", "abcdefgh12345678"}, {"restore", "abcdefgh12345678"}, {"trash"}, {"subscriptions"}, {"fav", "add", "fixture", "abcdefgh12345678"}} {
		t.Run(args[0], func(t *testing.T) {
			out, err := issueCapture(t, newApp(append(args, "--json")).Run)
			if err == nil || !strings.Contains(err.Error(), "synthetic transport disconnected") || out != "" {
				t.Fatalf("lost transport error: %v output=%s", err, out)
			}
		})
	}
}

func TestReleaseCoverageConfigClearPreservesOtherSettings(t *testing.T) {
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	t.Setenv("SHIMO_NET_ORIGIN", "")
	if err := os.WriteFile(configPath(), []byte(`{"origin":"https://old.invalid","savedAt":"recorded","otherSetting":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	before := releaseRun(t, "config", "show")
	if before["origin"] != "https://old.invalid" || before["source"] != "config" || before["savedAt"] != "recorded" {
		t.Fatalf("config read: %v", before)
	}
	out, err := issueCapture(t, newApp([]string{"config", "clear"}).Run)
	if err != nil || !strings.Contains(out, "已清除") {
		t.Fatalf("clear: %s %v", out, err)
	}
	config := readConfig()
	if config["origin"] != nil || config["otherSetting"] != true || config["savedAt"] != "recorded" {
		t.Fatalf("clear corrupted unrelated settings: %v", config)
	}
	if got := releaseRun(t, "config", "get"); got["source"] != "default" {
		t.Fatalf("cleared config remains effective: %v", got)
	}
	t.Setenv("SHIMO_NET_ORIGIN", "https://override.invalid")
	if got := releaseRun(t, "config", "list"); got["origin"] != "https://override.invalid" || got["envOverride"] != "SHIMO_NET_ORIGIN" {
		t.Fatalf("override: %v", got)
	}
	out, err = issueCapture(t, newApp([]string{"config", "show"}).Run)
	if err != nil || !strings.Contains(out, "环境变量") {
		t.Fatalf("text override: %s %v", out, err)
	}
	for _, args := range [][]string{{"config", "set"}, {"config", "unsupported"}} {
		if _, err := issueCapture(t, newApp(args).Run); err == nil {
			t.Fatalf("invalid config command accepted: %v", args)
		}
	}
}

func TestReleaseCoveragePaginationPrivateDeploymentPath(t *testing.T) {
	calls := 0
	releaseCommandTransport(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/office/api/v1/files" {
			t.Errorf("deployment prefix lost/duplicated: %s", r.URL)
		}
		if calls == 1 {
			return phase3Response(200, `{"list":[{"guid":"first12345678901"}],"nextURL":"https://release.invalid/office/api/v1/files?page=2"}`), nil
		}
		return phase3Response(200, `{"list":[{"guid":"second1234567890"}]}`), nil
	})
	t.Setenv("SHIMO_NET_ORIGIN", "https://release.invalid/office")
	rows, err := newApp(nil).listFolder("desktop", 10)
	if err != nil || len(rows) != 2 || calls != 2 {
		t.Fatalf("deployment pagination: %v %v calls=%d", rows, err, calls)
	}
}
