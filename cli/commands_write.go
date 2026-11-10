package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func (a *App) uploadFile(local, parent string) (map[string]any, error) {
	buf, err := os.ReadFile(local)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(local)
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	r, err := a.authRequest("/files/upload?encodedFileName="+url.QueryEscape(name), "POST", nil, buf,
		map[string]string{"content-type": "application/octet-stream", "x-file-parent-guid": parent,
			"x-file-size": fmt.Sprint(len(buf)), "x-file-type": ext})
	if err != nil {
		return nil, err
	}
	if r.Status != 200 {
		return nil, fmt.Errorf("上传失败：HTTP %d %s", r.Status, snippet(r.Raw))
	}
	m, _ := r.Data.(map[string]any)
	return m, nil
}

func (a *App) cmdUpload() error {
	if len(a.Pos) == 0 {
		return errors.New("用法：shimo-net upload <本地文件> [--to desktop|空间|文件夹|@别名]")
	}
	local := expandHome(a.Pos[0])
	if _, err := os.Stat(local); err != nil {
		return fmt.Errorf("文件不存在：%s", local)
	}
	to := a.opt("--to")
	if to == "" {
		aliases := loadFav()
		lines := []string{"请指定上传位置：", "  桌面：--to desktop"}
		if len(aliases) > 0 {
			names := []string{}
			for k := range aliases {
				names = append(names, "@"+k)
			}
			lines = append(lines, "  常用位置（@别名）："+strings.Join(names, "、"))
		}
		if spaces, err := a.listSpaces(8); err == nil && len(spaces) > 0 {
			names := []string{}
			for _, s := range spaces {
				names = append(names, str(s, "name"))
			}
			lines = append(lines, "  空间（示例）："+strings.Join(names, "、"))
		}
		lines = append(lines, "  文件夹：--to <文件夹链接|guid>")
		return errors.New(strings.Join(lines, "\n"))
	}
	parent, err := a.resolveTarget(to)
	if err != nil {
		return err
	}
	data, err := a.uploadFile(local, parent)
	if err != nil {
		return err
	}
	guid := str(data, "guid")
	o := map[string]any{"ok": true, "guid": guid, "name": str(data, "name"), "type": str(data, "type"),
		"url": a.urlOf(withGUID(data, guid)), "parent": a.describeTarget(parent),
		"isCloudFile": boolv(data, "isCloudFile"), "note": "云文件（原样上传，不转换）；要转在线文档请用 import"}
	if a.JSON {
		return a.printJSON(o)
	}
	fmt.Printf("上传完成：%s\n", str(o, "name"))
	pm := asMap(o["parent"])
	fmt.Printf("位置：%s\n", firstNonEmpty(str(pm, "name"), str(pm, "guid")))
	fmt.Printf("链接：%s\n", str(o, "url"))
	fmt.Println("提示：这是原样云文件；要转成在线文档/表格/幻灯片请用 import")
	return nil
}

func (a *App) importFile(cloudGUID, typ string) (string, error) {
	r, err := a.authRequest("/files/import/"+url.PathEscape(cloudGUID), "POST", map[string]any{"type": typ}, nil, nil)
	if err != nil {
		return "", err
	}
	tid := nestedString(r.Data, "data", "taskId")
	if r.Status != 200 || tid == "" {
		return "", fmt.Errorf("导入失败：HTTP %d %s", r.Status, snippet(r.Raw))
	}
	return tid, nil
}

func (a *App) importProgress(taskID string, timeout time.Duration) (map[string]any, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		r, err := a.authRequest("/files/import/progress", "POST", map[string]any{"taskId": taskID}, nil, nil)
		if err != nil {
			return nil, err
		}
		d := asMap(asMap(r.Data)["data"])
		if num(d, "progress") >= 100 {
			return d, nil
		}
		time.Sleep(700 * time.Millisecond)
	}
	return nil, errors.New("导入超时")
}

func (a *App) cmdImport() error {
	if len(a.Pos) == 0 {
		return errors.New("用法：shimo-net import <本地文件> --to <桌面|文件夹链接|@别名> [--title 名称] [--share 权限]")
	}
	local := expandHome(a.Pos[0])
	if _, err := os.Stat(local); err != nil {
		return fmt.Errorf("文件不存在：%s", local)
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(local)), ".")
	typ := importTypes[ext]
	if typ == "" {
		return fmt.Errorf("暂不支持 .%s —— 目前只支持 md / doc(x) / xls(x) / ppt(x)", ext)
	}
	to := a.opt("--to")
	if to == "" {
		return errors.New("必须指定导入位置：--to desktop|空间|文件夹链接|@别名")
	}
	parent, err := a.resolveTarget(to)
	if err != nil {
		return err
	}
	cloud, err := a.uploadFile(local, parent)
	if err != nil {
		return err
	}
	tid, err := a.importFile(str(cloud, "guid"), typ)
	if err != nil {
		return err
	}
	done, err := a.importProgress(tid, 120*time.Second)
	if err != nil {
		return err
	}
	guid := nestedString(done, "file", "guid")
	if guid == "" {
		guid = str(cloud, "guid")
	}
	info, err := a.fileInfo(guid)
	if err != nil {
		return err
	}
	if title := a.opt("--title"); title != "" {
		_, _ = a.renameFile(guid, title)
		info["name"] = title
	}
	result := map[string]any{"ok": true, "name": str(info, "name"), "guid": guid, "type": str(info, "type"),
		"typeLabel": typeLabel(info), "url": a.urlOf(withGUID(info, guid)), "shareMode": str(info, "shareMode"),
		"folder": a.describeTarget(parent)}
	if mode := a.opt("--share"); mode != "" {
		applied, err := a.setShareMode(guid, mode)
		if err != nil {
			return err
		}
		result["shareMode"] = applied
	}
	if a.JSON {
		return a.printJSON(result)
	}
	fmt.Printf("导入完成：%s\n", str(result, "name"))
	fmt.Printf("类型：%s（%s）\n", str(result, "typeLabel"), str(result, "type"))
	fmt.Printf("链接：%s\n", str(result, "url"))
	fmt.Printf("当前权限：%s（%s）\n", str(result, "shareMode"), shareModes[str(result, "shareMode")])
	if a.opt("--share") == "" {
		fmt.Printf("\n设置权限：shimo-net share %s <%s>\n", str(result, "url"), strings.Join(keys(shareModes), "|"))
	}
	return nil
}

func (a *App) cmdNew() error {
	if len(a.Pos) == 0 {
		return errors.New("用法：shimo-net new <名称> [--type doc|sheet|slide] [--to <文件夹|@别名>]")
	}
	name := a.Pos[0]
	typ := a.opt("--type")
	if typ == "" {
		typ = "doc"
	}
	shimoType := createTypes[typ]
	if shimoType == "" {
		return fmt.Errorf("不支持的新建类型：%s（可用：doc / sheet / slide）", typ)
	}
	parent, err := a.resolveTarget(firstNonEmpty(a.opt("--to"), "desktop"))
	if err != nil {
		return err
	}
	r, err := a.authRequest("/files/create/"+shimoType+"?parentGuid="+url.QueryEscape(parent)+"&name="+url.QueryEscape(name), "GET", nil, nil, nil)
	if err != nil {
		return err
	}
	if r.Status != 302 {
		return fmt.Errorf("新建失败：HTTP %d %s", r.Status, snippet(r.Raw))
	}
	loc := r.Header.Get("location")
	m := regexp.MustCompile(`/(docs|docx|sheets|presentation|tables|forms|flowcharts|mindmaps)/([A-Za-z0-9]+)`).FindStringSubmatch(loc)
	if len(m) != 3 {
		return fmt.Errorf("新建成功但没解析到 guid：%s", loc)
	}
	guid := m[2]
	info, err := a.fileInfo(guid)
	if err != nil {
		return err
	}
	o := map[string]any{"ok": true, "name": str(info, "name"), "guid": guid, "type": str(info, "type"),
		"typeLabel": typeLabel(info), "url": a.urlOf(withGUID(info, guid)), "shareMode": str(info, "shareMode")}
	if a.JSON {
		return a.printJSON(o)
	}
	fmt.Printf("新建完成：%s（%s）\n", str(o, "name"), str(o, "typeLabel"))
	fmt.Printf("链接：%s\n", str(o, "url"))
	fmt.Printf("当前权限：%s\n", str(o, "shareMode"))
	return nil
}

func (a *App) renameFile(guid, name string) (map[string]any, error) {
	r, err := a.authRequest("/files/"+url.PathEscape(guid), "PATCH", map[string]any{"name": name}, nil, nil)
	if err != nil {
		return nil, err
	}
	if r.Status != 200 {
		return nil, fmt.Errorf("重命名失败：HTTP %d", r.Status)
	}
	m, _ := r.Data.(map[string]any)
	return m, nil
}

func (a *App) cmdRename() error {
	if len(a.Pos) < 2 {
		return errors.New("用法：shimo-net rename <链接|guid|@别名> <新名字>")
	}
	guid, err := a.resolveTarget(a.Pos[0])
	if err != nil {
		return err
	}
	if _, err := a.renameFile(guid, a.Pos[1]); err != nil {
		return err
	}
	info, err := a.fileInfo(guid)
	if err != nil {
		return err
	}
	o := map[string]any{"ok": true, "guid": guid, "name": str(info, "name"), "url": a.urlOf(withGUID(info, guid))}
	if a.JSON {
		return a.printJSON(o)
	}
	fmt.Printf("已改名：%s\n", str(o, "name"))
	fmt.Printf("链接：%s\n", str(o, "url"))
	return nil
}

func (a *App) setShareMode(guid, mode string) (string, error) {
	if _, ok := shareModes[mode]; !ok {
		return "", fmt.Errorf("不支持的权限：%s，可选：%s", mode, strings.Join(keys(shareModes), ", "))
	}
	r, err := a.authRequest("/files/"+url.PathEscape(guid), "PATCH", map[string]any{"shareMode": mode}, nil, nil)
	if err != nil {
		return "", err
	}
	if r.Status != 200 {
		return "", fmt.Errorf("设置权限失败：HTTP %d %s", r.Status, snippet(r.Raw))
	}
	m := asMap(r.Data)
	if str(m, "shareMode") == "unknown_share_mode" {
		return "", errors.New("服务端拒绝了这个权限值")
	}
	return str(m, "shareMode"), nil
}

func (a *App) cmdShare() error {
	if len(a.Pos) < 2 {
		return fmt.Errorf("用法：shimo-net share <链接|guid|@别名> [更多…] <权限：%s>", strings.Join(keys(shareModes), "|"))
	}
	mode := a.Pos[len(a.Pos)-1]
	if _, ok := shareModes[mode]; !ok {
		return fmt.Errorf("不支持的权限：%s，可选：%s", mode, strings.Join(keys(shareModes), ", "))
	}
	targets := a.Pos[:len(a.Pos)-1]
	results := []map[string]any{}
	okCount := 0
	for _, t := range targets {
		guid, err := a.resolveTarget(t)
		if err != nil {
			results = append(results, map[string]any{"ok": false, "target": t, "error": err.Error()})
			continue
		}
		info, err := a.fileInfo(guid)
		if err != nil {
			results = append(results, map[string]any{"ok": false, "target": t, "error": err.Error()})
			continue
		}
		applied, err := a.setShareMode(guid, mode)
		if err != nil {
			results = append(results, map[string]any{"ok": false, "target": t, "error": err.Error()})
			continue
		}
		okCount++
		results = append(results, map[string]any{"ok": true, "guid": guid, "name": str(info, "name"),
			"shareMode": applied, "shareModeLabel": shareModes[applied], "url": a.urlOf(withGUID(info, guid))})
	}
	payload := map[string]any{"ok": okCount == len(targets), "mode": mode, "modeLabel": shareModes[mode],
		"total": len(targets), "changed": okCount, "results": results}
	if a.JSON {
		return a.printJSON(payload)
	}
	fmt.Printf("权限设为「%s」：%d/%d 成功\n", shareModes[mode], okCount, len(targets))
	for _, r := range results {
		if boolv(r, "ok") {
			fmt.Printf("✓ %s  %s\n", str(r, "name"), str(r, "url"))
		} else {
			fmt.Printf("✗ %s：%s\n", str(r, "target"), str(r, "error"))
		}
	}
	return nil
}

func (a *App) cmdDelete() error {
	if len(a.Pos) == 0 {
		return errors.New("用法：shimo-net delete <链接|guid|@别名>")
	}
	guid, err := a.resolveTarget(a.Pos[0])
	if err != nil {
		return err
	}
	info, err := a.fileInfo(guid)
	if err != nil {
		return err
	}
	r, err := a.authRequest("/files/"+url.PathEscape(guid), "DELETE", nil, nil, nil)
	if err != nil {
		return err
	}
	if r.Status != 200 && r.Status != 204 {
		return fmt.Errorf("删除失败：HTTP %d", r.Status)
	}
	o := map[string]any{"ok": true, "guid": guid, "name": str(info, "name"), "url": a.urlOf(withGUID(info, guid)),
		"note": "已进回收站，可用 restore 恢复；彻底删除请到石墨网页手动操作"}
	if a.JSON {
		return a.printJSON(o)
	}
	fmt.Printf("已删除（进回收站，可恢复）：%s\n", str(o, "name"))
	fmt.Printf("恢复：shimo-net restore %s\n", guid)
	return nil
}

func (a *App) listTrash() ([]map[string]any, error) {
	r, err := a.authRequest("/files/trashes", "GET", nil, nil, nil)
	if err != nil {
		return nil, err
	}
	if r.Status != 200 {
		return nil, fmt.Errorf("读取回收站失败：HTTP %d", r.Status)
	}
	arr := asSlice(r.Data)
	if arr == nil {
		arr = asSlice(asMap(r.Data)["list"])
	}
	out := []map[string]any{}
	for _, item := range arr {
		f := asMap(item)
		u := str(f, "url")
		if strings.HasPrefix(u, "/") {
			u = a.Origin + u
		} else {
			u = a.urlOf(f)
		}
		out = append(out, map[string]any{"guid": str(f, "guid"), "name": str(f, "name"), "type": str(f, "type"),
			"url": u, "deletedAt": firstNonEmptyVal(f["deletedAt"], f["updatedAt"])})
	}
	return out, nil
}

func (a *App) cmdTrash() error {
	rows, err := a.listTrash()
	if err != nil {
		return err
	}
	if a.JSON {
		return a.printJSON(map[string]any{"count": len(rows), "files": rows})
	}
	fmt.Printf("回收站：%d 项\n\n", len(rows))
	for i, f := range rows {
		fmt.Printf("%2d. [%s] %s", i+1, str(f, "type"), str(f, "name"))
		if fmtTime(f["deletedAt"]) != "" {
			fmt.Printf("  删除=%s", fmtTime(f["deletedAt"]))
		}
		fmt.Printf("\n    guid=%s\n", str(f, "guid"))
	}
	return nil
}

func (a *App) cmdRestore() error {
	if len(a.Pos) == 0 {
		return errors.New("用法：shimo-net restore <文件链接|guid>")
	}
	guid := guidFromURL(a.Pos[0])
	if guid == "" {
		guid = a.Pos[0]
	}
	r, err := a.authRequest("/files/trashes/"+url.PathEscape(guid), "PATCH", nil, nil, nil)
	if err != nil {
		return err
	}
	if r.Status != 200 {
		return fmt.Errorf("恢复失败：HTTP %d %s", r.Status, snippet(r.Raw))
	}
	o := map[string]any{"ok": true, "guid": guid}
	if info, err := a.fileInfo(guid); err == nil {
		o["name"], o["url"] = str(info, "name"), a.urlOf(withGUID(info, guid))
	}
	if a.JSON {
		return a.printJSON(o)
	}
	if str(o, "name") != "" {
		fmt.Printf("已恢复：%s\n", str(o, "name"))
	} else {
		fmt.Println("已恢复")
	}
	if str(o, "url") != "" {
		fmt.Printf("链接：%s\n", str(o, "url"))
	}
	return nil
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(homeDir(), p[2:])
	}
	return p
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstNonEmptyVal(vals ...any) any {
	for _, v := range vals {
		if v != nil && fmt.Sprint(v) != "" {
			return v
		}
	}
	return nil
}

func nestedString(v any, keys ...string) string {
	cur := v
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[k]
	}
	s, _ := cur.(string)
	return s
}
