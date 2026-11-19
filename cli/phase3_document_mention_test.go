package main

import (
	"os/exec"
	"testing"
)

func TestDocumentMentionRequestAndProof(t *testing.T) {
	s, err := prepareDocumentMention(map[string]any{"paragraph": float64(1), "text": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Start != 5 || s.End != 5 {
		t.Fatal("mention must append, never replace original text")
	}
	if _, err = prepareDocumentMention(map[string]any{"paragraph": float64(1), "text": "hello", "userId": "someone-else"}); err == nil {
		t.Fatal("allowed nonself user")
	}
	before := documentSnapshot{Paragraphs: []documentParagraph{{Text: "hello", Chars: []documentChar{{Text: "hello", Attrs: map[string]string{}}}}}}
	after := documentSnapshot{Paragraphs: []documentParagraph{{Text: "hello@Self", Chars: []documentChar{{Text: "hello", Attrs: map[string]string{}}, {Text: "@Self", Attrs: map[string]string{"color": "0000FF"}}}}}}
	if err = verifyDocumentMention(before, after, s, "Self"); err != nil {
		t.Fatal(err)
	}
	if err = verifyDocumentMention(before, before, s, "Self"); err == nil {
		t.Fatal("accepted no-op")
	}
	after.Paragraphs[0].Chars[0].Attrs["bold"] = "true"
	if err = verifyDocumentMention(before, after, s, "Self"); err == nil {
		t.Fatal("accepted collateral format change")
	}
}

func TestDocumentMentionRequiresRealUserNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable")
	}
	spec, _ := prepareDocumentMention(map[string]any{"paragraph": float64(1), "text": "hello"})
	script := `global.window=global;global.btoa=s=>Buffer.from(s,'binary').toString('base64');global.atob=s=>Buffer.from(s,'base64').toString('binary');let writes=0;const p={children:[{getText:()=>"hello",children:Array.from("hello")}]};const plugin={canAdd:()=>true,add:props=>{if(props.atType!==1||props.atInfo.userId!==7||props.getText()!=='@Self')throw Error('fake mention props');writes++}};const e={config:{mode:{role:'editor'}},docsApi:{isViewMode:false,pluginManager:{getInstanceAsync:async()=>plugin}},getParagraphs:()=>[p],getParagraphRange:()=>({start:btoa('0,3,|0|0|0'),getText:()=>"hello\r\n"}),generateRange:r=>({...r,getText:()=>''}),runSelectionCallback:(range,fn)=>fn()};global._sdk={editor:e,options:{user:{id:7}}};global.__shimoPhase3Presence=()=>({ready:true,connected:true,ids:['7']});(async()=>{const good=await ` + documentMentionJS(spec, "7", "Self") + `;if(!good.ok||writes!==1)throw Error(JSON.stringify(good));writes=0;const other=await ` + documentMentionJS(spec, "8", "Other") + `;if(other.ok||writes)throw Error('nonself mention allowed');e.getParagraphRange=()=>({getText:()=>"hello@Self\r\n"});p.children=[{getText:()=>"hello@Self"}];const fake=await ` + documentMentionReadbackJS(spec, "7", "Self") + `;if(fake.ok)throw Error('plain text accepted as mention');p.children=[{getText:()=>"hello"},{atType:1,atInfo:{userId:7},getText:()=>"@Self",getId:()=>"real-node"}];const real=await ` + documentMentionReadbackJS(spec, "7", "Self") + `;if(!real.ok||real.nodeId!=='real-node')throw Error(JSON.stringify(real));p.children[1].atInfo.userId=8;const wrong=await ` + documentMentionReadbackJS(spec, "7", "Self") + `;if(wrong.ok)throw Error('wrong mention user accepted')})().catch(e=>{console.error(e);process.exitCode=1})`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

func TestDocumentMentionReadbackNativeAccumulator(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable")
	}
	spec, _ := prepareDocumentMention(map[string]any{"paragraph": float64(1), "text": "hello"})
	script := `global.window=global;const m={atType:1,atInfo:{userId:7},getId:()=>"native",getText:out=>{if(out)out.Text+='@Self'}};const p={children:[m]};global._sdk={editor:{getParagraphs:()=>[p],getParagraphRange:()=>({getText:async()=>"hello@Self\r\n"})}};(async()=>{let r=await ` + documentMentionReadbackJS(spec, "7", "Self") + `;if(!r.ok||r.displayText!=='@Self'||r.nodeId!=='native')throw Error(JSON.stringify(r));m.atInfo.userId=8;r=await ` + documentMentionReadbackJS(spec, "7", "Self") + `;if(r.ok)throw Error('wrong ID accepted');m.atInfo.userId=7;m.getText=out=>{if(out)out.Text+='@Wrong'};r=await ` + documentMentionReadbackJS(spec, "7", "Self") + `;if(r.ok)throw Error('wrong label accepted')})().catch(e=>{console.error(e);process.exitCode=1})`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
