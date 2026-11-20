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
	if limit <= 0 || depth < 0 {
		return usageError("subscriptions 要求 --limit 大于 0、--depth 不小于 0")
	}
	scan := a.findSubscribedFiles(limit, depth)
	files := scan.Files
	out := map[string]any{"count": len(files), "limit": limit, "depth": depth, "files": files,
		"scanned": scan.Scanned, "truncated": scan.Truncated, "partial": len(scan.Failures) > 0,
		"complete": !scan.Truncated && len(scan.Failures) == 0, "failures": scan.Failures,
		"limitScope": "scanned-files", "orderingScope": "scanned-files",
		"note": "limit 是扫描文件数上限，不是返回条数；仅对扫描到的订阅文件按更新时间排序；最近更新正文需打开选中文件查看"}
	if len(scan.Failures) > 0 {
		first := scan.Failures[0]
		return &commandError{Message: fmt.Sprintf("订阅扫描部分失败（%d 项）：%s：%s；已扫描 %d 个文件，找到 %d 个订阅；结果不完整，未打开文件", len(scan.Failures), str(first, "stage"), str(first, "error"), scan.Scanned, len(files)), Result: out, ExitCode: 1, NoReplay: true}
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
	if a.JSON {
		return a.printJSON(out)
	}
	if len(files) == 0 {
		fmt.Printf("没有扫描到已订阅文件（已扫描 %d 个文件，截断=%t）。可提高 --limit 或 --depth 后重试。\n", scan.Scanned, scan.Truncated)
		return nil
	}
	fmt.Printf("已订阅文件：%d 个（已扫描 %d 个文件，截断=%t，仅在扫描范围内排序）\n\n", len(files), scan.Scanned, scan.Truncated)
	for i, f := range files {
		fmt.Printf("%d. %s [%s]\n   %s\n   最近更新：%s\n", i+1, terminalText(str(f, "name")), terminalText(typeLabel(f)), terminalText(str(f, "url")), terminalText(fmtTime(f["updatedAt"])))
	}
	fmt.Println("可使用上方链接；仅在用户授权打开本机浏览器后运行 shimo-net subscriptions --open <序号|guid>。")
	return nil
}

type subscriptionScan struct {
	Files     []map[string]any
	Scanned   int
	Truncated bool
	Failures  []map[string]any
}

func (a *App) findSubscribedFiles(limit, depth int) subscriptionScan {
	result := subscriptionScan{Files: []map[string]any{}, Failures: []map[string]any{}}
	failure := func(stage, target string, err error) {
		result.Failures = append(result.Failures, map[string]any{"stage": stage, "target": target, "error": err.Error()})
	}
	type item struct {
		target string
		depth  int
	}
	queue := []item{{target: "Desktop", depth: 0}}
	spaces, err := a.listSpaces(3000)
	if err != nil {
		failure("spaces", "", err)
		return result
	}
	// The space-list cap is a separate scan boundary and must not look complete.
	if len(spaces) >= 3000 {
		result.Truncated = true
	}
	for _, space := range spaces {
		queue = append(queue, item{target: str(space, "guid"), depth: 0})
	}
	seenFolders := map[string]bool{}
	seenFiles := map[string]bool{}
	for len(queue) > 0 && result.Scanned < limit {
		current := queue[0]
		queue = queue[1:]
		folderGUID, err := a.resolveTarget(current.target)
		if err != nil {
			failure("folder-target", current.target, err)
			continue
		}
		if seenFolders[folderGUID] {
			continue
		}
		seenFolders[folderGUID] = true
		complete, err := a.walkFolder(current.target, func(row map[string]any) bool {
			guid := str(row, "guid")
			if guid == "" {
				failure("file-entry", current.target, errors.New("文件列表条目缺少 guid"))
				return true
			}
			if boolv(row, "isFolder") || boolv(row, "isSpace") {
				if !seenFolders[guid] {
					if current.depth < depth {
						queue = append(queue, item{target: guid, depth: current.depth + 1})
					} else {
						result.Truncated = true
					}
				}
				return true
			}
			if seenFiles[guid] {
				return true
			}
			seenFiles[guid] = true
			result.Scanned++
			if subscriptionType(str(row, "type")) {
				subscribed, err := a.fileSubscription(guid)
				if err != nil {
					failure("subscription", guid, err)
				} else if subscribed {
					file := make(map[string]any, len(row)+1)
					for key, value := range row {
						file[key] = value
					}
					file["subscription"] = true
					result.Files = append(result.Files, file)
				}
			}
			return result.Scanned < limit
		})
		if err != nil {
			failure("folder", current.target, err)
		}
		if !complete {
			result.Truncated = true
		}
	}
	if len(queue) > 0 {
		result.Truncated = true
	}
	sort.SliceStable(result.Files, func(i, j int) bool {
		return timeOf(result.Files[i]["updatedAt"]) > timeOf(result.Files[j]["updatedAt"])
	})
	return result
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
