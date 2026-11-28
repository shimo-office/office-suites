package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A complete PDF is accepted even when the page-level CDP session never sends
// Browser.downloadProgress; an HTML error page in the same directory is not.
func TestWaitSheetPDFDownloadIgnoresNonPDF(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "downloads.html"), []byte("<html>"+strings.Repeat("x", 400)+"</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		body := "%PDF-1.7\n" + strings.Repeat("% padding\n", 12) + "%%EOF\n"
		_ = os.WriteFile(filepath.Join(dir, "表格 - 石墨办公.pdf"), []byte(body), 0600)
	}()
	got, err := waitSheetPDFDownload(&sheetPDFDownload{}, dir, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(got) != ".pdf" {
		t.Fatalf("accepted a non-PDF download: %s", got)
	}
	if _, _, err := verifySheetPDF(got); err != nil {
		t.Fatalf("returned file is not a complete PDF: %v", err)
	}
}

func TestWaitSheetPDFDownloadReportsCancel(t *testing.T) {
	d := &sheetPDFDownload{}
	d.handle(map[string]any{"method": "Browser.downloadWillBegin", "params": map[string]any{"guid": "12345678-abcd", "suggestedFilename": "x.pdf"}})
	d.handle(map[string]any{"method": "Browser.downloadProgress", "params": map[string]any{"guid": "12345678-abcd", "state": "canceled"}})
	if _, err := waitSheetPDFDownload(d, t.TempDir(), time.Second); err == nil || !strings.Contains(err.Error(), "取消") {
		t.Fatalf("cancel not reported: %v", err)
	}
}

func TestSheetPDFPagesLabel(t *testing.T) {
	body := "内容将自动保存\n导出设置\n共 5 页\n取消\n下一步"
	if got := sheetPDFPagesLabel(body); got != "共 5 页" {
		t.Fatalf("pages label = %q", got)
	}
	if got := sheetPDFPagesLabel("导出设置"); got != "" {
		t.Fatalf("unexpected label %q", got)
	}
}

func TestAgentViewSeparatesHostBrowserFromCLISession(t *testing.T) {
	a := newApp([]string{"preview", "x"})
	cli := a.agentView(viewTransportCLIBrowser, "https://shimo.net/files/abc")
	host, _ := cli["builtInBrowser"].(map[string]any)
	if host == nil || host["usable"] != nil || host["sessionShared"] != nil || host["authState"] != "unknown" {
		t.Fatalf("CLI cannot infer the host browser's session: %#v", cli)
	}
	if command := str(cli, "openWith"); !strings.Contains(command, "shimo-net open") || strings.Contains(command, "--verify") || cli["openSessionShared"] != false {
		t.Fatalf("open hint must match the real command and session behavior: %#v", cli)
	}
	if cli["usesCLISession"] != true {
		t.Fatal("cli transport must declare it uses the CLI session")
	}
	download := a.agentView(viewTransportDownload, "/tmp/report.pdf")
	host, _ = download["builtInBrowser"].(map[string]any)
	if host == nil || host["usable"] != true {
		t.Fatalf("artifacts are host-visible: %#v", download)
	}
	if _, exists := download["openWith"]; exists {
		t.Fatal("a local artifact must not be sent to shimo-net open")
	}
}

func TestAgentAuthDoesNotTreatCredentialsAsVerifiedIdentity(t *testing.T) {
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic-unverified")
	a := newApp([]string{"whoami"})
	auth := a.agentAuth()
	if auth["loginRequired"] != nil || auth["verified"] != false || auth["cliSession"] != true {
		t.Fatalf("credential presence must not claim successful authentication: %#v", auth)
	}
	for _, key := range []string{"cliSession", "credentialSource", "sharedSession", "authState"} {
		if _, ok := auth[key]; !ok {
			t.Fatalf("auth contract missing %s: %#v", key, auth)
		}
	}
}

func TestAgentViewForLinkDoesNotClaimBrowserWasOpened(t *testing.T) {
	a := newApp([]string{"preview", "x"})
	view := a.agentView("link", "https://shimo.net/files/abc")
	if view["usesCLISession"] != false || view["browserOpened"] != false {
		t.Fatalf("preview only returns a link: %#v", view)
	}
	host := asObj(view["builtInBrowser"])
	if host["authState"] != "unknown" || host["usable"] != nil {
		t.Fatalf("a link cannot prove the host is logged in: %#v", view)
	}
}
