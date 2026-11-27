package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// An isolated browser has one export in flight. Do not retain signed URLs from
// CDP events; correlate completion to its download GUID, never a file extension.
type sheetPDFDownload struct {
	guid      string
	suggested string
	completed bool
	err       error
}

var sheetPDFGUID = regexp.MustCompile(`^[A-Za-z0-9-]{8,80}$`)

func (d *sheetPDFDownload) handle(event map[string]any) {
	p := asMap(event["params"])
	switch str(event, "method") {
	case "Browser.downloadWillBegin":
		guid := str(p, "guid")
		if !sheetPDFGUID.MatchString(guid) {
			d.err = errors.New("浏览器返回无效下载标识")
			return
		}
		if d.guid != "" && d.guid != guid {
			d.err = errors.New("出现多个下载，无法确认唯一PDF产物")
			return
		}
		d.guid, d.suggested = guid, str(p, "suggestedFilename")
	case "Browser.downloadProgress":
		if d.guid == "" || str(p, "guid") != d.guid {
			return
		}
		switch str(p, "state") {
		case "completed":
			d.completed = true
		case "canceled":
			d.err = errors.New("浏览器取消了PDF下载")
		}
	}
}

// The download is confirmed on disk instead of through a browser-level CDP
// session: the page-level session used by the editor consistently receives no
// Browser.downloadProgress, and a listener that never fires cannot be trusted as
// evidence. Envelope checks on the produced file are the only proof accepted.

// Verify the downloaded envelope, not page appearance. Page count and rendering
// are deliberately outside this check and must not be reported as verified.
func verifySheetPDF(path string) (int64, string, error) {
	stat, err := os.Lstat(path)
	if err != nil {
		return 0, "", err
	}
	if !stat.Mode().IsRegular() || stat.Size() < 100 {
		return 0, "", errors.New("下载的PDF为空、过短或不是普通文件")
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	header := make([]byte, 5)
	if _, err = io.ReadFull(f, header); err != nil || string(header) != "%PDF-" {
		return 0, "", errors.New("下载内容不是PDF；可能是错误页或未完成文件")
	}
	tailLen := int64(1024)
	if stat.Size() < tailLen {
		tailLen = stat.Size()
	}
	if _, err = f.Seek(-tailLen, io.SeekEnd); err != nil {
		return 0, "", err
	}
	tail, err := io.ReadAll(f)
	if err != nil {
		return 0, "", err
	}
	if !strings.HasSuffix(strings.TrimSpace(string(tail)), "%%EOF") {
		return 0, "", errors.New("PDF缺少完整结束标记，不能当作导出完成")
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return 0, "", err
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, f); err != nil {
		return 0, "", err
	}
	return stat.Size(), hex.EncodeToString(hash.Sum(nil)), nil
}

// Copy with an exclusive destination reservation. The download directory is
// temporary and owned by this invocation; no remote name can overwrite a file.
func publishSheetPDF(source, outDir, name string) (string, error) {
	name = sanitizeName(strings.TrimSpace(name))
	if !strings.EqualFold(filepath.Ext(name), ".pdf") {
		name += ".pdf"
	}
	if name == ".pdf" {
		name = "spreadsheet.pdf"
	}
	in, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer in.Close()
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	for i := 1; ; i++ {
		candidate := name
		if i > 1 {
			candidate = fmt.Sprintf("%s (%d).pdf", stem, i)
		}
		dest := filepath.Join(outDir, candidate)
		out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			if _, e := os.Lstat(dest); e == nil {
				continue
			}
			return "", err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(dest)
			if copyErr != nil {
				return "", copyErr
			}
			return "", closeErr
		}
		return dest, nil
	}
}
