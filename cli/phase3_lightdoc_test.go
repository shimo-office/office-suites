package main

import (
	"strings"
	"testing"
)

func TestLightDocFormatSpecValidation(t *testing.T) {
	base := func() map[string]any {
		return map[string]any{"paragraph": 2, "text": "第一段文字", "start": 1, "end": 3, "attrs": map[string]any{"bold": true}}
	}
	if _, err := prepareLightDocFormat(base()); err != nil {
		t.Fatalf("合法请求被拒：%v", err)
	}
	type mutateFunc func(map[string]any)
	dropStart := mutateFunc(func(m map[string]any) { delete(m, "start") })
	dropEnd := mutateFunc(func(m map[string]any) { delete(m, "end") })
	zeroParagraph := mutateFunc(func(m map[string]any) { m["paragraph"] = 0 })
	blankText := mutateFunc(func(m map[string]any) { m["text"] = "  " })
	overflow := mutateFunc(func(m map[string]any) { m["end"] = 99 })
	reversed := mutateFunc(func(m map[string]any) { m["start"], m["end"] = 3, 1 })
	emptyAttrs := mutateFunc(func(m map[string]any) { m["attrs"] = map[string]any{} })
	unknownAttr := mutateFunc(func(m map[string]any) { m["attrs"] = map[string]any{"shadow": true} })
	badColor := mutateFunc(func(m map[string]any) { m["attrs"] = map[string]any{"color": "red"} })
	badSize := mutateFunc(func(m map[string]any) { m["attrs"] = map[string]any{"fontSize": 300} })
	badBool := mutateFunc(func(m map[string]any) { m["attrs"] = map[string]any{"bold": "yes"} })
	badAlign := mutateFunc(func(m map[string]any) { m["attrs"] = map[string]any{"verticalAlign": "up"} })

	cases := []struct {
		name   string
		mutate mutateFunc
		want   string
	}{
		{"缺 start", dropStart, "start"},
		{"缺 end", dropEnd, "end"},
		{"段落序号为 0", zeroParagraph, "paragraph"},
		{"空正文", blankText, "text"},
		{"范围越界", overflow, "超出段落长度"},
		{"范围反向", reversed, "超出段落长度"},
		{"空 attrs", emptyAttrs, "attrs"},
		{"未知属性", unknownAttr, "不支持的字符格式"},
		{"颜色非法", badColor, "颜色"},
		{"字号越界", badSize, "fontSize"},
		{"布尔类型错", badBool, "布尔"},
		{"上下标取值错", badAlign, "verticalAlign"},
	}
	for _, c := range cases {
		m := base()
		c.mutate(m)
		_, err := prepareLightDocFormat(m)
		if err == nil {
			t.Fatalf("%s：应被拒绝", c.name)
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s：错误信息缺少 %q，实际 %v", c.name, c.want, err)
		}
	}
}

func TestLightDocFormatNormalizesAttrs(t *testing.T) {
	spec, err := prepareLightDocFormat(map[string]any{
		"paragraph": 1, "text": "abc", "start": 0, "end": 3,
		"attrs": map[string]any{"color": "#f00", "fontSize": float64(18)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Attrs["color"] != "#FF0000" {
		t.Fatalf("三位色未展开：%v", spec.Attrs["color"])
	}
	if spec.Scope != "range" {
		t.Fatalf("缺省范围应为 range，实际 %q", spec.Scope)
	}
	if spec.Attrs["fontSize"] != 18 {
		t.Fatalf("字号未转成整数：%v(%T)", spec.Attrs["fontSize"], spec.Attrs["fontSize"])
	}
}

// 轻文档色板没有"无/清除"格，接口上声明 NONE 会变成假成功，因此在参数层就拒绝。
func TestLightDocColorsRejectClear(t *testing.T) {
	for _, key := range []string{"color", "highlight"} {
		for _, value := range []string{"NONE", "none", "clear"} {
			_, err := prepareLightDocFormat(map[string]any{
				"paragraph": 1, "text": "abc", "start": 0, "end": 3,
				"attrs": map[string]any{key: value},
			})
			if err == nil {
				t.Fatalf("%s=%s 应被拒绝", key, value)
			}
			if !strings.Contains(err.Error(), "没有清选项") {
				t.Fatalf("%s=%s 错误信息不可执行：%v", key, value, err)
			}
		}
	}
}

func TestLightDocFormatScopeAll(t *testing.T) {
	spec, err := prepareLightDocFormat(map[string]any{
		"scope": "all", "attrs": map[string]any{"fontSize": 20, "bold": true},
	})
	if err != nil {
		t.Fatalf("全文范围被拒：%v", err)
	}
	if spec.Scope != lightDocScopeAll {
		t.Fatalf("范围未识别：%q", spec.Scope)
	}
	if spec.Attrs["fontSize"] != 20 {
		t.Fatalf("字号未保留：%v", spec.Attrs["fontSize"])
	}
	// 全文范围不接段内字符区间，缺 paragraph 也要能用。
	if _, err := prepareLightDocFormat(map[string]any{"scope": "all"}); err == nil {
		t.Fatal("缺 attrs 应被拒绝")
	}
	// 上下标在轻文档上没有入口，参数层就要拒绝，不能收下再失败。
	for _, scope := range []string{"all", "range"} {
		m := map[string]any{"scope": scope, "attrs": map[string]any{"verticalAlign": "superscript"}}
		if scope == "range" {
			m["paragraph"], m["text"], m["start"], m["end"] = 1, "abc", 0, 1
		}
		_, err = prepareLightDocFormat(m)
		if err == nil || !strings.Contains(err.Error(), "工具栏没有上下标入口") {
			t.Fatalf("%s 范围应拒绝 verticalAlign，实际 %v", scope, err)
		}
	}
	_, err = prepareLightDocFormat(map[string]any{
		"scope": "bogus", "attrs": map[string]any{"bold": true},
	})
	if err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("未知范围应被拒绝，实际 %v", err)
	}
}

// 全文断言按可承载行内标记的段落计数，标题必须被排除，否则合法结果会被判失败。
func TestLightDocDocMarkJSExcludesHeadings(t *testing.T) {
	js := lightDocDocMarkJS("data-hk-font-size")
	for _, want := range []string{"hk-heading-node", "headingSkipped", "total"} {
		if !strings.Contains(js, want) {
			t.Fatalf("全文标记断言缺少 %q", want)
		}
	}
}

func TestLightDocParagraphSpecValidation(t *testing.T) {
	if _, err := prepareLightDocParagraph(map[string]any{"paragraph": 1, "text": "x", "attrs": map[string]any{"heading": 2}}); err != nil {
		t.Fatalf("合法请求被拒：%v", err)
	}
	if spec, err := prepareLightDocParagraph(map[string]any{"paragraph": 1, "text": "x", "attrs": map[string]any{"heading": "标题2"}}); err != nil || spec.Attrs["heading"] != 2 {
		t.Fatalf("中文标题级别未解析：%v %v", spec.Attrs, err)
	}
	bad := []map[string]any{
		{"paragraph": 0, "text": "x", "attrs": map[string]any{"heading": 1}},
		{"paragraph": 1, "text": "", "attrs": map[string]any{"heading": 1}},
		{"paragraph": 1, "text": "x", "attrs": map[string]any{"heading": 9}},
		{"paragraph": 1, "text": "x", "attrs": map[string]any{"align": "middle"}},
		{"paragraph": 1, "text": "x", "attrs": map[string]any{"list": "bullet"}},
		{"paragraph": 1, "text": "x", "attrs": map[string]any{"indent": 0}},
		{"paragraph": 1, "text": "x", "attrs": map[string]any{"unknown": 1}},
	}
	for i, m := range bad {
		if _, err := prepareLightDocParagraph(m); err == nil {
			t.Fatalf("第 %d 个非法请求未被拒绝：%v", i, m)
		}
	}
}

func TestLightDocReplaceSpecValidation(t *testing.T) {
	spec, err := prepareLightDocReplace(map[string]any{"find": "a", "replace": "b"})
	if err != nil || spec.Scope != "all" {
		t.Fatalf("默认 scope 应为 all：%v %v", spec, err)
	}
	if _, err := prepareLightDocReplace(map[string]any{"find": "", "replace": "b"}); err == nil {
		t.Fatal("空查找词应被拒绝")
	}
	if _, err := prepareLightDocReplace(map[string]any{"find": "a"}); err == nil {
		t.Fatal("缺 replace 应被拒绝")
	}
	if _, err := prepareLightDocReplace(map[string]any{"find": "a", "replace": "b", "scope": "page"}); err == nil {
		t.Fatal("未知 scope 应被拒绝")
	}
}

func TestLightDocPlainMarkdownKeepsText(t *testing.T) {
	md := "~~第二段~~：*~~XYZ-12~~*345\n"
	if got := lightDocPlainMarkdown(md); got != "第二段：XYZ-12345\n" {
		t.Fatalf("去标记结果不符：%q", got)
	}
}

func TestLightDocFontAliases(t *testing.T) {
	alias := strings.Join(lightDocFontAliases("宋体"), ",")
	if !strings.Contains(alias, "simsun") {
		t.Fatalf("宋体应映射到 simsun，实际 %s", alias)
	}
	if got := lightDocFontAliases("某不存在字体"); len(got) != 1 {
		t.Fatalf("未知字体应只返回自身，实际 %v", got)
	}
}

func TestLightDocSelectJSQuotesRequestText(t *testing.T) {
	js := lightDocSelectJS(1, `a"b</script><img src=x>`, 0, 1)
	if strings.Contains(js, "</script>") {
		t.Fatal("请求文本未做转义")
	}
	if !strings.Contains(js, `a\"b`) {
		t.Fatalf("请求文本应被 JSON 转义：%s", js)
	}
}

func TestLightDocPresenceRequiresSelf(t *testing.T) {
	js := lightDocPresenceJS("94148834")
	if !strings.Contains(js, "94148834") {
		t.Fatal("协作门禁未带上当前账号")
	}
	if !strings.Contains(js, "presence.ids.length") && !strings.Contains(js, "others") {
		t.Fatal("协作门禁应检查是否存在其他在线用户")
	}
}
