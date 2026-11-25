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
				return fmt.Errorf("共享会话已退出（revision %d），拒绝用旧会话覆盖；确认是新登录请重新读取后带 --expected-revision %d 提交", currentRev, currentRev)
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

func sessionCookieHeader(rec *sessionRecord) string {
	if rec == nil {
		return ""
	}
	host := hostOf(rec.Origin)
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

func domainMatchesHost(domain, host string) bool {
	domain = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(domain, ".")))
	host = strings.ToLower(strings.TrimSpace(host))
	if domain == "" || host == "" {
		return false
	}
	return host == domain || strings.HasSuffix(host, "."+domain)
}

// sharedSessionCookie 返回共享会话中的请求 cookie。第二个返回值用于区分
// “没有记录”和“记录为已退出”。
func sharedSessionCookie(origin string) (string, string) {
	rec, ok := readSessionRecord(origin)
	if !ok {
		return "", ""
	}
	if rec.AuthState == sessionSignedOut {
		return "", sessionSignedOut
	}
	header := sessionCookieHeader(rec)
	if header == "" {
		return "", ""
	}
	return header, sessionSignedIn
}

// mirrorSession 把已验证的登录态写入共享存储，account 可为空。
func mirrorSession(origin, cookieHeader, source string, account map[string]any) error {
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
	}, -1, true)
	return err
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

// mergeSetCookies 把响应里的 Set-Cookie 合并进共享会话。只在记录已存在时生效，
// 避免普通命令无端创建共享状态。
func mergeSetCookies(origin string, cookies []*http.Cookie) {
	if len(cookies) == 0 {
		return
	}
	rec, ok := readSessionRecord(origin)
	if !ok || rec.AuthState != sessionSignedIn {
		return
	}
	host := hostOf(origin)
	merged := append([]sessionCookie(nil), rec.Cookies...)
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
		for i, existing := range merged {
			if existing.Name == c.Name && existing.Path == path {
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
	if sessionCookieHeader(&sessionRecord{Origin: origin, Cookies: merged}) == "" {
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
		header := sessionCookieHeader(&sessionRecord{Origin: a.Origin, Cookies: cookies})
		if header == "" {
			return errSessionNoCookie
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
	if a.has("--expected-revision") {
		n, err := strconv.ParseInt(a.opt("--expected-revision"), 10, 64)
		if err != nil || n < 0 {
			return errors.New("--expected-revision 必须是非负整数")
		}
		expected = n
	}
	// 带 revision 说明调用方刚读过当前状态，视为一次明确的重新登录；
	// 不带 revision 时不允许覆盖已退出的记录。
	rev, err := commitSession(a.Origin, sessionRecord{
		Origin:    a.Origin,
		AuthState: sessionSignedIn,
		Account:   sessionAccount{ID: str(account, "id"), Name: str(account, "name"), Email: str(account, "email")},
		Cookies:   cookies,
		Source:    source,
	}, expected, expected >= 0)
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
