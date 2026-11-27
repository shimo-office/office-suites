package main

import (
	"errors"
	"fmt"
)

// The native form editor initializes its title from the drive file name on
// every open. Persist both parts of that product contract before fresh-profile
// verification. Never replay the draft batch to repair a failed rename.
func (a *App) phase3SyncFormTitle(guid, baselineName, title string) (map[string]any, error) {
	proof := map[string]any{"requested": title, "beforeFileName": baselineName, "mutationAttempted": false}
	info, err := a.fileInfo(guid)
	if err != nil {
		return proof, err
	}
	if str(info, "guid") != "" && str(info, "guid") != guid {
		return proof, errors.New("标题同步文件身份不匹配")
	}
	if str(info, "type") != "form" || boolv(info, "isCloudFile") {
		return proof, errors.New("标题同步目标不是在线表单")
	}
	name := str(info, "name")
	if baselineName != "" && name != baselineName && name != title {
		return proof, errors.New("文件名在批次执行期间已变化；未覆盖并发改名，请只读核对，不要重放批次")
	}
	if name == "" {
		// Without a readable file name the rename cannot be proven; report the
		// gap instead of renaming blindly or claiming the title will survive a
		// reopen. The form title itself was already written by the batch.
		proof["observedFileName"] = ""
		proof["verified"] = false
		proof["warning"] = "文件名不可读：重开表单时标题可能回退为文件名；请在页面上核对标题与文件名"
		return proof, nil
	}
	if name != title {
		proof["mutationAttempted"] = true
		if _, err = a.renameFile(guid, title); err != nil {
			return proof, err
		}
	}
	after, err := a.fileInfo(guid)
	if err != nil {
		return proof, err
	}
	proof["observedFileName"] = str(after, "name")
	if str(after, "name") != title {
		return proof, fmt.Errorf("表单草稿已保存，但文件名读回不等于请求标题；禁止重放批次")
	}
	proof["verified"] = true
	return proof, nil
}
