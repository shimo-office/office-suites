package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type phase3InitialReadyOps struct {
	now     func() time.Time
	wait    func(time.Duration) error
	record  func(error) error
	recover func(error, time.Duration) error
}

func phase3StartupRecoverable(err error) bool {
	message := strings.ToLower(err.Error())
	for _, deny := range []string{"编辑器动作抛出异常", "编辑器拒绝执行", "未登录", "无访问权限", "无权访问", "permission denied", "access denied"} {
		if strings.Contains(message, deny) {
			return false
		}
	}
	for _, transient := range []string{"等待编辑器或目标方法超时", "i/o timeout", "connection reset", "broken pipe", "use of closed network connection"} {
		if strings.Contains(message, transient) {
			return true
		}
	}
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || message == "eof" || strings.HasSuffix(message, ": eof") || strings.Contains(message, "unexpected eof")
}

// Only initial read readiness uses this bounded recovery. No edit callback is
// accepted here, and the second wait consumes the original remaining budget.
func phase3InitialReady(total, first time.Duration, ops phase3InitialReadyOps) error {
	if total <= 0 {
		return errors.New("初始编辑器预算已耗尽")
	}
	deadline := ops.now().Add(total)
	if first > total {
		first = total
	}
	firstErr := ops.wait(first)
	if firstErr == nil {
		return nil
	}
	if !phase3StartupRecoverable(firstErr) || !ops.now().Before(deadline) {
		return firstErr
	}
	if err := ops.record(firstErr); err != nil {
		return fmt.Errorf("首次就绪诊断无法保存，未刷新：%w", err)
	}
	remaining := deadline.Sub(ops.now())
	if remaining <= 0 {
		return fmt.Errorf("首次诊断后总预算耗尽：%w", firstErr)
	}
	if err := ops.recover(firstErr, remaining); err != nil {
		return fmt.Errorf("初始只读恢复失败；当前就绪检查为只读：%w", err)
	}
	remaining = deadline.Sub(ops.now())
	if remaining <= 0 {
		return errors.New("初始刷新后总预算耗尽；当前就绪检查为只读")
	}
	return ops.wait(remaining)
}

func (b *phase3EditorBrowser) waitInitialReady(readiness string, budget time.Duration) error {
	var report map[string]any
	var previousOrigin any
	err := phase3InitialReady(budget, 120*time.Second, phase3InitialReadyOps{
		now: time.Now,
		wait: func(remaining time.Duration) error {
			check := "typeof window.__shimoPhase3Presence==='function'&&(" + readiness + ")"
			if previousOrigin != nil {
				origin, _ := json.Marshal(previousOrigin)
				check = "performance.timeOrigin!==" + string(origin) + "&&(" + check + ")"
			}
			return b.WaitReady(check, remaining)
		},
		record: func(cause error) error {
			dir, err := os.MkdirTemp("", "shimo-phase3-startup-")
			if err != nil {
				return err
			}
			b.startupEvidence = filepath.Join(dir, "startup-recovery.json")
			report = map[string]any{"scope": "initial read readiness only", "mutationAttempted": false, "initialFailure": cause.Error(), "firstFailureAt": time.Now().UTC(), "diagnostic": b.captureReadinessFailure(cause).Error(), "reloadAttempted": false}
			return phase3WriteEvidence(b.startupEvidence, report)
		},
		recover: func(cause error, remaining time.Duration) error {
			deadline := time.Now().Add(remaining)
			limit := func(cap time.Duration) time.Duration {
				left := time.Until(deadline)
				if left < cap {
					return left
				}
				return cap
			}
			transport := !strings.Contains(cause.Error(), "等待编辑器或目标方法超时")
			if transport {
				callback := b.ws.onEvent
				b.ws.Close()
				ws, err := dialWSWithTimeout(b.debugURL, limit(15*time.Second))
				if err != nil {
					return fmt.Errorf("重新连接同一CDP目标失败: %w", err)
				}
				b.ws = ws
				b.ws.onEvent = callback
				report["reconnectedSameTarget"] = true
				// The new CDP session re-installs the observer; its guard makes duplicate
				// registrations harmless if Chrome retained the previous session's script.
				if err = b.installPresenceObserver(deadline); err != nil {
					return err
				}
				report["presenceObserverRegistered"] = true
			}
			state, err := b.evalTimed(`(()=>{const text=(document.body?.innerText||'').slice(0,3000);if(location.pathname.startsWith('/login'))return {ok:false,error:'隔离编辑器未登录，未刷新'};if(/没有访问权限|无权访问|申请访问权限|文件已被删除|文件不存在/.test(text))return {ok:false,error:'目标明确不可访问，未刷新'};return {ok:true,timeOrigin:performance.timeOrigin,path:location.pathname}})()`, limit(15*time.Second))
			if err != nil {
				return err
			}
			m, ok := state.(map[string]any)
			if !ok || num(m, "timeOrigin") <= 0 {
				return errors.New("初始重载世代不可确认，未刷新")
			}
			previousOrigin = m["timeOrigin"]
			report["beforeReload"] = state
			report["reloadAttempted"] = true
			report["reloadAt"] = time.Now().UTC()
			if err = phase3WriteEvidence(b.startupEvidence, report); err != nil {
				return err
			}
			// Page.reload is navigation only; no document mutation is ever retried.
			_, err = b.ws.Call(11002, "Page.reload", map[string]any{"ignoreCache": true}, limit(15*time.Second))
			return err
		},
	})
	if report != nil {
		report["ready"] = err == nil
		report["finishedAt"] = time.Now().UTC()
		if err != nil {
			report["finalError"] = err.Error()
		}
		if saveErr := phase3WriteEvidence(b.startupEvidence, report); saveErr != nil && err == nil {
			err = saveErr
		}
	}
	if err != nil {
		if b.startupEvidence != "" {
			err = fmt.Errorf("%w；初始恢复记录：%s", err, b.startupEvidence)
		}
		return &phase3ReadinessFailure{err}
	}
	return nil
}

// A reconnected CDP session must enable the Page domain before installing its
// script. Readiness additionally checks that the observer exists in the new page.
func (b *phase3EditorBrowser) installPresenceObserver(deadline time.Time) error {
	if b.presenceObserver == "" {
		return errors.New("在线名单观察脚本缺失")
	}
	for _, method := range []string{"Page.enable", "Page.addScriptToEvaluateOnNewDocument"} {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return errors.New("在线名单观察脚本安装预算耗尽")
		}
		if remaining > 10*time.Second {
			remaining = 10 * time.Second
		}
		var params map[string]any
		if method == "Page.addScriptToEvaluateOnNewDocument" {
			params = map[string]any{"source": b.presenceObserver}
		}
		b.nextID++
		result, err := b.ws.Call(b.nextID, method, params, remaining)
		if err != nil {
			return fmt.Errorf("在线名单观察脚本安装 %s 失败: %w", method, err)
		}
		if method == "Page.addScriptToEvaluateOnNewDocument" && str(result, "identifier") == "" {
			return errors.New("在线名单观察脚本未取得实际注册ID")
		}
	}
	return nil
}

// Foreground activation is part of the authorized interactive browser workflow.
// It consumes the same initial deadline and never triggers a fresh-profile retry.
func (b *phase3EditorBrowser) navigateInitial(target string, deadline time.Time) error {
	for _, method := range []string{"Page.bringToFront", "Page.navigate"} {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return &phase3ReadinessFailure{fmt.Errorf("初始 %s 预算耗尽；本次仅执行只读导航", method)}
		}
		if remaining > 15*time.Second {
			remaining = 15 * time.Second
		}
		var params map[string]any
		if method == "Page.navigate" {
			params = map[string]any{"url": target}
		}
		b.nextID++
		if _, err := b.ws.Call(b.nextID, method, params, remaining); err != nil {
			return &phase3ReadinessFailure{fmt.Errorf("初始 %s 失败；本次仅执行只读导航: %w", method, err)}
		}
	}
	return nil
}
