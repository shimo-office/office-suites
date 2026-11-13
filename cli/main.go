package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const version = "0.9.1"

func main() {
	app := newApp(os.Args[1:])
	if err := app.Run(); err != nil {
		app.fail(err)
	}
}

type App struct {
	Origin       string
	OriginSource string
	JSON         bool
	Relogin      bool
	Cmd          string
	Pos          []string
	Flags        map[string]string
	Bool         map[string]bool
}

func newApp(args []string) *App {
	a := &App{Flags: map[string]string{}, Bool: map[string]bool{}}
	if len(args) == 0 {
		a.Cmd = "help"
	} else {
		a.Cmd = args[0]
	}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--json" {
			a.JSON = true
			continue
		}
		if arg == "--relogin" {
			a.Relogin = true
			continue
		}
		if arg == "--default" {
			a.Bool[arg] = true
			continue
		}
		if strings.HasPrefix(arg, "--") {
			name := arg
			val := ""
			if idx := strings.Index(arg, "="); idx >= 0 {
				name, val = arg[:idx], arg[idx+1:]
			}
			if val == "" && i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				val = args[i+1]
				i++
			}
			a.Flags[name] = val
			continue
		}
		a.Pos = append(a.Pos, arg)
	}
	a.Origin, a.OriginSource = resolveOrigin()
	return a
}

func (a *App) opt(name string) string { return a.Flags[name] }
func (a *App) has(name string) bool {
	if _, ok := a.Flags[name]; ok {
		return true
	}
	return a.Bool[name]
}

func (a *App) Run() error {
	switch a.Cmd {
	case "help", "--help", "-h":
		fmt.Print(helpText())
		return nil
	case "version", "--version", "-V":
		if a.JSON {
			return a.printJSON(map[string]any{"version": version})
		}
		fmt.Println("shimo-net " + version)
		return nil
	case "schema":
		return a.printJSON(schema())
	}
	fn, ok := dispatch(a)[a.Cmd]
	if !ok {
		fmt.Fprint(os.Stderr, helpText())
		os.Exit(2)
	}
	err := fn()
	if err != nil && a.Relogin && isAuthError(err) {
		if loginErr := a.cmdLogin(); loginErr != nil {
			return loginErr
		}
		return fn()
	}
	return err
}

func dispatch(a *App) map[string]func() error {
	return map[string]func() error{
		"init": a.cmdInit, "config": a.cmdConfig,
		"login": a.cmdLogin, "logout": a.cmdLogout,
		"whoami": a.cmdWhoami, "doctor": a.cmdDoctor,
		"search": a.cmdSearch, "spaces": a.cmdSpaces, "ls": a.cmdLs, "info": a.cmdInfo,
		"upload": a.cmdUpload, "import": a.cmdImport, "new": a.cmdNew, "rename": a.cmdRename,
		"share": a.cmdShare, "audit": a.cmdAudit, "delete": a.cmdDelete, "trash": a.cmdTrash,
		"restore": a.cmdRestore, "purge": a.cmdPurge, "read": a.cmdRead, "pull": a.cmdPull,
		"get": a.cmdGet, "fav": a.cmdFav, "open": a.cmdOpen, "phase2": a.cmdPhase2, "subscriptions": a.cmdSubscriptions,
	}
}

func (a *App) printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (a *App) fail(err error) {
	if a.JSON {
		_ = json.NewEncoder(os.Stderr).Encode(map[string]any{"ok": false, "error": err.Error()})
	} else {
		fmt.Fprintln(os.Stderr, "错误："+err.Error())
	}
	os.Exit(1)
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "401") || strings.Contains(s, "未登录") || strings.Contains(s, "登录已过期") || strings.Contains(s, "登录不可用")
}
