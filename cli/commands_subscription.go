package main

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

func (a *App) cmdSubscriptions() error {
	limit := intArg(a.opt("--limit"), 200)
	depth := intArg(a.opt("--depth"), 3)
	if depth > 8 {
		depth = 8
	}
	files, err := a.findSubscribedFiles(limit, depth)
	if err != nil {
		return err
	}
	if selector := strings.TrimSpace(a.opt("--open")); selector != "" {
		file, err := selectSubscription(files, selector)
		if err != nil {
			return err
		}
		openApp := *a
		openApp.Pos = []string{str(file, "url")}
		return openApp.cmdOpen()
	}
	out := map[string]any{"count": len(files), "limit": limit, "depth": depth, "files": files,
		"note": "订阅 API 只提供逐文件状态；最近更新正文需打开选中文件查看"}
	if a.JSON {
		return a.printJSON(out)
	}
	if len(files) == 0 {
		fmt.Println("没有扫描到已订阅文件。可提高 --limit 或 --depth 后重试。")
		return nil
	}
	fmt.Printf("已订阅文件：%d 个\n\n", len(files))
	for i, f := range files {
		fmt.Printf("%d. %s [%s]\n   %s\n   最近更新：%s\n", i+1, str(f, "name"), typeLabel(f), str(f, "url"), fmtTime(f["updatedAt"]))
	}
	fmt.Println("查看某个文件：shimo-net subscriptions --open <序号|guid> [--default]")
	return nil
}

func (a *App) findSubscribedFiles(limit, depth int) ([]map[string]any, error) {
	if limit <= 0 {
		return []map[string]any{}, nil
	}
	type item struct {
		target string
		depth  int
	}
	queue := []item{{target: "Desktop", depth: 0}}
	spaces, err := a.listSpaces(3000)
	if err != nil {
		return nil, err
	}
	for _, space := range spaces {
		queue = append(queue, item{target: str(space, "guid"), depth: 0})
	}
	seenFolders := map[string]bool{}
	seenFiles := map[string]bool{}
	out := []map[string]any{}
	for len(queue) > 0 && len(seenFiles) < limit {
		current := queue[0]
		queue = queue[1:]
		folderGUID, err := a.resolveTarget(current.target)
		if err != nil || seenFolders[folderGUID] {
			continue
		}
		seenFolders[folderGUID] = true
		rows, err := a.listFolder(current.target, limit-len(seenFiles))
		if err != nil {
			continue
		}
		for _, row := range rows {
			guid := str(row, "guid")
			if guid == "" || seenFiles[guid] {
				continue
			}
			if boolv(row, "isFolder") || boolv(row, "isSpace") {
				if current.depth < depth {
					queue = append(queue, item{target: guid, depth: current.depth + 1})
				}
				continue
			}
			seenFiles[guid] = true
			if !subscriptionType(str(row, "type")) {
				continue
			}
			subscribed, err := a.fileSubscription(guid)
			if err != nil || !subscribed {
				continue
			}
			row["subscription"] = true
			out = append(out, row)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return timeOf(out[i]["updatedAt"]) > timeOf(out[j]["updatedAt"])
	})
	return out, nil
}

func subscriptionType(typ string) bool {
	switch typ {
	case "document", "documentPro", "spreadsheet":
		return true
	default:
		return false
	}
}

func (a *App) fileSubscription(guid string) (bool, error) {
	r, err := a.authRequest("/files/"+url.PathEscape(guid)+"/update_subscription", "GET", nil, nil, nil)
	if err != nil {
		return false, err
	}
	if r.Status != 200 {
		return false, fmt.Errorf("读取订阅状态失败：HTTP %d", r.Status)
	}
	return boolv(asMap(r.Data), "subscribed"), nil
}

func selectSubscription(files []map[string]any, selector string) (map[string]any, error) {
	for _, f := range files {
		if str(f, "guid") == selector {
			return f, nil
		}
	}
	index, err := strconv.Atoi(selector)
	if err == nil && index >= 1 && index <= len(files) {
		return files[index-1], nil
	}
	return nil, errors.New("订阅文件序号或 guid 不存在，请先运行 shimo-net subscriptions --json")
}
