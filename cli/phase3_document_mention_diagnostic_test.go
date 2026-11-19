package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPhase3MentionReadOnlyDiagnostic(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_MENTION_DIAGNOSTIC") != "1" {
		t.Skip("explicit private-fixture readback, no mention insertion")
	}
	parent := "../tmp/phase3-live/docx-slides/32-mention"
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(parent, "readback-")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("evidence", dir)
	app := newApp([]string{"phase3"})
	artifact, err := app.phase3OfficeExport("0qRl5elMORfyYJWE", filepath.Join(dir, "export"), "docx")
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "artifact.json"), artifact); err != nil {
		t.Fatal(err)
	}
	b, err := openPhase3EditorWhen("https://shimo.net/docx/0qRl5elMORfyYJWE", documentReadyJS)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	value, err := b.EvalReadOnly(`(async()=>{const e=window._sdk.editor,p=e.getParagraphs()[0];const text=n=>{if(typeof n.getText!=='function')return null;const v=n.getText();return typeof v==='string'?v:{type:typeof v,tag:Object.prototype.toString.call(v)}};return {ok:true,paragraphText:await e.getParagraphRange(p).getText(),nodes:p.children.map(n=>({atType:n.atType,nodeId:n.getId?.(),text:text(n),atInfo:n.atInfo?{keys:Object.keys(n.atInfo),userId:n.atInfo.userId,id:n.atInfo.id,name:n.atInfo.name}:null,keys:Object.keys(n),textSource:typeof n.getText==='function'?String(n.getText).slice(0,1200):null,children:Array.isArray(n.children)?n.children.map(c=>({text:text(c),atType:c.atType,keys:Object.keys(c)})):null}))}})()`)
	if err != nil {
		t.Fatal(err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "native-nodes.json"), value); err != nil {
		t.Fatal(err)
	}
	if err = b.Capture(filepath.Join(dir, "readback.png")); err != nil {
		t.Fatal(err)
	}
}

// Recovery reads an already-applied mention; it never calls the insertion path.
func TestPhase3MentionReadOnlyRecovery(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_MENTION_RECOVERY") != "1" {
		t.Skip("explicit read-only closure of existing private-fixture write")
	}
	const original = "../tmp/phase3-live/docx-slides/acceptance-driver/runs/self-mention/20261118T185113395481Z/phase3-1052607438"
	files, err := filepath.Glob(filepath.Join(original, "before", "*.docx"))
	if err != nil || len(files) != 1 {
		t.Fatalf("baseline unavailable: %v %v", files, err)
	}
	before, err := readDocumentSnapshot(files[0])
	if err != nil {
		t.Fatal(err)
	}
	spec, err := prepareDocumentMention(map[string]any{"paragraph": float64(1), "text": "PHASE3_SELF_MENTION_20261118"})
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("../tmp/phase3-live/docx-slides/32-mention", "recovery-")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("evidence", dir)
	app := newApp([]string{"phase3"})
	me, err := app.usersMe()
	if err != nil {
		t.Fatal(err)
	}
	id, name, err := documentMentionIdentity(me)
	if err != nil {
		t.Fatal(err)
	}
	b, err := openPhase3EditorWhen("https://shimo.net/docx/0qRl5elMORfyYJWE", documentReadyJS)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	evidence := map[string]any{"originalFailedRun": original, "mutationAttempted": false, "originalBeforeArtifact": files[0], "scope": "read-only recovery; original command failure preserved"}
	for _, stage := range []string{"after", "reload"} {
		if stage == "reload" {
			if _, err = b.EvalReadOnly(`(()=>{Object.defineProperty(window,"__shimoPhase3BeforeReload",{value:true});return {ok:true}})()`); err != nil {
				t.Fatal(err)
			}
			if _, err = b.ws.Call(8000, "Page.reload", map[string]any{"ignoreCache": true}, 15*time.Second); err != nil {
				t.Fatal(err)
			}
			if err = b.WaitReady(`!window.__shimoPhase3BeforeReload&&(`+documentReadyJS+`)`, phase3EditorReadyTimeout); err != nil {
				t.Fatal(err)
			}
		}
		node, err := b.EvalReadOnly(documentMentionReadbackJS(spec, id, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = phase3WriteEvidence(filepath.Join(dir, stage+"-native.json"), node); err != nil {
			t.Fatal(err)
		}
		artifact, err := app.phase3OfficeExport("0qRl5elMORfyYJWE", filepath.Join(dir, stage), "docx")
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := readDocumentSnapshot(artifact.Path)
		if err != nil {
			t.Fatal(err)
		}
		if err = verifyDocumentMention(before, snapshot, spec, name); err != nil {
			t.Fatal(err)
		}
		if err = phase3WriteEvidence(filepath.Join(dir, stage+"-readback.json"), snapshot); err != nil {
			t.Fatal(err)
		}
		if err = b.Capture(filepath.Join(dir, stage+".png")); err != nil {
			t.Fatal(err)
		}
		evidence[stage+"Artifact"] = artifact
		evidence[stage+"Native"] = node
	}
	evidence["verified"] = true
	if err = phase3WriteEvidence(filepath.Join(dir, "recovery-result.json"), evidence); err != nil {
		t.Fatal(err)
	}
}
