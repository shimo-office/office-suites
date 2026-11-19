package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPhase3InitialReadyRecoveryUsesOneSharedBudget(t *testing.T) {
	now := time.Unix(1000, 0)
	waits := []time.Duration{}
	recoveries := 0
	records := 0
	err := phase3InitialReady(300*time.Second, 120*time.Second, phase3InitialReadyOps{
		now: func() time.Time { return now },
		wait: func(budget time.Duration) error {
			waits = append(waits, budget)
			now = now.Add(budget)
			if len(waits) == 1 {
				return errors.New("等待编辑器或目标方法超时")
			}
			return nil
		},
		record: func(error) error { records++; now = now.Add(3 * time.Second); return nil },
		recover: func(_ error, budget time.Duration) error {
			recoveries++
			if budget != 177*time.Second {
				t.Fatalf("remaining %s", budget)
			}
			now = now.Add(2 * time.Second)
			return nil
		},
	})
	if err != nil || recoveries != 1 || records != 1 || !reflect.DeepEqual(waits, []time.Duration{120 * time.Second, 175 * time.Second}) {
		t.Fatal(err, recoveries, records, waits)
	}
}
func TestPhase3InitialReadyDoesNotRecoverPermissionsOrRetryTwice(t *testing.T) {
	for _, failure := range []string{"隔离编辑器未登录，未执行写入", "目标无访问权限", "actual method exception", "typeof value failed", "编辑器动作抛出异常（Error: i/o timeout）；可能未完成，禁止自动重放", "编辑器拒绝执行：read tcp i/o timeout"} {
		calls := 0
		err := phase3InitialReady(time.Minute, 10*time.Second, phase3InitialReadyOps{now: time.Now, wait: func(time.Duration) error { return errors.New(failure) }, record: func(error) error { return nil }, recover: func(error, time.Duration) error { calls++; return nil }})
		if err == nil || calls != 0 {
			t.Fatal("unsafe initial retry", failure, err, calls)
		}
	}
	now := time.Unix(1000, 0)
	calls, waits := 0, 0
	err := phase3InitialReady(time.Minute, 10*time.Second, phase3InitialReadyOps{now: func() time.Time { return now }, wait: func(b time.Duration) error { waits++; now = now.Add(b); return errors.New("read tcp: i/o timeout") }, record: func(error) error { return nil }, recover: func(error, time.Duration) error { calls++; return nil }})
	if err == nil || calls != 1 || waits != 2 {
		t.Fatal(err, calls, waits)
	}
}
func TestPhase3InitialReadyNeverRecoversAfterBudgetExhausted(t *testing.T) {
	now := time.Unix(1000, 0)
	calls := 0
	err := phase3InitialReady(5*time.Second, 120*time.Second, phase3InitialReadyOps{now: func() time.Time { return now }, wait: func(b time.Duration) error {
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
	err := phase3InitialReady(time.Minute, 10*time.Second, phase3InitialReadyOps{now: time.Now, wait: func(time.Duration) error { waits++; return errors.New("i/o timeout") }, record: func(error) error { return errors.New("disk full") }, recover: func(error, time.Duration) error { reloads++; return nil }})
	if err == nil || waits != 1 || reloads != 0 {
		t.Fatal(err, waits, reloads)
	}
}

func TestPhase3ReadinessToleratesNavigationContextHandoff(t *testing.T) {
	calls := 0
	_, err := phase3PollReady(time.Second, time.Millisecond, func() (map[string]any, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("Inspected target navigated or closed")
		}
		return map[string]any{"ready": true}, nil
	})
	if err != nil || calls != 2 {
		t.Fatal(err, calls)
	}
	calls = 0
	_, err = phase3PollReady(time.Second, time.Millisecond, func() (map[string]any, error) {
		calls++
		return nil, errors.New("Inspected target navigated or closed")
	})
	if err == nil || calls != 6 {
		t.Fatal("navigation must remain bounded", err, calls)
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
		err := phase3InitialReady(time.Minute, 10*time.Second, phase3InitialReadyOps{
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
