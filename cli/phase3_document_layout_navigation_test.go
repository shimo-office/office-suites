package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

func TestDocumentLayoutNavigationPageValidation(t *testing.T) {
	for _, p := range []map[string]any{{"page": float64(0)}, {"page": 1.5}, {"page": "2"}, {"page": float64(2), "unexpected": true}} {
		if _, e := prepareDocumentPageNavigation(p); e == nil {
			t.Fatal("invalid page accepted", p)
		}
	}
	if n, e := prepareDocumentPageNavigation(map[string]any{"page": float64(2)}); e != nil || n != 2 {
		t.Fatal(n, e)
	}
}

func TestDocumentNavigationWaitsForCalculatedTargetBeforeSingleCall(t *testing.T) {
	for _, readyAt := range []int{-1, 2} {
		reads, navigations := 0, 0
		read := func(string) (any, error) {
			reads++
			return map[string]any{"ready": readyAt > 0 && reads >= readyAt}, nil
		}
		navigate := func(string) (any, error) { navigations++; return map[string]any{"ok": true}, nil }
		states, _, err := documentNavigateCalculatedPage(2, read, navigate, func(time.Duration) {}, 3)
		if readyAt < 0 {
			if err == nil || navigations != 0 || len(states) != 3 {
				t.Fatalf("uncalculated page navigated: reads=%d calls=%d err=%v", reads, navigations, err)
			}
		} else if err != nil || navigations != 1 || reads != 2 || len(states) != 2 {
			t.Fatalf("late readiness not handled once: reads=%d calls=%d err=%v", reads, navigations, err)
		}
	}
}

func TestDocumentNavigationCalculationReadiness(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	for _, scenario := range []struct {
		setup string
		ready bool
	}{
		{`global.window={}`, false},
		{`global.window={_sdk:{editor:{docsApi:{controller:{paintingModel:{totalPages:3},calcEngine:{getPageCount:()=>1}}}}}}`, false},
		{`global.window={_sdk:{editor:{docsApi:{controller:{paintingModel:{totalPages:1},calcEngine:{getPageCount:()=>3}}}}}}`, false},
		{`global.window={_sdk:{editor:{docsApi:{controller:{paintingModel:{totalPages:3},calcEngine:{getPageCount:()=>2}}}}}}`, true},
	} {
		script := scenario.setup + `;const result=` + documentPageCalculationStateJS(2) + `;if(result.ready!==` + strconv.FormatBool(scenario.ready) + `)throw Error(JSON.stringify(result));`
		if out, err := exec.Command(node, "-e", script).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
}

func TestDocumentNavigationRechecksCalculationBeforeCallingNative(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	var script string
	_, _, err = documentNavigateCalculatedPage(2, func(string) (any, error) {
		return map[string]any{"ready": true}, nil
	}, func(s string) (any, error) {
		script = s
		return map[string]any{"ok": true}, nil
	}, func(time.Duration) {}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, calculated := range []int{1, 2} {
		want := 0
		if calculated == 2 {
			want = 1
		}
		setup := fmt.Sprintf(`let calls=[];global.window={_sdk:{editor:{docsApi:{controller:{paintingModel:{totalPages:3},calcEngine:{getPageCount:()=>%d},GoToPage:p=>calls.push(p)}}}}};`, calculated)
		check := fmt.Sprintf(`;if(calls.length!==%d||calls.some(p=>p!==1)||result.ok!==%t)throw Error(JSON.stringify({calls,result}));`, want, want == 1)
		if out, err := exec.Command(node, "-e", setup+`const result=`+script+check).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
}

func TestDocumentLayoutNavigationRejectsStaleFooterOrCursor(t *testing.T) {
	node, e := exec.LookPath("node")
	if e != nil {
		t.Skip("node unavailable")
	}
	for _, test := range []struct {
		footer           string
		cursor, document int
		want             bool
	}{{"页数： 2/3", 1, 1, true}, {"页数： 1/3", 1, 1, false}, {"页数： 2/3", 0, 1, false}, {"页数： 2/3", 1, 0, false}, {"页数： 2/4", 1, 1, false}, {"", 1, 1, false}} {
		footer, _ := json.Marshal(test.footer)
		setup := fmt.Sprintf(`global.window={_sdk:{editor:{docsApi:{getCurVisiblePage:()=>1,controller:{paintingModel:{totalPages:3,cursorInPage:%d},docModel:{CurPage:%d}}}}}};global.document={getElementById:id=>({innerText:%s})};`, test.cursor, test.document, footer)
		script := setup + `const result=` + documentPageNavigationStateJS(2) + `;if(result.verified!==` + strconv.FormatBool(test.want) + `)throw Error(JSON.stringify(result));`
		if out, e := exec.Command(node, "-e", script).CombinedOutput(); e != nil {
			t.Fatalf("%v %s", e, out)
		}
	}
}
