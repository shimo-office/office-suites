package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPhase3ReferenceCapabilityRequiresBoundVisibleMenu(t *testing.T) {
	good := map[string]any{"tableGuid": "t1", "fieldType": "ReferenceRecord", "menuInitialized": true, "referenceTypeOffered": true, "fieldOptionsDisableRefField": false, "renderedFieldTypes": []any{"SingleLineText", "ReferenceRecord"}}
	if err := verifyPhase3ReferenceCapability(good, "t1"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tableGuid", "fieldType", "menuInitialized", "referenceTypeOffered", "renderedFieldTypes"} {
		bad := phase3TableCopy(good)
		delete(bad, key)
		if err := verifyPhase3ReferenceCapability(bad, "t1"); err == nil {
			t.Fatalf("missing %s accepted", key)
		}
	}
	for _, change := range []map[string]any{{"tableGuid": "other"}, {"fieldType": "LongText"}, {"menuInitialized": false}, {"referenceTypeOffered": false}, {"fieldOptionsDisableRefField": true}, {"renderedFieldTypes": []any{"LongText"}}} {
		bad := phase3TableCopy(good)
		for k, v := range change {
			bad[k] = v
		}
		if err := verifyPhase3ReferenceCapability(bad, "t1"); err == nil {
			t.Fatal("invalid capability accepted", change)
		}
	}
}

type phase3ReferenceCapabilityTestGuard struct {
	tableTestGuard
	capability map[string]any
}

func (g *phase3ReferenceCapabilityTestGuard) ReferenceCapability(table string) (map[string]any, error) {
	return g.capability, nil
}

func TestPhase3ReferenceUnavailableOrUnknownNeverSubmits(t *testing.T) {
	t.Setenv("SHIMO_NET_COOKIE", "sid=test")
	for _, scenario := range []string{"missing-bridge", "unknown", "unavailable", "wrong-table"} {
		t.Run(scenario, func(t *testing.T) {
			a := tableFixtureForSnapshot()
			a["baseGuid"] = "base12345678"
			a["tableType"] = float64(0)
			b := phase3TableCopy(a)
			b["guid"] = "t2"
			b["name"] = "related"
			b["views"].([]any)[0].(map[string]any)["tableGuid"] = "t2"
			posts := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "POST" {
					posts++
					http.Error(w, "must not write", 400)
					return
				}
				switch r.URL.Path {
				case "/api/v1/files/base12345678":
					fmt.Fprint(w, `{"guid":"base12345678","type":"table"}`)
				case "/tables/base12345678":
					fmt.Fprint(w, `const envs = [{"__RUNTIME_ENV__":{"SDK_V2_PATH_PREFIX":"/sdk/v2/api","SDK_V2_SIGNATURE":"test-signature","SDK_V2_TOKEN":"test-token"}}];`)
				case "/sdk/v2/api/table-api/bases/base12345678/rev":
					fmt.Fprint(w, `{"rev":1,"filePermission":{"canRead":true,"canEdit":true}}`)
				case "/sdk/v2/api/table-api/v2/bases/base12345678":
					json.NewEncoder(w).Encode(map[string]any{"tables": []any{a, b}})
				case "/sdk/v2/api/table-api/tables/t1":
					json.NewEncoder(w).Encode(a)
				case "/sdk/v2/api/table-api/tables/t2":
					json.NewEncoder(w).Encode(b)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			original := phase3OpenTableGuard
			defer func() { phase3OpenTableGuard = original }()
			phase3OpenTableGuard = func(string) (phase3TableGuard, error) {
				if scenario == "missing-bridge" {
					return &tableTestGuard{}, nil
				}
				c := map[string]any{}
				if scenario != "unknown" {
					c = map[string]any{"tableGuid": "t1", "fieldType": "ReferenceRecord", "menuInitialized": true, "referenceTypeOffered": false, "renderedFieldTypes": []any{"LongText"}}
				}
				if scenario == "wrong-table" {
					c["tableGuid"] = "other"
					c["referenceTypeOffered"] = true
					c["renderedFieldTypes"] = []any{"ReferenceRecord"}
				}
				return &phase3ReferenceCapabilityTestGuard{capability: c}, nil
			}
			out := t.TempDir()
			app := &App{Origin: server.URL, JSON: true, Pos: []string{server.URL + "/tables/base12345678"}, Flags: map[string]string{"--action": "table-reference-add", "--payload": `{"tableGuid":"t1","referenceTableGuid":"t2","name":"related"}`, "--out": out}}
			err := app.cmdPhase3BaseEdit()
			if err == nil || posts != 0 || strings.Contains(err.Error(), "写请求已提交") {
				t.Fatalf("guard err=%v POST=%d", err, posts)
			}
			if !strings.Contains(err.Error(), "关联字段") {
				t.Fatalf("failed before reference guard: %v", err)
			}
			files, e := filepath.Glob(filepath.Join(out, "phase3-*", "failure.json"))
			if e != nil || len(files) != 1 {
				t.Fatalf("failure evidence missing: %v %v", files, e)
			}
			data, e := os.ReadFile(files[0])
			if e != nil {
				t.Fatal(e)
			}
			var failure map[string]any
			if e = json.Unmarshal(data, &failure); e != nil {
				t.Fatal(e)
			}
			if failure["mutationAttempted"] != false {
				t.Fatal("failure incorrectly claims submission", failure)
			}
		})
	}
}

func TestPhase3ReferenceCapabilityReadsActualMenuBinding(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	script := `let mode='';global.getComputedStyle=()=>({visibility:'visible',display:'block'});const oi={Ea:{getConfig:k=>{if(k!=='FieldOptions')throw Error('wrong config');return {disableRefField:mode==='disabled'}}},getActiveTable:()=>({getGuid:()=> 't1'})};const element=(type,model=oi)=>({getBoundingClientRect:()=>({width:10,height:10}),getAttribute:()=>type,__reactFiber$test:{memoizedProps:{oi:model},return:null}});global.document={querySelectorAll:()=>mode==='empty'?[]:mode==='mixed'?[element('LongText'),element('ReferenceRecord',{...oi})]:mode==='unbound'?[element('ReferenceRecord',null)]:[element('LongText'),element(mode==='unavailable'?'Number':'ReferenceRecord')]};for(const scenario of ['','disabled','unavailable','mixed','unbound','empty']){mode=scenario;let result,error;try{result=` + phase3ReferenceCapabilityRead + `}catch(e){error=e}if(['mixed','unbound','empty'].includes(mode)){if(!error)throw Error('unbound menu accepted '+mode)}else{if(error)throw error;if(result.tableGuid!=='t1'||result.fieldType!=='ReferenceRecord'||result.menuInitialized!==true||result.referenceTypeOffered!==(mode!=='unavailable')||result.fieldOptionsDisableRefField!==(mode==='disabled'))throw Error(JSON.stringify(result))}}`
	if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
