package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"reflect"
	"testing"
)

func formEditFixture() map[string]any {
	var m map[string]any
	_ = json.Unmarshal([]byte(`{"title":"baseline","desc":"keep","questions":[{"guid":"questionA","type":"text","required":false,"subject":"A","question":{}},{"guid":"questionB","type":"text","required":true,"subject":"B","question":{}}],"rules":[]}`), &m)
	return m
}
func TestPhase3FormEditExpected(t *testing.T) {
	before := formEditFixture()
	original, _ := json.Marshal(before)
	req := map[string]any{"operation": "question-add", "index": float64(1), "question": map[string]any{"guid": "questionC", "type": "single_choice", "required": true, "subject": "C", "question": map[string]any{"options": []any{map[string]any{"type": "normal", "value": "optA", "label": "yes"}, map[string]any{"type": "normal", "value": "optB", "label": "no"}}}}}
	after, err := phase3FormExpected(before, req)
	if err != nil {
		t.Fatal(err)
	}
	qs := after["questions"].([]any)
	if len(qs) != 3 || qs[1].(map[string]any)["guid"] != "questionC" {
		t.Fatal(after)
	}
	stable, _ := json.Marshal(before)
	if string(original) != string(stable) {
		t.Fatal("mutated before")
	}
	if err := phase3VerifyForm(after, before); err == nil {
		t.Fatal("accepted stale readback")
	}
	if err := phase3VerifyForm(after, after); err != nil {
		t.Fatal(err)
	}
	reorder, err := phase3FormExpected(before, map[string]any{"operation": "question-reorder", "from": float64(0), "to": float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if reorder["questions"].([]any)[0].(map[string]any)["guid"] != "questionB" {
		t.Fatal(reorder)
	}
	updated, err := phase3FormExpected(before, map[string]any{"operation": "question-update", "guid": "questionA", "patch": map[string]any{"subject": "changed", "required": true}})
	if err != nil {
		t.Fatal(err)
	}
	if updated["questions"].([]any)[0].(map[string]any)["subject"] != "changed" || updated["desc"] != "keep" {
		t.Fatal(updated)
	}
	deleted, err := phase3FormExpected(before, map[string]any{"operation": "question-delete", "guid": "questionA"})
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted["questions"].([]any)) != 1 {
		t.Fatal(deleted)
	}
	if reflect.DeepEqual(before, after) {
		t.Fatal("no mutation")
	}
}
func TestPhase3FormEditRejectsInvalid(t *testing.T) {
	for _, req := range []map[string]any{
		{"operation": "question-delete", "guid": "missing"},
		{"operation": "question-reorder", "from": float64(-1), "to": float64(1)},
		{"operation": "question-reorder", "from": float64(0.5), "to": float64(1)},
		{"operation": "question-update", "guid": "questionA", "patch": map[string]any{"guid": "else"}},
		{"operation": "question-add", "question": map[string]any{"guid": "questionA", "type": "text", "required": true, "subject": "x", "question": map[string]any{}}},
		{"operation": "question-add", "question": map[string]any{"guid": "questionC", "type": "single_choice", "required": true, "subject": "x", "question": map[string]any{"options": []any{}}}},
		{"operation": "meta", "patch": map[string]any{"questions": []any{}}},
		{"operation": "meta", "patch": map[string]any{"title": ""}},
		{"operation": "unknown"},
	} {
		if _, err := phase3FormExpected(formEditFixture(), req); err == nil {
			t.Fatalf("accepted %v", req)
		}
	}
	b := formEditFixture()
	b["rules"] = []any{map[string]any{"question": "questionA"}}
	if _, err := phase3FormExpected(b, map[string]any{"operation": "question-delete", "guid": "questionA"}); err == nil {
		t.Fatal("deleted referenced question")
	}
}

func TestPhase3FormRulesAndRate(t *testing.T) {
	b := formEditFixture()
	q := b["questions"].([]any)[0].(map[string]any)
	q["type"] = "single_choice"
	q["question"] = map[string]any{"options": []any{map[string]any{"type": "normal", "value": "yes1", "label": "yes"}}}
	r := map[string]any{"operation": "rule-add", "trigger": "questionA", "options": []any{"yes1"}, "targets": []any{"questionB"}}
	out, err := phase3FormExpected(b, r)
	if err != nil {
		t.Fatal(err)
	}
	if len(out["rules"].([]any)) != 1 {
		t.Fatal(out)
	}
	r["options"] = []any{"missing"}
	if _, err = phase3FormExpected(b, r); err == nil {
		t.Fatal("accepted missing option")
	}
	r["options"] = []any{"yes1"}
	r["targets"] = []any{"questionA"}
	if _, err = phase3FormExpected(b, r); err == nil {
		t.Fatal("accepted target before trigger")
	}
	if _, err = phase3FormExpected(out, map[string]any{"operation": "question-reorder", "from": float64(0), "to": float64(1)}); err == nil {
		t.Fatal("accepted reordered invalid rule")
	}
	rate := map[string]any{"guid": "rate1234", "type": "rate", "required": true, "subject": "Score", "question": map[string]any{"shape": "star", "sum": float64(5)}}
	if err = phase3ValidateQuestion(rate); err != nil {
		t.Fatal(err)
	}
	rate["question"] = map[string]any{"shape": "star", "sum": float64(0)}
	if phase3ValidateQuestion(rate) == nil {
		t.Fatal("accepted invalid rate max")
	}
}

func TestPhase3FormMutationRejectsStaleBeforeWriting(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node needed for emitted JS regression")
	}
	before := formEditFixture()
	req := map[string]any{"operation": "question-update", "guid": "questionA", "patch": map[string]any{"subject": "changed"}}
	expected, err := phase3FormExpected(before, req)
	if err != nil {
		t.Fatal(err)
	}
	prior := map[string]any{"form": before, "rev": float64(7)}
	expression := phase3FormMutationJS(req, expected, prior)
	formJSON, _ := json.Marshal(before)
	for _, stale := range []bool{false, true} {
		script := `let writes=0;const form=` + string(formJSON) + `;global.window={__shimoPhase3Presence:()=>({ready:true,connected:true,ids:['42']}),logged:{edit:{query:{getForm:()=>form,getRev:()=>7,permissions:{canEditForm:()=>true,DEBUG_getContext:()=>({user:{id:42}})}},command:{updateQuestion:q=>{writes++;}}}}};`
		if stale {
			script += `form.title='concurrent change';`
		}
		script += `Promise.resolve(` + expression + `).then(result=>{if(writes!==` + fmt.Sprint(map[bool]int{false: 1, true: 0}[stale]) + `)throw new Error('wrong mutation count '+writes);if(result.ok===` + fmt.Sprint(stale) + `)throw new Error('wrong rejection status')}).catch(e=>{console.error(e.message);process.exit(1)})`
		if output, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
			t.Fatalf("stale=%v: %v: %s", stale, err, output)
		}
	}
}

func TestPhase3FormBatchRequests(t *testing.T) {
	got, err := phase3FormRequests(`{"operations":[{"operation":"question-delete","guid":"one"},{"operation":"meta","patch":{"title":"T"}}]}`)
	if err != nil || len(got) != 2 {
		t.Fatal(got, err)
	}
	for _, p := range []string{`null`, `[]`, `{"operations":[]}`, `{"operations":[1]}`, `{"operations":[{}]}`, `{"operations":[{"operation":"meta"}],"other":true}`} {
		if _, err := phase3FormRequests(p); err == nil {
			t.Fatalf("accepted %s", p)
		}
	}
}
