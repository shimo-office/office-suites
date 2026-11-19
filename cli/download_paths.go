package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Reserve the final name atomically, so another download (or an existing file,
// directory or symlink) cannot silently lose its place to this download.
func fetchToUniqueFile(u, outDir, filename string) (string, int64, error) {
	// Sanitize the assembled name too: a cloud URL without an extension can
	// leave an untrusted suffix (including NTFS stream syntax) after the base.
	filename = sanitizeName(strings.TrimSpace(filename))
	if strings.Trim(filename, ". ") == "" {
		filename = "download"
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", 0, err
	}
	ext := filepath.Ext(filename)
	stem := strings.TrimSuffix(filename, ext)
	for sequence := 1; ; sequence++ {
		candidate := filename
		if sequence > 1 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, sequence, ext)
		}
		dest := filepath.Join(outDir, candidate)
		reservation, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			// Windows can report EISDIR/access denied for an occupied directory
			// instead of EEXIST. Never reopen it for writing; try another name.
			if _, statErr := os.Lstat(dest); statErr == nil {
				continue
			}
			return "", 0, err
		}
		reserved, statErr := reservation.Stat()
		closeErr := reservation.Close()
		cleanup := func() {
			current, e := os.Lstat(dest)
			if e == nil && reserved != nil && current.Mode().IsRegular() && current.Size() == 0 && os.SameFile(reserved, current) {
				_ = os.Remove(dest)
			}
		}
		if statErr != nil {
			cleanup()
			return "", 0, statErr
		}
		if closeErr != nil {
			cleanup()
			return "", 0, closeErr
		}
		n, err := fetchToFile(u, dest)
		if err != nil {
			cleanup()
			return "", n, err
		}
		return dest, n, nil
	}
}

func verifyDownloadedFile(file map[string]any) error {
	info, err := os.Lstat(str(file, "file"))
	if err != nil {
		return fmt.Errorf("下载后核验失败：%w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("下载后核验失败：目标不是普通文件")
	}
	expected, ok := file["bytes"].(int64)
	if !ok || expected < 0 {
		return errors.New("下载后核验失败：下载字节数无效")
	}
	if info.Size() != expected {
		return fmt.Errorf("下载后核验失败：文件大小为 %d 字节，下载记录为 %d 字节", info.Size(), expected)
	}
	return nil
}
