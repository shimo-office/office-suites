package main

import (
	"encoding/json"
	"os/exec"
	"testing"
)

func TestPhase3KanbanRecordSpecIncludesActualGroupOracle(t *testing.T) {
	table := tableFixtureForEdit()
	table["fields"] = append(table["fields"].([]any), map[string]any{"guid": "status", "name": "状态", "type": "SingleChoice", "props": map[string]any{"choices": map[string]any{"todo": map[string]any{"name": "待办"}, "done": map[string]any{"name": "完成"}}}})
	view := phase3TableCopy(phase3TableMap(table, "views")["v1"])
	view["type"] = "Kanban"
	view["groups"] = []any{map[string]any{"fieldGuid": "status", "order": "Asc"}}
	phase3TableMap(table, "records")["r1"]["valueMap"].(map[string]any)["status"] = "todo"
	phase3TableMap(table, "records")["r2"]["valueMap"].(map[string]any)["status"] = "done"
	spec, err := phase3ViewRecordSpec(table, view, "")
	if err != nil {
		t.Fatal(err)
	}
	rows := spec["expectedRecords"].(map[string]any)
	if rows["r1"].(map[string]any)["groupValue"] != "todo" || rows["r2"].(map[string]any)["groupValue"] != "done" || rows["r1"].(map[string]any)["groupFieldGuid"] != "status" {
		t.Fatal("missing independent record-to-lane mapping", rows)
	}
}
func TestPhase3KanbanNativeLaneCannotBeSynthesizedFromSpec(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable")
	}
	spec := map[string]any{"tableGuid": "t1", "viewGuid": "v1", "viewType": "Kanban", "primaryFieldGuid": "f1", "expectedRecords": map[string]any{"r1": map[string]any{"title": "A", "groupFieldGuid": "status", "groupValue": "todo"}, "r2": map[string]any{"title": "B", "groupFieldGuid": "status", "groupValue": "done"}}}
	raw, _ := json.Marshal(spec)
	for _, mode := range []string{"good", "wrong-lane", "wrong-field", "missing-lane", "missing-member"} {
		t.Run(mode, func(t *testing.T) {
			modeJSON, _ := json.Marshal(mode)
			script := `const mode=` + string(modeJSON) + `,spec=` + string(raw) + `;global.innerWidth=1000;global.innerHeight=1000;global.performance={timeOrigin:20};global.getComputedStyle=()=>({visibility:'visible',display:'block',overflowX:'visible',overflowY:'visible'});const view={getGuid:()=> 'v1',getTable:()=>({getGuid:()=> 't1'}),getType:()=> 'Kanban'};const nodes=['r1','r2'].map((id,i)=>{const lane={viewModel:view,horizontalField:{getGuid:()=>mode==='wrong-field'?'other':'status'},horizontalGroup:{value:mode==='wrong-lane'?'todo':['todo','done'][i]},recordGuids:mode==='missing-member'?[]:[id],verticalGroups:{}};const el={innerText:['A','B'][i],parentElement:null,getBoundingClientRect:()=>({width:100,height:30,left:0,top:i*40,right:100,bottom:i*40+30}),closest:()=>({getAttribute:()=>id})};el.__reactFiber$test={memoizedProps:{viewModel:view,record:{guid:id,valueMap:{f1:['A','B'][i],status:['todo','done'][i]}}},return:mode==='missing-lane'?null:{memoizedProps:lane,return:null}};return el});global.document={querySelectorAll:()=>nodes};try{process.stdout.write(JSON.stringify((` + phase3ReadNativeView + `)(spec,'.sm-kanban-item-name')))}catch(e){process.stdout.write(JSON.stringify({ok:false,error:e.message}))}`
			output, err := exec.Command(node, "-e", script).CombinedOutput()
			if err != nil {
				t.Fatalf("%v %s", err, output)
			}
			var actual map[string]any
			if err = json.Unmarshal(output, &actual); err != nil {
				t.Fatal(err, string(output))
			}
			err = verifyPhase3NativeViewProof(actual, spec, 10)
			if mode == "good" && err != nil {
				t.Fatal(err, actual)
			}
			if mode != "good" && err == nil {
				t.Fatal("wrong native lane accepted", mode, actual)
			}
		})
	}
}
