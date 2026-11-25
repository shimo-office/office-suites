package main

// 浏览器调试链路失败时，Chrome 已经把原因写在自己的 stderr 上（沙箱初始化被拒绝、
// GPU/渲染进程崩溃并带崩浏览器进程等）。CLI 以前把 stderr 丢进 /dev/null，用户只能
// 拿到一个光秃秃的 EOF，无法判断是环境问题还是工具问题。这里统一留档，并在失败时
// 给出可执行的结论。

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	browserLogMaxBytes  = 256 << 10
	browserLogTailLines = 40
)

// captureBrowserOutput 把浏览器的 stdout/stderr 指向临时文件。返回的 cleanup 必须调用。
// 日志里不含 cookie 值：Chrome 不打印这类内容。
func captureBrowserOutput(cmd *exec.Cmd) (string, func()) {
	f, err := os.CreateTemp("", "shimo-net-browser-*.log")
	if err != nil {
		cmd.Stdout, cmd.Stderr = nil, nil
		return "", func() {}
	}
	_ = f.Chmod(0o600)
	cmd.Stdout, cmd.Stderr = f, f
	return f.Name(), func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}
}

// readBrowserLog 只读尾部，避免异常大的日志拖慢失败路径。
func readBrowserLog(path string) string {
	if path == "" {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return ""
	}
	offset := int64(0)
	if st.Size() > browserLogMaxBytes {
		offset = st.Size() - browserLogMaxBytes
	}
	if _, err := f.Seek(offset, 0); err != nil {
		return ""
	}
	buf := make([]byte, browserLogMaxBytes)
	n, _ := f.Read(buf)
	return string(buf[:n])
}

func browserLogTail(path string, maxLines int) string {
	log := readBrowserLog(path)
	if log == "" || maxLines <= 0 {
		return ""
	}
	lines := strings.Split(strings.TrimRight(log, "\n"), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// browserSandboxDenied 判断日志里是否有"子进程沙箱建不起来"的证据。
// Chrome 的 GPU 进程沙箱初始化失败时先打 sandbox 错误，随后 abort，浏览器进程再以
// "GPU process isn't usable. Goodbye." 整体退出——两种情况一起认。
func browserSandboxDenied(log string) bool {
	lower := strings.ToLower(log)
	for _, marker := range []string{
		"sandbox initialization failed",
		"failed to initialize sandbox",
		"gpu process isn't usable",
		"gpu process exited unexpectedly",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return strings.Contains(lower, "operation not permitted") && strings.Contains(lower, "sandbox")
}

func browserCrashed(log string) bool {
	lower := strings.ToLower(log)
	for _, marker := range []string{"targetcrashed", "renderer process crashed", "check failed", "fatal error"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// explainBrowserFailure 把 CDP 链路错误翻译成能直接照做的结论。
// 识别不出来时保留原始错误，不编造原因。
func explainBrowserFailure(err error, logPath string) error {
	if err == nil {
		return nil
	}
	log := readBrowserLog(logPath)
	if browserSandboxDenied(log) {
		if browserSandboxDisabled() {
			return fmt.Errorf("浏览器的子进程沙箱仍无法初始化：%w。已设置 SHIMO_NET_NO_SANDBOX=1，请确认当前环境允许启动 Chrome 的 GPU 与渲染进程", err)
		}
		return fmt.Errorf("浏览器的子进程沙箱无法初始化（sandbox_init 被拒绝），Chrome 因此在启动后整体退出，与登录态无关：%w。当前进程处在受限/沙箱终端里；请改用系统终端直接运行，或设置 SHIMO_NET_NO_SANDBOX=1 后重试", err)
	}
	if browserCrashed(log) {
		return fmt.Errorf("浏览器进程异常退出：%w。Chrome 日志：%s", err, browserLogReason(log))
	}
	return err
}

// browserLogReason 取最有信息量的一行：优先崩溃类标记，其次最后一行非空日志。
func browserLogReason(log string) string {
	lines := strings.Split(log, "\n")
	last := ""
	for _, line := range lines {
		s := strings.TrimSpace(line)
		if s == "" {
			continue
		}
		last = s
		lower := strings.ToLower(s)
		if strings.Contains(lower, "targetcrashed") || strings.Contains(lower, "renderer process crashed") ||
			strings.Contains(lower, "check failed") || strings.Contains(lower, "fatal error") {
			return truncateLine(s)
		}
	}
	return truncateLine(last)
}

func truncateLine(s string) string {
	if len(s) > 300 {
		return s[:300]
	}
	return s
}
