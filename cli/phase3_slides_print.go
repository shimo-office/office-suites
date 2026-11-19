package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// This command delivers the requested PDF. Parsing/rendering acceptance is
// deliberately a separate result, never inferred from a header or HTTP 200.
func (a *App) cmdPhase3SlidesPrint() (runErr error) {
	if a.opt("--payload") != "" {
		return fmt.Errorf("slide-print-pdf 当前只支持整套PDF；不接受未证实的范围/拼版参数")
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	if !phase3SameOrigin(target, a.Origin) {
		return fmt.Errorf("目标与登录origin不一致")
	}
	guid := guidFromURL(target)
	info, err := a.fileInfo(guid)
	if err != nil {
		return err
	}
	if str(info, "type") != "presentation" || boolv(info, "isCloudFile") {
		return fmt.Errorf("打印PDF仅接受在线幻灯片")
	}
	dir, err := phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	started := time.Now()
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	binary, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	record := map[string]any{"argv": os.Args, "targetGuid": guid, "startedUTC": started.UTC().Format(time.RFC3339), "startedShanghai": started.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format(time.RFC3339), "binarySHA256": fmt.Sprintf("%x", sha256.Sum256(binary))}
	defer func() {
		record["finishedUTC"] = time.Now().UTC().Format(time.RFC3339)
		record["exitCode"] = 0
		if runErr != nil {
			record["exitCode"] = 1
			_ = phase3WriteEvidence(filepath.Join(dir, "failure.json"), map[string]any{"ok": false, "mutationAttempted": false, "error": runErr.Error()})
		}
		_ = phase3WriteEvidence(filepath.Join(dir, "command-log.json"), record)
	}()
	before, err := a.phase3OfficeExport(guid, filepath.Join(dir, "before"), "pptx")
	if err != nil {
		return err
	}
	pages, err := phase3SlidePrintPages(before.Path)
	if err != nil {
		return err
	}
	visible, hidden := []string{}, []int{}
	for _, p := range pages {
		if p.Hidden {
			hidden = append(hidden, p.Index)
		} else {
			visible = append(visible, p.Text)
		}
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "expected-pages.json"), map[string]any{"pptxPageCount": len(pages), "visiblePageCount": len(visible), "pages": pages, "visibleTextInOrder": visible, "hiddenPagePolicy": "PDF export default excludes hidden pages on the verified deployment; actual PDF still requires independent verification"}); err != nil {
		return err
	}
	exported, err := a.exportFile(guid, "pdf")
	if err != nil {
		return err
	}
	if exported["type"] != "pdf" {
		return fmt.Errorf("导出服务没有返回PDF类型")
	}
	filename := filepath.Join(dir, "presentation.pdf")
	bytes, err := fetchToFile(exported["url"], filename)
	if err != nil {
		return err
	}
	if bytes < 128 {
		return fmt.Errorf("PDF下载为空或过短：%s", filename)
	}
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	header := make([]byte, 5)
	if _, err = io.ReadFull(f, header); err != nil {
		return err
	}
	if string(header) != "%PDF-" {
		return fmt.Errorf("导出响应不是PDF文件：%s", filename)
	}
	if _, err = f.Seek(0, 0); err != nil {
		return err
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, f); err != nil {
		return err
	}
	result := map[string]any{"ok": true, "action": "slide-print-pdf", "url": target, "exported": true, "verified": false, "status": "exported-not-verified", "pdf": filename, "bytes": bytes, "sha256": hex.EncodeToString(hash.Sum(nil)), "beforeArtifact": before, "evidenceDir": dir, "verificationScope": "PDF downloaded; page count, text and rendering require independent PDF parsing/rendering acceptance", "sourcePages": len(pages), "expectedVisiblePages": len(visible), "hiddenSlideIndices": hidden, "selection": "all visible slides; include-hidden and custom ranges are not supported"}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	if a.JSON {
		return a.printJSON(result)
	}
	fmt.Printf("PDF已生成：%s\n页数与逐页渲染尚待验收；证据：%s\n", filename, dir)
	return nil
}

type phase3SlidePrintPage struct {
	Index  int    `json:"slideIndex"`
	Text   string `json:"text"`
	Hidden bool   `json:"hidden"`
}

func phase3SlidePrintPages(filename string) ([]phase3SlidePrintPage, error) {
	z, entries, err := openArtifact(filename)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	root, err := artifactPart(entries, "ppt/presentation.xml", artifactSlideNS, "presentation")
	if err != nil {
		return nil, err
	}
	rels, err := artifactRelations(entries, "ppt/_rels/presentation.xml.rels", "ppt")
	if err != nil {
		return nil, err
	}
	list := root.child("sldIdLst")
	if list == nil {
		return nil, fmt.Errorf("PPTX缺少页面列表")
	}
	pages := []phase3SlidePrintPage{}
	for i, n := range list.children("sldId") {
		rel, ok := rels[relationID(n)]
		if !ok || rel.Kind != artifactOfficeRelNS+"/slide" {
			return nil, fmt.Errorf("页面关系无效")
		}
		slide, e := artifactPart(entries, rel.Target, artifactSlideNS, "sld")
		if e != nil {
			return nil, e
		}
		show := slide.attr("show")
		if show != "" && show != "1" && show != "0" && show != "true" && show != "false" {
			return nil, fmt.Errorf("未知页面隐藏状态")
		}
		pages = append(pages, phase3SlidePrintPage{Index: i + 1, Text: phase3SlideText(slide), Hidden: show == "0" || show == "false"})
	}
	return pages, nil
}
