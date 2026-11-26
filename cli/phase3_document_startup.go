package main

import (
	"errors"
	"fmt"
	"io"
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
	for _, deny := range []string{"编辑器动作抛出异常", "编辑器拒绝执行", "未登录", "无访问权限", "无权访问", "permission denied", "access denied", "等待编辑器或目标方法超时"} {
		if strings.Contains(message, deny) {
			return false
		}
	}
	for _, transient := range []string{"i/o timeout", "connection reset", "broken pipe", "use of closed network connection"} {
		if strings.Contains(message, transient) {
			return true
		}
	}
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || message == "eof" || strings.HasSuffix(message, ": eof") || strings.Contains(message, "unexpected eof")
}

// Slow pages keep the whole initial budget, without a reload. Only an early
// transport failure can reconnect once; it consumes the same remaining budget
// and never accepts an edit callback.
func phase3InitialReady(total time.Duration, ops phase3InitialReadyOps) error {
	if total <= 0 {
		return errors.New("初始编辑器预算已耗尽")
	}
	deadline := ops.now().Add(total)
	firstErr := ops.wait(total)
	if firstErr == nil {
		return nil
	}
	if !phase3StartupRecoverable(firstErr) || !ops.now().Before(deadline) {
		return firstErr
	}
	if err := ops.record(firstErr); err != nil {
		return fmt.Errorf("首次就绪诊断无法保存，未重连：%w", err)
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
		return errors.New("初始重连后总预算耗尽；当前就绪检查为只读")
	}
	return ops.wait(remaining)
}

func (b *phase3EditorBrowser) waitInitialReady(readiness string, budget time.Duration) error {
	var report map[string]any
	err := phase3InitialReady(budget, phase3InitialReadyOps{
		now: time.Now,
		wait: func(remaining time.Duration) error {
			check := "typeof window.__shimoPhase3Presence==='function'&&(" + readiness + ")"
			return b.WaitReady(check, remaining)
		},
		record: func(cause error) error {
			dir, err := b.newDiagnosticDir("shimo-phase3-startup-")
			if err != nil {
				return err
			}
			b.startupEvidence = filepath.Join(dir, "startup-recovery.json")
			report = map[string]any{"scope": "initial read readiness only", "mutationAttempted": false, "initialFailure": cause.Error(), "firstFailureAt": time.Now().UTC(), "diagnostic": b.captureReadinessFailure(cause).Error(), "reloadAttempted": false}
			return phase3WriteEvidence(b.startupEvidence, report)
		},
		recover: func(_ error, remaining time.Duration) error {
			// A transport timeout may have left a partial WebSocket frame.
			// Reconnect only the same page; its existing JS observer and loading
			// progress remain intact, and the next wait still requires them.
			if err := b.reconnectInitial(remaining); err != nil {
				return err
			}
			report["reconnectedSameTarget"] = true
			return phase3WriteEvidence(b.startupEvidence, report)
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

// Reconnection preserves the existing page and its collaboration observer.
// Do not reload or reinstall an observer that only activates after navigation.
func (b *phase3EditorBrowser) reconnectInitial(remaining time.Duration) error {
	if remaining > 15*time.Second {
		remaining = 15 * time.Second
	}
	callback := b.ws.onEvent
	b.ws.Close()
	ws, err := dialWSWithTimeout(b.debugURL, remaining)
	if err != nil {
		return fmt.Errorf("重新连接同一CDP目标失败（未刷新或重开窗口）: %w", err)
	}
	b.ws = ws
	b.ws.onEvent = callback
	return nil
}

// Before initial navigation, enable the Page domain and register the observer.
// Readiness checks that the observer actually exists in the loaded page.
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
