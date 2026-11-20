package main

import (
	"errors"
	"strings"
	"time"
)

// Keep the exact-host cookie rule. A parent-domain sid explains why a browser
// login can succeed while this CLI cannot accept that session; do not wait ten
// minutes or silently relax the credential boundary in that case.
func loginCookiesForOrigin(res map[string]any, origin string) ([]string, error) {
	if cookies := cookiesForOrigin(res, origin); len(cookies) > 0 {
		return cookies, nil
	}
	host := strings.ToLower(hostOf(origin))
	if host == "" {
		return nil, nil
	}
	arr, _ := res["cookies"].([]any)
	now := float64(time.Now().Unix())
	for _, item := range arr {
		cookie, _ := item.(map[string]any)
		if str(cookie, "name") != "sid" || str(cookie, "value") == "" {
			continue
		}
		if expires := num(cookie, "expires"); !boolv(cookie, "session") && expires > 0 && expires < now {
			continue
		}
		if boolv(cookie, "secure") && !strings.HasPrefix(strings.ToLower(origin), "https://") {
			continue
		}
		domain := strings.ToLower(strings.TrimPrefix(str(cookie, "domain"), "."))
		if domain != "" && domain != host && strings.HasSuffix(host, "."+domain) {
			return nil, errors.New("检测到登录 sid 使用父域 Cookie，与当前网址主机不完全匹配；CLI 未保存或发送该会话。请联系部署管理员确认登录网址与 Cookie 域配置，当前仅支持精确主机匹配")
		}
	}
	return nil, nil
}
