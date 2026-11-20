package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPhase3InitialReadyWaitsFullBudgetWithoutRecovery(t *testing.T) {
	now := time.Unix(1000, 0)
	waits, recoveries := 0, 0
	err := phase3InitialReady(300*time.Second, phase3InitialReadyOps{
		now: func() time.Time { return now },
		wait: func(budget time.Duration) error {
			waits++
			if budget != 300*time.Second {
				t.Errorf("slow page received only %s", budget)
			}
			now = now.Add(180 * time.Second)
			return nil
		},
		record:  func(error) error { t.Error("slow loading recorded as failure"); return nil },
		recover: func(error, time.Duration) error { recoveries++; return nil },
	})
	if err != nil || waits != 1 || recoveries != 0 {
		t.Fatal(err, waits, recoveries)
	}
}

func TestPhase3InitialReadyRecoveryUsesOneSharedBudget(t *testing.T) {
	now := time.Unix(1000, 0)
	waits := []time.Duration{}
	recoveries, records := 0, 0
	err := phase3InitialReady(300*time.Second, phase3InitialReadyOps{
		now: func() time.Time { return now },
		wait: func(budget time.Duration) error {
			waits = append(waits, budget)
			if len(waits) == 1 {
				now = now.Add(30 * time.Second)
				return errors.New("connection reset")
			}
			return nil
		},
		record: func(error) error { records++; now = now.Add(3 * time.Second); return nil },
		recover: func(_ error, budget time.Duration) error {
			recoveries++
			if budget != 267*time.Second {
				t.Fatalf("remaining %s", budget)
			}
			now = now.Add(2 * time.Second)
			return nil
		},
	})
	if err != nil || recoveries != 1 || records != 1 || !reflect.DeepEqual(waits, []time.Duration{300 * time.Second, 265 * time.Second}) {
		t.Fatal(err, recoveries, records, waits)
	}
}

func TestPhase3InitialReadyNeverRecoversSlowPageTimeout(t *testing.T) {
	calls := 0
	err := phase3InitialReady(time.Minute, phase3InitialReadyOps{
		now:     time.Now,
		wait:    func(time.Duration) error { return errors.New("等待编辑器或目标方法超时") },
		record:  func(error) error { return nil },
		recover: func(error, time.Duration) error { calls++; return nil },
	})
	if err == nil || calls != 0 {
		t.Fatal(err, calls)
	}
}

func TestPhase3StartupDoesNotRelaunchAfterTransportFailure(t *testing.T) {
	original := phase3StartEditor
	t.Cleanup(func() { phase3StartEditor = original })
	for _, message := range []string{"EOF", "connection reset", "broken pipe", "i/o timeout", "permission denied"} {
		calls := 0
		expected := errors.New(message)
		phase3StartEditor = func(string, string) (*phase3EditorBrowser, error) { calls++; return nil, expected }
		browser, err := openPhase3EditorWhen("https://example.invalid/file", "true")
		if browser != nil || !errors.Is(err, expected) || calls != 1 {
			t.Errorf("startup replayed: %s calls=%d err=%v", message, calls, err)
		}
	}
}

func TestPhase3InitialReadyDoesNotRecoverPermissionsOrRetryTwice(t *testing.T) {
	for _, failure := range []string{"隔离编辑器未登录，未执行写入", "目标无访问权限", "actual method exception", "typeof value failed", "编辑器动作抛出异常（Error: i/o timeout）；可能未完成，禁止自动重放", "编辑器拒绝执行：read tcp i/o timeout"} {
		calls := 0
		err := phase3InitialReady(time.Minute, phase3InitialReadyOps{now: time.Now, wait: func(time.Duration) error { return errors.New(failure) }, record: func(error) error { return nil }, recover: func(error, time.Duration) error { calls++; return nil }})
		if err == nil || calls != 0 {
			t.Fatal("unsafe initial retry", failure, err, calls)
		}
	}
	now := time.Unix(1000, 0)
	calls, waits := 0, 0
	err := phase3InitialReady(time.Minute, phase3InitialReadyOps{now: func() time.Time { return now }, wait: func(b time.Duration) error {
		waits++
		now = now.Add(10 * time.Second)
		return errors.New("read tcp: i/o timeout")
	}, record: func(error) error { return nil }, recover: func(error, time.Duration) error { calls++; return nil }})
	if err == nil || calls != 1 || waits != 2 {
		t.Fatal(err, calls, waits)
	}
}
func TestPhase3InitialReadyNeverRecoversAfterBudgetExhausted(t *testing.T) {
	now := time.Unix(1000, 0)
	calls := 0
	err := phase3InitialReady(5*time.Second, phase3InitialReadyOps{now: func() time.Time { return now }, wait: func(b time.Duration) error {
		if b != 5*time.Second {
			t.Fatal(b)
		}
		now = now.Add(b)
		return errors.New("i/o timeout")
	}, record: func(error) error { return nil }, recover: func(error, time.Duration) error { calls++; return nil }})
	if err == nil || calls != 0 {
		t.Fatal(err, calls)
	}
}

func TestPhase3InitialRecoveryRequiresSavedEvidence(t *testing.T) {
	waits, reloads := 0, 0
	err := phase3InitialReady(time.Minute, phase3InitialReadyOps{now: time.Now, wait: func(time.Duration) error { waits++; return errors.New("i/o timeout") }, record: func(error) error { return errors.New("disk full") }, recover: func(error, time.Duration) error { reloads++; return nil }})
	if err == nil || waits != 1 || reloads != 0 {
		t.Fatal(err, waits, reloads)
	}
}

func TestPhase3ReadinessToleratesNavigationContextHandoff(t *testing.T) {
	calls := 0
	_, err := phase3PollReady(time.Second, time.Millisecond, func() (map[string]any, error) {
		calls++
		if calls <= 8 {
			return nil, errors.New("Inspected target navigated or closed")
		}
		return map[string]any{"ready": true}, nil
	})
	if err != nil || calls != 9 {
		t.Fatal(err, calls)
	}
	calls = 0
	started := time.Now()
	_, err = phase3PollReady(15*time.Millisecond, time.Millisecond, func() (map[string]any, error) {
		calls++
		return nil, errors.New("Inspected target navigated or closed")
	})
	if err == nil || !strings.Contains(err.Error(), "超时") || time.Since(started) < 15*time.Millisecond || time.Since(started) > time.Second {
		t.Fatal("navigation must consume bounded wait", err, calls)
	}
}

func TestPhase3ReadinessNeverRetriesExplicitJSException(t *testing.T) {
	for _, message := range []string{"编辑器动作抛出异常（Error: Inspected target navigated or closed）", "编辑器拒绝执行：cannot find context"} {
		calls := 0
		_, err := phase3PollReady(time.Second, time.Millisecond, func() (map[string]any, error) { calls++; return nil, errors.New(message) })
		if err == nil || calls != 1 {
			t.Fatal("explicit script failure retried", err, calls)
		}
	}
}
func TestPhase3ReadinessErrorDoesNotClaimOverallWriteState(t *testing.T) {
	_, err := phase3PollReady(time.Millisecond, time.Millisecond, func() (map[string]any, error) { return map[string]any{"ready": false}, nil })
	if err == nil || strings.Contains(err.Error(), "未执行写入") {
		t.Fatal(err)
	}
}

func TestPhase3RecoveryErrorsDescribeOnlyCurrentReadiness(t *testing.T) {
	for _, failure := range []bool{true, false} {
		now := time.Unix(1000, 0)
		err := phase3InitialReady(time.Minute, phase3InitialReadyOps{
			now:    func() time.Time { return now },
			wait:   func(time.Duration) error { return errors.New("i/o timeout") },
			record: func(error) error { return nil },
			recover: func(error, time.Duration) error {
				if failure {
					return errors.New("reconnect failed")
				}
				now = now.Add(time.Minute)
				return nil
			},
		})
		if err == nil || strings.Contains(err.Error(), "未执行编辑") || strings.Contains(err.Error(), "未执行内容写入") || !strings.Contains(err.Error(), "当前就绪检查") {
			t.Fatalf("error claims overall write state: %v", err)
		}
	}
}

func TestPhase3InitialTransportRecoveryKeepsPageAndObserver(t *testing.T) {
	for _, outcome := range []string{"ready", "observer-missing", "login"} {
		t.Run(outcome, func(t *testing.T) {
			root := t.TempDir()
			for _, key := range []string{"TMPDIR", "TEMP", "TMP"} {
				t.Setenv(key, root)
			}
			requests := make(chan map[string]any, 10)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/devtools/page/original" {
					t.Errorf("changed CDP target: %s", r.URL.Path)
				}
				conn, rdwr, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				_, _ = rdwr.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
				_ = rdwr.Flush()
				peer := &WSConn{conn: conn, rd: rdwr.Reader}
				for {
					raw, err := peer.readMessage()
					if err != nil {
						return
					}
					var request map[string]any
					if err = json.Unmarshal(raw, &request); err != nil {
						t.Error(err)
						return
					}
					requests <- request
					result := map[string]any{}
					switch request["method"] {
					case "Page.captureScreenshot":
						result["data"] = ""
					case "Runtime.evaluate":
						params, _ := request["params"].(map[string]any)
						expr := str(params, "expression")
						if !strings.Contains(expr, "typeof window.__shimoPhase3Presence==='function'") {
							t.Errorf("lost existing observer guard: %s", expr)
						}
						result["result"] = map[string]any{"value": map[string]any{"ready": outcome == "ready", "login": outcome == "login", "budgetExpired": outcome == "observer-missing"}}
					default:
						t.Errorf("startup changed page instead of waiting: %v", request["method"])
					}
					_ = peer.sendJSON(map[string]any{"id": request["id"], "result": result})
				}
			}))
			defer server.Close()
			client, initial := net.Pipe()
			defer client.Close()
			initialDone := make(chan struct{})
			go func() {
				defer close(initialDone)
				defer initial.Close()
				peer := &WSConn{conn: initial, rd: bufio.NewReader(initial)}
				_, _ = peer.readMessage()
			}()
			b := &phase3EditorBrowser{ws: &WSConn{conn: client, rd: bufio.NewReader(client)}, debugURL: "ws" + strings.TrimPrefix(server.URL, "http") + "/devtools/page/original"}
			defer func() { b.ws.Close() }()
			err := b.waitInitialReady("true", 2*time.Second)
			<-initialDone
			if (err == nil) != (outcome == "ready") {
				t.Fatalf("unexpected recovery outcome %s: %v", outcome, err)
			}
			if len(requests) != 2 {
				t.Fatalf("wanted diagnosis and read-only probe, got %d calls", len(requests))
			}
			if (<-requests)["method"] != "Page.captureScreenshot" || (<-requests)["method"] != "Runtime.evaluate" {
				t.Fatal("unexpected recovery commands")
			}
			data, readErr := os.ReadFile(b.startupEvidence)
			if readErr != nil {
				t.Fatal(readErr)
			}
			var report map[string]any
			if json.Unmarshal(data, &report) != nil || report["reloadAttempted"] != false || report["reconnectedSameTarget"] != true || report["mutationAttempted"] != false {
				t.Fatalf("incorrect recovery evidence: %s", data)
			}
		})
	}
}
