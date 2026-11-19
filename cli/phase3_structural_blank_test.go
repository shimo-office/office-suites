package main

import "testing"

func phase3BlankSnapshot(t *testing.T, rows string) artifactSheetSnapshot {
	t.Helper()
	path := artifactTestZip(t, artifactWorkbookEntries(`<worksheet xmlns="`+artifactNS+`"><sheetData>`+rows+`</sheetData></worksheet>`))
	snapshot, err := readArtifactSheet(path, "Target")
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestPhase3InsertMustMoveStyledBlankCells(t *testing.T) {
	before := phase3BlankSnapshot(t, `<row r="1"><c r="A1" t="inlineStr"><is><t>x</t></is></c><c r="B1" s="1"/></row>`)
	spec := phase3Spec{Action: "sheet-insert", Sheet: "Target", Index: 0, Count: 1}
	for _, tc := range []struct {
		name, rows string
		pass       bool
	}{
		{"blank fill moves with row", `<row r="2"><c r="A2" t="inlineStr"><is><t>x</t></is></c><c r="B2" s="1"/></row>`, true},
		{"blank fill left on old row", `<row r="1"><c r="B1" s="1"/></row><row r="2"><c r="A2" t="inlineStr"><is><t>x</t></is></c></row>`, false},
		{"blank fill silently lost", `<row r="2"><c r="A2" t="inlineStr"><is><t>x</t></is></c></row>`, false},
		{"blank fill reset at correct coordinate", `<row r="2"><c r="A2" t="inlineStr"><is><t>x</t></is></c><c r="B2" s="0"/></row>`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			after := phase3BlankSnapshot(t, tc.rows)
			err := phase3VerifyStructural(before, after, spec)
			if (err == nil) != tc.pass {
				t.Fatalf("pass=%v err=%v", tc.pass, err)
			}
		})
	}
}

func TestPhase3DeleteMustShiftStyledBlankCells(t *testing.T) {
	before := phase3BlankSnapshot(t, `<row r="1"><c r="A1" t="inlineStr"><is><t>remove</t></is></c></row><row r="2"><c r="A2" t="inlineStr"><is><t>keep</t></is></c><c r="B2" s="1"/></row>`)
	spec := phase3Spec{Action: "sheet-delete", Sheet: "Target", Index: 0, Count: 1}
	correct := phase3BlankSnapshot(t, `<row r="1"><c r="A1" t="inlineStr"><is><t>keep</t></is></c><c r="B1" s="1"/></row>`)
	if err := phase3VerifyStructural(before, correct, spec); err != nil {
		t.Fatal(err)
	}
	wrong := phase3BlankSnapshot(t, `<row r="1"><c r="A1" t="inlineStr"><is><t>keep</t></is></c></row><row r="2"><c r="B2" s="1"/></row>`)
	if err := phase3VerifyStructural(before, wrong, spec); err == nil {
		t.Fatal("stale styled blank accepted after delete")
	}
}

func TestPhase3AppendMustPreserveBlankStyles(t *testing.T) {
	before := phase3BlankSnapshot(t, `<row r="1"><c r="A1" t="inlineStr"><is><t>existing</t></is></c><c r="B1" s="1"/></row><row r="2"><c r="A2" s="1"/></row>`)
	spec := phase3Spec{Action: "sheet-append", Sheet: "Target", Range: "Target!A2:A2"}
	correct := phase3BlankSnapshot(t, `<row r="1"><c r="A1" t="inlineStr"><is><t>existing</t></is></c><c r="B1" s="1"/></row><row r="2"><c r="A2" s="1" t="inlineStr"><is><t>added</t></is></c></row>`)
	if err := phase3VerifyStructural(before, correct, spec); err != nil {
		t.Fatal(err)
	}
	for _, rows := range []string{
		`<row r="1"><c r="A1" t="inlineStr"><is><t>existing</t></is></c></row><row r="2"><c r="A2" s="1" t="inlineStr"><is><t>added</t></is></c></row>`,
		`<row r="1"><c r="A1" t="inlineStr"><is><t>existing</t></is></c><c r="B1" s="1"/></row><row r="2"><c r="A2" s="0" t="inlineStr"><is><t>added</t></is></c></row>`,
	} {
		if err := phase3VerifyStructural(before, phase3BlankSnapshot(t, rows), spec); err == nil {
			t.Fatal("append lost an existing blank-cell style")
		}
	}
}
