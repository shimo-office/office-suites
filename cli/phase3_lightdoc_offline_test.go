package main

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// 轻文档的动作全部落在页面上，CI 里没有登录态、也没有浏览器窗口。
// 这里只替换传输边界：一个 net.Pipe 承担的假 CDP 端点，按表达式特征返回预设值。
// 被验证的仍然是真实的定位逻辑、断言逻辑与错误分支，不是被测代码的复述。

type lightDocStub struct {
	mu      sync.Mutex
	methods []string
	exprs   []string
	reply   func(expr string) any
}

func (s *lightDocStub) seen(method string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, m := range s.methods {
		if m == method {
			n++
		}
	}
	return n
}

func (s *lightDocStub) lastExpr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.exprs) == 0 {
		return ""
	}
	return s.exprs[len(s.exprs)-1]
}

func newLightDocStub(t *testing.T, reply func(expr string) any) (*phase3EditorBrowser, *lightDocStub) {
	t.Helper()
	client, server := net.Pipe()
	stub := &lightDocStub{reply: reply}
	b := &phase3EditorBrowser{
		ws:      &WSConn{conn: client, rd: bufio.NewReader(client)},
		nextID:  1,
		navURL:  "https://shimo.net/docs/demo",
		profile: t.TempDir(),
	}
	go func() {
		defer server.Close()
		peer := &WSConn{conn: server, rd: bufio.NewReader(server)}
		for {
			raw, err := peer.readMessage()
			if err != nil {
				return
			}
			var request map[string]any
			if json.Unmarshal(raw, &request) != nil {
				continue
			}
			method := str(request, "method")
			result := map[string]any{}
			stub.mu.Lock()
			stub.methods = append(stub.methods, method)
			if method == "Runtime.evaluate" {
				params, _ := request["params"].(map[string]any)
				expr := str(params, "expression")
				stub.exprs = append(stub.exprs, expr)
			}
			stub.mu.Unlock()
			if method == "Runtime.evaluate" {
				result["result"] = map[string]any{"value": stub.reply(stub.lastExpr())}
			}
			if method == "Page.captureScreenshot" {
				result["data"] = lightDocTestPNG()
			}
			if err := peer.sendJSON(map[string]any{"id": request["id"], "result": result}); err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() { _ = client.Close() })
	return b, stub
}

// 一个覆盖全部定位请求的默认应答：坐标、标记值、段落清单都按请求里出现的特征区分。
func lightDocDefaultReply(expr string) any {
	switch {
	case strings.Contains(expr, "headingSkipped"):
		return map[string]any{"ok": true, "total": 3, "counts": map[string]any{"true": 3}, "values": []any{"true"}, "samples": []any{}, "headingSkipped": 1, "headingSamples": []any{"标题"}}
	case strings.Contains(expr, "const vals=[];const hits=[]"):
		return map[string]any{"ok": true, "values": []any{"true"}, "covered": "abc", "count": 1}
	case strings.Contains(expr, "selectNodeContents(ce)"):
		return map[string]any{"ok": true, "selected": 100, "doc": 100}
	case strings.Contains(expr, "const body=[],headings=[]"):
		return map[string]any{"ok": true, "body": []any{
			map[string]any{"index": 1, "text": "第一段"},
			map[string]any{"index": 3, "text": "第三段"},
		}, "headings": []any{map[string]any{"index": 2, "text": "标题"}}, "total": 3}
	case strings.Contains(expr, "data-shimo-pick=\"1\""):
		return map[string]any{"ok": true, "x": 40.0, "y": 50.0}
	case strings.Contains(expr, "data-shimo-pick"):
		return map[string]any{"ok": true, "mark": true, "label": "宋体", "color": "BE1A1D"}
	case strings.Contains(expr, "pickLast="):
		return map[string]any{"ok": true, "x": 30.0, "y": 40.0}
	case strings.Contains(expr, "anchorIcon="):
		return map[string]any{"ok": true, "x": 10.0, "y": 20.0, "label": "字号", "icon": ""}
	case strings.Contains(expr, "hk-heading-h("):
		return map[string]any{"ok": true, "heading": "2", "align": "center", "list": "", "indent": "1", "lineHeight": "2.5", "chain": []any{}, "text": "第一段"}
	case strings.Contains(expr, "data-hk-heading"):
		return map[string]any{"ok": true, "text": "第一段", "heading": ""}
	case strings.Contains(expr, "hk-table-node"):
		return map[string]any{"ok": true, "tables": []any{
			map[string]any{"rows": "2", "cols": 2.0, "guids": []any{"g1", "g2"}, "text": "A B"},
		}}
	case strings.Contains(expr, "hk-inline-image-node"):
		return map[string]any{"ok": true, "count": 2.0, "loaded": 2.0}
	case strings.Contains(expr, "position==='before'"):
		return map[string]any{"ok": true, "offset": 3, "paragraphText": "第一段"}
	case strings.Contains(expr, "查找"):
		return map[string]any{"ok": true}
	case strings.Contains(expr, "input,textarea"):
		return map[string]any{"ok": true, "value": "x"}
	case strings.Contains(expr, "Array.from(document.querySelectorAll('button'))"):
		return map[string]any{"ok": true, "x": 60.0, "y": 70.0}
	}
	return map[string]any{"ok": true}
}

func TestLightDocOfflineMarkValuesBothScopes(t *testing.T) {
	b, _ := newLightDocStub(t, lightDocDefaultReply)
	rangeValues, err := b.lightDocMarkValues("data-hk-font-bold", lightDocFormatSpec{Paragraph: 1, Text: "第一段", Start: 0, End: 2})
	if err != nil || len(rangeValues) != 1 || rangeValues[0] != "true" {
		t.Fatalf("区间标记读取不符：%v %v", rangeValues, err)
	}
	allValues, err := b.lightDocMarkValues("data-hk-font-size", lightDocFormatSpec{Scope: lightDocScopeAll})
	if err != nil || len(allValues) != 1 || allValues[0] != "true" {
		t.Fatalf("全文标记读取不符：%v %v", allValues, err)
	}
}

func TestLightDocOfflineSelectAllGuardsShortRange(t *testing.T) {
	b, _ := newLightDocStub(t, func(string) any { return map[string]any{"ok": true, "selected": 5, "doc": 100} })
	if _, err := b.lightDocSelectAll(); err == nil || !strings.Contains(err.Error(), "全选范围不足") {
		t.Fatalf("选区不足应被拒绝：%v", err)
	}
	b, _ = newLightDocStub(t, func(string) any { return map[string]any{"ok": false, "error": "正文编辑器未就绪"} })
	if _, err := b.lightDocSelectAll(); err == nil || !strings.Contains(err.Error(), "正文编辑器未就绪") {
		t.Fatalf("页面拒绝应原样带出：%v", err)
	}
	b, _ = newLightDocStub(t, lightDocDefaultReply)
	selection, err := b.lightDocSelectAll()
	if err != nil || int(num(selection, "selectedChars")) != 100 {
		t.Fatalf("全文选区不符：%v %v", selection, err)
	}
}

func TestLightDocOfflineBodyParagraphsSkipsHeadings(t *testing.T) {
	b, _ := newLightDocStub(t, lightDocDefaultReply)
	paras, err := b.lightDocBodyParagraphs()
	if err != nil || len(paras) != 2 || paras[0].Index != 1 || paras[1].Text != "第三段" {
		t.Fatalf("正文段落清单不符：%v %v", paras, err)
	}
	b, _ = newLightDocStub(t, func(string) any {
		return map[string]any{"ok": true, "body": []any{}, "headings": []any{map[string]any{"index": 1, "text": "全是标题"}}, "total": 1}
	})
	if _, err := b.lightDocBodyParagraphs(); err == nil || !strings.Contains(err.Error(), "没有可承载") {
		t.Fatalf("整篇标题应给出可执行错误：%v", err)
	}
}

func TestLightDocOfflineClicksUseRealMouseEvents(t *testing.T) {
	b, stub := newLightDocStub(t, lightDocDefaultReply)
	if err := b.clickControl("SvgTextBold", "", "", "", 0); err != nil {
		t.Fatal(err)
	}
	if got := stub.seen("Input.dispatchMouseEvent"); got != 3 {
		t.Fatalf("一次点击应发三个鼠标事件，实际 %d", got)
	}
	if expr := stub.lastExpr(); !strings.Contains(expr, "anchorIcon=") && !strings.Contains(expr, "SvgTextBold") {
		t.Fatalf("控件定位未带上语义锚点：%s", expr)
	}
	// 坐标为零说明控件不可见，必须报错而不是点击 (0,0)。
	b, _ = newLightDocStub(t, func(string) any { return map[string]any{"ok": true, "x": 0.0, "y": 0.0} })
	if err := b.choosePopoverItem("宋体", ""); err == nil || !strings.Contains(err.Error(), "没有可见坐标") {
		t.Fatalf("零坐标应被拒绝：%v", err)
	}
}

func TestLightDocOfflinePressKeySendsDownAndUp(t *testing.T) {
	b, stub := newLightDocStub(t, lightDocDefaultReply)
	if err := b.pressKey(lightDocKey{"Escape", "Escape", 27, 0}); err != nil {
		t.Fatal(err)
	}
	if got := stub.seen("Input.dispatchKeyEvent"); got != 2 {
		t.Fatalf("按键应发送按下与抬起两个事件，实际 %d", got)
	}
}

func TestLightDocOfflineCheckAndSelectTextRejectsMismatch(t *testing.T) {
	b, _ := newLightDocStub(t, func(expr string) any {
		if strings.Contains(expr, "data-hk-heading") {
			return map[string]any{"ok": true, "text": "实际正文", "heading": ""}
		}
		return map[string]any{"ok": true}
	})
	err := b.checkAndSelectText(2, "请求正文")
	if err == nil || !strings.Contains(err.Error(), "不匹配") || !strings.Contains(err.Error(), "实际正文") {
		t.Fatalf("段落错位应带出实际正文：%v", err)
	}
	b, _ = newLightDocStub(t, func(expr string) any {
		if strings.Contains(expr, "data-hk-heading") {
			return map[string]any{"ok": true, "text": "请求正文", "heading": ""}
		}
		return map[string]any{"ok": true}
	})
	if err := b.checkAndSelectText(2, "请求正文"); err != nil {
		t.Fatalf("正文一致时应通过：%v", err)
	}
}

func TestLightDocOfflineParaShapeAndAssertions(t *testing.T) {
	b, _ := newLightDocStub(t, lightDocDefaultReply)
	shape, err := b.lightDocParaShape(2)
	if err != nil || str(shape, "heading") != "2" {
		t.Fatalf("段落形态读取不符：%v %v", shape, err)
	}
	spec := lightDocParaSpec{Paragraph: 2, Text: "第一段", Attrs: map[string]any{"heading": 2, "align": "center", "list": "none", "indent": 1, "lineHeight": "1.5"}}
	if err := b.assertLightDocPara(spec, shape); err != nil {
		t.Fatalf("合法段落形态被判失败：%v", err)
	}
	// 每一条断言都必须能在不符时失败，否则「验收通过」没有意义。
	for _, tc := range []struct {
		name  string
		attrs map[string]any
		shape map[string]any
		want  string
	}{
		{"标题级别", map[string]any{"heading": 3}, map[string]any{"heading": "2"}, "heading"},
		{"正文级别", map[string]any{"heading": 0}, map[string]any{"heading": "2"}, "heading"},
		{"对齐", map[string]any{"align": "right"}, map[string]any{"align": "center"}, "align"},
		{"列表", map[string]any{"list": "ordered"}, map[string]any{"list": ""}, "list"},
		{"缩进", map[string]any{"indent": 1}, map[string]any{"indent": ""}, "indent"},
		{"行距缺失", map[string]any{"lineHeight": "1.5"}, map[string]any{}, "lineHeight"},
	} {
		err := b.assertLightDocPara(lightDocParaSpec{Paragraph: 2, Attrs: tc.attrs}, tc.shape)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s：应判失败，实际 %v", tc.name, err)
		}
	}
}

func TestLightDocOfflineAssertRangeAttrs(t *testing.T) {
	cases := []struct {
		name  string
		attrs map[string]any
		reply func(string) any
		ok    bool
		want  string
	}{
		{"加粗生效", map[string]any{"bold": true}, nil, true, ""},
		{"加粗未生效", map[string]any{"bold": true}, func(string) any {
			return map[string]any{"ok": true, "values": []any{""}}
		}, false, "bold"},
		{"取消加粗生效", map[string]any{"bold": false}, func(string) any {
			return map[string]any{"ok": true, "values": []any{""}}
		}, true, ""},
		{"颜色按 # 归一", map[string]any{"color": "#BE1A1D"}, func(string) any {
			return map[string]any{"ok": true, "values": []any{"#be1a1d"}}
		}, true, ""},
		{"字体按别名判等", map[string]any{"fontFamily": "宋体"}, func(string) any {
			return map[string]any{"ok": true, "values": []any{"SimSun"}}
		}, true, ""},
		{"字体不符", map[string]any{"fontFamily": "宋体"}, func(string) any {
			return map[string]any{"ok": true, "values": []any{"SimHei"}}
		}, false, "fontFamily"},
		{"字号不符", map[string]any{"fontSize": 18}, func(string) any {
			return map[string]any{"ok": true, "values": []any{"16"}}
		}, false, "fontSize"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reply := tc.reply
			if reply == nil {
				reply = func(string) any { return map[string]any{"ok": true, "values": []any{"true"}} }
			}
			b, _ := newLightDocStub(t, reply)
			evidence, err := b.assertLightDocAttrs(lightDocFormatSpec{Paragraph: 1, Text: "x", Start: 0, End: 1, Attrs: tc.attrs})
			if tc.ok {
				if err != nil {
					t.Fatalf("应通过，实际 %v", err)
				}
				if len(evidence) != len(tc.attrs) {
					t.Fatalf("证据应逐项留痕：%v", evidence)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("应判失败并点名 %q，实际 %v", tc.want, err)
			}
		})
	}
}

func TestLightDocOfflineAssertWholeDocCoverage(t *testing.T) {
	full := func(string) any {
		return map[string]any{"ok": true, "total": 3, "counts": map[string]any{"true": 3}, "headingSkipped": 1, "samples": []any{}}
	}
	b, _ := newLightDocStub(t, full)
	if _, err := b.assertLightDocDocAttrs(lightDocFormatSpec{Scope: lightDocScopeAll, Attrs: map[string]any{"bold": true}}); err != nil {
		t.Fatalf("全覆盖应通过：%v", err)
	}
	// 只覆盖一部分时必须失败，并把分布带出来。
	partial := func(string) any {
		return map[string]any{"ok": true, "total": 3, "counts": map[string]any{"true": 2, "": 1}, "headingSkipped": 0}
	}
	b, _ = newLightDocStub(t, partial)
	_, err := b.assertLightDocDocAttrs(lightDocFormatSpec{Scope: lightDocScopeAll, Attrs: map[string]any{"bold": true}})
	if err == nil || !strings.Contains(err.Error(), "未覆盖全文") {
		t.Fatalf("部分覆盖应判失败：%v", err)
	}
	// 取消加粗时，「没有 true 标记」的节点才算覆盖。
	cleared := func(string) any {
		return map[string]any{"ok": true, "total": 3, "counts": map[string]any{"": 3}, "headingSkipped": 0}
	}
	b, _ = newLightDocStub(t, cleared)
	if _, err = b.assertLightDocDocAttrs(lightDocFormatSpec{Scope: lightDocScopeAll, Attrs: map[string]any{"bold": false}}); err != nil {
		t.Fatalf("全部清除应通过：%v", err)
	}
	// 色值大小写、# 前缀不统一也要能匹配。
	mixed := func(expr string) any {
		if strings.Contains(expr, "data-hk-font-color") {
			return map[string]any{"ok": true, "total": 3, "counts": map[string]any{"#be1a1d": 3}, "headingSkipped": 0}
		}
		return map[string]any{"ok": true, "total": 3, "counts": map[string]any{"simsun": 3}, "headingSkipped": 0}
	}
	b, _ = newLightDocStub(t, mixed)
	if _, err = b.assertLightDocDocAttrs(lightDocFormatSpec{Scope: lightDocScopeAll, Attrs: map[string]any{"color": "#BE1A1D", "fontFamily": "宋体"}}); err != nil {
		t.Fatalf("色值与字体归一后应通过：%v", err)
	}
	// 整篇都是标题时不能声称覆盖。
	b, _ = newLightDocStub(t, func(string) any { return map[string]any{"ok": true, "total": 0} })
	if _, err = b.assertLightDocDocAttrs(lightDocFormatSpec{Scope: lightDocScopeAll, Attrs: map[string]any{"bold": true}}); err == nil || !strings.Contains(err.Error(), "整篇都是标题") {
		t.Fatalf("零文本节点应判失败：%v", err)
	}
}

func TestLightDocOfflineApplyToggles(t *testing.T) {
	// 已经是目标状态时不应再点一次，否则会把「取消加粗」执行成「继续加粗」。
	b, stub := newLightDocStub(t, func(expr string) any {
		if strings.Contains(expr, "const vals=[];const hits=[]") {
			return map[string]any{"ok": true, "values": []any{"true"}}
		}
		return lightDocDefaultReply(expr)
	})
	applied, err := b.applyLightDocAttrs(lightDocFormatSpec{Paragraph: 1, Text: "x", Attrs: map[string]any{"bold": true}})
	if err != nil || applied["bold"] != true {
		t.Fatalf("已加粗时应用应通过：%v %v", applied, err)
	}
	if got := stub.seen("Input.dispatchMouseEvent"); got != 0 {
		t.Fatalf("已是目标状态不应点击，实际 %d 次鼠标事件", got)
	}
	// 与目标不符时必须真实点击一次。
	b, stub = newLightDocStub(t, func(expr string) any {
		if strings.Contains(expr, "const vals=[];const hits=[]") {
			return map[string]any{"ok": true, "values": []any{""}}
		}
		return lightDocDefaultReply(expr)
	})
	if _, err = b.applyLightDocAttrs(lightDocFormatSpec{Paragraph: 1, Text: "x", Attrs: map[string]any{"italic": true}}); err != nil {
		t.Fatalf("切换应通过：%v", err)
	}
	if got := stub.seen("Input.dispatchMouseEvent"); got != 3 {
		t.Fatalf("切换应点击一次（三个鼠标事件），实际 %d", got)
	}
}

func TestLightDocOfflineWholeDocSplitsSettersAndToggles(t *testing.T) {
	b, _ := newLightDocStub(t, func(expr string) any {
		if strings.Contains(expr, "const vals=[];const hits=[]") {
			return map[string]any{"ok": true, "values": []any{"true"}}
		}
		return lightDocDefaultReply(expr)
	})
	applied, err := b.applyLightDocAttrsToBody(lightDocFormatSpec{Scope: lightDocScopeAll, Attrs: map[string]any{"bold": true}})
	if err != nil {
		t.Fatalf("仅勾选类属性应逐段下发：%v", err)
	}
	if int(num(applied, "appliedParagraphs")) != 2 || int(num(applied, "paragraphs")) != 2 {
		t.Fatalf("逐段下发应覆盖全部正文段落：%v", applied)
	}
	if _, ok := applied["setterSelection"]; ok {
		t.Fatalf("没有设值类属性时不应有全选步骤：%v", applied)
	}
	// 第一段就失败时必须停下，不能把后面几段也改一半。
	b, _ = newLightDocStub(t, func(expr string) any {
		if strings.Contains(expr, "const vals=[];const hits=[]") {
			return map[string]any{"ok": true, "values": []any{""}}
		}
		if strings.Contains(expr, "anchorIcon=") || strings.Contains(expr, "pickLast=") {
			return map[string]any{"ok": false, "error": "工具栏未找到目标控件"}
		}
		return lightDocDefaultReply(expr)
	})
	applied, err = b.applyLightDocAttrsToBody(lightDocFormatSpec{Scope: lightDocScopeAll, Attrs: map[string]any{"strike": true}})
	if err == nil || !strings.Contains(err.Error(), "已停止") {
		t.Fatalf("中途失败应停下并说明：%v", err)
	}
	if int(num(applied, "appliedParagraphs")) != 0 {
		t.Fatalf("第一段失败时不应有已下发段落：%v", applied)
	}
	if _, ok := applied["failedParagraphs"]; !ok {
		t.Fatalf("失败段号应留证：%v", applied)
	}
}

func TestLightDocOfflineReplaceFlow(t *testing.T) {
	b, stub := newLightDocStub(t, lightDocDefaultReply)
	if err := b.runLightDocReplace(lightDocReplaceSpec{Find: "旧", Replace: "新", Scope: "first"}); err != nil {
		t.Fatalf("替换流程应通过：%v", err)
	}
	if stub.seen("Input.dispatchMouseEvent") == 0 {
		t.Fatal("替换按钮必须用真实点击，不能只调 element.click()")
	}
	// 查找框没出现时要立刻失败，不能继续往空面板里填内容。
	b, _ = newLightDocStub(t, func(expr string) any {
		if strings.Contains(expr, "查找") && strings.Contains(expr, "input") {
			return map[string]any{"ok": false, "error": "查找输入框未出现"}
		}
		return lightDocDefaultReply(expr)
	})
	if err := b.runLightDocReplace(lightDocReplaceSpec{Find: "旧", Replace: "新", Scope: "all"}); err == nil || !strings.Contains(err.Error(), "查找") {
		t.Fatalf("面板未就绪应失败：%v", err)
	}
}

func TestLightDocOfflineMediaCounts(t *testing.T) {
	b, _ := newLightDocStub(t, lightDocDefaultReply)
	tables, err := b.lightDocTableShapes()
	if err != nil || len(tables) != 1 || tableCellKey(tables[0]) != "g1,g2" {
		t.Fatalf("表格快照不符：%v %v", tables, err)
	}
	images, err := b.lightDocImageCount()
	if err != nil || images != 2 {
		t.Fatalf("图片计数不符：%d %v", images, err)
	}
	b, _ = newLightDocStub(t, func(string) any { return "not-a-map" })
	if _, err = b.lightDocTableShapes(); err == nil || !strings.Contains(err.Error(), "结构无效") {
		t.Fatalf("非法快照应被拒绝：%v", err)
	}
}

func TestLightDocOfflinePureHelpers(t *testing.T) {
	if lightDocExpectAttr("bold", true) != "true" || lightDocExpectAttr("fontSize", 18) != "18" {
		t.Fatal("布尔与整数应转成页面标记里的写法")
	}
	if lightDocExpectAttr("color", "#be1a1d") != "BE1A1D" {
		t.Fatal("色值应归一成大写无 # 形式")
	}
	if !lightDocSameValue("fontFamily", "SimSun", "宋体") {
		t.Fatal("宋体应能与 SimSun 判等")
	}
	if lightDocSameValue("fontFamily", "SimHei", "宋体") {
		t.Fatal("黑体不应与宋体判等")
	}
	if !lightDocSameValue("color", "#be1a1d", "BE1A1D") || lightDocSameValue("color", "BE1A1D", "BE1A1E") {
		t.Fatal("色值比较应忽略大小写与 # 前缀，但不该放过不同值")
	}
	if plainIntString(float64(94148834)) != "94148834" {
		t.Fatal("float64 用户 ID 不应被打印成科学计数法")
	}
	if plainIntString(12.5) != "12.5" || plainIntString(" 7 ") != "7" || plainIntString(true) != "" {
		t.Fatal("用户 ID 的其它形态处理不符")
	}
	if !isLightDocURL("https://shimo.net/docs/abc") || isLightDocURL("https://shimo.net/docx/abc") {
		t.Fatal("轻文档识别应只看 /docs/")
	}
	if got := keysOf(map[string]string{"3": "c", "1": "a", "2": "b"}); got != "1,2,3" {
		t.Fatalf("失败段号应按序输出：%s", got)
	}
	if firstValue(map[string]string{"2": "second", "1": "first"}) != "first" {
		t.Fatal("应取最小段号的错误")
	}
	if got := lightDocAvailableColors(map[string]any{"available": []any{"#BE1A1D", "nope", " #FFFFFF "}}); strings.Join(got, ",") != "#BE1A1D,#FFFFFF" {
		t.Fatalf("可用色值应过滤并排序：%v", got)
	}
	if truncateRunes("一二三四五", 3) != "一二三…" || truncateRunes("一二", 3) != "一二" {
		t.Fatal("截断应按字符计且只在超长时加省略号")
	}
	if got := escapeHTMLEntities(`<a href="x">'&'</a>`); strings.ContainsAny(got, `<>"`) {
		t.Fatalf("表格单元格内容必须转义：%s", got)
	}
}

func TestLightDocOfflineTablePayload(t *testing.T) {
	spec, err := prepareLightDocTable(map[string]any{
		"paragraph": 2, "text": "第一段",
		"values": []any{[]any{"姓名", "分数"}, []any{"张三", 90}},
	})
	if err != nil {
		t.Fatalf("二维数组应可用：%v", err)
	}
	if spec.Rows != 2 || spec.Cols != 2 || spec.Values[1][1] != "90" {
		t.Fatalf("行列应由 values 推出：%+v", spec)
	}
	html, plain := lightDocTableHTML(spec)
	if !strings.Contains(html, "<td>姓名</td>") || !strings.Contains(html, "<td>90</td>") {
		t.Fatalf("表格 HTML 不符：%s", html)
	}
	if !strings.Contains(plain, "姓名\t分数") {
		t.Fatalf("纯文本副本应以制表符分列：%q", plain)
	}
	// 单元格内容必须转义，否则会被当成标签注入。
	html, _ = lightDocTableHTML(lightDocTableSpec{Rows: 1, Cols: 1, Values: [][]string{{"<b>&"}}})
	if strings.Contains(html, "<b>") || !strings.Contains(html, "&lt;b&gt;&amp;") {
		t.Fatalf("单元格内容未转义：%s", html)
	}
	for _, bad := range []map[string]any{
		{"paragraph": 0, "text": "x", "rows": 1, "cols": 1},
		{"paragraph": 1, "text": "", "rows": 1, "cols": 1},
		{"paragraph": 1, "text": "x", "position": "middle", "rows": 1, "cols": 1},
		{"paragraph": 1, "text": "x", "rows": 0, "cols": 0},
		{"paragraph": 1, "text": "x", "rows": 51, "cols": 1},
		{"paragraph": 1, "text": "x", "values": []any{"not-a-row"}},
	} {
		if _, err := prepareLightDocTable(bad); err == nil {
			t.Fatalf("非法表格请求未被拒绝：%v", bad)
		}
	}
	if spec, err := prepareLightDocTable(map[string]any{"paragraph": 1, "text": "x", "rows": 1, "cols": 1}); err != nil || spec.Position != "after" {
		t.Fatalf("缺省位置应为 after：%v %v", spec, err)
	}
}

func TestLightDocOfflineImagePayload(t *testing.T) {
	dir := t.TempDir()
	png := filepath.Join(dir, "a.PNG")
	if err := os.WriteFile(png, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	spec, err := prepareLightDocImage(map[string]any{"paragraph": 1, "text": "x", "file": png})
	if err != nil || spec.Position != "after" || lightDocImageMIME(spec.File) != "image/png" {
		t.Fatalf("合法图片请求应通过：%v %v", spec, err)
	}
	if lightDocImageMIME("a.jpg") != "image/jpeg" || lightDocImageMIME("a.webp") != "image/webp" || lightDocImageMIME("a.gif") != "image/gif" {
		t.Fatal("MIME 应按扩展名推断")
	}
	txt := filepath.Join(dir, "a.txt")
	_ = os.WriteFile(txt, []byte("x"), 0o600)
	big := filepath.Join(dir, "big.png")
	_ = os.WriteFile(big, make([]byte, lightDocImageMaxBytes+1), 0o600)
	for name, payload := range map[string]map[string]any{
		"段落序号":  {"paragraph": 0, "text": "x", "file": png},
		"正文为空":  {"paragraph": 1, "text": " ", "file": png},
		"位置非法":  {"paragraph": 1, "text": "x", "file": png, "position": "middle"},
		"缺文件":   {"paragraph": 1, "text": "x"},
		"文件不存在": {"paragraph": 1, "text": "x", "file": filepath.Join(dir, "missing.png")},
		"超限":    {"paragraph": 1, "text": "x", "file": big},
		"格式不支持": {"paragraph": 1, "text": "x", "file": txt},
	} {
		if _, err := prepareLightDocImage(payload); err == nil {
			t.Fatalf("%s：非法图片请求未被拒绝", name)
		}
	}
}

// 五个动作在碰到网络与浏览器之前就必须拒绝写错的请求：这条路径在 CI 里能完整跑。
func TestLightDocOfflineCommandsRejectBeforeNetwork(t *testing.T) {
	// 图片动作会先读本地文件再校验目标，所以这里必须给一个真实存在的图片，
	// 否则测到的是「文件不存在」而不是「目标不合法」。
	img := filepath.Join(t.TempDir(), "a.png")
	if err := os.WriteFile(img, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := map[string][]string{
		"doc-format":    {"phase3", "https://shimo.net/docs/abcdefgh", "--action", "doc-format", "--payload", `{"paragraph":1,"text":"x","start":0,"end":1,"attrs":{"bold":true}}`},
		"doc-paragraph": {"phase3", "https://shimo.net/docs/abcdefgh", "--action", "doc-paragraph", "--payload", `{"paragraph":1,"text":"x","attrs":{"align":"center"}}`},
		"doc-replace":   {"phase3", "https://shimo.net/docs/abcdefgh", "--action", "doc-replace", "--payload", `{"find":"a","replace":"b"}`},
		"doc-table":     {"phase3", "https://shimo.net/docs/abcdefgh", "--action", "doc-table", "--payload", `{"paragraph":1,"text":"x","rows":1,"cols":1}`},
		"doc-image":     {"phase3", "https://shimo.net/docs/abcdefgh", "--action", "doc-image", "--payload", `{"paragraph":1,"text":"x","file":"` + img + `"}`},
	}
	run := map[string]func(*App) error{
		"doc-format":    func(a *App) error { return a.cmdPhase3LightDocFormat() },
		"doc-paragraph": func(a *App) error { return a.cmdPhase3LightDocParagraph() },
		"doc-replace":   func(a *App) error { return a.cmdPhase3LightDocReplace() },
		"doc-table":     func(a *App) error { return a.cmdPhase3LightDocTable() },
		"doc-image":     func(a *App) error { return a.cmdPhase3LightDocImage() },
	}
	for action, argv := range base {
		call := run[action]
		t.Run(action+"/缺链接", func(t *testing.T) {
			a := newApp([]string{"phase3", "--action", action, "--payload", "{}"})
			a.Origin = "https://shimo.net"
			if err := call(a); err == nil || !strings.Contains(err.Error(), "需要一个轻文档链接") {
				t.Fatalf("缺链接应被拒绝：%v", err)
			}
		})
		t.Run(action+"/payload 非法", func(t *testing.T) {
			argv := append([]string{}, argv...)
			argv[len(argv)-1] = "{not json"
			a := newApp(argv)
			a.Origin = "https://shimo.net"
			if err := call(a); err == nil {
				t.Fatal("非法 payload 应被拒绝")
			}
		})
		t.Run(action+"/跨域", func(t *testing.T) {
			argv := append([]string{}, argv...)
			argv[1] = "https://other.example/docs/abcdefgh"
			a := newApp(argv)
			a.Origin = "https://shimo.net"
			if err := call(a); err == nil || !strings.Contains(err.Error(), "origin 不符") {
				t.Fatalf("跨域目标应被拒绝：%v", err)
			}
		})
		t.Run(action+"/非轻文档", func(t *testing.T) {
			argv := append([]string{}, argv...)
			argv[1] = "https://shimo.net/docx/abcdefgh"
			a := newApp(argv)
			a.Origin = "https://shimo.net"
			if err := call(a); err == nil || !strings.Contains(err.Error(), "不是轻文档") {
				t.Fatalf("文稿链接应被拒绝：%v", err)
			}
		})
		// 走到导出这一步会因为没有登录态而失败；此时必须留下证据目录，
		// 并明确说明「未执行写入」，不能让人以为内容已经改过。
		t.Run(action+"/导出失败留证", func(t *testing.T) {
			t.Setenv("SHIMO_NET_COOKIE", "")
			t.Setenv("SHIMO_NET_HOME", t.TempDir())
			argv := append([]string{}, argv...)
			argv = append(argv, "--out", t.TempDir())
			a := newApp(argv)
			a.Origin = "https://shimo.net"
			err := call(a)
			if err == nil {
				t.Fatal("没有登录态时导出应失败")
			}
			if !strings.Contains(err.Error(), "未执行写入") || !strings.Contains(err.Error(), "证据") {
				t.Fatalf("失败信息应说明写入状态并指出证据目录：%v", err)
			}
		})
	}
}

func TestLightDocOfflineFormatKeepsRangeWithSurrogates(t *testing.T) {
	// 全选与区间脚本必须用字符（码点）而不是字节偏移，否则中文与表情会错位。
	js := lightDocSelectJS(2, "🙂中文", 1, 3)
	if !strings.Contains(js, "Array.from") {
		t.Fatalf("字符偏移应按码点计算：%s", js)
	}
	if strings.Contains(js, "🙂中文\",") {
		t.Fatal("请求文本应被 JSON 转义后嵌入")
	}
	// 段落读取脚本要带出标题标记，供定位断言使用。
	if !strings.Contains(lightDocParagraphTextJS(4), "data-hk-heading") {
		t.Fatal("段落读取缺少标题标记")
	}
	// 光标脚本要区分 before / after。
	before := lightDocCursorJS(1, "x", "before")
	after := lightDocCursorJS(1, "x", "after")
	if !strings.Contains(before, `"position":"before"`) || !strings.Contains(after, `"position":"after"`) {
		t.Fatal("光标位置未写进脚本")
	}
}

func TestLightDocOfflineCloseReleasesPipe(t *testing.T) {
	b, stub := newLightDocStub(t, lightDocDefaultReply)
	profile := b.profile
	b.Close()
	if stub.seen("Browser.close") != 1 {
		t.Fatalf("关闭时应通知浏览器，实际 %d 次", stub.seen("Browser.close"))
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatalf("关闭后应清掉临时 profile：%v", err)
	}
}

func TestLightDocOfflineApplyParagraphAttrs(t *testing.T) {
	// 段落样式、对齐、行距走浮层，列表与缩进走图标按钮；这里验证每种都有真实点击。
	b, stub := newLightDocStub(t, lightDocDefaultReply)
	spec := lightDocParaSpec{Paragraph: 1, Text: "第一段", Attrs: map[string]any{
		"heading": 2, "align": "center", "list": "ordered", "indent": 1, "lineHeight": "1.5",
	}}
	if err := b.applyLightDocPara(spec); err != nil {
		t.Fatalf("段落属性应用应通过：%v", err)
	}
	if stub.seen("Input.dispatchMouseEvent") < 12 {
		t.Fatalf("每类段落属性都应产生真实点击，实际 %d 个鼠标事件", stub.seen("Input.dispatchMouseEvent"))
	}
	// 减少缩进时要换成减号图标，否则会把缩进越推越深。
	b, stub = newLightDocStub(t, lightDocDefaultReply)
	if err := b.applyLightDocPara(lightDocParaSpec{Paragraph: 1, Text: "x", Attrs: map[string]any{"indent": -2}}); err != nil {
		t.Fatalf("减少缩进应通过：%v", err)
	}
	if !strings.Contains(strings.Join(stubExprs(stub), " "), "SvgTextIndentDecrease") {
		t.Fatal("负缩进应点击减少缩进控件")
	}
	// list=none 表示取消列表，此时不应再点列表按钮。
	b, stub = newLightDocStub(t, lightDocDefaultReply)
	if err := b.applyLightDocPara(lightDocParaSpec{Paragraph: 1, Text: "x", Attrs: map[string]any{"list": "none"}}); err != nil {
		t.Fatalf("取消列表应通过：%v", err)
	}
	if stub.seen("Input.dispatchMouseEvent") != 0 {
		t.Fatal("取消列表不应点击列表按钮")
	}
	// 标题名称要映射到面板里的选项文本。
	for _, value := range []any{"title", "subtitle", 0} {
		b, stub = newLightDocStub(t, lightDocDefaultReply)
		if err := b.applyLightDocPara(lightDocParaSpec{Paragraph: 1, Text: "x", Attrs: map[string]any{"heading": value}}); err != nil {
			t.Fatalf("heading=%v 应通过：%v", value, err)
		}
		if !strings.Contains(strings.Join(stubExprs(stub), " "), "wantText") {
			t.Fatalf("heading=%v 应打开段落样式浮层", value)
		}
	}
	// 浮层打不开时必须报错，不能当成已应用。
	b, _ = newLightDocStub(t, func(expr string) any {
		if strings.Contains(expr, "data-shimo-pick") {
			return map[string]any{"ok": false, "error": "浮层中没有匹配的选项"}
		}
		return lightDocDefaultReply(expr)
	})
	if err := b.applyLightDocPara(lightDocParaSpec{Paragraph: 1, Text: "x", Attrs: map[string]any{"heading": 3}}); err == nil || !strings.Contains(err.Error(), "段落样式") {
		t.Fatalf("浮层缺失应报错：%v", err)
	}
}

func TestLightDocOfflineApplyValueSetters(t *testing.T) {
	// 字号与字色是「设值」类：必须先在工具栏打开面板，再在浮层里选中目标值。
	b, _ := newLightDocStub(t, lightDocDefaultReply)
	applied, err := b.applyLightDocAttrs(lightDocFormatSpec{Paragraph: 1, Text: "x", Attrs: map[string]any{"fontSize": 18, "color": "#BE1A1D"}})
	if err != nil {
		t.Fatalf("设值类属性应通过：%v", err)
	}
	if applied["fontSize"] != 18 || applied["color"] != "#BE1A1D" {
		t.Fatalf("设值结果未留证：%v", applied)
	}
	// 面板打不开时报错要指明是哪一类属性，便于排查是控件变了还是版面变了。
	b, _ = newLightDocStub(t, func(expr string) any {
		if strings.Contains(expr, "anchorIcon=") {
			return map[string]any{"ok": false, "error": "工具栏未找到目标控件"}
		}
		return lightDocDefaultReply(expr)
	})
	if _, err = b.applyLightDocAttrs(lightDocFormatSpec{Paragraph: 1, Text: "x", Attrs: map[string]any{"fontSize": 18}}); err == nil || !strings.Contains(err.Error(), "字号") {
		t.Fatalf("字号面板缺失应点名：%v", err)
	}
}

func TestLightDocOfflineLineHeightLabel(t *testing.T) {
	b, stub := newLightDocStub(t, func(expr string) any {
		if strings.Contains(expr, "cg-editor-select--item") {
			return map[string]any{"ok": true, "label": "1.5 倍"}
		}
		return lightDocDefaultReply(expr)
	})
	label, err := b.lightDocLineHeightLabel()
	if err != nil || label != "1.5 倍" {
		t.Fatalf("行距面板读数不符：%q %v", label, err)
	}
	if stub.seen("Input.dispatchMouseEvent") == 0 {
		t.Fatal("行距面板应由真实点击打开")
	}
	// 面板里没有选中项时不能返回空字符串冒充成功。
	b, _ = newLightDocStub(t, func(expr string) any {
		if strings.Contains(expr, "cg-editor-select--item") {
			return map[string]any{"ok": false, "error": "行距面板没有选中项"}
		}
		return lightDocDefaultReply(expr)
	})
	if _, err = b.lightDocLineHeightLabel(); err == nil {
		t.Fatal("没有选中项时应失败")
	}
}

// stubExprs 取出桩收到过的全部表达式，用于断言「点了哪个控件」。
func stubExprs(s *lightDocStub) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.exprs...)
}
