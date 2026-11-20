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
