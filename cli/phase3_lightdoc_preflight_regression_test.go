package main

import (
	"strings"
	"testing"
)

func TestLightDocInvalidColorRejectedBeforeFontSizeWrite(t *testing.T) {
	flow := newLightDocFlow(t, "doc-format", `{"paragraph":1,"text":"第一段","start":0,"end":3,"attrs":{"fontSize":16,"color":"#2563EB"}}`, "https://shimo.net/docs/abcdefgh", func(int) string { return "第一段" }, func(expr string, stub *lightDocStub) any {
		if strings.Contains(expr, `wantColor="2563EB"`) {
			return map[string]any{"ok": false, "error": "当前色板无此颜色"}
		}
		return lightDocFlowReply(expr, stub)
	})
	err := flow.app.cmdPhase3LightDocFormat()
	if err == nil {
		t.Fatal("unavailable palette color accepted")
	}
	failure := readEvidenceJSON(t, flow.dir, "failure.json")
	if failure["mutationAttempted"] != false {
		t.Fatalf("font size must not change before invalid color rejected: %v", failure)
	}
	for _, expr := range flow.stub.exprs {
		if strings.Contains(expr, `document.querySelector('[data-shimo-pick="1"]')`) {
			t.Fatal("preflight clicked a formatting value")
		}
	}
}
