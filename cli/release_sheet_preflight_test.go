package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseSheetCommandsRejectInvalidInputBeforeOpeningBrowser(t *testing.T) {
	for _, tc := range []struct {
		name, action, target, payload string
		args                          []string
	}{
		{name: "missing-target"},
		{name: "browser-forbidden", target: "https://shimo.net/sheets/abcdefgh", args: []string{"--no-browser"}},
		{name: "invalid-json", target: "https://shimo.net/sheets/abcdefgh", payload: "{"},
		{name: "invalid-object", target: "https://shimo.net/sheets/abcdefgh", payload: "[1]"},
		{name: "unsupported-pdf-option", target: "https://shimo.net/sheets/abcdefgh", payload: `{"paper":"A4"}`},
		{name: "wrong-origin", target: "https://other.test/sheets/abcdefgh"},
		{name: "freeze-invalid-plan", action: "sheet-freeze", target: "https://shimo.net/sheets/abcdefgh", payload: `{"sheetName":"数据"}`},
		{name: "freeze-wrong-origin", action: "sheet-freeze", target: "https://other.test/sheets/abcdefgh", payload: `{"sheetName":"数据","rows":1}`},
		{name: "freeze-invalid-json", action: "sheet-freeze", target: "https://shimo.net/sheets/abcdefgh", payload: "{"},
		{name: "freeze-invalid-object", action: "sheet-freeze", target: "https://shimo.net/sheets/abcdefgh", payload: "[1]"},
		{name: "freeze-forbidden", action: "sheet-freeze", target: "https://shimo.net/sheets/abcdefgh", args: []string{"--no-browser"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"editor"}
			if tc.target != "" {
				args = append(args, tc.target)
			}
			action := tc.action
			if action == "" {
				action = "sheet-export-pdf"
			}
			args = append(args, "--action", action)
			if tc.payload != "" {
				args = append(args, "--payload", tc.payload)
			}
			args = append(args, tc.args...)
			a := newApp(args)
			a.Origin = "https://shimo.net"
			var err error
			if action == "sheet-export-pdf" {
				err = a.cmdSheetExportPDF()
			} else {
				err = a.runSheetBrowser()
			}
			if err == nil {
				t.Fatal("invalid request was accepted")
			}
		})
	}
}

func TestReleaseSheetCommandsRejectNonSheetsAndUnwritableEvidence(t *testing.T) {
	for _, action := range []string{"sheet-export-pdf", "sheet-freeze"} {
		for _, kind := range []string{"http-error", "wrong-type", "cloud-file", "evidence-file"} {
			t.Run(action+"/"+kind, func(t *testing.T) {
				sessionTestState(t)
				requests := 0
				issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
					requests++
					if r.URL.Path != "/api/v1/files/abcdefgh" {
						t.Fatal("preflight reached a write or export endpoint", r.URL.Path)
					}
					if kind == "http-error" {
						return nil, errors.New("synthetic metadata failure")
					}
					typ := "spreadsheet"
					if kind == "wrong-type" {
						typ = "document"
					}
					raw, _ := json.Marshal(map[string]any{"guid": "abcdefgh", "type": typ, "isCloudFile": kind == "cloud-file"})
					return phase3Response(200, string(raw)), nil
				})
				out := filepath.Join(t.TempDir(), "existing-file")
				if err := os.WriteFile(out, []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
				a := newApp([]string{"editor", "https://shimo.net/sheets/abcdefgh", "--action", action, "--payload", `{"sheetName":"数据","rows":1}`, "--out", out})
				a.Origin = "https://shimo.net"
				var err error
				if action == "sheet-export-pdf" {
					a.Flags["--payload"] = `{"sheetName":"数据"}`
					err = a.cmdSheetExportPDF()
				} else {
					err = a.runSheetBrowser()
				}
				if err == nil || requests != 1 {
					t.Fatalf("unsafe preflight: %v, requests=%d", err, requests)
				}
				data, _ := os.ReadFile(out)
				if string(data) != "preserve" {
					t.Fatal("preflight changed the output path")
				}
			})
		}
	}
}

func TestReleaseSheetFacadePreservesPartialFreezeAndRefusesExistingFilter(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("requires existing Node")
	}
	before := map[string]any{"sheetId": "id-a", "sheetName": "数据", "rows": 1, "columns": 2}
	rows := 3
	setup := `global.window={__SM__:{editor:{activeSheet:{id:'id-a',name:'数据',editable:true,rowCount:100,columnCount:20,tn:{_AGS:{frozenRowCount:()=>1,frozenColumnCount:()=>2},getFilter:()=>({range:{row:0,col:0,rowCount:3,colCount:2}})},setFrozenRowCount:n=>events.push(['rows',n]),setFrozenColumnCount:n=>events.push(['cols',n])},workbook:{save:async()=>events.push(['save'])}}}};`
	for _, tc := range []struct{ name, expression, assertion string }{
		{"partial-freeze", sheetFreezeApplyJS(&rows, nil, before), `if(!r.submitted||JSON.stringify(events)!=='[["rows",3],["save"]]')throw Error('partial freeze changed columns or did not save')`},
		{"freeze-read", sheetFreezeStateJS(), `if(r.rows!==1||r.columns!==2||r.sheetId!=='id-a')throw Error('wrong frozen state')`},
		{"filter-read", sheetFilterStateJS(), `if(r.range.rowCount!==3||r.range.colCount!==2)throw Error('wrong filter state')`},
		{"refuse-filter-overwrite", sheetFilterApplyJS(sheetBrowserSpec{Action: "sheet-filter", RowCount: 4, ColumnCount: 2}, before), `if(r.found!==false||events.length!==0)throw Error('overwrote existing filter')`},
		{"clear-filter", sheetFilterApplyJS(sheetBrowserSpec{Action: "sheet-filter-clear"}, before), `if(r.found!==false||events.length!==0)throw Error('missing clear method was accepted')`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := `const events=[];` + setup + `Promise.resolve(` + tc.expression + `).then(r=>{` + tc.assertion + `}).catch(e=>{console.error(e);process.exit(1)})`
			out, err := exec.Command(node, "-e", script).CombinedOutput()
			if err != nil {
				t.Fatalf("%v: %s", err, strings.TrimSpace(string(out)))
			}
		})
	}
}
