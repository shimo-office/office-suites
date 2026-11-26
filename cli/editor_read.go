package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var editorReadAnchor = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func (a *App) cmdEditorRead(action, method string) error {
	// Even resolving a GUID or alias must use stored credentials, never launch
	// a browser to harvest a session. Keep the caller's explicit flags intact.
	readApp := *a
	readApp.Bool = make(map[string]bool, len(a.Bool)+1)
	for key, value := range a.Bool {
		readApp.Bool[key] = value
	}
	readApp.Bool["--no-browser"] = true
	if _, err := phase2Args(a.opt("--payload")); err != nil {
		return err
	}
	target, err := readApp.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	if !sameCredentialOrigin(target, a.Origin) {
		return errors.New("编辑器目标与当前配置的 origin 不一致；本次未执行")
	}
	page, err := url.Parse(target)
	if err != nil || guidFromURL(target) == "" {
		return errors.New("读取提及或评论需要包含文件 guid 的石墨链接")
	}
	page.Fragment, page.RawFragment = "", ""
	query := page.Query()
	query.Del("mentionAt")
	page.RawQuery = query.Encode()
	body, records, err := readApp.readEditorRecords(page, method)
	if err != nil {
		return err
	}
	links, unlocated := editorReadLinks(page, action, records)
	out := map[string]any{
		"ok": true, "action": action, "method": method, "url": page.String(),
		"result": map[string]any{"ok": true, "value": body},
		"count":  len(records), "links": links, "unlocatedCount": unlocated,
		"verified": true, "verifyWith": "http",
		"note": "只读 API 通过 HTTP 读取成功；定位链接尚未做页面跳转核验",
	}
	if a.JSON {
		return a.printJSON(out)
	}
	printEditorRead(action, page.String(), len(records), links, unlocated)
	return nil
}

func (a *App) readEditorRecords(page *url.URL, method string) (map[string]any, []any, error) {
	r, err := a.authRequestPath(page.RequestURI(), http.MethodGet, nil, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	if r.Status != http.StatusOK {
		return nil, nil, fmt.Errorf("读取编辑器签名失败（HTTP %d）", r.Status)
	}
	runtime, err := phase3ParseSDKRuntime(r.Raw)
	if err != nil {
		return nil, nil, err
	}
	runtime.headers["referer"] = page.String()
	// These collab-files endpoints are outside runtime.prefix (/sdk/v2/api).
	endpoint := strings.TrimPrefix(method, "api:")
	path := "/sdk/v2/collab-files/" + url.PathEscape(guidFromURL(page.String())) + "/" + endpoint
	r, err = a.authRequestPath(path, http.MethodGet, nil, nil, runtime.headers)
	if err != nil {
		return nil, nil, err
	}
	if r.Status != http.StatusOK {
		return nil, nil, fmt.Errorf("读取提及或评论失败（HTTP %d）", r.Status)
	}
	key := "list"
	if endpoint == "mention-at-list" {
		key = "mentionAtList"
	}
	data, err := phase3SDKJSON(r)
	if err != nil {
		return nil, nil, errors.New("提及或评论 API 返回无效数据或业务错误")
	}
	body, ok := data.(map[string]any)
	if !ok {
		return nil, nil, errors.New("提及或评论 API 未返回有效 JSON 对象")
	}
	if body["ok"] == false || body["success"] == false {
		return nil, nil, errors.New("提及或评论 API 返回业务错误")
	}
	records, ok := body[key].([]any)
	if !ok {
		return nil, nil, fmt.Errorf("提及或评论 API 缺少有效的 %s 列表", key)
	}
	for _, record := range records {
		if _, ok := record.(map[string]any); !ok {
			return nil, nil, errors.New("提及或评论 API 列表含无效记录")
		}
	}
	return body, records, nil
}

func editorReadLinks(page *url.URL, action string, records []any) ([]string, int) {
	links := []string{}
	seen := map[string]bool{}
	unlocated := 0
	parts := strings.Split(strings.Trim(page.Path, "/"), "/")
	for _, record := range records {
		row := record.(map[string]any)
		anchor, _ := row["atGuid"].(string)
		if action == "comments-list" {
			anchor, _ = row["selectionGuid"].(string)
		}
		valid := editorReadAnchor.MatchString(anchor)
		if action == "comments-list" {
			valid = valid && strings.HasPrefix(anchor, "comment-") && len(anchor) > len("comment-")
		}
		// Only /docs anchors are verified against the deployed editor. Other
		// suites retain their raw records without inventing navigation rules.
		if len(parts) != 2 || parts[0] != "docs" || !valid {
			unlocated++
			continue
		}
		link := *page
		link.Fragment = anchor
		value := link.String()
		if !seen[value] {
			seen[value] = true
			links = append(links, value)
		}
	}
	return links, unlocated
}

func printEditorRead(action, target string, count int, links []string, unlocated int) {
	label := "提及"
	if action == "comments-list" {
		label = "评论"
	}
	if count == 0 {
		fmt.Printf("暂无%s\n", label)
		return
	}
	fmt.Printf("%s：%d 条，%d 个定位链接\n", label, count, len(links))
	for _, link := range links {
		fmt.Println(terminalText(link))
	}
	if unlocated > 0 {
		fmt.Printf("%d 条记录暂无可用定位链接（缺少有效位置标识或此套件尚不支持）；原始数据可用 --json 查看。\n文档：%s\n", unlocated, terminalText(target))
	}
}
