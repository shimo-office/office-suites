package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

func loadFav() map[string]map[string]any {
	out := map[string]map[string]any{}
	b, err := os.ReadFile(favPath())
	if err != nil {
		return out
	}
	var doc struct {
		Aliases map[string]map[string]any `json:"aliases"`
	}
	if json.Unmarshal(b, &doc) == nil && doc.Aliases != nil {
		return doc.Aliases
	}
	return out
}

func saveFav(aliases map[string]map[string]any) error {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(map[string]any{"aliases": aliases}, "", "  ")
	return os.WriteFile(favPath(), append(b, '\n'), 0o600)
}

func ensureAlias(name string) (string, error) {
	name = strings.TrimPrefix(strings.TrimSpace(name), "@")
	if name == "" {
		return "", errors.New("别名不能为空")
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_\p{Han}-]{1,40}$`).MatchString(name) {
		return "", errors.New("别名只能用中英文、数字、下划线、短横线，最长 40 字")
	}
	return name, nil
}

func (a *App) cmdFav() error {
	sub := "list"
	if len(a.Pos) > 0 {
		sub = a.Pos[0]
	}
	aliases := loadFav()
	switch sub {
	case "list", "ls":
		if a.JSON {
			return a.printJSON(map[string]any{"count": len(aliases), "aliases": aliases})
		}
		if len(aliases) == 0 {
			fmt.Println("还没有常用位置。加一个：shimo-net fav add <别名> <链接> [--note 特征]")
			return nil
		}
		fmt.Printf("常用位置（%d 个，命令里用 @别名）：\n\n", len(aliases))
		for k, v := range aliases {
			fmt.Printf("@%s  %s\n     %s\n", k, str(v, "name"), str(v, "url"))
			if str(v, "note") != "" {
				fmt.Printf("     特征：%s\n", str(v, "note"))
			}
		}
		return nil
	case "add":
		if len(a.Pos) < 3 {
			return errors.New("用法：shimo-net fav add <别名> <链接|guid> [--note 特征]")
		}
		name, err := ensureAlias(a.Pos[1])
		if err != nil {
			return err
		}
		guid, err := a.resolveTarget(a.Pos[2])
		if err != nil {
			return err
		}
		info, err := a.fileInfo(guid)
		if err != nil {
			return err
		}
		kind := str(info, "type")
		if boolv(info, "isSpace") {
			kind = "space"
		} else if boolv(info, "isFolder") {
			kind = "folder"
		}
		aliases[name] = map[string]any{"name": str(info, "name"), "url": a.urlOf(withGUID(info, guid)),
			"note": a.opt("--note"), "kind": kind, "savedAt": time.Now().Format("2006-01-02T15:04:05")}
		if err := saveFav(aliases); err != nil {
			return err
		}
		if a.JSON {
			out := map[string]any{"ok": true, "alias": "@" + name}
			for k, v := range aliases[name] {
				out[k] = v
			}
			return a.printJSON(out)
		}
		fmt.Printf("已记住 @%s → %s\n", name, str(aliases[name], "url"))
		if str(aliases[name], "note") != "" {
			fmt.Printf("特征：%s\n", str(aliases[name], "note"))
		}
		fmt.Printf("下次直接：shimo-net ls @%s\n", name)
		return nil
	case "rm", "remove", "del":
		if len(a.Pos) < 2 {
			return errors.New("用法：shimo-net fav rm <别名>")
		}
		name := strings.TrimPrefix(a.Pos[1], "@")
		if _, ok := aliases[name]; !ok {
			return errors.New("没有这个别名：" + name)
		}
		delete(aliases, name)
		if err := saveFav(aliases); err != nil {
			return err
		}
		if a.JSON {
			return a.printJSON(map[string]any{"ok": true, "removed": "@" + name})
		}
		fmt.Printf("已删除 @%s\n", name)
		return nil
	case "show":
		if len(a.Pos) < 2 {
			return errors.New("用法：shimo-net fav show <别名>")
		}
		name := strings.TrimPrefix(a.Pos[1], "@")
		v, ok := aliases[name]
		if !ok {
			return errors.New("没有这个别名：" + name)
		}
		if a.JSON {
			return a.printJSON(v)
		}
		fmt.Printf("@%s  %s\n", name, str(v, "url"))
		if str(v, "name") != "" {
			fmt.Printf("名称：%s\n", str(v, "name"))
		}
		if str(v, "note") != "" {
			fmt.Printf("特征：%s\n", str(v, "note"))
		}
		return nil
	default:
		return errors.New("用法：shimo-net fav <list|add|rm|show> …")
	}
}

func (a *App) cmdOpen() error {
	if len(a.Pos) == 0 {
		return errors.New("用法：shimo-net open <链接|guid|@别名>")
	}
	raw := a.Pos[0]
	target := raw
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		guid, err := a.resolveTarget(raw)
		if err != nil {
			return err
		}
		if guid == "Desktop" {
			return errors.New("桌面没有可打开的链接")
		}
		info, err := a.fileInfo(guid)
		if err != nil {
			return err
		}
		target = a.urlOf(withGUID(info, guid))
	}
	chrome, err := findChrome()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(profilePath(), 0o700); err != nil {
		return err
	}
	cmd := exec.Command(chrome, "--user-data-dir="+profilePath(), target)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		return err
	}
	o := map[string]any{"ok": true, "url": target, "profile": profilePath(), "note": "用已登录的专用 Chrome 配置打开；如未登录会跳到登录页"}
	if a.JSON {
		return a.printJSON(o)
	}
	fmt.Printf("已用登录态浏览器打开：%s\n", target)
	return nil
}

func (a *App) cmdPurge() error {
	return errors.New("彻底删除不在 CLI 支持范围：请到石墨网页的「回收站」里手动删除。CLI 只提供 delete（进回收站）和 restore（恢复）。")
}
