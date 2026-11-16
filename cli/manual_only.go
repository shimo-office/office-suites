package main

import "fmt"

// These operations intentionally stay outside the CLI. They affect people,
// access control, or irreversible state and must be confirmed and performed
// by a human in the Shimo web UI.
var manualOnlyOperations = []map[string]string{
	{"name": "文件锁定", "instruction": "请在石墨网页中人工确认并操作"},
	{"name": "限制编辑", "instruction": "请在石墨网页中人工确认并操作"},
	{"name": "高级权限", "instruction": "请在石墨网页中人工确认并操作"},
	{"name": "成员级授权", "instruction": "请在石墨网页中人工确认并操作"},
	{"name": "彻底删除", "instruction": "请在石墨网页回收站中人工确认并操作"},
}

func manualOnlyError(operation string) error {
	return fmt.Errorf("%s禁止由 CLI 处理：请打开石墨网页，由有权限的人员人工确认并操作", operation)
}
