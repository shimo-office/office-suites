package main

// Shared session store
//
// 同一系统用户下的所有 CLI（无论装在哪个 Agent 里）读写同一份会话记录，用 revision
// 做乐观并发。石墨办公通过 `session import` 提交自己的会话，通过读取本文件或
// `session watch` 感知变化。Cookie 只落盘、不回显：除 whoami 等网络请求外，任何命令
// 都不会把 cookie 值打进 stdout/stderr。

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const sessionSchemaVersion = 1

const (
	sessionSignedIn  = "signedIn"
	sessionSignedOut = "signedOut"
)

var (
	errSharedSignedOut   = errors.New("共享会话已退出登录（可能是其他客户端退出）；请重新运行 shimo-net login")
	errSessionRevision   = errors.New("共享会话版本已变化；请重新读取后再提交")
	errSessionNoCookie   = errors.New("共享会话没有可用 cookie")
	errSessionBadPayload = errors.New("无法解析会话数据：需要 cookie 头文本，或 {\"cookie\":\"...\"}，或 {\"cookies\":[...]}")

	// errSessionNotApplicable 表示记录存在，但没有 cookie 适用于这次请求。
	// 这种情况必须直接失败：静默回退到钥匙串会用到另一个账号的旧凭据。
	errSessionNotApplicable = errors.New("共享会话没有适用于该请求的 cookie（Secure 与 Path 属性不匹配）；请核对导入的 cookie 属性")
)

// sessionCookie 保留 Domain/Path/Secure/HttpOnly/SameSite/Expires，便于宿主回填浏览器。
type sessionCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain,omitempty"`
	Path     string `json:"path,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	HttpOnly bool   `json:"httpOnly,omitempty"`
	SameSite string `json:"sameSite,omitempty"`
	Expires  int64  `json:"expires,omitempty"`
}

type sessionAccount struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

type sessionRecord struct {
	SchemaVersion int             `json:"schemaVersion"`
	Origin        string          `json:"origin"`
	AuthState     string          `json:"authState"`
	Account       sessionAccount  `json:"account"`
	Cookies       []sessionCookie `json:"cookies"`
	Revision      int64           `json:"revision"`
	UpdatedAt     string          `json:"updatedAt"`
	Source        string          `json:"source"`
}

func sessionDir() string {
	if v := strings.TrimSpace(os.Getenv("SHIMO_NET_SESSION_DIR")); v != "" {
		return v
	}
	return filepath.Join(stateDir(), "sessions")
}

func sessionStorePath(origin string) (string, error) {
	canonical, err := canonicalCredentialOrigin(origin)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(canonical))
	return filepath.Join(sessionDir(), hex.EncodeToString(sum[:])+".json"), nil
}

func readSessionRecord(origin string) (*sessionRecord, bool) {
	p, err := sessionStorePath(origin)
	if err != nil {
		return nil, false
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	var rec sessionRecord
	if json.Unmarshal(b, &rec) != nil || rec.AuthState == "" {
		return nil, false
	}
	return &rec, true
}

// sessionFileStamp 返回会话文件的 mtime（纳秒）与大小，用于廉价探测变化。
// 命中时只做一次 stat，不读文件、不解析 JSON。
func sessionFileStamp(origin string) (int64, int64, bool) {
	p, err := sessionStorePath(origin)
	if err != nil {
		return 0, 0, false
	}
	st, err := os.Stat(p)
	if err != nil {
		return 0, 0, false
	}
	return st.ModTime().UnixNano(), st.Size(), true
}

func signedOutOverwriteError(rev int64) error {
	return fmt.Errorf("共享会话已退出（revision %d），拒绝用旧会话覆盖；确认是新登录请重新读取后带 --expected-revision %d 提交", rev, rev)
}

func accountLabel(rec *sessionRecord) string {
	if rec == nil {
		return ""
	}
	for _, v := range []string{rec.Account.Email, rec.Account.Name, rec.Account.ID} {
		if v != "" {
			return v
		}
	}
	return "未记录"
}

// withSessionLock 用 O_EXCL 锁文件串行化读改写；超过 15 秒的锁视为残留。
func withSessionLock(origin string, fn func() error) error {
	p, err := sessionStorePath(origin)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	lock := p + ".lock"
	deadline := time.Now().Add(3 * time.Second)
	for {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%d %d", os.Getpid(), time.Now().Unix())
			_ = f.Close()
			defer os.Remove(lock)
			return fn()
		}
		if st, statErr := os.Stat(lock); statErr == nil && time.Since(st.ModTime()) > 15*time.Second {
			_ = os.Remove(lock)
			continue
		}
		if time.Now().After(deadline) {
			return errors.New("共享会话正被其他进程写入，请稍后重试")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// commitSession 写入新版本。expectedRevision < 0 表示不带版本前提写入。
// allowAfterLogout 为 false 时，拒绝把 signedOut 记录改回 signedIn：退出后只有
// 明确读过当前 revision 的调用方（新的登录）才能覆盖，避免旧会话把退出"复活"。
func commitSession(origin string, next sessionRecord, expectedRevision int64, allowAfterLogout bool) (int64, error) {
	var rev int64
	err := withSessionLock(origin, func() error {
		current, _ := readSessionRecord(origin)
		var currentRev int64
		if current != nil {
			currentRev = current.Revision
			if !allowAfterLogout && current.AuthState == sessionSignedOut && next.AuthState != sessionSignedOut {
				return signedOutOverwriteError(currentRev)
			}
		}
		if expectedRevision >= 0 && currentRev != expectedRevision {
			return errSessionRevision
		}
		next.SchemaVersion = sessionSchemaVersion
		next.Origin = origin
		next.Revision = currentRev + 1
		next.UpdatedAt = time.Now().UTC().Format("2006-01-02T15:04:05Z")
		b, err := json.MarshalIndent(next, "", "  ")
		if err != nil {
			return err
		}
		p, err := sessionStorePath(origin)
		if err != nil {
			return err
		}
		f, err := os.CreateTemp(filepath.Dir(p), ".session-")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		if _, err := f.Write(append(b, '\n')); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		if err := os.Chmod(f.Name(), 0o600); err != nil {
			return err
		}
		rev = next.Revision
		return os.Rename(f.Name(), p)
	})
	return rev, err
}

func parseCookieHeader(header string) []sessionCookie {
	out := []sessionCookie{}
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if name == "" || value == "" {
			continue
		}
		out = append(out, sessionCookie{Name: name, Value: value, Path: "/"})
	}
	return out
}

// sessionCookieHeader 返回按 origin 根路径计算的 cookie 头，仅用于“是否存在可用
// 会话”的判断，不用于实际请求。
func sessionCookieHeader(rec *sessionRecord) string {
	return sessionCookieHeaderFor(rec, nil)
}

// sessionCookieHeaderFor 按实际要请求的 URL 过滤 cookie：Domain、Expires、
// Secure（只发给 https）和 Path 前缀。u 为 nil 时取 origin 的 scheme 与根路径。
// 浏览器的同源策略就是按这四条挑 cookie 的，CLI 直接把整份记录拼成头会突破它。
func sessionCookieHeaderFor(rec *sessionRecord, u *url.URL) string {
	if rec == nil {
		return ""
	}
	host := hostOf(rec.Origin)
	scheme, path := "", "/"
	if ou, err := url.Parse(rec.Origin); err == nil {
		scheme = ou.Scheme
	}
	if u != nil {
		scheme = u.Scheme
		host = u.Hostname()
		path = u.Path
	}
	now := time.Now().Unix()
	values := map[string]string{}
	for _, c := range rec.Cookies {
		if c.Name == "" || c.Value == "" {
			continue
		}
		if c.Domain != "" && !domainMatchesHost(c.Domain, host) {
			continue
		}
		if c.Expires > 0 && c.Expires < now {
			continue
		}
		if c.Secure && scheme != "https" {
			continue
		}
		if !cookiePathMatches(c.Path, path) {
			continue
		}
		values[c.Name] = c.Value
	}
	if values["sid"] == "" {
		return ""
	}
	names := make([]string, 0, len(values))
	for n := range values {
		names = append(names, n)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, n+"="+values[n])
	}
	return strings.Join(parts, "; ")
}

// cookiePathMatches 按 RFC 6265 §5.1.4 判断请求路径是否命中 cookie 的 Path。
func cookiePathMatches(cookiePath, requestPath string) bool {
	if cookiePath == "" {
		cookiePath = "/"
	}
	if requestPath == "" {
		requestPath = "/"
	}
	if cookiePath == requestPath {
		return true
	}
	if !strings.HasPrefix(requestPath, cookiePath) {
		return false
	}
	if strings.HasSuffix(cookiePath, "/") {
		return true
	}
	return len(requestPath) > len(cookiePath) && requestPath[len(cookiePath)] == '/'
}

func domainMatchesHost(domain, host string) bool {
	domain = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(domain, ".")))
	host = strings.ToLower(strings.TrimSpace(host))
	if domain == "" || host == "" {
		return false
	}
	return host == domain || strings.HasSuffix(host, "."+domain)
}

// sharedSessionCookieFor 返回共享会话中适用于该请求的 cookie 头。第二个返回值
// 区分“没有记录”（""）、“记录为已退出”（signedOut）和“有记录但没有适用的 cookie”
// （signedIn + 空头，调用方应报错而不是回退旧凭据）。
func sharedSessionCookieFor(origin string, u *url.URL) (string, string) {
	rec, ok := readSessionRecord(origin)
	if !ok {
		return "", ""
	}
	if rec.AuthState == sessionSignedOut {
		return "", sessionSignedOut
	}
	return sessionCookieHeaderFor(rec, u), sessionSignedIn
}

func sharedSessionCookie(origin string) (string, string) {
	return sharedSessionCookieFor(origin, nil)
}

// mirrorSession 把已验证的登录态写入共享存储，account 可为空。
//
// baseRev 是本次登录/回收开始前读到的 revision（无记录为 0）。用它做写入前提，
// 保证登录期间发生的退出或切号不会被这次写入覆盖：中途有人写过，revision 就变了，
// 这次镜像会以冲突告终，由调用方提示而不是静默盖掉别人的状态。
func mirrorSession(origin, cookieHeader, source string, account map[string]any, baseRev int64) error {
	cookies := parseCookieHeader(cookieHeader)
	if len(cookies) == 0 {
		return errSessionNoCookie
	}
	_, err := commitSession(origin, sessionRecord{
		Origin:    origin,
		AuthState: sessionSignedIn,
		Account: sessionAccount{
			ID: str(account, "id"), Name: str(account, "name"), Email: str(account, "email"),
		},
		Cookies: cookies,
		Source:  source,
	}, baseRev, true)
	return err
}

// sessionRevision 返回当前 revision；没有记录时为 0，用于登录前的写入前提。
func sessionRevision(origin string) int64 {
	rec, ok := readSessionRecord(origin)
	if !ok {
		return 0
	}
	return rec.Revision
}

func cookieValue(cookies []sessionCookie, name string) string {
	for _, c := range cookies {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

// warnSharedMirror 在共享写入失败时提示：本地凭据已经有效，但其他 Agent 拿不到新会话。
// 不把登录判为失败——本机确实登录成功了，只是共享那一步降级了。
func warnSharedMirror(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "提示：共享会话未更新（%v）；其他 Agent 与宿主仍在使用旧会话\n", err)
}

func markSessionSignedOut(origin, source string) error {
	_, err := commitSession(origin, sessionRecord{
		Origin:    origin,
		AuthState: sessionSignedOut,
		Source:    source,
		// 退出后保留账号标识便于核对，但不再保留 cookie。
		Account: sessionAccount{},
	}, -1, true)
	return err
}

// mergeSetCookies 只续期本次请求对应的共享记录。请求发出后发生切号、退出或
// 其他更新时，revision 冲突就丢弃本次续期，不能重读新账号再合并旧响应。
func mergeSetCookies(origin string, rec *sessionRecord, cookies []*http.Cookie) {
	if len(cookies) == 0 || rec == nil || rec.AuthState != sessionSignedIn {
		return
	}
	merged := mergeCookieList(rec.Cookies, cookies, hostOf(origin))
	if cookieValue(merged, "sid") == "" {
		return
	}
	_, _ = commitSession(origin, sessionRecord{
		Origin:    origin,
		AuthState: sessionSignedIn,
		Account:   rec.Account,
		Cookies:   merged,
		Source:    "renew",
	}, rec.Revision, false)
}

func mergeCookieList(existing []sessionCookie, cookies []*http.Cookie, host string) []sessionCookie {
	merged := append([]sessionCookie(nil), existing...)
	for _, c := range cookies {
		if c == nil || c.Name == "" {
			continue
		}
		domain := c.Domain
		if domain == "" {
			domain = host
		}
		if !domainMatchesHost(domain, host) {
			continue
		}
		path := c.Path
		if path == "" {
			path = "/"
		}
		idx := -1
		for i, e := range merged {
			if e.Name == c.Name && e.Path == path {
				idx = i
				break
			}
		}
		if c.MaxAge < 0 {
			if idx >= 0 {
				merged = append(merged[:idx], merged[idx+1:]...)
			}
			continue
		}
		value := c.Value
		if value == "" && idx >= 0 {
			continue
		}
		expires := int64(0)
		if !c.Expires.IsZero() {
			expires = c.Expires.Unix()
		}
		updated := sessionCookie{
			Name: c.Name, Value: value, Domain: domain, Path: path,
			Secure: c.Secure, HttpOnly: c.HttpOnly, SameSite: sameSiteText(c.SameSite), Expires: expires,
		}
		if idx >= 0 {
			merged[idx] = updated
		} else {
			merged = append(merged, updated)
		}
	}
	return merged
}

func sameSiteText(v http.SameSite) string {
	switch v {
	case http.SameSiteStrictMode:
		return "Strict"
	case http.SameSiteLaxMode:
		return "Lax"
	case http.SameSiteNoneMode:
		return "None"
	}
	return ""
}

// sessionStatusMap 供 CLI 输出与宿主订阅使用：只有元数据，没有 cookie 值。
func sessionStatusMap(origin string) map[string]any {
	out := map[string]any{"origin": origin, "shared": false}
	rec, ok := readSessionRecord(origin)
	if !ok {
		return out
	}
	names := make([]string, 0, len(rec.Cookies))
	for _, c := range rec.Cookies {
		names = append(names, c.Name)
	}
	sort.Strings(names)
	out["shared"] = true
	out["authState"] = rec.AuthState
	out["account"] = map[string]any{"id": rec.Account.ID, "name": rec.Account.Name, "email": rec.Account.Email}
	out["revision"] = rec.Revision
	out["updatedAt"] = rec.UpdatedAt
	out["source"] = rec.Source
	out["cookieCount"] = len(rec.Cookies)
	out["cookieNames"] = names
	out["signedOut"] = rec.AuthState == sessionSignedOut
	out["store"] = sessionStorePathOrEmpty(origin)
	return out
}

func sessionStorePathOrEmpty(origin string) string {
	p, err := sessionStorePath(origin)
	if err != nil {
		return ""
	}
	return p
}

// readSessionPayload 接受纯 cookie 头，或带属性的 JSON：
// {"cookie":"..."} 只给名称和值；{"cookies":[{...}]} 保留 Domain/Path/Secure 等属性。
func readSessionPayload(r *bufio.Reader) ([]sessionCookie, map[string]any, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	text := strings.TrimSpace(string(b))
	if text == "" {
		return nil, nil, errSessionBadPayload
	}
	if !strings.HasPrefix(text, "{") {
		cookies := parseCookieHeader(text)
		if len(cookies) == 0 {
			return nil, nil, errSessionNoCookie
		}
		return cookies, nil, nil
	}
	var payload struct {
		Cookie  string          `json:"cookie"`
		Cookies []sessionCookie `json:"cookies"`
		Account map[string]any  `json:"account"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return nil, nil, errSessionBadPayload
	}
	if len(payload.Cookies) > 0 {
		cookies := make([]sessionCookie, 0, len(payload.Cookies))
		for _, c := range payload.Cookies {
			if strings.TrimSpace(c.Name) == "" || c.Value == "" {
				continue
			}
			if c.Path == "" {
				c.Path = "/"
			}
			cookies = append(cookies, c)
		}
		if len(cookies) == 0 {
			return nil, nil, errSessionNoCookie
		}
		return cookies, payload.Account, nil
	}
	if strings.TrimSpace(payload.Cookie) == "" {
		return nil, nil, errSessionBadPayload
	}
	cookies := parseCookieHeader(payload.Cookie)
	if len(cookies) == 0 {
		return nil, nil, errSessionNoCookie
	}
	return cookies, payload.Account, nil
}

func (a *App) cmdSessionImport() error {
	cookies, account, err := readSessionPayload(bufio.NewReader(os.Stdin))
	if err != nil {
		return err
	}
	if _, err := canonicalCredentialOrigin(a.Origin); err != nil {
		return err
	}
	if os.Getenv("SHIMO_NET_COOKIE") != "" {
		return errors.New("SHIMO_NET_COOKIE 正在覆盖本机账号；请先取消该环境变量再导入会话")
	}
	if !a.has("--no-verify") {
		// 按真实校验地址算头：属性不适用（例如 Secure 的 cookie 交给 http 网址）
		// 在这里就失败，不要等到正式请求才发现凭据发不出去。
		header := sessionCookieHeaderFor(&sessionRecord{Origin: a.Origin, Cookies: cookies}, parseURLOrNil(a.apiURL("/users/me")))
		if header == "" {
			return errSessionNotApplicable
		}
		me, err := (&App{Origin: a.Origin}).usersMeWithCookie(header)
		if err != nil {
			return fmt.Errorf("导入的会话不可用：%w", err)
		}
		if account == nil {
			account = me
		}
	}
	source := "import"
	if v := a.opt("--source"); v != "" {
		source = v
	}
	hasSID := false
	for _, c := range cookies {
		if c.Name == "sid" {
			hasSID = true
			break
		}
	}
	if !hasSID {
		return errors.New("会话缺少 sid，拒绝写入（共享会话只接受已登录的 cookie 头）")
	}
	expected := int64(-1)
	explicit := a.has("--expected-revision")
	if explicit {
		n, err := strconv.ParseInt(a.opt("--expected-revision"), 10, 64)
		if err != nil || n < 0 {
			return errors.New("--expected-revision 必须是非负整数")
		}
		expected = n
	}
	// 已有记录时不允许无条件覆盖，否则并发切号会互相盖掉、迟到的旧导入会把
	// 已完成的新登录改回旧账号：
	//   同一 sid —— 同一会话的属性刷新（宿主重复上报），按当前 revision 提交；
	//   不同 sid —— 切号，必须由调用方带 --expected-revision 明确表示；
	//   已退出   —— 必须重新读取 revision 后提交，退出优先。
	if !explicit {
		if cur, ok := readSessionRecord(a.Origin); ok {
			if cur.AuthState == sessionSignedOut {
				return signedOutOverwriteError(cur.Revision)
			}
			if cookieValue(cur.Cookies, "sid") != cookieValue(cookies, "sid") {
				return fmt.Errorf("共享会话已有其他账号的会话（revision %d，%s）；切号请重新读取后带 --expected-revision %d 提交",
					cur.Revision, terminalText(accountLabel(cur)), cur.Revision)
			}
			expected = cur.Revision
		}
	}
	// 首次写入也带上版本前提：两个进程同时初始化时，后一个会撞版本而不是覆盖。
	if expected < 0 {
		expected = 0
	}
	rev, err := commitSession(a.Origin, sessionRecord{
		Origin:    a.Origin,
		AuthState: sessionSignedIn,
		Account:   sessionAccount{ID: str(account, "id"), Name: str(account, "name"), Email: str(account, "email")},
		Cookies:   cookies,
		Source:    source,
	}, expected, explicit)
	if err != nil {
		return err
	}
	out := sessionStatusMap(a.Origin)
	out["ok"] = true
	if a.JSON {
		return a.printJSON(out)
	}
	fmt.Printf("已写入共享会话：%s（revision %d，%d 个 cookie）\n", terminalText(a.Origin), rev, len(cookies))
	return nil
}

func (a *App) cmdSessionWatch() error {
	interval := time.Second
	if v := a.opt("--interval"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return errors.New("--interval 需要正的时间间隔，例如 1s")
		}
		interval = d
	}
	once := a.has("--once")
	emit := func() int64 {
		status := sessionStatusMap(a.Origin)
		if a.JSON {
			_ = json.NewEncoder(os.Stdout).Encode(status)
		} else {
			fmt.Printf("%s revision=%v authState=%v\n", status["origin"], status["revision"], status["authState"])
		}
		if rev, ok := status["revision"].(int64); ok {
			return rev
		}
		return 0
	}
	last := emit()
	if once {
		return nil
	}
	// 快速路径：每轮只 stat 文件头，mtime 与大小都没变就跳过整次读取和反序列化。
	// 少数文件系统时间戳精度较粗（如部分网络盘），因此每 fullScanEvery 轮强制读一次兜底。
	const fullScanEvery = 30
	lastMtime, lastSize, lastOK := sessionFileStamp(a.Origin)
	tick := 0
	for {
		time.Sleep(interval)
		tick++
		mtime, size, ok := sessionFileStamp(a.Origin)
		if ok == lastOK && mtime == lastMtime && size == lastSize && tick%fullScanEvery != 0 {
			continue
		}
		lastMtime, lastSize, lastOK = mtime, size, ok
		rec, found := readSessionRecord(a.Origin)
		var rev int64
		if found {
			rev = rec.Revision
		}
		if rev == last {
			continue
		}
		last = emit()
	}
}

func (a *App) cmdSessionLogout() error {
	if err := markSessionSignedOut(a.Origin, "logout"); err != nil {
		return err
	}
	out := sessionStatusMap(a.Origin)
	out["ok"] = true
	if a.JSON {
		return a.printJSON(out)
	}
	fmt.Printf("已标记共享会话退出：%s\n", terminalText(a.Origin))
	return nil
}
