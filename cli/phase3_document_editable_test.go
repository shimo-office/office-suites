package main

import (
	"os/exec"
	"testing"
)

func TestDocumentEditableReadinessRequiresActualEditRole(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable")
	}
	script := `global.window={};const ready=()=>Boolean(` + documentEditableReadyJS + `);if(ready())throw Error('empty editor accepted');window._sdk={editor:{getParagraphs:()=>[],docsApi:{setFontSize:()=>{},isViewMode:false},config:{mode:{}},canHandleChanges:{}}};if(ready())throw Error('role not yet known accepted');const e=window._sdk.editor;e.config.mode.role='viewer';if(ready())throw Error('viewer accepted');e.config.mode.role='editor';if(!ready())throw Error('native object gate rejected');e.docsApi.isViewMode=true;if(ready())throw Error('view mode accepted');e.docsApi.isViewMode=false;e.canHandleChanges=false;if(ready())throw Error('blocked changes accepted');`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
