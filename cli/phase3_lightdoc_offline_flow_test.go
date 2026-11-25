package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// 截图验收要求「有内容」，纯白会被判空白；全黑图能让截图链路真正跑完。
var lightDocTestPNG = func() func() string {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return func() string { return encoded }
}()

// lightDocFlow 把命令主流程需要的外部依赖都收敛到本地：
// 登录态走环境变量、服务端走假 HTTP、浏览器走假 CDP、导出内容走本地 httptest。
// 这样命令的完整路径（含证据落盘与结果 JSON）在 CI 里也能跑，不依赖登录与窗口。
type lightDocFlow struct {
	app     *App
	browser *phase3EditorBrowser
	stub    *lightDocStub
	dir     string
	srv     *httptest.Server
	exports int32
}

func newLightDocFlow(t *testing.T, action, payload, target string, md func(export int) string, reply func(expr string, stub *lightDocStub) any) *lightDocFlow {
	t.Helper()
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	t.Setenv("SHIMO_NET_HOME", t.TempDir())

	flow := &lightDocFlow{dir: t.TempDir()}
	flow.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(atomic.AddInt32(&flow.exports, 1))
		_, _ = fmt.Fprint(w, md(n))
	}))
	t.Cleanup(flow.srv.Close)

	originalClient := httpClient
	t.Cleanup(func() { httpClient = originalClient })
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(r.URL.Path, "/files/export/progress"):
			return phase3Response(200, `{"data":{"progress":100,"downloadUrl":"`+flow.srv.URL+`/export.md"}}`), nil
		case strings.Contains(r.URL.Path, "/files/export/"):
			return phase3Response(200, `{"data":{"taskId":"t1"}}`), nil
		case strings.HasSuffix(r.URL.Path, "/users/me"):
			return phase3Response(200, `{"id":94148834,"name":"tester"}`), nil
		}
		return phase3Response(200, `{}`), nil
	})}

	var stub *lightDocStub
	flow.browser, stub = newLightDocStub(t, func(expr string) any { return reply(expr, stub) })
	flow.stub = stub

	originalOpen := phase3StartEditor
	t.Cleanup(func() { phase3StartEditor = originalOpen })
	phase3StartEditor = func(string, string) (*phase3EditorBrowser, error) { return flow.browser, nil }

	flow.app = newApp([]string{"phase3", target, "--action", action, "--payload", payload, "--out", flow.dir})
	flow.app.Origin = "https://shimo.net"
	return flow
}

// 就绪探测与段落/标记读取都靠同一批表达式，这里统一按特征应答。
func lightDocFlowReply(expr string, stub *lightDocStub) any {
	switch {
	case strings.Contains(expr, "documentState"):
		return map[string]any{"ready": true, "login": false}
	case strings.Contains(expr, "headingSkipped"):
		// 全文断言按目标值统计覆盖数，字号要比的是 "20" 而不是布尔标记。
		counts := map[string]any{"true": 3}
		if strings.Contains(expr, "data-hk-font-size") {
			counts = map[string]any{"20": 3}
		}
		return map[string]any{"ok": true, "total": 3, "counts": counts, "headingSkipped": 1, "samples": []any{}}
	case strings.Contains(expr, "const vals=[];const hits=[]"):
		// 点击前标记为空，点击后才出现，才能验证「确实点了」。
		if stub.seen("Input.dispatchMouseEvent") == 0 {
			return map[string]any{"ok": true, "values": []any{""}}
		}
		return map[string]any{"ok": true, "values": []any{"true"}}
	}
	return lightDocDefaultReply(expr)
}

func TestLightDocFlowFormatWritesEvidence(t *testing.T) {
	payload := `{"paragraph":1,"text":"第一段","start":0,"end":3,"attrs":{"bold":true}}`
	flow := newLightDocFlow(t, "doc-format", payload, "https://shimo.net/docs/abcdefgh",
		func(int) string { return "第一段\n\n第二段\n" }, lightDocFlowReply)
	if err := flow.app.cmdPhase3LightDocFormat(); err != nil {
		t.Fatalf("命令应成功：%v", err)
	}
	for _, name := range []string{"request.json", "before.png", "after.png", "reload.png", "marks.json", "marks-reloaded.json", "result.json"} {
		if _, err := os.Stat(evidencePath(t, flow.dir, name)); err != nil {
			t.Fatalf("证据 %s 缺失：%v", name, err)
		}
	}
	result := readEvidenceJSON(t, flow.dir, "result.json")
	if result["verified"] != true || result["action"] != "doc-format" {
		t.Fatalf("结果未声明已验证：%v", result)
	}
	if result["paragraphTextKept"] != true {
		t.Fatalf("段落正文应保持不变：%v", result["paragraphTextKept"])
	}
}

func TestLightDocFlowParagraphAppliesAndVerifies(t *testing.T) {
	payload := `{"paragraph":1,"text":"第一段","attrs":{"align":"center","heading":2}}`
	flow := newLightDocFlow(t, "doc-paragraph", payload, "https://shimo.net/docs/abcdefgh",
		func(int) string { return "第一段\n" }, lightDocFlowReply)
	if err := flow.app.cmdPhase3LightDocParagraph(); err != nil {
		t.Fatalf("命令应成功：%v", err)
	}
	result := readEvidenceJSON(t, flow.dir, "result.json")
	if result["verified"] != true || result["evidencedAfterReload"] != true {
		t.Fatalf("段落动作未留下刷新后复验：%v", result)
	}
	// 行距是换算值，只有请求了行距才核对「变化」；这里没请求，不应出现该分支。
	if _, ok := result["lineHeight"]; ok {
		t.Fatal("未请求行距时不应凭空产生行距证据")
	}
}

func TestLightDocFlowReplaceCountsHits(t *testing.T) {
	payload := `{"find":"旧标题","replace":"新标题","scope":"first"}`
	flow := newLightDocFlow(t, "doc-replace", payload, "https://shimo.net/docs/abcdefgh",
		func(export int) string {
			if export == 1 {
				return "旧标题\n\n正文\n"
			}
			return "新标题\n\n正文\n"
		}, lightDocFlowReply)
	if err := flow.app.cmdPhase3LightDocReplace(); err != nil {
		t.Fatalf("命令应成功：%v", err)
	}
	result := readEvidenceJSON(t, flow.dir, "result.json")
	if num(result, "beforeOccurrences") != 1 || num(result, "afterOccurrences") != 0 {
		t.Fatalf("命中计数不符：%v", result)
	}
	if num(result, "replaceAdded") != 1 {
		t.Fatalf("替换文本未出现：%v", result)
	}
}

func TestLightDocFlowReplaceStopsWhenTargetMissing(t *testing.T) {
	payload := `{"find":"不存在的词","replace":"新","scope":"all"}`
	flow := newLightDocFlow(t, "doc-replace", payload, "https://shimo.net/docs/abcdefgh",
		func(int) string { return "别的内容\n" }, lightDocFlowReply)
	err := flow.app.cmdPhase3LightDocReplace()
	if err == nil || !strings.Contains(err.Error(), "找不到") {
		t.Fatalf("找不到目标时应停止：%v", err)
	}
	if flow.stub.seen("Input.dispatchMouseEvent") != 0 {
		t.Fatal("未定位到目标时不应点击页面")
	}
	if _, statErr := os.Stat(evidencePath(t, flow.dir, "failure.json")); statErr != nil {
		t.Fatalf("失败必须留下 failure.json：%v", statErr)
	}
}

func TestLightDocFlowTableRequiresNewTable(t *testing.T) {
	payload := `{"paragraph":1,"text":"第一段","rows":1,"cols":2,"values":[["A","B"]]}`
	snapshots := 0
	flow := newLightDocFlow(t, "doc-table", payload, "https://shimo.net/docs/abcdefgh",
		func(int) string { return "第一段\n" },
		func(expr string, stub *lightDocStub) any {
			if strings.Contains(expr, "hk-table-node") {
				snapshots++
				if snapshots == 1 {
					return map[string]any{"ok": true, "tables": []any{}}
				}
				return map[string]any{"ok": true, "tables": []any{
					map[string]any{"rows": "1", "cols": 2.0, "guids": []any{"g3", "g4"}, "text": "A B"},
				}}
			}
			return lightDocFlowReply(expr, stub)
		})
	if err := flow.app.cmdPhase3LightDocTable(); err != nil {
		t.Fatalf("命令应成功：%v", err)
	}
	result := readEvidenceJSON(t, flow.dir, "result.json")
	if num(result, "tableBefore") != 0 || num(result, "tableAfter") != 1 {
		t.Fatalf("表格前后计数不符：%v", result)
	}
}

func TestLightDocFlowImageRequiresPersistedReference(t *testing.T) {
	// POSIX 上也保留反斜杠，确保 Windows 路径的 JSON 转义问题能在本机复现。
	dir := filepath.Join(t.TempDir(), `image\fixture`)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	img := filepath.Join(dir, "a.png")
	if err := os.WriteFile(img, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"paragraph": 1, "text": "第一段", "file": img})
	if err != nil {
		t.Fatal(err)
	}
	counts := 0
	flow := newLightDocFlow(t, "doc-image", string(payload), "https://shimo.net/docs/abcdefgh",
		func(export int) string {
			if export == 1 {
				return "第一段\n"
			}
			return "第一段\n\n![](data:image/png;base64,AAA)\n"
		},
		func(expr string, stub *lightDocStub) any {
			if strings.Contains(expr, "hk-inline-image-node") {
				counts++
				if counts == 1 {
					return map[string]any{"ok": true, "count": 1.0}
				}
				return map[string]any{"ok": true, "count": 2.0}
			}
			return lightDocFlowReply(expr, stub)
		})
	if err := flow.app.cmdPhase3LightDocImage(); err != nil {
		t.Fatalf("命令应成功：%v", err)
	}
	result := readEvidenceJSON(t, flow.dir, "result.json")
	if num(result, "imagesBefore") != 1 || num(result, "imagesAfter") != 2 || num(result, "imagesAfterReload") != 2 {
		t.Fatalf("图片计数不符：%v", result)
	}
}

// phase3EvidenceDir 会在 --out 下再建一个 phase3-* 子目录，证据落在子目录里。
func evidencePath(t *testing.T, dir, name string) string {
	t.Helper()
	direct := filepath.Join(dir, name)
	if _, err := os.Stat(direct); err == nil {
		return direct
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "phase3-*", name))
	if len(matches) == 0 {
		t.Fatalf("证据 %s 不存在于 %s 及其 phase3-* 子目录", name, dir)
	}
	return matches[0]
}

// readEvidenceJSON 读取证据目录里的一个 JSON 文件，让断言直接对着落盘结果。
func readEvidenceJSON(t *testing.T, dir, name string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(evidencePath(t, dir, name))
	if err != nil {
		t.Fatalf("读取证据 %s 失败：%v", name, err)
	}
	var out map[string]any
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("证据 %s 不是合法 JSON：%v", name, err)
	}
	return out
}

func TestLightDocFlowWholeDocStylesWithoutParagraph(t *testing.T) {
	// 全文样式不带段落参数：范围由 scope 决定，标题段落单列而不是算进分母。
	payload := `{"scope":"all","attrs":{"fontSize":20}}`
	flow := newLightDocFlow(t, "doc-format", payload, "https://shimo.net/docs/abcdefgh",
		func(int) string { return "第一段\n\n# 标题\n" }, lightDocFlowReply)
	if err := flow.app.cmdPhase3LightDocFormat(); err != nil {
		t.Fatalf("全文样式应成功：%v", err)
	}
	result := readEvidenceJSON(t, flow.dir, "result.json")
	if result["scope"] != "all" || result["verified"] != true {
		t.Fatalf("全文范围未留证：%v", result)
	}
	if _, ok := result["paragraphTextKept"]; ok {
		t.Fatal("全文范围没有目标段落，不应出现段落正文核对结果")
	}
	marks, ok := result["marks"].(map[string]any)
	if !ok {
		t.Fatalf("全文标记证据缺失：%v", result["marks"])
	}
	size, ok := marks["fontSize"].(map[string]any)
	if !ok || num(size, "covered") != 3 || num(size, "total") != 3 {
		t.Fatalf("全文覆盖数不符：%v", size)
	}
	if num(size, "headingSkipped") != 1 {
		t.Fatalf("标题段落应单列：%v", size)
	}
	if got := int(num(map[string]any{"v": flow.stub.seen("Input.dispatchMouseEvent")}, "v")); got == 0 {
		t.Fatal("全文字号应由真实点击下发")
	}
}
