package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
)

func (a *App) probeOrigin(origin string) (int, error) {
	o, err := normalizeOrigin(origin)
	if err != nil {
		return 0, err
	}
	// /users/me 未登录时在 shimo.net 返回 404（找不到该用户），不能用来判断站点。
	// 用无需登录的 auth/register/is_new 探测，避免把正常实例误判成官网。
	req, _ := http.NewRequest("GET", o+"/api/v1/auth/register/is_new", nil)
	req.Header.Set("x-requested-with", "XMLHttpRequest")
	req.Header.Set("user-agent", "shimo-net/"+version)
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("连不上 %s：%w", origin, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == http.StatusOK {
		ct := strings.ToLower(resp.Header.Get("Content-Type"))
		trimmed := strings.TrimSpace(string(body))
		if strings.Contains(ct, "json") || strings.HasPrefix(trimmed, "{") {
			return http.StatusOK, nil
		}
		return 0, fmt.Errorf("网址可达但接口返回 HTML，请确认是石墨办公套件地址：%s", origin)
	}
	return resp.StatusCode, nil
}

func (a *App) cmdInit() error {
	if v := a.opt("--origin"); v != "" {
		if err := a.setOrigin(v); err != nil {
			return err
		}
	} else {
		if err := a.ensureOrigin(); err != nil {
			return err
		}
		if readConfig()["origin"] == nil {
			if err := a.setOrigin(a.Origin); err != nil {
				return err
			}
		}
	}
	status, err := a.probeOrigin(a.Origin)
	if err != nil {
		return err
	}
	if status != 200 && status != 401 && status != 403 {
		return fmt.Errorf("网址可达但接口返回 HTTP %d，请确认是石墨办公套件地址：%s", status, a.Origin)
	}
	fmt.Printf("产品网址：%s\n", a.Origin)
	fmt.Printf("配置已保存：%s\n", configPath())
	fmt.Printf("接口可达（HTTP %d）。下一步：shimo-net login\n", status)
	if a.has("--login") {
		return a.cmdLogin()
	}
	return nil
}

func (a *App) cmdConfig() error {
	sub := "show"
	if len(a.Pos) > 0 {
		sub = a.Pos[0]
	}
	switch sub {
	case "show", "get", "list":
		o := map[string]any{"origin": a.Origin, "source": a.OriginSource, "configFile": configPath(),
			"savedAt": readConfig()["savedAt"]}
		if os.Getenv("SHIMO_NET_ORIGIN") != "" {
			o["envOverride"] = "SHIMO_NET_ORIGIN"
		}
		if a.JSON {
			return a.printJSON(o)
		}
		fmt.Printf("产品网址：%s（来源：%s）\n", a.Origin, a.OriginSource)
		fmt.Printf("配置文件：%s\n", configPath())
		if os.Getenv("SHIMO_NET_ORIGIN") != "" {
			fmt.Printf("环境变量：SHIMO_NET_ORIGIN=%s（优先级最高）\n", a.Origin)
		}
		return nil
	case "set", "use":
		v := ""
		if len(a.Pos) > 1 {
			v = a.Pos[1]
		}
		if v == "" {
			v = a.opt("--origin")
		}
		if v == "" {
			return errors.New("用法：shimo-net config set <https://产品域名>")
		}
		if err := a.setOrigin(v); err != nil {
			return err
		}
		status, err := a.probeOrigin(a.Origin)
		if err != nil {
			return err
		}
		fmt.Printf("已保存：%s（HTTP %d）\n", a.Origin, status)
		return nil
	case "clear", "reset":
		cfg := readConfig()
		delete(cfg, "origin")
		b, _ := json.MarshalIndent(cfg, "", "  ")
		if err := os.MkdirAll(stateDir(), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(configPath(), append(b, '\n'), 0o600); err != nil {
			return err
		}
		fmt.Println("已清除 config.json 里的 origin；如设置了 SHIMO_NET_ORIGIN，仍以环境变量为准")
		return nil
	default:
		return errors.New("用法：shimo-net config [show|set <url>|clear]")
	}
}

func (a *App) cmdWhoami() error {
	me, err := a.usersMe()
	if err != nil {
		return err
	}
	if a.JSON {
		return a.printJSON(me)
	}
	fmt.Printf("%s <%s>（id %s）\n", str(me, "name"), str(me, "email"), str(me, "id"))
	return nil
}

func (a *App) usersMe() (map[string]any, error) {
	r, err := a.authRequest("/users/me", "GET", nil, nil, nil)
	if err != nil {
		return nil, err
	}
	if r.Status != 200 {
		return nil, fmt.Errorf("登录已过期或不可用（HTTP %d）", r.Status)
	}
	m, _ := r.Data.(map[string]any)
	return m, nil
}

func (a *App) cmdDoctor() error {
	chrome, _ := findChrome()
	storageLabel := "macOS Keychain"
	if runtime.GOOS == "windows" {
		storageLabel = "Windows Credential Manager"
	}
	storage := map[string]string{"keychain": storageLabel, "file": "本机文件 0600", "env": "环境变量 SHIMO_NET_COOKIE"}[credSource()]
	if storage == "" {
		storage = "未登录"
	}
	o := map[string]any{"version": version, "runtime": runtime.Version(), "chrome": chrome, "home": stateDir(),
		"origin": a.Origin, "originSource": a.OriginSource, "configFile": nil, "credentialsFile": nil, "storage": storage}
	if _, err := os.Stat(configPath()); err == nil {
		o["configFile"] = configPath()
	}
	if _, err := os.Stat(credPath()); err == nil {
		o["credentialsFile"] = credPath()
	}

	checks := []map[string]any{}
	add := func(name string, ok bool, detail string) {
		checks = append(checks, map[string]any{"name": name, "ok": ok, "detail": detail})
	}
	add("runtime", runtime.Version() != "", runtime.Version())
	add("chrome", chrome != "", chrome)
	_, originErr := normalizeOrigin(a.Origin)
	add("origin", originErr == nil, a.Origin)
	add("credentials", credSource() != "", storage)
	account := ""
	if me, err := a.usersMe(); err == nil {
		account = fmt.Sprintf("%s <%s>（id %s）", str(me, "name"), str(me, "email"), str(me, "id"))
		add("account", true, account)
	} else {
		account = err.Error()
		add("account", false, account)
	}
	o["account"] = account

	ok := true
	for _, c := range checks {
		if !boolv(c, "ok") {
			ok = false
		}
	}
	o["ok"] = ok
	o["checks"] = checks

	if a.JSON {
		if err := a.printJSON(o); err != nil {
			return err
		}
		if !ok {
			return errors.New("doctor: 有检查项未通过")
		}
		return nil
	}
	for _, k := range []string{"version", "runtime", "chrome", "home", "origin", "originSource", "configFile", "credentialsFile", "storage", "account"} {
		if v := o[k]; v != nil {
			fmt.Printf("%s: %v\n", k, v)
		}
	}
	fmt.Println("检查项：")
	for _, c := range checks {
		mark := "🟢"
		if !boolv(c, "ok") {
			mark = "🔴"
		}
		fmt.Printf("  %s   %-12s  %s\n", mark, str(c, "name"), str(c, "detail"))
	}
	if !ok {
		fmt.Println("结果： 🔴 doctor: FAIL")
		return errors.New("doctor: 有检查项未通过")
	}
	fmt.Println("结果： 🟢 doctor: OK")
	return nil
}

func (a *App) cmdSearch() error {
	if len(a.Pos) == 0 {
		return errors.New("用法：shimo-net search <关键词> [--limit 20]")
	}
	keyword := a.Pos[0]
	limit := intArg(a.opt("--limit"), 20)
	r, err := a.authRequest("/search/files", "POST", map[string]any{"keyword": keyword}, nil, nil)
	if err != nil {
		return err
	}
	if r.Status != 200 {
		return fmt.Errorf("搜索失败：HTTP %d %s", r.Status, snippet(r.Raw))
	}
	dm := asMap(r.Data)
	results := asSlice(dm["results"])
	out := []map[string]any{}
	for _, item := range results {
		im := asMap(item)
		s := asMap(im["source"])
		isFolder := boolv(s, "isFolder") || str(s, "type") == "folder"
		f := map[string]any{
			"guid": str(s, "guid"), "name": str(s, "name"), "type": str(s, "type"),
			"isFolder": isFolder, "isSpace": boolv(s, "isSpace"), "shareMode": str(s, "shareMode"),
			"updatedAt": s["updatedAt"], "path": joinAncestors(asSlice(s["ancestors"])),
		}
		f["url"] = a.urlOf(withFolder(f, isFolder))
		out = append(out, f)
		if len(out) >= limit {
			break
		}
	}
	if a.JSON {
		return a.printJSON(map[string]any{"keyword": keyword, "count": len(out), "results": out})
	}
	fmt.Printf("搜索「%s」：%d 条\n\n", keyword, len(out))
	for i, f := range out {
		fmt.Println(fileRow(f, i))
	}
	return nil
}

func (a *App) listSpaces(limit int) ([]map[string]any, error) {
	out := []map[string]any{}
	path := "/files/spaces"
	for i := 0; i < 1000; i++ {
		r, err := a.authRequest(path, "GET", nil, nil, nil)
		if err != nil {
			return nil, err
		}
		if r.Status != 200 {
			return nil, fmt.Errorf("读取空间列表失败：HTTP %d", r.Status)
		}
		j := asMap(r.Data)
		for _, item := range asSlice(j["spaces"]) {
			s := asMap(item)
			out = append(out, map[string]any{"guid": str(s, "guid"), "name": str(s, "name"),
				"url":     strings.TrimRight(a.Origin, "/") + "/space/" + str(s, "guid"),
				"isSpace": true, "role": str(s, "role"), "updatedAt": s["updatedAt"], "fileSize": s["fileSize"]})
			if limit > 0 && len(out) >= limit {
				return out, nil
			}
		}
		next := str(j, "next")
		if next == "" {
			break
		}
		if strings.HasPrefix(next, "?") {
			path = "/files/spaces" + next
		} else {
			path = "/files/spaces?" + next
		}
	}
	return out, nil
}

func (a *App) cmdSpaces() error {
	limit := intArg(a.opt("--limit"), 300)
	out, err := a.listSpaces(limit)
	if err != nil {
		return err
	}
	if a.JSON {
		return a.printJSON(map[string]any{"count": len(out), "spaces": out})
	}
	fmt.Printf("我参与的空间：%d 个\n\n", len(out))
	for i, s := range out {
		fmt.Printf("%3d. %s\n     %s\n     guid=%s", i+1, str(s, "name"), str(s, "url"), str(s, "guid"))
		if str(s, "role") != "" {
			fmt.Printf("  角色=%s", str(s, "role"))
		}
		if fmtTime(s["updatedAt"]) != "" {
			fmt.Printf("  更新=%s", fmtTime(s["updatedAt"]))
		}
		fmt.Println()
	}
	return nil
}

func (a *App) fileInfo(guid string) (map[string]any, error) {
	r, err := a.authRequest("/files/"+url.PathEscape(guid), "GET", nil, nil, nil)
	if err != nil {
		return nil, err
	}
	if r.Status != 200 {
		return nil, fmt.Errorf("读取文件信息失败：HTTP %d", r.Status)
	}
	m, _ := r.Data.(map[string]any)
	return m, nil
}

func (a *App) ancestors(guid string) ([]map[string]any, error) {
	r, err := a.authRequest("/files/"+url.PathEscape(guid)+"/ancestors", "GET", nil, nil, nil)
	if err != nil {
		return nil, err
	}
	if r.Status != 200 {
		return nil, fmt.Errorf("读取所在路径失败：HTTP %d", r.Status)
	}
	m := asMap(r.Data)
	out := []map[string]any{}
	for _, it := range asSlice(m["ancestors"]) {
		x := asMap(it)
		u := str(x, "url")
		if strings.HasPrefix(u, "/") {
			u = a.Origin + u
		} else {
			u = a.urlOf(withFolder(x, true))
		}
		out = append(out, map[string]any{"guid": str(x, "guid"), "name": str(x, "name"),
			"isFolder": boolv(x, "isFolder"), "isSpace": boolv(x, "isSpace"), "url": u})
	}
	return out, nil
}

func (a *App) listFolder(target string, limit int) ([]map[string]any, error) {
	guid, err := a.resolveTarget(target)
	if err != nil {
		return nil, err
	}
	if guid == "" {
		guid = "Desktop"
	}
	path := "/files?folder=" + url.QueryEscape(guid)
	out := []map[string]any{}
	for i := 0; i < 1000; i++ {
		r, err := a.authRequest(path, "GET", nil, nil, nil)
		if err != nil {
			return nil, err
		}
		if r.Status != 200 {
			return nil, fmt.Errorf("读取文件夹失败：HTTP %d %s", r.Status, snippet(r.Raw))
		}
		j := asMap(r.Data)
		for _, item := range asSlice(j["list"]) {
			f := asMap(item)
			isFolder := boolv(f, "isFolder")
			out = append(out, map[string]any{
				"guid": str(f, "guid"), "name": str(f, "name"), "type": str(f, "type"),
				"isFolder": isFolder, "isSpace": boolv(f, "isSpace"), "isCloudFile": boolv(f, "isCloudFile"),
				"fileSize": f["fileSize"], "updatedAt": f["updatedAt"], "shareMode": str(f, "shareMode"),
				"url": a.urlOf(withFolder(f, isFolder)),
			})
			if limit > 0 && len(out) >= limit {
				return out, nil
			}
		}
		next := ""
		switch n := j["nextURL"].(type) {
		case string:
			next = n
		case map[string]any:
			next = str(n, "url")
			if next == "" {
				next = str(n, "nextURL")
			}
		}
		if next == "" {
			break
		}
		next = strings.Replace(next, a.Origin, "", 1)
		if !strings.HasPrefix(next, "/") {
			next = "/" + next
		}
		path = next
	}
	return out, nil
}

func (a *App) cmdLs() error {
	if len(a.Pos) == 0 {
		return errors.New("用法：shimo-net ls <文件夹/空间链接|guid|@别名> [--since 7d] [--limit 200]")
	}
	since, err := parseSince(a.opt("--since"))
	if err != nil {
		return err
	}
	rows, err := a.listFolder(a.Pos[0], intArg(a.opt("--limit"), 200))
	if err != nil {
		return err
	}
	if since > 0 {
		filtered := rows[:0]
		for _, f := range rows {
			if timeOf(f["updatedAt"]) >= since {
				filtered = append(filtered, f)
			}
		}
		rows = filtered
	}
	if a.JSON {
		return a.printJSON(map[string]any{"target": a.Pos[0], "since": a.opt("--since"), "count": len(rows), "files": rows})
	}
	fmt.Printf("共 %d 项%s：\n\n", len(rows), map[bool]string{true: "（自 " + fmtTime(float64(since)/1000) + "）", false: ""}[since > 0])
	for i, f := range rows {
		fmt.Println(fileRow(f, i))
	}
	return nil
}

func (a *App) cmdInfo() error {
	if len(a.Pos) == 0 {
		return errors.New("用法：shimo-net info <文件/文件夹链接|guid|@别名>")
	}
	guid, err := a.resolveTarget(a.Pos[0])
	if err != nil {
		return err
	}
	if guid == "Desktop" {
		if a.JSON {
			return a.printJSON(map[string]any{"guid": "Desktop", "name": "我的桌面", "kind": "desktop"})
		}
		fmt.Println("我的桌面")
		return nil
	}
	info, err := a.fileInfo(guid)
	if err != nil {
		return err
	}
	chain, _ := a.ancestors(guid)
	path := []string{}
	for _, x := range chain {
		path = append(path, str(x, "name"))
	}
	o := map[string]any{"guid": guid, "name": str(info, "name"), "type": str(info, "type"),
		"typeLabel": typeLabel(info), "isFolder": boolv(info, "isFolder"), "isCloudFile": boolv(info, "isCloudFile"),
		"shareMode": str(info, "shareMode"), "updatedAt": info["updatedAt"], "fileSize": info["fileSize"],
		"url": a.urlOf(withGUID(info, guid)), "path": strings.Join(path, " / "), "ancestors": chain}
	if a.JSON {
		return a.printJSON(o)
	}
	fmt.Printf("名称：%s\n", str(o, "name"))
	fmt.Printf("类型：%s（%s）%s\n", str(o, "typeLabel"), str(o, "type"), map[bool]string{true: " · 云文件", false: ""}[boolv(o, "isCloudFile")])
	fmt.Printf("链接：%s\n", str(o, "url"))
	fmt.Printf("权限：%s（%s）\n", str(o, "shareMode"), shareModes[str(o, "shareMode")])
	if str(o, "path") != "" {
		fmt.Printf("路径：%s\n", str(o, "path"))
	}
	if fmtTime(o["updatedAt"]) != "" {
		fmt.Printf("更新：%s\n", fmtTime(o["updatedAt"]))
	}
	return nil
}

func (a *App) cmdAudit() error {
	if len(a.Pos) == 0 {
		return errors.New("用法：shimo-net audit <文件夹/空间链接|guid|@别名> [--limit 200]")
	}
	limit := intArg(a.opt("--limit"), 200)
	files, err := a.listFolder(a.Pos[0], limit)
	if err != nil {
		return err
	}
	items := []map[string]any{}
	for _, f := range files {
		if !boolv(f, "isFolder") && !boolv(f, "isSpace") {
			items = append(items, f)
		}
	}
	results := make([]map[string]any, len(items))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, f := range items {
		wg.Add(1)
		go func(i int, f map[string]any) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			info, err := a.fileInfo(str(f, "guid"))
			if err != nil {
				results[i] = map[string]any{"guid": str(f, "guid"), "name": str(f, "name"), "type": str(f, "type"), "url": str(f, "url"), "shareMode": "unknown", "error": err.Error()}
				return
			}
			results[i] = map[string]any{"guid": str(f, "guid"), "name": str(info, "name"), "type": str(info, "type"),
				"url": a.urlOf(withGUID(info, str(f, "guid"))), "shareMode": str(info, "shareMode")}
		}(i, f)
	}
	wg.Wait()
	sort.Slice(results, func(i, j int) bool { return str(results[i], "name") < str(results[j], "name") })
	groups := map[string][]map[string]any{"public": {}, "enterprise": {}, "private": {}}
	for _, r := range results {
		mode := str(r, "shareMode")
		switch {
		case mode == "readonly" || mode == "commentable" || mode == "editable":
			groups["public"] = append(groups["public"], r)
		case strings.HasPrefix(mode, "enterprise"):
			groups["enterprise"] = append(groups["enterprise"], r)
		default:
			groups["private"] = append(groups["private"], r)
		}
	}
	truncated := limit > 0 && len(files) >= limit
	o := map[string]any{"target": a.Pos[0], "count": len(results), "limit": limit, "truncated": truncated, "groups": groups, "results": results}
	if a.JSON {
		return a.printJSON(o)
	}
	fmt.Printf("权限巡检：%d 个文件\n", len(results))
	if truncated {
		fmt.Printf("注意：已达到 --limit %d，结果可能被截断；需要完整巡检请调大 --limit。\n", limit)
	}
	fmt.Printf("  公开链接：%d   企业内：%d   私有：%d\n\n", len(groups["public"]), len(groups["enterprise"]), len(groups["private"]))
	if len(groups["public"]) > 0 {
		fmt.Println("公开可访问（建议收回或确认）：")
		for _, f := range groups["public"] {
			fmt.Printf("  ! %s  [%s]  %s\n", str(f, "name"), str(f, "shareMode"), str(f, "url"))
		}
		fmt.Println()
	}
	fmt.Println("全部：")
	for _, f := range results {
		mark := "·"
		mode := str(f, "shareMode")
		if mode == "readonly" || mode == "commentable" || mode == "editable" {
			mark = "!"
		}
		fmt.Printf("  %s %s  [%s]\n", mark, str(f, "name"), mode)
	}
	if len(groups["public"]) > 0 {
		links := []string{}
		for _, f := range groups["public"] {
			links = append(links, `"`+str(f, "url")+`"`)
		}
		fmt.Printf("\n批量收回：shimo-net share %s enterprise_readonly\n", strings.Join(links, " "))
	}
	return nil
}

func (a *App) resolveTarget(spec string) (string, error) {
	s := strings.TrimSpace(spec)
	if s == "" {
		return "Desktop", nil
	}
	if strings.EqualFold(s, "desktop") || s == "桌面" || s == "我的桌面" {
		return "Desktop", nil
	}
	alias := strings.TrimPrefix(s, "@")
	favs := loadFav()
	if v, ok := favs[alias]; ok {
		if g := guidFromURL(str(v, "url")); g != "" {
			return g, nil
		}
	}
	if g := guidFromURL(s); g != "" {
		return g, nil
	}
	spaces, err := a.listSpaces(3000)
	if err == nil {
		for _, sp := range spaces {
			if str(sp, "name") == s {
				return str(sp, "guid"), nil
			}
		}
	}
	return "", fmt.Errorf("认不出这个位置：%s（可传链接、guid、desktop 或 @别名；用 `shimo-net fav list` 看已有别名）", s)
}

func (a *App) describeTarget(guid string) map[string]any {
	if guid == "" || guid == "Desktop" {
		return map[string]any{"guid": "Desktop", "name": "我的桌面", "kind": "desktop"}
	}
	info, err := a.fileInfo(guid)
	if err != nil {
		return map[string]any{"guid": guid, "name": nil, "kind": "unknown"}
	}
	kind := str(info, "type")
	if boolv(info, "isSpace") {
		kind = "space"
	} else if boolv(info, "isFolder") {
		kind = "folder"
	}
	return map[string]any{"guid": guid, "name": str(info, "name"), "kind": kind, "url": a.urlOf(withGUID(info, guid))}
}

func withGUID(f map[string]any, guid string) map[string]any {
	out := map[string]any{}
	for k, v := range f {
		out[k] = v
	}
	out["guid"] = guid
	return out
}

func withFolder(f map[string]any, isFolder bool) map[string]any {
	out := map[string]any{}
	for k, v := range f {
		out[k] = v
	}
	out["isFolder"] = isFolder
	return out
}

func intArg(v string, def int) int {
	if v == "" {
		return def
	}
	var n int
	_, err := fmt.Sscanf(v, "%d", &n)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func snippet(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func fileRow(f map[string]any, i int) string {
	badge := typeLabel(f)
	path := ""
	if p := str(f, "path"); p != "" {
		path = "  （" + p + "）"
	}
	extra := ""
	if fmtTime(f["updatedAt"]) != "" {
		extra += "  更新=" + fmtTime(f["updatedAt"])
	}
	if str(f, "shareMode") != "" {
		extra += "  权限=" + str(f, "shareMode")
	}
	return fmt.Sprintf("%2d. [%s] %s%s\n    %s\n    guid=%s%s", i+1, badge, str(f, "name"), path, str(f, "url"), str(f, "guid"), extra)
}

func joinAncestors(arr []any) string {
	names := []string{}
	for _, x := range arr {
		if n := str(asMap(x), "name"); n != "" {
			names = append(names, n)
		}
	}
	return strings.Join(names, " / ")
}
