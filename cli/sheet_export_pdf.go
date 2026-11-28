package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The export service refuses PDF for spreadsheets (code 3613), so the only way
// to produce the menu's PDF is the page wizard itself. This command drives that
// wizard with real clicks and captures the browser download; it never infers a
// PDF from an HTTP status.

type sheetPDFStage string

const (
	sheetPDFStagePreflight sheetPDFStage = "preflight"
	sheetPDFStageMenu      sheetPDFStage = "menu"
	sheetPDFStageDialog    sheetPDFStage = "dialog"
	sheetPDFStageDownload  sheetPDFStage = "download"
)

type sheetPDFExportRun struct {
	browser   *phase3EditorBrowser
	download  *sheetPDFDownload
	tempDir   string
	sheetName string
	pages     string
	settings  map[string]any
	evidence  string
}

func (a *App) cmdSheetExportPDF() (runErr error) {
	if len(a.Pos) != 1 {
		return errors.New("用法：shimo-net editor <表格链接> --action sheet-export-pdf [--payload '{\"sheetName\":\"工作表1\"}'] [--out 证据目录] [--json]")
	}
	if explicitNoBrowser(a) {
		return &commandError{Message: "表格 PDF 导出通过页面菜单生成；--no-browser 已禁止启动浏览器，本次未导出", Result: map[string]any{"code": "BROWSER_REQUIRED", "action": "sheet-export-pdf", "mutationAttempted": false}, ExitCode: 3}
	}
	payload := map[string]any{}
	if raw := strings.TrimSpace(a.opt("--payload")); raw != "" {
		args, err := phase2Args(raw)
		if err != nil {
			return err
		}
		payload, err = phase2ObjectArg(args)
		if err != nil {
			return err
		}
	}
	sheetName := strings.TrimSpace(str(payload, "sheetName"))
	for key := range payload {
		if key != "sheetName" {
			return fmt.Errorf("sheet-export-pdf 不支持参数 %s；只接受 sheetName（缺省为当前工作表）", key)
		}
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	if !sameCredentialOrigin(target, a.Origin) {
		return errors.New("目标与配置 origin 不一致；本次未导出")
	}
	guid := guidFromURL(target)
	if guid == "" {
		return errors.New("表格 PDF 导出需要包含文件 guid 的链接")
	}
	info, err := a.fileInfo(guid)
	if err != nil {
		return err
	}
	if str(info, "type") != "spreadsheet" || boolv(info, "isCloudFile") {
		return errors.New("表格 PDF 导出只接受在线电子表格（应用表格与上传件不支持）")
	}
	dir, err := a.phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	run := &sheetPDFExportRun{sheetName: sheetName, evidence: dir}
	stage := sheetPDFStagePreflight
	defer func() {
		if runErr != nil && run.browser != nil {
			// Preserve the page where the menu/wizard failed instead of
			// leaving only the last successful screenshot for diagnosis.
			_ = run.browser.Capture(filepath.Join(dir, "failure.png"))
		}
		if run.browser != nil {
			run.browser.Close()
		}
		if run.tempDir != "" {
			_ = os.RemoveAll(run.tempDir)
		}
		if runErr != nil {
			_ = phase3WriteEvidence(filepath.Join(dir, "failure.json"), map[string]any{
				"ok": false, "action": "sheet-export-pdf", "stage": string(stage),
				"mutationAttempted": false, "mayHaveApplied": false, "noReplay": false,
				"error": runErr.Error(),
				"note":  "PDF 导出是只读操作：不修改文档内容；重新执行不会产生重复数据，但会重新生成一份 PDF",
			})
		}
	}()
	if err = phase3WriteEvidence(filepath.Join(dir, "request.json"), map[string]any{"action": "sheet-export-pdf", "target": target, "sheetName": sheetName, "transport": "browser-ui"}); err != nil {
		return err
	}

	stage = sheetPDFStageMenu
	b, err := openPhase3EditorWhen(target, sheetBrowserReadyJS)
	if err != nil {
		return err
	}
	run.browser = b
	if err = b.uiWaitFound("更多菜单按钮", `[data-testid="header-more-btn"]`, 60*time.Second); err != nil {
		return err
	}
	if err = b.uiSwitchSheet(sheetName); err != nil {
		return err
	}
	if err = b.Capture(filepath.Join(dir, "before.png")); err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp("", "shimo-sheet-pdf-")
	if err != nil {
		return err
	}
	run.tempDir = tempDir
	run.download = &sheetPDFDownload{}
	previous := b.ws.onEvent
	b.ws.onEvent = func(event map[string]any) {
		if previous != nil {
			previous(event)
		}
		run.download.handle(event)
	}
	b.nextID++
	if _, err = b.ws.Call(b.nextID, "Browser.setDownloadBehavior", map[string]any{"behavior": "allow", "downloadPath": tempDir, "eventsEnabled": true}, 10*time.Second); err != nil {
		return fmt.Errorf("无法把浏览器下载限制到临时目录：%w", err)
	}
	if err = b.uiClickBox("更多菜单按钮", `[data-testid="header-more-btn"]`, ""); err != nil {
		return err
	}
	if err = b.uiWaitFound("下载菜单项", `[data-testid="menu-item-download"]`, 20*time.Second); err != nil {
		return err
	}
	if err = b.uiHoverRow(`[data-testid="menu-item-download"]`); err != nil {
		return fmt.Errorf("展开「下载」子菜单失败：%w", err)
	}
	if err = b.waitUIText([]string{"纯图PDF"}, nil, 20*time.Second); err != nil {
		return fmt.Errorf("「下载」子菜单没有出现 PDF 选项：%w", err)
	}
	if err = b.Capture(filepath.Join(dir, "menu.png")); err != nil {
		return err
	}
	if err = b.uiClickBox("纯图PDF", "", "纯图PDF"); err != nil {
		return err
	}
	stage = sheetPDFStageDialog
	// The wizard renders pages before the confirm button becomes usable.
	if err = b.waitUIText([]string{"导出设置", "共"}, []string{"正在准备导出"}, 180*time.Second); err != nil {
		return fmt.Errorf("导出设置对话框没有完成页面渲染：%w", err)
	}
	run.pages = sheetPDFPagesLabel(b.uiBodyText())
	if err = b.Capture(filepath.Join(dir, "dialog.png")); err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "dialog.json"), map[string]any{"pages": run.pages, "body": truncateRunes(b.uiBodyText(), 2000)}); err != nil {
		return err
	}
	if err = b.uiClickBox("下一步", "", "下一步"); err != nil {
		return err
	}
	stage = sheetPDFStageDownload
	downloaded, err := waitSheetPDFDownload(run.download, tempDir, 180*time.Second)
	if err != nil {
		return err
	}
	size, sha, err := verifySheetPDF(downloaded)
	if err != nil {
		return fmt.Errorf("下载的文件不是完整 PDF（%s）：%w", filepath.Base(downloaded), err)
	}
	published, err := publishSheetPDF(downloaded, dir, filepath.Base(downloaded))
	if err != nil {
		return err
	}
	if err = b.Capture(filepath.Join(dir, "after.png")); err != nil {
		return err
	}
	result := map[string]any{
		"ok": true, "action": "sheet-export-pdf", "target": target, "transport": "browser-ui",
		"file": map[string]any{"path": published, "name": filepath.Base(published), "bytes": size, "sha256": sha, "type": "pdf"},
		"pdf":  published, "bytes": size, "sha256": sha,
		"pages": run.pages, "sheetName": run.sheetName,
		"exportMode": "menu-share-download-pdf-image",
		"settings":   map[string]any{"source": "current-page-settings", "modified": false, "snapshot": filepath.Join(dir, "dialog.json"), "note": "未修改导出设置；页面可能保留此前设置，纸张、范围和网格线以对话框物证为准，不假定固定默认值"},
		"verified":   true,
		"verification": map[string]any{
			"level": "artifact", "scope": "PDF 文件信封、大小与下载路径已核验；页数、文本与渲染需独立解析，不在本命令断言范围",
			"serverAPI": false, "serverAPIReason": "服务端 /files/export 对电子表格返回 3613「暂不支持该导出类型」，PDF 只能由页面生成",
		},
		"evidenceDir": dir,
		"view":        a.agentView(viewTransportDownload, published),
		"auth":        a.agentAuth(),
		"screenshots": map[string]string{"before": filepath.Join(dir, "before.png"), "menu": filepath.Join(dir, "menu.png"), "dialog": filepath.Join(dir, "dialog.png"), "after": filepath.Join(dir, "after.png")},
		"browser":     "CLI 隔离配置（已注入 CLI 登录态）",
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	if a.JSON {
		return a.printJSON(result)
	}
	fmt.Printf("PDF 已导出：%s（%d 字节，%s）\n证据：%s\n", terminalText(published), size, run.pages, terminalText(dir))
	return nil
}

// waitSheetPDFDownload accepts a file only when it is a complete PDF on disk and
// its size stopped changing. Download events are used when the browser session
// reports them, but they are not required: a page-level CDP session often does
// not receive Browser.downloadProgress at all.
func waitSheetPDFDownload(d *sheetPDFDownload, dir string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	// Stability is tracked per path: several files can land in the directory
	// (an HTML error page plus the PDF), and a single "last seen" slot would
	// reset on every directory walk.
	type sample struct {
		size int64
		hits int
	}
	seen := map[string]sample{}
	for {
		if d != nil && d.err != nil {
			return "", fmt.Errorf("浏览器下载失败：%w", d.err)
		}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() || strings.HasSuffix(e.Name(), ".crdownload") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			info, err := os.Stat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 128 {
				continue
			}
			prior, known := seen[path]
			if known && prior.size == info.Size() {
				prior.hits++
			} else {
				prior = sample{size: info.Size()}
			}
			seen[path] = prior
			if prior.hits < 1 {
				continue
			}
			if _, _, err := verifySheetPDF(path); err == nil {
				return path, nil
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("等待浏览器下载 PDF 超时（%s）：目录中没有完整 PDF（事件：started=%v completed=%v）", timeout, d != nil && d.guid != "", d != nil && d.completed)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func sheetPDFPagesLabel(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "共") && strings.Contains(line, "页") {
			return line
		}
	}
	return ""
}
