package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func documentDistributionParagraphFixture(t *testing.T, widths, heights []int, content string) documentTablesSnapshot {
	t.Helper()
	grid, rows := "", ""
	for _, w := range widths {
		grid += fmt.Sprintf(`<w:gridCol w:w="%d"/>`, w)
	}
	for ri, h := range heights {
		rows += fmt.Sprintf(`<w:tr><w:trPr><w:trHeight w:val="%d" w:hRule="atLeast"/></w:trPr>`, h)
		for ci, w := range widths {
			text := `<w:p><w:r><w:t>OTHER</w:t></w:r></w:p>`
			if ri == 0 && ci == 0 {
				text = content
			}
			rows += fmt.Sprintf(`<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/></w:tcPr>%s</w:tc>`, w, text)
		}
		rows += `</w:tr>`
	}
	xml := `<w:document xmlns:w="` + artifactWordNS + `"><w:body><w:tbl><w:tblGrid>` + grid + `</w:tblGrid>` + rows + `</w:tbl></w:body></w:document>`
	s, err := readDocumentTables(artifactTestZip(t, map[string]string{"word/document.xml": xml}))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDocumentTableDistributionAfterSplitParagraphs(t *testing.T) {
	content := `<w:p><w:r><w:t>A</w:t></w:r></w:p><w:p><w:r><w:t>B</w:t></w:r></w:p>`
	before := documentDistributionParagraphFixture(t, []int{1000, 3000}, []int{600, 1200}, content)
	for _, op := range []string{"distribute-rows", "distribute-columns"} {
		t.Run(op, func(t *testing.T) {
			spec := documentTableSpec{Operation: op, Table: 1, Row: 1, Column: 1, ExpectedCell: "A\nB"}
			anchor, err := documentTableAnchor(before, spec)
			if err != nil || anchor.Text != "A" || anchor.End != 1 {
				t.Fatalf("distribution must bind first paragraph while checking the full cell: %+v %v", anchor, err)
			}
			widths, heights := []int{1000, 3000}, []int{900, 900}
			if op == "distribute-columns" {
				widths, heights = []int{2000, 2000}, []int{600, 1200}
			}
			if err := verifyDocumentTable(before, documentDistributionParagraphFixture(t, widths, heights, content), spec); err != nil {
				t.Fatal(err)
			}
			for _, bad := range []string{
				strings.Replace(content, ">B<", ">CHANGED<", 1),
				strings.Replace(content, `<w:t>B</w:t>`, `<w:rPr><w:b/></w:rPr><w:t>B</w:t>`, 1),
				`<w:p><w:r><w:t>A</w:t><w:br/><w:t>B</w:t></w:r></w:p>`,
			} {
				if verifyDocumentTable(before, documentDistributionParagraphFixture(t, widths, heights, bad), spec) == nil {
					t.Fatal("distribution accepted changed paragraph text, style or topology")
				}
			}
		})
	}
	if _, err := documentTableAnchor(before, documentTableSpec{Operation: "delete-column", Table: 1, Row: 1, Column: 1, ExpectedCell: "A\nB"}); err == nil {
		t.Fatal("distribution support must not expand CRUD preconditions")
	}
}

func TestDocumentTableDistributionBindsEveryParagraph(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable")
	}
	content := `<w:p><w:r><w:t>A</w:t></w:r></w:p><w:p><w:r><w:t>B</w:t></w:r></w:p>`
	before := documentDistributionParagraphFixture(t, []int{1000, 3000}, []int{600, 1200}, content)
	raw, _ := json.Marshal(before.Tables)
	for _, op := range []string{"distribute-rows", "distribute-columns"} {
		spec := documentTableSpec{Operation: op, Table: 1, Row: 1, Column: 1, ExpectedCell: "A\nB"}
		script := `global.window=global;const baseline=` + string(raw) + `;
async function run(mode){let writes=0;const paragraphs=[],table={children:[],getRowsLen(){return this.children.length},getRow(i){return this.children[i]},getLayoutRowContentHeight:()=>100,getLayoutTable:()=>({getGridCol:()=>[100,300],getGridColMin:()=>[10,10]}),getPreferredMeasurementType:()=>[0],rebuildFromPositions:()=>writes++};
table.children=baseline[0].rows.map((br,ri)=>{const row={Index:ri,Table:table,children:[],setRowHeight:()=>writes++};row.children=br.map((bc,ci)=>{const cell={Index:ci,Row:row,getGridSpan:()=>1,getVMergedRows:()=>1};bc.paragraphs.forEach((pi,j)=>paragraphs[pi-1]={text:bc.paragraphTexts[j],getParent:()=>({getParent:()=>cell})});return cell});return row});table.curCell=table.children[0].children[0];for(const p of paragraphs)p.children=[{getText:()=>p.text}];
if(mode==='second-text')paragraphs[1].text='CHANGED';if(mode==='second-binding')paragraphs[1].getParent=()=>({getParent:()=>table.children[0].children[1]});if(mode==='extra-paragraph')paragraphs.push({text:'EXTRA',getParent:()=>({getParent:()=>table.curCell})});
const e={config:{mode:{role:'editor'}},docsApi:{isViewMode:false,getCurTable:()=>table,startAction:()=>{},finalizeAction:()=>{}},getParagraphs:()=>mode==='copied-array'?paragraphs.slice():paragraphs,getParagraphRange:p=>({getText:()=>p.text+'\r\n'}),runSelectionCallback:(range,fn)=>{if(mode==='late-extra-table')paragraphs.push({text:'NEW',getParent:()=>({getParent:()=>({Row:{Table:{}}})})});if(mode==='late-text')paragraphs[1].text='CHANGED';if(mode==='late-nontarget-text')paragraphs[2].text='CHANGED';if(mode==='late-replace')paragraphs[1]={...paragraphs[1]};if(mode==='late-binding')paragraphs[1].getParent=()=>({getParent:()=>table.children[0].children[1]});return fn()}};
global._sdk={editor:e,options:{user:{id:1}}};global.__shimoPhase3Presence=()=>({ready:true,connected:true,ids:['1']});const result=await ` + documentTableJS(spec, before) + `;if(mode===''||mode==='copied-array'){if(!result.ok||writes===0)throw Error('valid distribution rejected '+JSON.stringify(result))}else if(result.ok||writes!==0)throw Error('unsafe '+mode+' '+JSON.stringify(result))}
(async()=>{for(const mode of ['','copied-array','second-text','second-binding','extra-paragraph','late-binding','late-text','late-nontarget-text','late-replace','late-extra-table'])await run(mode)})().catch(e=>{console.error(e);process.exitCode=1})`
		cmd := exec.Command(node, "-")
		cmd.Stdin = strings.NewReader(script)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", op, err, out)
		}
	}
}
