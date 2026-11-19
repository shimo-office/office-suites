package main

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestPhase3SlidePlaceholderSpec(t *testing.T) {
	s, e := preparePhase3Slides("slide-placeholder-fill", map[string]any{"slideId": "a", "shapeName": "Title 1", "text": "三期验收\n第二行"})
	if e != nil || s.Placeholder == nil {
		t.Fatalf("%v", e)
	}
	for _, p := range []map[string]any{{"slideId": "a", "shapeName": "Title 1", "text": ""}, {"slideId": "a", "text": "title"}, {"slideId": "a", "shapeName": "Title 1", "text": "title", "fontSize": float64(28)}} {
		if _, e := preparePhase3Slides("slide-placeholder-fill", p); e == nil {
			t.Fatal("invalid placeholder payload accepted")
		}
	}
}
func TestPhase3SlidePlaceholderTarget(t *testing.T) {
	raw := `<p:sp xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><p:nvSpPr><p:cNvPr id="7" name="Title 1"/><p:nvPr><p:ph type="title"/></p:nvPr></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>三期验收</a:t></a:r></a:p><a:p><a:r><a:t>第二行</a:t></a:r></a:p></p:txBody></p:sp>`
	n, _ := parseArtifactXML([]byte(raw))
	s := phase3SlidePlaceholderSpec{Name: "Title 1", Text: "三期验收\n第二行"}
	if _, e := phase3PlaceholderTarget(n, s, true); e != nil {
		t.Fatal(e)
	}
	for _, v := range [][2]string{{`<p:ph type="title"/>`, ""}, {`第二行`, `第三行`}, {`name="Title 1"`, `name="Other"`}, {`<a:r><a:t>三期验收`, `<a:r><a:rPr sz="0"/><a:t>三期验收`}, {`<a:r><a:t>三期验收`, `<a:r><a:rPr><a:noFill/></a:rPr><a:t>三期验收`}} {
		bad, _ := parseArtifactXML([]byte(strings.Replace(raw, v[0], v[1], 1)))
		if _, e := phase3PlaceholderTarget(bad, s, true); e == nil {
			t.Fatalf("accepted %v", v)
		}
	}
	if _, e := phase3PlaceholderTarget(n, s, false); e == nil {
		t.Fatal("nonempty placeholder overwritten")
	}
}

func TestPhase3SlidePlaceholderArtifactPreservesIdentityAndOtherText(t *testing.T) {
	fixture := func(content, kind, other string) string {
		return artifactTestZip(t, map[string]string{
			"ppt/presentation.xml":            `<p:presentation xmlns:p="` + artifactSlideNS + `" xmlns:r="` + artifactOfficeRelNS + `"><p:sldIdLst><p:sldId r:id="one"/></p:sldIdLst></p:presentation>`,
			"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="` + artifactRelNS + `"><Relationship Id="one" Type="` + artifactOfficeRelNS + `/slide" Target="slides/one.xml"/></Relationships>`,
			"ppt/slides/one.xml":              `<p:sld xmlns:p="` + artifactSlideNS + `" xmlns:a="` + artifactDrawingNS + `"><p:cSld><p:spTree><p:sp><p:nvSpPr><p:cNvPr id="7" name="Title"/><p:nvPr><p:ph type="` + kind + `" idx="0"/></p:nvPr></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/>` + content + `</p:txBody></p:sp><p:sp><p:nvSpPr><p:cNvPr id="9" name="Other"/></p:nvSpPr><p:txBody><a:p><a:r><a:t>` + other + `</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`})
	}
	s := phase3SlidesSpec{Action: "slide-placeholder-fill", SlideID: "a", Placeholder: &phase3SlidePlaceholderSpec{Name: "Title", Text: "第一行\n第二行"}}
	b := fixture(`<a:p/>`, "title", "keep")
	body := `<a:p><a:r><a:t>第一行</a:t></a:r></a:p><a:p><a:r><a:t>第二行</a:t></a:r></a:p>`
	if e := verifyPhase3SlidePlaceholderArtifacts(b, fixture(body, "title", "keep"), []string{"a"}, []string{"a"}, s); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{fixture(body, "body", "keep"), fixture(body, "title", "changed"), fixture(`<a:p><a:r><a:t>第一行第二行</a:t></a:r></a:p>`, "title", "keep"), fixture(`<a:p><a:r><a:t>第一行</a:t></a:r><a:br/><a:r><a:t>第二行</a:t></a:r></a:p>`, "title", "keep")} {
		if e := verifyPhase3SlidePlaceholderArtifacts(b, bad, []string{"a"}, []string{"a"}, s); e == nil {
			t.Fatal("placeholder collateral change or missing paragraph accepted")
		}
	}
}

func TestPhase3SlidePlaceholderUsesNativeParagraphAction(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("optional Node test runtime")
	}
	spec := phase3SlidesSpec{SlideID: "a", Placeholder: &phase3SlidePlaceholderSpec{Name: "Body", Text: "第一行\n第二行"}}
	for _, missing := range []bool{false, true} {
		script := fmt.Sprintf(`let paragraphs=[''],writes=0,selected=false;
const shape={id:'shape',name:'Body',get textContent(){return paragraphs.join('\n')},setContent(s){writes++;paragraphs=[s]}};
const handler=%t?{}:{_$addNewParagraph$_(){if(!selected)throw Error('wrong selection');writes++;paragraphs.push('')}};
global.window={__shimoPhase3Presence:()=>({ready:true,connected:true,ids:[1]}),presentationSDK:{editor:{workbench:{isReady:()=>true,permissions:{canEdit:()=>true},context:{getUser:()=>({id:1})},editor:{_$editorUI$_:{_$editorHandler$_:handler}},core:{setTextRange(){},selectElementByIds(e,ids){selected=JSON.stringify(ids)==='["shape"]'},clearContent(){if(!selected)throw Error('wrong selection');writes++;paragraphs=['']},addText(e,text){if(text.includes('\n'))throw Error('soft break passed as paragraph');writes++;paragraphs[paragraphs.length-1]+=text}}}},API:{slides:{getSlides:()=>[{id:'a',getShapes:()=>[shape]}],setCurrentSlideIndex(){},setSelectedSlides(){}}}}};
const result=%s;
if(%t){if(writes!==0||result.ok!==false)throw Error('missing native paragraph action wrote content')}
else if(result.ok!==true||JSON.stringify(paragraphs)!=='["第一行","第二行"]')throw Error('not two native paragraphs');`, missing, phase3SlidePlaceholderMutationJS(spec, []string{"a"}), missing)
		cmd := exec.Command(node, "-e", script)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("missing=%v: %v\n%s", missing, err, out)
		}
	}
}
