package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
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
	if err := mirrorSession(origin, "sid=abc; fp=1", "login", map[string]any{"id": "u1", "name": "张三"}, sessionRevision(origin)); err != nil {
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
		Cookies: parseCookieHeader("sid=a")}, -1, true)
	if err != nil || rev != 1 {
		t.Fatalf("首次写入 revision=%d err=%v", rev, err)
	}
	if _, err := commitSession(origin, sessionRecord{Origin: origin, AuthState: sessionSignedIn,
		Cookies: parseCookieHeader("sid=b")}, rev, true); err != nil {
		t.Fatalf("正常提交失败：%v", err)
	}
	if _, err := commitSession(origin, sessionRecord{Origin: origin, AuthState: sessionSignedIn,
		Cookies: parseCookieHeader("sid=c")}, rev, true); err != errSessionRevision {
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
	if err := mirrorSession(origin, "sid=old; fp=1", "login", nil, sessionRevision(origin)); err != nil {
		t.Fatal(err)
	}
	requestSession, _ := readSessionRecord(origin)
	mergeSetCookies(origin, requestSession, []*http.Cookie{
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
	if err := mirrorSession(origin, "sid=old; fp=1", "login", nil, sessionRevision(origin)); err != nil {
		t.Fatal(err)
	}
	requestSession, _ := readSessionRecord(origin)
	mergeSetCookies(origin, requestSession, []*http.Cookie{
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
	if err := mirrorSession(origin, "sid=secret-value; fp=1", "import", nil, sessionRevision(origin)); err != nil {
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
	if err := mirrorSession("https://shimo.net", "sid=a", "login", nil, sessionRevision("https://shimo.net")); err != nil {
		t.Fatal(err)
	}
	if err := mirrorSession("https://docs.example.com", "sid=b", "login", nil, sessionRevision("https://docs.example.com")); err != nil {
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
	origin := "https://shimo.net"

	cookies, account, err := readSessionPayload(bufioReader(`{"cookie":"sid=x","account":{"id":"9"}}`))
	if err != nil || str(account, "id") != "9" {
		t.Fatalf("JSON cookie 字段解析失败：%v %v", account, err)
	}
	if sessionCookieHeader(&sessionRecord{Origin: origin, Cookies: cookies}) != "sid=x" || cookies[0].Path != "/" {
		t.Fatalf("字符串形式应可用并补默认 Path：%+v", cookies)
	}

	cookies, account, err = readSessionPayload(bufioReader("sid=y; fp=2"))
	if err != nil || account != nil || len(cookies) != 2 {
		t.Fatalf("原始 cookie 头解析失败：%+v %v", cookies, err)
	}

	// 带属性的对象数组：Domain/Path/属性/有效期都要原样保留。
	cookies, account, err = readSessionPayload(bufioReader(`{"cookies":[{"name":"sid","value":"z","domain":"shimo.net","path":"/d","secure":true,"httpOnly":true,"sameSite":"Lax","expires":1893456000}],"account":{"email":"a@b.c"}}`))
	if err != nil || len(cookies) != 1 {
		t.Fatalf("cookies 数组解析失败：%+v %v", cookies, err)
	}
	c := cookies[0]
	if c.Domain != "shimo.net" || c.Path != "/d" || !c.Secure || !c.HttpOnly || c.SameSite != "Lax" || c.Expires != 1893456000 {
		t.Fatalf("cookie 属性在导入时丢失：%+v", c)
	}
	if str(account, "email") != "a@b.c" {
		t.Fatalf("account 未解析：%v", account)
	}

	if _, _, err := readSessionPayload(bufioReader(`{"cookies":[{"name":"","value":""}]}`)); err != errSessionNoCookie {
		t.Fatalf("空 cookie 数组应报无凭据，得到 %v", err)
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
	if err := mirrorSession(origin, "sid=aaa; fp=1", "login", nil, sessionRevision(origin)); err != nil {
		t.Fatalf("首次写入失败：%v", err)
	}
	m1, s1, ok := sessionFileStamp(origin)
	if !ok || s1 == 0 {
		t.Fatalf("写入后应能取到 stamp，得到 size=%d ok=%v", s1, ok)
	}

	// 同样的 cookie 长度、只改值，文件大小不变，只能靠 mtime 发现变化。
	time.Sleep(2 * time.Millisecond)
	if err := mirrorSession(origin, "sid=bbb; fp=1", "login", nil, sessionRevision(origin)); err != nil {
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
	if err := mirrorSession(origin, "sid=abc; fp=1; lang=zh", "login", nil, sessionRevision(origin)); err != nil {
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

// importAfterLogout 用真实的 cmdSessionImport 跑一遍，stdin 走管道。
func importAfterLogout(t *testing.T, origin, payload string, extra ...string) error {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("创建管道失败：%v", err)
	}
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = old
		_ = r.Close()
	})
	go func() {
		_, _ = w.WriteString(payload)
		_ = w.Close()
	}()
	args := append([]string{"session", "import", "--no-verify"}, extra...)
	a := newApp(args)
	a.Origin = origin
	return a.cmdSessionImport()
}

func TestSessionImportRefusesToReviveSignedOut(t *testing.T) {
	sessionTestState(t)
	origin := "https://shimo.net"
	if err := mirrorSession(origin, "sid=a; fp=1", "login", nil, sessionRevision(origin)); err != nil {
		t.Fatalf("首次写入失败：%v", err)
	}
	if err := markSessionSignedOut(origin, "logout"); err != nil {
		t.Fatalf("标记退出失败：%v", err)
	}
	rec, _ := readSessionRecord(origin)

	// 不带 revision：旧会话不得把退出改回 signedIn。
	err := importAfterLogout(t, origin, `sid=old; fp=1`)
	if err == nil || !strings.Contains(err.Error(), "共享会话已退出") {
		t.Fatalf("退出后不带 revision 的导入应被拒绝，得到 %v", err)
	}
	if cur, _ := readSessionRecord(origin); cur.AuthState != sessionSignedOut {
		t.Fatalf("拒绝后状态应保持 signedOut，得到 %s", cur.AuthState)
	}

	// 带上刚读到的 revision：视为一次明确的新登录，允许覆盖。
	if err := importAfterLogout(t, origin, `sid=new; fp=1`, "--expected-revision", strconv.FormatInt(rec.Revision, 10)); err != nil {
		t.Fatalf("明确带 revision 的新登录应允许：%v", err)
	}
	if cur, _ := readSessionRecord(origin); cur.AuthState != sessionSignedIn || !strings.Contains(sessionCookieHeader(cur), "sid=new") {
		t.Fatalf("应恢复 signedIn 并使用新 cookie，得到 %s %s", cur.AuthState, sessionCookieHeader(cur))
	}
}

func TestLogoutReportsSharedWriteFailure(t *testing.T) {
	sessionTestState(t)
	origin := "https://shimo.net"
	if err := mirrorSession(origin, "sid=a; fp=1", "login", nil, sessionRevision(origin)); err != nil {
		t.Fatalf("首次写入失败：%v", err)
	}
	// 只读目录在 Windows 上拦不住写入（os.Chmod 只切只读属性，不实现 POSIX 权限位），
	// 因此用一个同名目录占住记录文件的位置：rename 到已存在目录在两个平台都会失败。
	// 原记录先挪到旁边，测完再放回去，断言才能检查「写入失败时会话仍是 signedIn」。
	record, err := sessionStorePath(origin)
	if err != nil {
		t.Fatalf("解析会话路径失败：%v", err)
	}
	backup := record + ".bak"
	if err = os.Rename(record, backup); err != nil {
		t.Fatalf("把记录移开失败：%v", err)
	}
	if err = os.Mkdir(record, 0o700); err != nil {
		t.Fatalf("占位目录创建失败：%v", err)
	}
	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		_ = os.RemoveAll(record)
		_ = os.Rename(backup, record)
	}
	t.Cleanup(restore)

	a := newApp([]string{"logout"})
	a.Origin = origin
	err = a.cmdLogout()
	if err == nil || !strings.Contains(err.Error(), "共享退出未写入") {
		t.Fatalf("共享写入失败必须上报，得到 %v", err)
	}
	// 占位期间记录路径不可读，先恢复再把内容读回来核对。
	restore()
	if rec, _ := readSessionRecord(origin); rec == nil || rec.AuthState != sessionSignedIn {
		t.Fatal("测试前提：写入失败时会话应仍为 signedIn")
	}
}

// 共享会话的 cookie 必须按请求地址投递：Secure 只发 https，Path 只发命中前缀。
func TestSessionCookieScopeFollowsRequestURL(t *testing.T) {
	sessionTestState(t)
	rec := &sessionRecord{Origin: "http://shimo.internal:8080", Cookies: []sessionCookie{
		{Name: "sid", Value: "s", Path: "/"},
		{Name: "secureOnly", Value: "x", Path: "/", Secure: true},
		{Name: "admin", Value: "a", Path: "/admin"},
		{Name: "other", Value: "o", Domain: "example.com", Path: "/"},
	}}

	plain := sessionCookieHeaderFor(rec, parseURLOrNil("http://shimo.internal:8080/api/v1/users/me"))
	if strings.Contains(plain, "secureOnly") {
		t.Fatalf("Secure cookie 不应发给 http：%q", plain)
	}
	if strings.Contains(plain, "admin=") {
		t.Fatalf("Path=/admin 的 cookie 不应发给 /api/v1：%q", plain)
	}
	if strings.Contains(plain, "other=") {
		t.Fatalf("域名不匹配的 cookie 不应发送：%q", plain)
	}
	if !strings.Contains(plain, "sid=s") {
		t.Fatalf("同源同路径的 cookie 应发送：%q", plain)
	}

	tls := sessionCookieHeaderFor(rec, parseURLOrNil("https://shimo.internal:8080/api/v1/users/me"))
	if !strings.Contains(tls, "secureOnly=x") {
		t.Fatalf("https 下应带上 Secure cookie：%q", tls)
	}

	admin := sessionCookieHeaderFor(rec, parseURLOrNil("https://shimo.internal:8080/admin/users"))
	if !strings.Contains(admin, "admin=a") {
		t.Fatalf("命中 Path 前缀时应发送：%q", admin)
	}
	api := sessionCookieHeaderFor(rec, parseURLOrNil("https://shimo.internal:8080/administrator"))
	if strings.Contains(api, "admin=a") {
		t.Fatalf("Path=/admin 不应命中 /administrator：%q", api)
	}

	// sid 本身不适用时不能返回半套 cookie，更不能回退旧凭据。
	narrow := &sessionRecord{Origin: "https://shimo.net", Cookies: []sessionCookie{
		{Name: "sid", Value: "s", Path: "/app"},
	}}
	if got := sessionCookieHeaderFor(narrow, parseURLOrNil("https://shimo.net/api/v1/users/me")); got != "" {
		t.Fatalf("sid 的 Path 不匹配时应返回空头，得到 %q", got)
	}
}

func TestSessionNotApplicableDoesNotFallBackToKeychain(t *testing.T) {
	sessionTestState(t)
	origin := "https://shimo.net"
	// 模拟本机另有一份旧凭据，共享会话不应在属性不适用时回退到它。
	if err := writeCred(origin, "sid=stale; fp=old"); err != nil {
		t.Fatalf("写入本地凭据失败：%v", err)
	}
	if _, err := commitSession(origin, sessionRecord{Origin: origin, AuthState: sessionSignedIn, Cookies: []sessionCookie{
		{Name: "sid", Value: "fresh", Path: "/app"}, {Name: "fp", Value: "new", Path: "/app"},
	}}, -1, true); err != nil {
		t.Fatalf("写入共享会话失败：%v", err)
	}
	_, source, err := credentialForOriginURL(origin, parseURLOrNil("https://shimo.net/api/v1/users/me"))
	if err == nil || !strings.Contains(err.Error(), "适用于该请求") {
		t.Fatalf("应明确报属性不适用而不是回退，得到 source=%s err=%v", source, err)
	}
	if _, err := commitSession(origin, sessionRecord{Origin: origin, AuthState: sessionSignedIn, Cookies: []sessionCookie{
		{Name: "sid", Value: "fresh"}, {Name: "fp", Value: "new"},
	}}, sessionRevision(origin), true); err != nil {
		t.Fatalf("改回根路径失败：%v", err)
	}
	if cookie, source, err := credentialForOriginURL(origin, parseURLOrNil("https://shimo.net/api/v1/users/me")); err != nil || source != "session" || !strings.Contains(cookie, "sid=fresh") {
		t.Fatalf("根路径 cookie 应可用，得到 source=%s cookie=%q err=%v", source, cookie, err)
	}
}

// 已有 signedIn 记录时，不带 revision 只能刷新同一 sid，不能切号。
func TestSessionImportRequiresRevisionToSwitchAccount(t *testing.T) {
	sessionTestState(t)
	origin := "https://shimo.net"
	if err := mirrorSession(origin, "sid=aaa; fp=1", "login", nil, 0); err != nil {
		t.Fatalf("首次写入失败：%v", err)
	}

	// 同一 sid：视为属性刷新，允许且自动按当前 revision 提交。
	if err := importAfterLogout(t, origin, "sid=aaa; fp=2"); err != nil {
		t.Fatalf("同一 sid 的刷新应允许：%v", err)
	}
	if rec, _ := readSessionRecord(origin); cookieValue(rec.Cookies, "fp") != "2" {
		t.Fatalf("刷新应更新属性，得到 %v", rec.Cookies)
	}

	// 不同 sid：切号，必须带 revision。
	err := importAfterLogout(t, origin, "sid=bbb; fp=1")
	if err == nil || !strings.Contains(err.Error(), "--expected-revision") {
		t.Fatalf("未带 revision 的切号应被拒绝，得到 %v", err)
	}
	if rec, _ := readSessionRecord(origin); cookieValue(rec.Cookies, "sid") != "aaa" {
		t.Fatalf("被拒绝后不应改动会话，得到 %s", cookieValue(rec.Cookies, "sid"))
	}

	rev := sessionRevision(origin)
	if err := importAfterLogout(t, origin, "sid=bbb; fp=1", "--expected-revision", strconv.FormatInt(rev, 10)); err != nil {
		t.Fatalf("带 revision 的切号应允许：%v", err)
	}
	if rec, _ := readSessionRecord(origin); cookieValue(rec.Cookies, "sid") != "bbb" {
		t.Fatalf("切号未生效：%s", cookieValue(rec.Cookies, "sid"))
	}
}

// 登录期间别人退出，登录成功后的镜像不能用旧前提覆盖退出状态。
func TestLoginMirrorDoesNotResurrectConcurrentLogout(t *testing.T) {
	sessionTestState(t)
	origin := "https://shimo.net"
	if err := mirrorSession(origin, "sid=aaa", "login", nil, 0); err != nil {
		t.Fatalf("首次写入失败：%v", err)
	}
	baseRev := sessionRevision(origin) // 登录开始
	if err := markSessionSignedOut(origin, "logout"); err != nil {
		t.Fatalf("并发退出失败：%v", err)
	}
	err := mirrorSession(origin, "sid=aaa; fp=2", "login", nil, baseRev)
	if !errors.Is(err, errSessionRevision) {
		t.Fatalf("过期前提的镜像应冲突，得到 %v", err)
	}
	if rec, _ := readSessionRecord(origin); rec.AuthState != sessionSignedOut {
		t.Fatalf("退出状态不应被登录镜像覆盖，得到 %s", rec.AuthState)
	}

	// 退出后用户重新登录：前提是退出后的 revision，允许恢复。
	if err := mirrorSession(origin, "sid=aaa; fp=3", "login", nil, sessionRevision(origin)); err != nil {
		t.Fatalf("退出后的新登录应允许：%v", err)
	}
	if rec, _ := readSessionRecord(origin); rec.AuthState != sessionSignedIn {
		t.Fatalf("重新登录后应为 signedIn，得到 %s", rec.AuthState)
	}
}
