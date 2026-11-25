package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func sessionTestState(t *testing.T) {
	t.Helper()
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	t.Setenv("SHIMO_NET_SESSION_DIR", "")
	t.Setenv("SHIMO_NET_COOKIE", "")
	t.Setenv("SHIMO_NET_ORIGIN", "")
}

func bufioReader(s string) *bufio.Reader { return bufio.NewReader(strings.NewReader(s)) }

func TestSharedSessionPrecedenceAndSignedOut(t *testing.T) {
	sessionTestState(t)
	origin := "https://shimo.net"

	if _, _, err := credentialForOrigin(origin); err == nil || !strings.Contains(err.Error(), "未登录") {
		t.Fatalf("空状态应报未登录，得到 %v", err)
	}
	if err := mirrorSession(origin, "sid=abc; fp=1", "login", map[string]any{"id": "u1", "name": "张三"}); err != nil {
		t.Fatalf("写入共享会话失败：%v", err)
	}
	cookie, source, err := credentialForOrigin(origin)
	if err != nil || source != "session" || !strings.Contains(cookie, "sid=abc") {
		t.Fatalf("应优先使用共享会话，得到 source=%s cookie=%q err=%v", source, cookie, err)
	}
	if err := markSessionSignedOut(origin, "logout"); err != nil {
		t.Fatalf("标记退出失败：%v", err)
	}
	if _, _, err := credentialForOrigin(origin); err != errSharedSignedOut {
		t.Fatalf("退出后应拒绝回退本地凭据，得到 %v", err)
	}
}

func TestSessionRevisionConflict(t *testing.T) {
	sessionTestState(t)
	origin := "https://shimo.net"
	rev, err := commitSession(origin, sessionRecord{Origin: origin, AuthState: sessionSignedIn,
		Cookies: parseCookieHeader("sid=a")}, -1)
	if err != nil || rev != 1 {
		t.Fatalf("首次写入 revision=%d err=%v", rev, err)
	}
	if _, err := commitSession(origin, sessionRecord{Origin: origin, AuthState: sessionSignedIn,
		Cookies: parseCookieHeader("sid=b")}, rev); err != nil {
		t.Fatalf("正常提交失败：%v", err)
	}
	if _, err := commitSession(origin, sessionRecord{Origin: origin, AuthState: sessionSignedIn,
		Cookies: parseCookieHeader("sid=c")}, rev); err != errSessionRevision {
		t.Fatalf("过期 revision 应被拒绝，得到 %v", err)
	}
	rec, _ := readSessionRecord(origin)
	if rec.Revision != 2 || sessionCookieHeader(rec) != "sid=b" {
		t.Fatalf("会话被过期写入覆盖：rev=%d cookie=%s", rec.Revision, sessionCookieHeader(rec))
	}
}

func TestSetCookieRenewalMergesIntoSharedSession(t *testing.T) {
	sessionTestState(t)
	origin := "https://shimo.net"
	if err := mirrorSession(origin, "sid=old; fp=1", "login", nil); err != nil {
		t.Fatal(err)
	}
	mergeSetCookies(origin, []*http.Cookie{
		{Name: "sid", Value: "new", Path: "/", Domain: "shimo.net", Secure: true, HttpOnly: true},
		{Name: "deviceId", Value: "d2", Path: "/", Domain: "shimo.net"},
	})
	rec, _ := readSessionRecord(origin)
	header := sessionCookieHeader(rec)
	if !strings.Contains(header, "sid=new") || !strings.Contains(header, "deviceId=d2") || strings.Contains(header, "sid=old") {
		t.Fatalf("续期未合并：%s", header)
	}
	if rec.Source != "renew" || rec.Revision != 2 {
		t.Fatalf("续期应产生新版本：source=%s rev=%d", rec.Source, rec.Revision)
	}
}

func TestMergeSetCookiesIgnoresForeignDomainAndDeletion(t *testing.T) {
	sessionTestState(t)
	origin := "https://shimo.net"
	if err := mirrorSession(origin, "sid=old; fp=1", "login", nil); err != nil {
		t.Fatal(err)
	}
	mergeSetCookies(origin, []*http.Cookie{
		{Name: "sid", Value: "evil", Domain: "example.com", Path: "/"},
		{Name: "fp", Value: "", MaxAge: -1, Domain: "shimo.net", Path: "/"},
	})
	rec, _ := readSessionRecord(origin)
	header := sessionCookieHeader(rec)
	if !strings.Contains(header, "sid=old") || strings.Contains(header, "fp=") {
		t.Fatalf("跨域 cookie 应忽略、删除应生效：%s", header)
	}
}

func TestSessionStatusHidesCookieValues(t *testing.T) {
	sessionTestState(t)
	origin := "https://shimo.net"
	if err := mirrorSession(origin, "sid=secret-value; fp=1", "import", nil); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(sessionStatusMap(origin))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "secret-value") {
		t.Fatalf("状态输出泄露了 cookie 值：%s", b)
	}
	if !strings.Contains(string(b), "sid") {
		t.Fatalf("状态输出应包含 cookie 名称：%s", b)
	}
}

func TestSessionOriginIsolation(t *testing.T) {
	sessionTestState(t)
	if err := mirrorSession("https://shimo.net", "sid=a", "login", nil); err != nil {
		t.Fatal(err)
	}
	if err := mirrorSession("https://docs.example.com", "sid=b", "login", nil); err != nil {
		t.Fatal(err)
	}
	a, _ := sharedSessionCookie("https://shimo.net")
	b, _ := sharedSessionCookie("https://docs.example.com")
	c, _ := sharedSessionCookie("https://other.example.com")
	if !strings.Contains(a, "sid=a") || !strings.Contains(b, "sid=b") || c != "" {
		t.Fatalf("环境未隔离：%q %q %q", a, b, c)
	}
}

func TestSessionCookieExpiryAndHostMatching(t *testing.T) {
	sessionTestState(t)
	origin := "https://shimo.net"
	rec := &sessionRecord{Origin: origin, AuthState: sessionSignedIn, Cookies: []sessionCookie{
		{Name: "sid", Value: "a", Domain: "shimo.net", Path: "/"},
		{Name: "dead", Value: "x", Domain: "shimo.net", Path: "/", Expires: time.Now().Add(-time.Hour).Unix()},
		{Name: "foreign", Value: "y", Domain: "example.com", Path: "/"},
	}}
	header := sessionCookieHeader(rec)
	if !strings.Contains(header, "sid=a") || strings.Contains(header, "dead=") || strings.Contains(header, "foreign=") {
		t.Fatalf("过期或跨域 cookie 不应发出：%s", header)
	}
}

func TestSessionImportPayloadShapes(t *testing.T) {
	header, account, err := readSessionPayload(bufioReader(`{"cookie":"sid=x","account":{"id":"9"}}`))
	if err != nil || header != "sid=x" || str(account, "id") != "9" {
		t.Fatalf("JSON 形式解析失败：%q %v %v", header, account, err)
	}
	header, account, err = readSessionPayload(bufioReader("sid=y; fp=2"))
	if err != nil || account != nil || !strings.Contains(header, "sid=y") {
		t.Fatalf("原始 cookie 头解析失败：%q %v", header, err)
	}
	if _, _, err := readSessionPayload(bufioReader("   ")); err == nil {
		t.Fatal("空输入应报错")
	}
}

func TestSessionFileStampDetectsSameSizeRewrite(t *testing.T) {
	sessionTestState(t)
	origin := "https://shimo.net"

	if _, _, ok := sessionFileStamp(origin); ok {
		t.Fatal("没有会话文件时不应命中 stamp")
	}
	if err := mirrorSession(origin, "sid=aaa; fp=1", "login", nil); err != nil {
		t.Fatalf("首次写入失败：%v", err)
	}
	m1, s1, ok := sessionFileStamp(origin)
	if !ok || s1 == 0 {
		t.Fatalf("写入后应能取到 stamp，得到 size=%d ok=%v", s1, ok)
	}

	// 同样的 cookie 长度、只改值，文件大小不变，只能靠 mtime 发现变化。
	time.Sleep(2 * time.Millisecond)
	if err := mirrorSession(origin, "sid=bbb; fp=1", "login", nil); err != nil {
		t.Fatalf("二次写入失败：%v", err)
	}
	m2, s2, ok := sessionFileStamp(origin)
	if !ok {
		t.Fatal("二次写入后不应丢失 stamp")
	}
	if s1 != s2 {
		t.Fatalf("测试前提不成立：两次写入大小应相同，得到 %d 与 %d", s1, s2)
	}
	if m2 <= m1 {
		t.Fatalf("同大小重写应改变 mtime，得到 %d -> %d", m1, m2)
	}

	// 记录被删掉后要回到未命中，watch 才能感知到状态消失。
	if err := os.Remove(sessionStorePathOrEmpty(origin)); err != nil {
		t.Fatalf("删除会话文件失败：%v", err)
	}
	if _, _, ok := sessionFileStamp(origin); ok {
		t.Fatal("文件删除后不应命中 stamp")
	}
}

func benchSessionState(b *testing.B) string {
	b.Helper()
	b.Setenv("SHIMO_NET_STATE_DIR", b.TempDir())
	b.Setenv("SHIMO_NET_SESSION_DIR", "")
	origin := "https://shimo.net"
	if err := mirrorSession(origin, "sid=abc; fp=1; lang=zh", "login", nil); err != nil {
		b.Fatalf("准备会话失败：%v", err)
	}
	return origin
}

func BenchmarkSessionFileStampHit(b *testing.B) {
	origin := benchSessionState(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, ok := sessionFileStamp(origin); !ok {
			b.Fatal("应命中会话文件")
		}
	}
}

func BenchmarkReadSessionRecord(b *testing.B) {
	origin := benchSessionState(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := readSessionRecord(origin); !ok {
			b.Fatal("应读到会话记录")
		}
	}
}
