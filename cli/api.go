package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var shareModes = map[string]string{
	"private": "私有（仅自己）", "enterprise_readonly": "企业内只读",
	"enterprise_commentable": "企业内可评论", "enterprise_editable": "企业内可编辑",
	"readonly": "公开链接 · 只读", "commentable": "公开链接 · 可评论", "editable": "公开链接 · 可编辑",
}

var importTypes = map[string]string{"md": "document", "doc": "documentPro", "docx": "documentPro",
	"xls": "spreadsheet", "xlsx": "spreadsheet", "ppt": "presentation", "pptx": "presentation"}

var createTypes = map[string]string{"doc": "document", "document": "document", "sheet": "spreadsheet",
	"spreadsheet": "spreadsheet", "slide": "presentation", "presentation": "presentation",
	"table": "table", "form": "form", "mindmap": "mindmap", "flowchart": "flowchart"}

var typeRoute = map[string]string{"document": "docs", "documentPro": "docx", "spreadsheet": "sheets",
	"presentation": "presentation", "table": "tables", "form": "forms", "flowchart": "flowcharts", "mindmap": "mindmaps"}

type Response struct {
	Status int
	Header http.Header
	Data   any
	Raw    []byte
}

var httpClient = &http.Client{
	Timeout: 90 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func (a *App) request(path, method string, body any, raw []byte, headers map[string]string, cookie string) (*Response, error) {
	u := strings.TrimRight(a.Origin, "/") + "/api/v1" + path
	var rdr io.Reader
	if raw != nil {
		rdr = bytes.NewReader(raw)
	} else if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, u, rdr)
	if err != nil {
		return nil, err
	}
	if cookie == "" {
		cookie, _ = readCookie()
	}
	req.Header.Set("referer", strings.TrimRight(a.Origin, "/")+"/")
	req.Header.Set("x-requested-with", "XMLHttpRequest")
	if cookie != "" {
		req.Header.Set("cookie", cookie)
	}
	if body != nil && raw == nil {
		req.Header.Set("content-type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var data any
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &data)
	}
	return &Response{Status: resp.StatusCode, Header: resp.Header, Data: data, Raw: payload}, nil
}

func (a *App) authRequest(path, method string, body any, raw []byte, headers map[string]string) (*Response, error) {
	cookie, err := readCookie()
	if err != nil {
		return nil, err
	}
	r, err := a.request(path, method, body, raw, headers, cookie)
	if err != nil {
		return nil, err
	}
	if r.Status == 401 {
		return nil, errors.New("登录已过期（HTTP 401）：请先运行 shimo-net login")
	}
	return r, nil
}

func (a *App) usersMeWithCookie(cookie string) (map[string]any, error) {
	r, err := a.request("/users/me", "GET", nil, nil, nil, cookie)
	if err != nil {
		return nil, err
	}
	if r.Status != 200 {
		return nil, fmt.Errorf("登录已过期或不可用（HTTP %d）", r.Status)
	}
	m, _ := r.Data.(map[string]any)
	return m, nil
}

func httpGet(u string) ([]byte, error) {
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func str(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		return fmt.Sprint(x)
	}
}

func num(m map[string]any, key string) float64 {
	switch x := m[key].(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	}
	return 0
}

func boolv(m map[string]any, key string) bool {
	switch x := m[key].(type) {
	case bool:
		return x
	case string:
		return x == "true"
	case float64:
		return x != 0
	}
	return false
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func (a *App) urlOf(f map[string]any) string {
	origin := strings.TrimRight(a.Origin, "/")
	if boolv(f, "isSpace") {
		g := str(f, "guid")
		if g == "" {
			g = str(f, "space_guid")
		}
		return origin + "/space/" + g
	}
	if boolv(f, "isFolder") {
		return origin + "/folder/" + str(f, "guid")
	}
	u := str(f, "url")
	if strings.HasPrefix(u, "/") {
		return origin + u
	}
	seg := typeRoute[str(f, "type")]
	if seg == "" {
		seg = "files"
	}
	return origin + "/" + seg + "/" + str(f, "guid")
}

func typeLabel(f map[string]any) string {
	if boolv(f, "isSpace") {
		return "空间"
	}
	if boolv(f, "isFolder") || str(f, "type") == "folder" {
		return "文件夹"
	}
	labels := map[string]string{"document": "文档", "documentPro": "文稿", "spreadsheet": "表格",
		"presentation": "幻灯片", "table": "应用表格", "form": "表单", "flowchart": "流程图", "mindmap": "思维导图"}
	if l := labels[str(f, "type")]; l != "" {
		return l
	}
	if boolv(f, "isCloudFile") {
		return "云文件"
	}
	if t := str(f, "type"); t != "" {
		return t
	}
	return "文件"
}

func guidFromURL(v string) string {
	s := strings.TrimSpace(v)
	re := regexp.MustCompile(`/(folder|space|docs|docx|sheets|presentation|tables|forms|flowcharts|mindmaps|files)/([A-Za-z0-9]+)`)
	if m := re.FindStringSubmatch(s); len(m) == 3 {
		return m[2]
	}
	if regexp.MustCompile(`^[A-Za-z0-9]{8,}$`).MatchString(s) {
		return s
	}
	return ""
}

func extFromURL(u, fallback string) string {
	parsed, err := url.Parse(u)
	if err == nil {
		if m := regexp.MustCompile(`\.([A-Za-z0-9]{1,6})$`).FindStringSubmatch(parsed.Path); len(m) == 2 {
			return strings.ToLower(m[1])
		}
	}
	return fallback
}

func fmtTime(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case float64:
		n := int64(x)
		if n > 1e12 {
			n /= 1000
		}
		return time.Unix(n, 0).Format("2006-01-02")
	case string:
		if len(x) >= 10 {
			return x[:10]
		}
		return x
	}
	return ""
}

func timeOf(v any) int64 {
	switch x := v.(type) {
	case float64:
		n := int64(x)
		if n < 1e12 {
			return n * 1000
		}
		return n
	case string:
		if len(x) >= 19 {
			if t, err := time.ParseInLocation("2006-01-02T15:04:05", x[:19], time.Local); err == nil {
				return t.UnixMilli()
			}
		}
	}
	return 0
}

func parseSince(v string) (int64, error) {
	if v == "" {
		return 0, nil
	}
	re := regexp.MustCompile(`(?i)^(\d+)([dhmw])$`)
	if m := re.FindStringSubmatch(v); len(m) == 3 {
		n, _ := strconv.Atoi(m[1])
		unit := map[string]int64{"d": 86400000, "h": 3600000, "m": 60000, "w": 604800000}[strings.ToLower(m[2])]
		return time.Now().UnixMilli() - int64(n)*unit, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
		return t.UnixMilli(), nil
	}
	return 0, fmt.Errorf("看不懂的时间：%s（示例：7d / 24h / 2026-11-01）", v)
}
