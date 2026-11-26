package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// The SDK credentials are scoped to an authenticated editor page. They must
// never be included in JSON results, evidence, log messages, or cross-origin
// requests. This session only accepts internal, fixed API routes.
type phase3SDKSession struct {
	app     *App
	target  string
	guid    string
	prefix  string
	headers map[string]string
}
type phase3SDKRuntime struct {
	prefix  string
	headers map[string]string
}

func phase3ParseSDKRuntime(html []byte) (phase3SDKRuntime, error) {
	const marker = "const envs = "
	i := bytes.Index(html, []byte(marker))
	if i < 0 {
		return phase3SDKRuntime{}, errors.New("编辑器页面未返回 SDK runtime；请检查登录和文件权限")
	}
	var envs []struct {
		Runtime map[string]any `json:"__RUNTIME_ENV__"`
	}
	if err := json.NewDecoder(bytes.NewReader(html[i+len(marker):])).Decode(&envs); err != nil || len(envs) == 0 {
		return phase3SDKRuntime{}, errors.New("无法解析 SDK runtime")
	}
	env := envs[0].Runtime
	prefix := str(env, "SDK_V2_PATH_PREFIX")
	u, err := url.Parse(prefix)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(prefix, "/") || strings.HasPrefix(prefix, "//") || u.RawQuery != "" || u.Fragment != "" || strings.Contains(prefix, "..") {
		return phase3SDKRuntime{}, errors.New("SDK API 前缀不是安全的同源路径")
	}
	hp := str(env, "SDK_V2_HEADER_PREFIX")
	if hp == "" {
		hp = "x-weboffice-"
	}
	if strings.ContainsAny(hp, "\r\n :") || !strings.HasPrefix(strings.ToLower(hp), "x-") {
		return phase3SDKRuntime{}, errors.New("SDK 请求头前缀无效")
	}
	sig, token := str(env, "SDK_V2_SIGNATURE"), str(env, "SDK_V2_TOKEN")
	if sig == "" || token == "" {
		return phase3SDKRuntime{}, errors.New("SDK runtime 缺少访问签名")
	}
	return phase3SDKRuntime{strings.TrimRight(prefix, "/") + "/", map[string]string{hp + "signature": sig, hp + "token": token, hp + "product": "sdk", hp + "user-uuid": str(env, "SDK_V2_USER_UUID")}}, nil
}

func (a *App) phase3SDK(target, typ string) (*phase3SDKSession, error) {
	if !phase3SameOrigin(target, a.Origin) {
		return nil, errors.New("目标链接与已配置 origin 不一致")
	}
	guid := guidFromURL(target)
	if guid == "" {
		return nil, errors.New("文件 guid 无效")
	}
	info, err := a.fileInfo(guid)
	if err != nil {
		return nil, err
	}
	if str(info, "type") != typ || boolv(info, "isCloudFile") || boolv(info, "isFolder") {
		return nil, fmt.Errorf("此动作仅支持在线 %s 文件", typ)
	}
	route := typeRoute[typ]
	if route == "" {
		return nil, errors.New("不支持的编辑器类型")
	}
	page := strings.TrimRight(a.Origin, "/") + "/" + route + "/" + url.PathEscape(guid)
	r, err := a.authRequestPath("/"+route+"/"+url.PathEscape(guid), "GET", nil, nil, nil)
	if err != nil {
		return nil, err
	}
	if r.Status != 200 {
		return nil, fmt.Errorf("读取编辑器 runtime 失败（HTTP %d）", r.Status)
	}
	runtime, err := phase3ParseSDKRuntime(r.Raw)
	if err != nil {
		return nil, err
	}
	runtime.headers["referer"] = page
	return &phase3SDKSession{app: a, target: page, guid: guid, prefix: runtime.prefix, headers: runtime.headers}, nil
}

func phase3SDKJSON(r *Response) (any, error) {
	if r.Status < 200 || r.Status >= 300 {
		if detail := phase3SDKErrorDetail(r); detail != "" {
			return nil, fmt.Errorf("SDK 请求失败（HTTP %d）：%s", r.Status, detail)
		}
		return nil, fmt.Errorf("SDK 请求失败（HTTP %d）", r.Status)
	}
	if r.Data == nil {
		return nil, errors.New("SDK 未返回 JSON；HTML/空响应不能视为成功")
	}
	if m, ok := r.Data.(map[string]any); ok {
		if code := str(m, "errorCode"); code != "" && code != "0" {
			return nil, fmt.Errorf("SDK 业务错误（%s）", phase3SDKErrorDetail(r))
		}
		if code := str(m, "code"); code != "" && code != "0" && code != "200" {
			return nil, fmt.Errorf("SDK 业务错误（%s）", phase3SDKErrorDetail(r))
		}
		if e := str(m, "error"); e != "" {
			return nil, fmt.Errorf("SDK 业务错误（%s）", phase3SDKErrorDetail(r))
		}
	}
	return r.Data, nil
}

// phase3SDKErrorDetail surfaces the server's own error fields. Without them a
// 400 is indistinguishable from a permission problem or a bad request shape.
func phase3SDKErrorDetail(r *Response) string {
	if m, ok := r.Data.(map[string]any); ok {
		parts := []string{}
		for _, k := range []string{"errorCode", "code", "error", "errMsg", "message", "msg", "errorMessage"} {
			if v := str(m, k); v != "" {
				parts = append(parts, k+"="+v)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "，")
		}
	}
	raw := strings.TrimSpace(string(r.Raw))
	if raw == "" {
		return ""
	}
	if len(raw) > 300 {
		raw = raw[:300] + "…"
	}
	return "响应正文 " + raw
}
func (s *phase3SDKSession) get(path string) (any, error) {
	r, err := s.app.authRequestPath(s.prefix+path, "GET", nil, nil, s.headers)
	if err != nil {
		return nil, err
	}
	return phase3SDKJSON(r)
}

func phase3TableSummary(table map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range []string{"guid", "baseGuid", "name", "desc", "order", "tableType", "defaultViewGuid", "isMotable"} {
		if v, ok := table[k]; ok {
			out[k] = v
		}
	}
	return out
}
func phase3ValidateTableSnapshot(table map[string]any, baseGuid, tableGuid string) error {
	if str(table, "guid") != tableGuid || str(table, "baseGuid") != baseGuid {
		return errors.New("数据表读回身份不匹配")
	}
	for _, key := range []string{"fields", "views", "records"} {
		entries, ok := table[key].([]any)
		if !ok {
			return fmt.Errorf("数据表读回缺少 %s 数组，无法验证", key)
		}
		seen := map[string]bool{}
		for _, item := range entries {
			m, ok := item.(map[string]any)
			id, typed := m["guid"].(string)
			if !ok || !typed || id == "" || seen[id] {
				return fmt.Errorf("数据表 %s 存在缺失或重复 guid", key)
			}
			seen[id] = true
			if key == "records" {
				if _, ok := m["valueMap"].(map[string]any); !ok {
					return errors.New("数据表记录缺少 valueMap 对象")
				}
			}
		}
	}
	return nil
}
func (s *phase3SDKSession) tableSnapshot(tableGuid string) (map[string]any, error) {
	beforeRev, err := s.tableReadRevision()
	if err != nil {
		return nil, err
	}
	data, err := s.get("table-api/tables/" + url.PathEscape(tableGuid) + "?withData=true")
	if err != nil {
		return nil, err
	}
	table, ok := data.(map[string]any)
	if !ok {
		return nil, errors.New("数据表读回不是对象")
	}
	if err := phase3ValidateTableSnapshot(table, s.guid, tableGuid); err != nil {
		return nil, err
	}
	if err = s.hydrateTableViews(table); err != nil {
		return nil, err
	}
	afterRev, err := s.tableReadRevision()
	if err != nil {
		return nil, err
	}
	if beforeRev != afterRev {
		return nil, errors.New("应用表格读取期间revision改变，未获得一致快照")
	}
	return table, nil
}

// cmdPhase3TableRead is a read-only inventory of application-table structure.
// It does not accept spreadsheet files and never reports mutation acceptance.
func (a *App) cmdPhase3TableRead() error {
	if len(a.Pos) != 1 {
		return errors.New("用法：phase3 <应用表格链接> --action table-inspect [--table 数据表guid]")
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	s, err := a.phase3SDK(target, "table")
	if err != nil {
		return err
	}
	data, err := s.get("table-api/v2/bases/" + url.PathEscape(s.guid))
	if err != nil {
		return err
	}
	base, ok := data.(map[string]any)
	if !ok {
		return errors.New("应用表格 base 响应不是对象")
	}
	list, ok := base["tables"].([]any)
	if !ok {
		return errors.New("应用表格 base 缺少 tables")
	}
	rev, err := s.get("table-api/bases/" + url.PathEscape(s.guid) + "/rev")
	if err != nil {
		return err
	}
	result := map[string]any{"ok": true, "action": "table-inspect", "fileType": "table", "url": s.target, "revision": rev, "mutationAttempted": false}
	recordLimit := 20
	if raw := strings.TrimSpace(a.opt("--records-limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 500 {
			return errors.New("--records-limit 必须是 0 到 500 之间的整数")
		}
		recordLimit = n
	}
	summaries := []any{}
	found := false
	for _, v := range list {
		table, ok := v.(map[string]any)
		if !ok || str(table, "guid") == "" {
			return errors.New("应用表格包含无效数据表")
		}
		summary := phase3TableSummary(table)
		// 只读盘点必须直接给出字段/视图/记录 GUID：写动作一律按 GUID 取键，
		// 让调用方再猜或再跑一次 --table 属于实现不足。
		if num(table, "tableType") != 4 {
			structure, err := s.tableStructure(str(table, "guid"), recordLimit)
			if err != nil {
				return err
			}
			summary["fields"] = structure["fields"]
			summary["views"] = structure["views"]
			summary["recordCount"] = structure["recordCount"]
			summary["recordGuids"] = structure["recordGuids"]
			summary["recordGuidsTruncated"] = structure["recordGuidsTruncated"]
		} else {
			summary["dashboard"] = true
		}
		summaries = append(summaries, summary)
		if id := a.opt("--table"); id != "" && id == str(table, "guid") {
			var detail map[string]any
			var err error
			if num(table, "tableType") == 4 {
				detail, err = s.dashboardSnapshot(table)
			} else {
				detail, err = s.tableSnapshot(id)
			}
			if err != nil {
				return err
			}
			result["table"] = detail
			found = true
		}
	}
	if a.opt("--table") != "" && !found {
		return errors.New("指定数据表不属于目标应用表格")
	}
	result["tables"] = summaries
	result["recordLimit"] = recordLimit
	return a.printJSON(result)
}

// tableStructure 是 table-inspect 的轻量结构读取：一次 GET 拿到字段、视图与
// 记录 GUID，不做 revision 双读与视图顺序补全——盘点不需要，写动作才需要
// tableSnapshot 的严格快照。
func (s *phase3SDKSession) tableStructure(tableGuid string, recordLimit int) (map[string]any, error) {
	data, err := s.get("table-api/tables/" + url.PathEscape(tableGuid) + "?withData=true")
	if err != nil {
		return nil, err
	}
	table, ok := data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("数据表 %s 结构响应不是对象", tableGuid)
	}
	if str(table, "guid") != tableGuid || str(table, "baseGuid") != s.guid {
		return nil, fmt.Errorf("数据表 %s 结构读回身份不匹配", tableGuid)
	}
	pick := func(key string, keys []string) ([]any, error) {
		entries, ok := table[key].([]any)
		if !ok {
			return nil, fmt.Errorf("数据表 %s 结构缺少 %s 数组", tableGuid, key)
		}
		out := make([]any, 0, len(entries))
		for _, raw := range entries {
			m, ok := raw.(map[string]any)
			if !ok || str(m, "guid") == "" {
				return nil, fmt.Errorf("数据表 %s 的 %s 存在无效条目", tableGuid, key)
			}
			item := map[string]any{"guid": str(m, "guid")}
			for _, k := range keys {
				if v, exists := m[k]; exists {
					item[k] = v
				}
			}
			out = append(out, item)
		}
		return out, nil
	}
	fields, err := pick("fields", []string{"name", "type", "isPrimary"})
	if err != nil {
		return nil, err
	}
	views, err := pick("views", []string{"name", "type", "isDefault"})
	if err != nil {
		return nil, err
	}
	records, err := pick("records", nil)
	if err != nil {
		return nil, err
	}
	guids := make([]any, 0, len(records))
	for i, raw := range records {
		if recordLimit >= 0 && i >= recordLimit {
			break
		}
		guids = append(guids, raw.(map[string]any)["guid"])
	}
	return map[string]any{
		"fields":               fields,
		"views":                views,
		"recordCount":          len(records),
		"recordGuids":          guids,
		"recordGuidsTruncated": len(guids) < len(records),
	}, nil
}
