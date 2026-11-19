package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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
		return nil, fmt.Errorf("SDK 请求失败（HTTP %d）", r.Status)
	}
	if r.Data == nil {
		return nil, errors.New("SDK 未返回 JSON；HTML/空响应不能视为成功")
	}
	if m, ok := r.Data.(map[string]any); ok {
		if code := str(m, "errorCode"); code != "" && code != "0" {
			return nil, fmt.Errorf("SDK 业务错误（code %s）", code)
		}
		if code := str(m, "code"); code != "" && code != "0" && code != "200" {
			return nil, fmt.Errorf("SDK 业务错误（code %s）", code)
		}
		if e := str(m, "error"); e != "" {
			return nil, errors.New("SDK 返回业务错误")
		}
	}
	return r.Data, nil
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
	summaries := []any{}
	found := false
	for _, v := range list {
		table, ok := v.(map[string]any)
		if !ok || str(table, "guid") == "" {
			return errors.New("应用表格包含无效数据表")
		}
		summaries = append(summaries, phase3TableSummary(table))
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
	return a.printJSON(result)
}
