package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

const version = "0.9.21"

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
	parseErr     error
}

func newApp(args []string) *App {
	a := &App{Flags: map[string]string{}, Bool: map[string]bool{}}
	a.Cmd = "help"
	// Help is a side-effect-free request, even after a command or a missing
	// option value. An explicit -- terminates options and permits literal text.
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--help" || arg == "-h" {
			return a
		}
	}
	commandSeen := false
	literal := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if literal {
			if !commandSeen {
				a.Cmd, commandSeen = arg, true
			} else {
				a.Pos = append(a.Pos, arg)
			}
			continue
		}
		if arg == "--" {
			literal = true
			continue
		}
		if arg == "--json" {
			a.JSON = true
			continue
		}
		if arg == "--relogin" {
			a.Relogin = true
			continue
		}
		if !commandSeen {
			a.Cmd, commandSeen = arg, true
			continue
		}
		if isBooleanOption(arg) {
			a.Bool[arg] = true
			continue
		}
		if strings.HasPrefix(arg, "--") {
			name := arg
			val := ""
			hasEquals := strings.Contains(arg, "=")
			if idx := strings.Index(arg, "="); idx >= 0 {
				name, val = arg[:idx], arg[idx+1:]
			}
			if !hasEquals && i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				val = args[i+1]
				i++
			}
			if !hasEquals && val == "" && a.parseErr == nil {
				a.parseErr = usageError("参数 %s 缺少值", name)
			}
			a.Flags[name] = val
			continue
		}
		if invalidShortOption(arg) && a.parseErr == nil {
			a.parseErr = usageError("未知参数 %s；运行 shimo-net --help 查看用法", arg)
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
	if err := a.validateOptions(); err != nil {
		return err
	}
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
		return &commandError{Message: fmt.Sprintf("未知命令 %q；运行 shimo-net --help 查看用法", a.Cmd), ExitCode: 2}
	}
	err := fn()
	// Phase3 may fail during readback/export after a mutation was accepted.
	// Replaying the whole command can duplicate appends or delete another row.
	if a.canRelogin(err) {
		login := newApp([]string{"login", "--new", "--origin", a.Origin})
		if loginErr := login.cmdLogin(); loginErr != nil {
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
		"get": a.cmdGet, "fav": a.cmdFav, "open": a.cmdOpen, "editor": a.cmdPhase2, "phase2": a.cmdPhase2, "subscriptions": a.cmdSubscriptions,
		"phase3": a.cmdPhase3,
		"mkdir":  a.cmdMkdir, "session": a.cmdSession,
	}
}

func (a *App) printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (a *App) fail(err error) {
	os.Exit(a.writeFailure(err))
}

func (a *App) writeFailure(err error) int {
	code := 1
	result := map[string]any{}
	var detail *commandError
	if errors.As(err, &detail) {
		for k, v := range detail.Result {
			result[k] = v
		}
		if detail.ExitCode > 0 {
			code = detail.ExitCode
		}
	}
	result["ok"], result["error"] = false, err.Error()
	if a.JSON {
		_ = json.NewEncoder(os.Stderr).Encode(result)
	} else {
		fmt.Fprintln(os.Stderr, "错误："+terminalText(err.Error()))
	}
	return code
}

func (a *App) canRelogin(err error) bool {
	var detail *commandError
	if err == nil || !a.Relogin || explicitNoBrowser(a) || !isAuthError(err) || (errors.As(err, &detail) && detail.NoReplay) {
		return false
	}
	// Only read-only commands may restart after an explicitly requested fresh
	// login. Never automatically repeat a mutation, even outside phase3.
	switch a.Cmd {
	case "whoami", "doctor", "search", "spaces", "ls", "info", "audit", "trash", "read", "get", "pull", "subscriptions":
		return true
	case "editor", "phase2":
		return a.opt("--action") == "sheet-values" || a.opt("--action") == "mentions-list" || a.opt("--action") == "comments-list"
	}
	return false
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "401") || strings.Contains(s, "未登录") || strings.Contains(s, "登录已过期") || strings.Contains(s, "登录不可用")
}
