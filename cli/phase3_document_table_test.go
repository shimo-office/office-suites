package main

import (
	"os/exec"
	"testing"
)

func TestDocumentTableStructureProof(t *testing.T) {
	spec, err := prepareDocumentTable(map[string]any{"operation": "insert-row-after", "table": float64(1), "row": float64(1), "column": float64(1), "expectedCell": "a"})
	if err != nil {
		t.Fatal(err)
	}
	cell := func(s string) documentTableCell { return documentTableCell{Text: s, Style: "same"} }
	before := documentTablesSnapshot{Tables: []documentTable{{Rows: [][]documentTableCell{{cell("a"), cell("b")}, {cell("c"), cell("d")}}}}}
	after := documentTablesSnapshot{Tables: []documentTable{{Rows: [][]documentTableCell{{cell("a"), cell("b")}, {cell(""), cell("")}, {cell("c"), cell("d")}}}}}
	if err := verifyDocumentTable(before, after, spec); err != nil {
		t.Fatal(err)
	}
	if err := verifyDocumentTable(before, before, spec); err == nil {
		t.Fatal("accepted no-op")
	}
	wrong := documentTablesSnapshot{Tables: []documentTable{{Rows: [][]documentTableCell{{cell(""), cell("")}, {cell("a"), cell("b")}, {cell("c"), cell("d")}}}}}
	if err := verifyDocumentTable(before, wrong, spec); err == nil {
		t.Fatal("accepted insertion at wrong row")
	}
	for _, op := range []string{"delete-row", "insert-column-before", "delete-column"} {
		s := spec
		s.Operation = op
		var good documentTablesSnapshot
		switch op {
		case "delete-row":
			good = documentTablesSnapshot{Tables: []documentTable{{Rows: [][]documentTableCell{{cell("c"), cell("d")}}}}}
		case "insert-column-before":
			good = documentTablesSnapshot{Tables: []documentTable{{Rows: [][]documentTableCell{{cell(""), cell("a"), cell("b")}, {cell(""), cell("c"), cell("d")}}}}}
		case "delete-column":
			good = documentTablesSnapshot{Tables: []documentTable{{Rows: [][]documentTableCell{{cell("b")}, {cell("d")}}}}}
		}
		if err := verifyDocumentTable(before, good, s); err != nil {
			t.Fatalf("%s: %v", op, err)
		}
	}
}
func TestDocumentTableRequestRejectsAmbiguity(t *testing.T) {
	for _, p := range []map[string]any{{"operation": "merge"}, {"operation": "delete-row", "table": float64(1), "row": float64(0), "column": float64(1), "expectedCell": "a"}, {"operation": "delete-row", "table": float64(1), "row": float64(1), "column": float64(1), "expectedCell": "a", "ignoreConflicts": true}} {
		if _, err := prepareDocumentTable(p); err == nil {
			t.Fatalf("accepted %#v", p)
		}
	}
}

func TestDocumentTableJavaScriptRefusesWrongCell(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable")
	}
	spec := documentTableSpec{Operation: "insert-row-after", Table: 1, Row: 1, Column: 1, ExpectedCell: "a"}
	before := documentTablesSnapshot{Tables: []documentTable{{Rows: [][]documentTableCell{{{Text: "a", Paragraph: 1}, {Text: "b", Paragraph: 2}}}}}}
	script := `global.window=global;let writes=0;const table={children:[],curCell:null}, row={Index:0,Table:table,children:[]};table.children=[row];const cells=['a','b'].map((text,i)=>({Index:i,Row:row,getGridSpan:()=>1,getVMergedRows:()=>1}));row.children=cells;table.curCell=cells[0];const paragraphs=cells.map((cell,i)=>({getParent:()=>({getParent:()=>cell}),text:['a','b'][i]}));const e={config:{mode:{role:'editor'}},docsApi:{isViewMode:false,addTblRowBelow:()=>writes++},getParagraphs:()=>paragraphs,getParagraphRange:p=>({getText:()=>p.text+'\r\n'}),runSelectionCallback:(range,fn)=>fn()};global._sdk={editor:e,options:{user:{id:1}}};global.__shimoPhase3Presence=()=>({ready:true,connected:true,ids:['1']});(async()=>{const good=await ` + documentTableJS(spec, before) + `;if(!good.ok||writes!==1)throw Error(JSON.stringify(good));writes=0;table.curCell=cells[1];const wrong=await ` + documentTableJS(spec, before) + `;if(wrong.ok||writes)throw Error('wrong cell mutated');table.curCell=cells[0];let ids=['1'];global.__shimoPhase3Presence=()=>({ready:true,connected:true,ids});e.runSelectionCallback=(range,fn)=>{ids=['1','2'];return fn()};const changed=await ` + documentTableJS(spec, before) + `;if(changed.ok||writes)throw Error('collaborator guard bypassed')})().catch(e=>{console.error(e);process.exitCode=1})`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

func TestDocumentTableReadbackPreservesOutsideWhitespaceAndTableProperties(t *testing.T) {
	build := func(outside, prop string) documentTablesSnapshot {
		name := artifactTestZip(t, map[string]string{"word/document.xml": `<w:document xmlns:w="` + artifactWordNS + `"><w:body><w:p><w:r><w:t xml:space="preserve">` + outside + `</w:t></w:r></w:p><w:tbl><w:tblPr><w:jc w:val="` + prop + `"/></w:tblPr><w:tr><w:tc><w:p><w:r><w:t>a</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:body></w:document>`})
		value, err := readDocumentTables(name)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := build(" outside ", "left")
	changed := build("outside", "right")
	if before.OutsideTables == changed.OutsideTables {
		t.Fatal("outside whitespace change was ignored")
	}
	if before.Tables[0].Properties == changed.Tables[0].Properties {
		t.Fatal("table properties omitted")
	}
	if before.Tables[0].Rows[0][0].Paragraph != 2 {
		t.Fatal("paragraph order excludes table cell")
	}
}

func TestDocumentTableDistributionProof(t *testing.T) {
	cell := func(i int) documentTableCell {
		return documentTableCell{Text: "a", Style: "same", Paragraph: i, Paragraphs: []int{i}, ParagraphTexts: []string{"a"}}
	}
	before := documentTablesSnapshot{Tables: []documentTable{{RawStructure: "before", GridWidths: []float64{100, 300}, Rows: [][]documentTableCell{{cell(1), cell(2)}, {cell(3), cell(4)}}, RowHeights: []documentTableHeight{{Value: 100, Rule: "atLeast"}, {Value: 300, Rule: "atLeast"}}, RowOtherStyles: []string{"x", "x"}}}}
	after := before
	after.Tables = append([]documentTable(nil), before.Tables...)
	after.Tables[0].RawStructure = "after"
	after.Tables[0].RowHeights = []documentTableHeight{{Value: 200, Rule: "atLeast"}, {Value: 200, Rule: "atLeast"}}
	spec := documentTableSpec{Operation: "distribute-rows", Table: 1, Row: 1, Column: 1, ExpectedCell: "a"}
	if err := verifyDocumentTable(before, after, spec); err != nil {
		t.Fatal(err)
	}
	if err := verifyDocumentTable(before, before, spec); err == nil {
		t.Fatal("unequal row heights accepted")
	}
	spec.Operation = "distribute-columns"
	after.Tables[0].RowHeights = before.Tables[0].RowHeights
	after.Tables[0].GridWidths = []float64{200, 200}
	if err := verifyDocumentTable(before, after, spec); err != nil {
		t.Fatal(err)
	}
	after.Tables[0].GridWidths = []float64{150, 250}
	if err := verifyDocumentTable(before, after, spec); err == nil {
		t.Fatal("unequal columns accepted")
	}
}
