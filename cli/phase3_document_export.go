package main

import (
	"fmt"
	"os"
)

func (a *App) phase3OfficeExport(guid, dir, kind string) (ArtifactEvidence, error) {
	info, err := a.fileInfo(guid)
	if err != nil {
		return ArtifactEvidence{}, err
	}
	want := map[string]string{"docx": "documentPro", "pptx": "presentation"}[kind]
	if want == "" || str(info, "type") != want || boolv(info, "isCloudFile") {
		// 文档（轻文档）与文稿是两种类型：原地改写的前后证据都建立在 DOCX 导出上，
		// 只有文稿能导出 DOCX。这里直接说清楚，避免用户以为是 CLI 不支持改文档。
		if kind == "docx" && str(info, "type") == "document" {
			return ArtifactEvidence{}, fmt.Errorf("目标是文档（轻文档），不在原地改写范围：文档只能读取、导出或另存，原地改写动作仅对文稿（Word 类在线文档）生效")
		}
		return ArtifactEvidence{}, fmt.Errorf("目标类型不符 %s", kind)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return ArtifactEvidence{}, err
	}
	got, err := a.downloadOne(withGUID(info, guid), dir, kind)
	if err != nil {
		return ArtifactEvidence{}, err
	}
	if str(got, "exportType") != kind || str(got, "file") == "" {
		return ArtifactEvidence{}, fmt.Errorf("未取得 %s 导出文件", kind)
	}
	return verifyArtifact(str(got, "file"), kind, nil)
}
