package main

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestReadinessSDKGraceAndClassification(t *testing.T) {
	if phase3EditorSdkMissingGrace != 60*time.Second {
		t.Errorf("SDK grace = %s, want 60s", phase3EditorSdkMissingGrace)
	}
	calls := 0
	_, err := phase3PollReady(time.Second, time.Millisecond, func() (map[string]any, error) {
		calls++
		return map[string]any{"ready": calls == 2, "login": false, "documentState": "complete", "sdk": "undefined", "presentationSDK": "undefined", "pageElapsedMs": float64(45000)}, nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("45s cold start must still become ready: calls=%d err=%v", calls, err)
	}
	for _, tc := range []struct {
		name  string
		state map[string]any
		code  string
	}{
		{"missing-sdk", map[string]any{"ready": false, "login": false, "documentState": "complete", "sdk": "undefined", "presentationSDK": "undefined", "pageElapsedMs": float64(61000)}, "EDITOR_NOT_READY"},
		{"login-redirect", map[string]any{"ready": false, "login": true}, "AUTH_REQUIRED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := phase3PollReady(time.Second, time.Millisecond, func() (map[string]any, error) { return tc.state, nil })
			var detail *commandError
			if !errors.As(err, &detail) || detail.Result["code"] != tc.code {
				t.Fatalf("missing classification: %v", err)
			}
			if tc.code == "EDITOR_NOT_READY" && (strings.Contains(err.Error(), "判定登录态失效") || strings.Contains(err.Error(), "shimo-net login")) {
				t.Fatal("SDK readiness must not request login", err)
			}
		})
	}
	_, err = phase3PollReady(time.Second, time.Millisecond, func() (map[string]any, error) {
		return map[string]any{"ready": true, "documentState": "complete", "sdk": "undefined", "presentationSDK": "undefined", "pageElapsedMs": float64(61000)}, nil
	})
	if err != nil {
		t.Fatal("requested readiness is authoritative even without legacy globals", err)
	}
}

func TestPhase2ReadinessMatchesExecutionRoots(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("requires existing Node")
	}
	cases := []struct {
		name, method, setup string
		want                bool
	}{
		{"root-sdk", "sdk.history.show", `window.__SM__={history:{show(){}}}`, true},
		{"docx-sdk", "sdk.print", `window._sdk={editor:{docsApi:{print(){}}}}`, true},
		{"spreadsheet-sdk", "sdk.version.createRevision", `window.smsdk={__DRIVE_SDK__:{editor:{version:{createRevision(){}}}}}`, true},
		{"missing-specific-method", "sdk.history.show", `window.smsdk={__DRIVE_SDK__:{editor:{print(){}}}}`, false},
		{"hide-specific-method", "sdk.history.hide", `window.__SM__={editor:{history:{show(){}}}}`, false},
		{"non-function", "sdk.print", `window.__SM__={editor:{print:true}}`, false},
		{"empty-shell", "sdk.print", `window.__SM__={editor:{print(){}}};document.querySelector=()=>null;document.body.innerText=''`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code := `global.window=global;global.document={body:{innerText:'synthetic content'},querySelector:()=>({})};` + tc.setup + `;const got=` + phase2ReadyExpression(tc.method) + `;if(got!==` + map[bool]string{true: "true", false: "false"}[tc.want] + `)throw Error('unexpected readiness '+got);`
			out, err := exec.Command(node, "-e", code).CombinedOutput()
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
		})
	}
}
