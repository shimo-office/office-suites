package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func (a *App) exportFile(guid, prefer string, fileType ...string) (map[string]string, error) {
	alts := map[string][]string{
		"document":     {"md", "docx", "pdf"},
		"documentPro":  {"docx", "pdf"},
		"spreadsheet":  {"xlsx", "pdf"},
		"presentation": {"pptx", "pdf"},
	}
	// Only default get/read exports opt into a type-specific fallback. Callers
	// that require one exact format (including verification exports) keep it.
	types := []string{prefer}
	if len(fileType) > 0 {
		for _, typ := range alts[fileType[0]] {
			if typ != prefer {
				types = append(types, typ)
			}
		}
	}
	last := ""
	for _, typ := range types {
		r, err := a.authRequest("/files/export/"+url.PathEscape(guid), "POST", map[string]any{"type": typ}, nil, nil)
		if err != nil {
			return nil, err
		}
		tid := nestedString(r.Data, "data", "taskId")
		if r.Status != 200 || tid == "" {
			last = snippet(r.Raw)
			continue
		}
		deadline := time.Now().Add(90 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(800 * time.Millisecond)
			p, err := a.authRequest("/files/export/progress", "POST", map[string]any{"taskId": tid}, nil, nil)
			if err != nil {
				return nil, err
			}
			d := asMap(asMap(p.Data)["data"])
			if num(d, "progress") >= 100 && str(d, "downloadUrl") != "" {
				return map[string]string{"type": typ, "url": str(d, "downloadUrl")}, nil
			}
		}
		last = "导出超时"
	}
	return nil, errors.New("导出失败：" + last)
}

func (a *App) downloadURL(guid string) (string, error) {
	r, err := a.authRequest("/files/"+url.PathEscape(guid)+"/download?requestResourceURL=true", "GET", nil, nil, nil)
	if err != nil {
		return "", err
	}
	if r.Status == 200 {
		if u := nestedString(r.Data, "resourceURL"); u != "" {
			return u, nil
		}
	}
	r2, err := a.authRequest("/files/"+url.PathEscape(guid)+"/download", "GET", nil, nil, nil)
	if err != nil {
		return "", err
	}
	if loc := r2.Header.Get("location"); loc != "" {
		return loc, nil
	}
	return "", fmt.Errorf("获取下载地址失败：HTTP %d", r.Status)
}

func fetchToFile(u, dest string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return 0, err
	}
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("下载失败：HTTP %d", resp.StatusCode)
	}
	f, err := os.CreateTemp(filepath.Dir(dest), ".shimo-download-*")
	if err != nil {
		return 0, err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return n, err
	}
	// Preserve an existing regular file's permissions without following symlinks.
	if previous, statErr := os.Lstat(dest); statErr == nil && previous.Mode().IsRegular() {
		if err := f.Chmod(previous.Mode().Perm()); err != nil {
			return n, err
		}
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return n, statErr
	}
	if err := f.Close(); err != nil {
		return n, err
	}
	if err := os.Rename(f.Name(), dest); err != nil {
		return n, err
	}
	return n, nil
}

func sanitizeName(name string) string {
	re := regexp.MustCompile(`[\\/:*?"<>|]`)
	s := re.ReplaceAllString(name, "_")
	s = strings.TrimLeft(s, ".")
	return strings.TrimSpace(s)
}

func (a *App) downloadOne(f map[string]any, outDir string) (map[string]any, error) {
	if boolv(f, "isFolder") || boolv(f, "isSpace") {
		return map[string]any{"skipped": true, "guid": str(f, "guid"), "name": str(f, "name"), "url": str(f, "url"), "reason": "仅下载当前层，不递归子目录"}, nil
	}
	guid := str(f, "guid")
	name := str(f, "name")
	if boolv(f, "isCloudFile") {
		u, err := a.downloadURL(guid)
		if err != nil {
			return nil, err
		}
		ext := extFromURL(u, strings.TrimPrefix(filepath.Ext(name), "."))
		if ext == "" {
			ext = "bin"
		}
		base := sanitizeName(strings.TrimSuffix(name, filepath.Ext(name)))
		dest := filepath.Join(outDir, base+"."+ext)
		n, err := fetchToFile(u, dest)
		if err != nil {
			return nil, err
		}
		return map[string]any{"name": name, "type": "cloud", "file": dest, "bytes": n, "url": str(f, "url")}, nil
	}
	prefer := map[string]string{"document": "md", "documentPro": "docx", "spreadsheet": "xlsx", "presentation": "pptx"}[str(f, "type")]
	if prefer == "" {
		prefer = "pdf"
	}
	exp, err := a.exportFile(guid, prefer, str(f, "type"))
	if err != nil {
		return nil, err
	}
	ext := exp["type"]
	dest := filepath.Join(outDir, sanitizeName(name)+"."+ext)
	n, err := fetchToFile(exp["url"], dest)
	if err != nil {
		return nil, err
	}
	return map[string]any{"name": name, "type": str(f, "type"), "exportType": exp["type"], "file": dest, "bytes": n, "url": str(f, "url")}, nil
}

func (a *App) cmdGet() error {
	if len(a.Pos) == 0 {
		return errors.New("用法：shimo-net get <链接|guid|@别名> [--out <目录>]")
	}
	if f := strings.TrimSpace(a.opt("--format")); f != "" {
		return fmt.Errorf("get 暂不支持 --format %s；当前按文件类型导出，表格导出 xlsx", f)
	}
	guid, err := a.resolveTarget(a.Pos[0])
	if err != nil {
		return err
	}
	info, err := a.fileInfo(guid)
	if err != nil {
		return err
	}
	outDir := a.opt("--out")
	if outDir == "" {
		outDir = "./shimo-pull"
	}
	outDir, _ = filepath.Abs(expandHome(outDir))
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	got, err := a.downloadOne(withGUID(info, guid), outDir)
	if err != nil {
		return err
	}
	if boolv(got, "skipped") {
		return errors.New("目标是目录：请用 shimo-net pull <目录> [--out <本地目录>]")
	}
	got["url"] = a.urlOf(withGUID(info, guid))
	if a.JSON {
		return a.printJSON(got)
	}
	fmt.Printf("已下载：%s（%v bytes）\n", str(got, "file"), got["bytes"])
	return nil
}

func (a *App) cmdPull() error {
	if len(a.Pos) == 0 {
		return errors.New("用法：shimo-net pull <文件夹/空间链接|guid|@别名> [--out <目录>] [--since 7d] [--limit N]")
	}
	outDir := a.opt("--out")
	if outDir == "" {
		outDir = "./shimo-pull"
	}
	outDir, _ = filepath.Abs(expandHome(outDir))
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	since, err := parseSince(a.opt("--since"))
	if err != nil {
		return err
	}
	files, err := a.listFolder(a.Pos[0], intArg(a.opt("--limit"), 0))
	if err != nil {
		return err
	}
	if since > 0 {
		filtered := files[:0]
		for _, f := range files {
			if boolv(f, "isFolder") || boolv(f, "isSpace") || timeOf(f["updatedAt"]) >= since {
				filtered = append(filtered, f)
			}
		}
		files = filtered
	}
	manifest := []map[string]any{}
	downloaded, failed, skipped := 0, 0, 0
	for _, f := range files {
		got, err := a.downloadOne(f, outDir)
		if err != nil {
			failed++
			if !a.JSON {
				fmt.Printf("✗ %s：%v\n", str(f, "name"), err)
			}
			manifest = append(manifest, map[string]any{"guid": str(f, "guid"), "name": str(f, "name"), "url": str(f, "url"), "error": err.Error()})
			continue
		}
		if boolv(got, "skipped") {
			skipped++
			manifest = append(manifest, got)
			if !a.JSON {
				fmt.Printf("- %s：%s\n", str(f, "name"), str(got, "reason"))
			}
			continue
		}
		downloaded++
		manifest = append(manifest, got)
		if !a.JSON {
			fmt.Printf("✓ %s → %s (%v bytes)\n", str(f, "name"), filepath.Base(str(got, "file")), got["bytes"])
		}
	}
	manifestPath := filepath.Join(outDir, "manifest.json")
	result := map[string]any{"ok": failed == 0, "target": a.Pos[0], "source": a.Pos[0], "dir": outDir, "since": a.opt("--since"), "count": len(manifest), "files": manifest,
		"recursive": false, "scope": "current-directory", "downloaded": downloaded, "failed": failed, "skipped": skipped, "manifest": manifestPath}
	b, _ := json.MarshalIndent(result, "", "  ")
	if err := os.WriteFile(manifestPath, append(b, '\n'), 0o644); err != nil {
		return err
	}
	if failed > 0 {
		return &commandError{Message: fmt.Sprintf("下载未全部完成：成功 %d、失败 %d、跳过目录 %d；清单：%s", downloaded, failed, skipped, manifestPath), Result: result, NoReplay: true}
	}
	if a.JSON {
		return a.printJSON(result)
	}
	fmt.Printf("\n当前层下载完成：%d 个文件，跳过 %d 个目录；位置：%s（清单：manifest.json）\n", downloaded, skipped, outDir)
	return nil
}

func (a *App) readText(guid string, maxChars int) (map[string]any, error) {
	if maxChars <= 0 {
		return nil, errors.New("--max-chars 必须是正整数")
	}
	info, err := a.fileInfo(guid)
	if err != nil {
		return nil, err
	}
	typ := str(info, "type")
	text := ""
	via := ""
	tmp, err := os.MkdirTemp("", "shimo-read-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	switch typ {
	case "document", "documentPro", "spreadsheet", "presentation":
		prefer := map[string]string{"document": "md", "documentPro": "docx", "spreadsheet": "xlsx", "presentation": "pptx"}[typ]
		exp, err := a.exportFile(guid, prefer, typ)
		if err != nil {
			return nil, err
		}
		if exp["type"] == "pdf" {
			return nil, errors.New("导出已回退为 PDF；当前不能解析 PDF 正文，请用 get 下载后读取，不将二进制当作正文")
		}
		fp := filepath.Join(tmp, "file."+exp["type"])
		if _, err := fetchToFile(exp["url"], fp); err != nil {
			return nil, err
		}
		switch exp["type"] {
		case "md":
			var body []byte
			body, err = os.ReadFile(fp)
			text = string(body)
		case "docx":
			text, err = extractDocx(fp)
		case "xlsx":
			text, err = extractXlsx(fp)
		case "pptx":
			text, err = extractPptx(fp)
		}
		if err != nil {
			return nil, err
		}
		via = "export " + exp["type"]
	default:
		if boolv(info, "isCloudFile") {
			u, err := a.downloadURL(guid)
			if err != nil {
				return nil, err
			}
			ext := extFromURL(u, strings.TrimPrefix(filepath.Ext(str(info, "name")), "."))
			fp := filepath.Join(tmp, "file."+ext)
			if _, err := fetchToFile(u, fp); err != nil {
				return nil, err
			}
			switch ext {
			case "md", "txt", "csv", "json", "html":
				b, err := os.ReadFile(fp)
				if err != nil {
					return nil, err
				}
				text = string(b)
			default:
				return nil, fmt.Errorf("云文件 .%s 不能直接读正文，可用 get 下载到本地", ext)
			}
			via = "download"
		} else {
			return nil, fmt.Errorf("不支持读取的类型：%s", typ)
		}
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	truncated := len([]rune(text)) > maxChars
	runes := []rune(text)
	if truncated {
		text = string(runes[:maxChars])
	}
	return map[string]any{"ok": true, "guid": guid, "name": str(info, "name"), "type": typ, "typeLabel": typeLabel(info),
		"url": a.urlOf(withGUID(info, guid)), "via": via, "chars": len(runes), "truncated": truncated, "text": text}, nil
}

func (a *App) cmdRead() error {
	if len(a.Pos) == 0 {
		return errors.New("用法：shimo-net read <链接|guid|@别名> [--max-chars 12000]")
	}
	guid, err := a.resolveTarget(a.Pos[0])
	if err != nil {
		return err
	}
	maxChars := 12000
	if a.has("--max-chars") {
		maxChars, err = strconv.Atoi(a.opt("--max-chars"))
		if err != nil || maxChars <= 0 {
			return errors.New("--max-chars 必须是正整数")
		}
	}
	r, err := a.readText(guid, maxChars)
	if err != nil {
		return err
	}
	if a.JSON {
		return a.printJSON(r)
	}
	fmt.Printf("《%s》（%s，%v 字%s）\n\n", str(r, "name"), str(r, "typeLabel"), r["chars"], map[bool]string{true: "，截断", false: ""}[boolv(r, "truncated")])
	fmt.Println(str(r, "text"))
	if boolv(r, "truncated") {
		fmt.Println("\n…（已截断，加 --max-chars 20000 看更多）")
	}
	return nil
}
