package main

import (
	"fmt"
	"strconv"
	"strings"
)

func usageError(format string, args ...any) error {
	return &commandError{Message: fmt.Sprintf(format, args...), ExitCode: 2}
}

func isBooleanOption(name string) bool {
	switch name {
	case "--check", "--default", "--no-verify", "--no-browser", "--verify", "--new", "--login", "--no-save", "--once", "--keep-open", "--close":
		return true
	}
	return false
}

// Keep options scoped to their actual commands: a misspelled or misplaced
// option must fail before the dispatcher can perform a mutation.
func (a *App) validateOptions() error {
	options := map[string]string{
		"update": "--check", "init": "--origin --login", "login": "--origin --new",
		"config": "--origin", "session": "--no-save --no-verify --interval --source --expected-revision --once",
		"search": "--limit", "spaces": "--limit", "ls": "--limit --since",
		"upload": "--to", "import": "--to --title --share",
		"new": "--type --to", "mkdir": "--to", "audit": "--limit",
		"read": "--max-chars", "get": "--format --out", "pull": "--out --since --limit",
		"fav": "--note", "open": "--default",
		"subscriptions": "--limit --depth --open --default",
		"editor":        "--action --payload --screenshot --verify --no-verify --no-browser --keep-open --close",
		"phase2":        "--action --payload --screenshot --verify --no-verify --no-browser --keep-open --close",
		"phase3":        "--action --payload --read-range --out --kind --table --confirm-text --no-browser --verify",
	}
	allowed := " " + options[a.Cmd] + " --no-browser "
	for name := range a.Flags {
		if !strings.Contains(allowed, " "+name+" ") || isBooleanOption(name) {
			return usageError("命令 %s 不支持参数 %s；运行 shimo-net %s --help 查看用法", a.Cmd, name, a.Cmd)
		}
	}
	for name := range a.Bool {
		if !strings.Contains(allowed, " "+name+" ") {
			return usageError("命令 %s 不支持参数 %s", a.Cmd, name)
		}
	}
	return a.parseErr
}

func invalidShortOption(value string) bool {
	if !strings.HasPrefix(value, "-") || value == "-" {
		return false
	}
	_, err := strconv.ParseFloat(value, 64)
	return err != nil
}
