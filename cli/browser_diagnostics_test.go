package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 只验证失败分类，不启动浏览器：这些日志片段取自 Chrome 158 在被沙箱终端里
// 启动时的真实输出。
const sampleSandboxDeniedLog = `[1125/114537.878257:WARNING:crashpad.cc:493] could not read crashpad info
sandbox initialization failed: Operation not permitted
aperitif: Failed to initialize sandbox
[78345:23605189:1125/114537.893186:ERROR:content/browser/gpu/gpu_process_host.cc:1042] GPU process exited unexpectedly: exit_code=6
[78345:23605189:1125/114537.893247:FATAL:content/browser/gpu/gpu_data_manager_impl_private.cc:385] GPU process isn't usable. Goodbye.
`

func writeTempLog(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "browser.log")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBrowserSandboxDeniedMatchesRealChromeOutput(t *testing.T) {
	if !browserSandboxDenied(sampleSandboxDeniedLog) {
		t.Fatal("GPU 退出与沙箱初始化失败必须被识别")
	}
	for _, line := range []string{
		"sandbox initialization failed: Operation not permitted",
		"aperitif: Failed to initialize sandbox",
		"GPU process isn't usable. Goodbye.",
		"GPU process exited unexpectedly: exit_code=6",
	} {
		if !browserSandboxDenied(line) {
			t.Fatalf("单行未识别：%s", line)
		}
	}
	if browserSandboxDenied("DevTools listening on ws://127.0.0.1:9222/devtools/browser/x") {
		t.Fatal("正常日志不应被判为沙箱失败")
	}
	if browserSandboxDenied("Operation not permitted") {
		t.Fatal("缺少 sandbox 关键词时不应猜测")
	}
}

func TestExplainBrowserFailureKeepsUnknownErrors(t *testing.T) {
	boom := errors.New("connection reset by peer")
	log := writeTempLog(t, "DevTools listening on ws://127.0.0.1:1/devtools/browser/x\n")
	got := explainBrowserFailure(boom, log)
	if got != boom {
		t.Fatalf("未识别的错误应原样返回，得到 %v", got)
	}
	if explainBrowserFailure(nil, log) != nil {
		t.Fatal("nil 错误应保持 nil")
	}
}

func TestExplainBrowserFailureReportsSandboxDenial(t *testing.T) {
	log := writeTempLog(t, sampleSandboxDeniedLog)
	err := explainBrowserFailure(errors.New("EOF"), log)
	msg := err.Error()
	if !strings.Contains(msg, "沙箱") || !strings.Contains(msg, "SHIMO_NET_NO_SANDBOX") {
		t.Fatalf("诊断必须给出可执行建议：%s", msg)
	}
	if !strings.Contains(msg, "EOF") {
		t.Fatalf("诊断应保留原始错误：%s", msg)
	}
}

func TestExplainBrowserFailureWhenSandboxAlreadyDisabled(t *testing.T) {
	t.Setenv("SHIMO_NET_NO_SANDBOX", "1")
	log := writeTempLog(t, sampleSandboxDeniedLog)
	msg := explainBrowserFailure(errors.New("EOF"), log).Error()
	if strings.Contains(msg, "设置 SHIMO_NET_NO_SANDBOX=1 后重试") {
		t.Fatalf("已开启无沙箱时不应再让用户设置同一变量：%s", msg)
	}
	if !strings.Contains(msg, "SHIMO_NET_NO_SANDBOX=1") {
		t.Fatalf("应说明当前已设置该变量：%s", msg)
	}
}

func TestExplainBrowserFailureReportsCrashWithoutSandboxHint(t *testing.T) {
	log := writeTempLog(t, "[1:2:ERROR:inspector.cc:1] check failed: crashed\n")
	err := explainBrowserFailure(errors.New("EOF"), log)
	if !strings.Contains(err.Error(), "异常退出") {
		t.Fatalf("崩溃日志应被单独识别：%v", err)
	}
	if strings.Contains(err.Error(), "SHIMO_NET_NO_SANDBOX") {
		t.Fatalf("普通崩溃不应误报为沙箱问题：%v", err)
	}
}

func TestBrowserLogTailKeepsTailOnly(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 100; i++ {
		b.WriteString("line\n")
	}
	b.WriteString("last")
	path := writeTempLog(t, b.String())
	tail := browserLogTail(path, 3)
	if !strings.HasSuffix(tail, "last") {
		t.Fatalf("应保留末尾内容：%q", tail)
	}
	if strings.Count(tail, "line") != 2 {
		t.Fatalf("应只保留最后 3 行：%q", tail)
	}
	if browserLogTail(path, 0) != "" || browserLogTail("", 10) != "" {
		t.Fatal("空参数应返回空串")
	}
}

func TestCaptureBrowserOutputWritesLog(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo 沙箱初始化失败 >&2")
	path, cleanup := captureBrowserOutput(cmd)
	if path == "" {
		t.Skip("当前环境无法创建临时日志文件")
	}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	log := readBrowserLog(path)
	if !strings.Contains(log, "沙箱初始化失败") {
		t.Fatalf("浏览器 stderr 必须落到日志：%q", log)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("日志权限必须是 0600：%v %v", info, err)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("cleanup 应删除日志")
	}
}
