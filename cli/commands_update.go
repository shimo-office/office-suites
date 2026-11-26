package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const updateReleaseURL = "https://api.github.com/repos/shimo-office/office-suites/releases/latest"
const updateMaxBinary int64 = 128 << 20

// Update networking is deliberately separate from the authenticated Shimo client.
type updateRuntime struct {
	current, goos, goarch string
	client                *http.Client
	executable            func() (string, error)
	smoke                 func(string, string) error
	replace               func(string, string) (string, error)
}
type updateRelease struct {
	Tag        string        `json:"tag_name"`
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
	Assets     []updateAsset `json:"assets"`
}
type updateAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}
type updateResult struct {
	OK              bool   `json:"ok"`
	Source          string `json:"source"`
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	Updated         bool   `json:"updated"`
	CheckOnly       bool   `json:"checkOnly"`
	Path            string `json:"path"`
	BackupPath      string `json:"backupPath,omitempty"`
}

func (a *App) cmdUpdate() error {
	if len(a.Pos) != 0 {
		return usageError("用法：shimo-net update [--check] [--json]")
	}
	u := updateRuntime{current: version, goos: runtime.GOOS, goarch: runtime.GOARCH, executable: os.Executable, smoke: smokeUpdateBinary, replace: replaceUpdateBinary, client: &http.Client{Timeout: 2 * time.Minute, CheckRedirect: updateRedirect}}
	result, err := u.run(a.has("--check"))
	if err != nil {
		return err
	}
	if a.JSON {
		return a.printJSON(result)
	}
	switch {
	case result.Updated:
		fmt.Printf("已更新 shimo-net：%s → %s\n", result.CurrentVersion, result.LatestVersion)
	case result.UpdateAvailable:
		fmt.Printf("发现新版本：%s → %s；运行 shimo-net update 安装\n", result.CurrentVersion, result.LatestVersion)
	default:
		fmt.Printf("无需更新：当前 %s，最新发布版 %s\n", result.CurrentVersion, result.LatestVersion)
	}
	if result.BackupPath != "" {
		fmt.Printf("旧程序保留在 %s；当前进程退出后可删除\n", result.BackupPath)
	}
	return nil
}

func updateRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return fmt.Errorf("更新下载重定向过多")
	}
	host := req.URL.Hostname()
	if req.URL.Scheme != "https" || req.URL.User != nil || (req.URL.Port() != "" && req.URL.Port() != "443") || (host != "github.com" && host != "api.github.com" && host != "release-assets.githubusercontent.com" && host != "objects.githubusercontent.com") {
		return fmt.Errorf("更新下载拒绝非官方 HTTPS 地址")
	}
	return nil
}

func (u updateRuntime) run(check bool) (updateResult, error) {
	result := updateResult{OK: true, Source: "github", CurrentVersion: u.current, CheckOnly: check}
	path, err := u.executable()
	if err != nil {
		return result, fmt.Errorf("定位当前程序：%w", err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return result, fmt.Errorf("解析当前程序：%w", err)
	}
	result.Path = path
	if isNPMUpdateBinary(path) {
		return result, fmt.Errorf("此程序由 npm 管理；请通过 npm 安装的 shimo-net update 入口运行，或执行 npm install -g shimo-net-cli@latest --registry=https://registry.npmjs.org")
	}
	assetName, err := updateAssetName(u.goos, u.goarch)
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	body, err := u.get(ctx, updateReleaseURL, 2<<20)
	if err != nil {
		return result, err
	}
	var release updateRelease
	if err = json.Unmarshal(body, &release); err != nil {
		return result, fmt.Errorf("无法解析最新版本：%w", err)
	}
	if release.Draft || release.Prerelease || !strings.HasPrefix(release.Tag, "v") {
		return result, fmt.Errorf("最新发布信息不是稳定版本")
	}
	latest := strings.TrimPrefix(release.Tag, "v")
	comparison, err := compareUpdateVersions(latest, u.current)
	if err != nil {
		return result, err
	}
	result.LatestVersion = latest
	result.UpdateAvailable = comparison > 0
	if comparison <= 0 {
		return result, nil
	}
	asset, err := release.asset(assetName)
	if err != nil {
		return result, err
	}
	if check {
		return result, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return result, err
	}
	if !info.Mode().IsRegular() {
		return result, fmt.Errorf("当前程序不是普通文件")
	}
	// A per-target lock prevents two updater processes replacing one another.
	lock, err := os.OpenFile(path+".update.lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return result, fmt.Errorf("无法锁定更新目标（检查目录写权限或已有 .update.lock）：%w", err)
	}
	lock.Close()
	defer os.Remove(path + ".update.lock")
	staged, err := u.stage(ctx, path, asset, info.Mode().Perm())
	if err != nil {
		return result, err
	}
	defer os.Remove(staged)
	if err = u.smoke(staged, latest); err != nil {
		return result, fmt.Errorf("新版程序校验失败，保留原版本：%w", err)
	}
	result.BackupPath, err = u.replace(staged, path)
	if err != nil {
		return result, fmt.Errorf("替换程序失败：%w", err)
	}
	result.Updated = true
	return result, nil
}

func updateAssetName(goos, goarch string) (string, error) {
	if (goos != "darwin" && goos != "linux" && goos != "windows") || (goarch != "amd64" && goarch != "arm64") {
		return "", fmt.Errorf("不支持自动更新的平台：%s/%s", goos, goarch)
	}
	name := "shimo-net_" + goos + "_" + goarch
	if goos == "windows" {
		name += ".exe"
	}
	return name, nil
}
func isNPMUpdateBinary(path string) bool {
	dir := filepath.Dir(path)
	if filepath.Base(dir) != "binaries" {
		return false
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(dir), "package.json"))
	if err != nil {
		return false
	}
	var pkg struct {
		Name string `json:"name"`
	}
	return json.Unmarshal(data, &pkg) == nil && pkg.Name == "shimo-net-cli"
}
func (r updateRelease) asset(name string) (updateAsset, error) {
	var found updateAsset
	for _, asset := range r.Assets {
		if asset.Name == name {
			if found.Name != "" {
				return found, fmt.Errorf("发布包含重复程序：%s", name)
			}
			found = asset
		}
	}
	expected := "https://github.com/shimo-office/office-suites/releases/download/" + r.Tag + "/" + name
	if found.Name == "" || found.URL != expected {
		return found, fmt.Errorf("发布缺少官方平台程序：%s", name)
	}
	digest, err := hex.DecodeString(strings.TrimPrefix(found.Digest, "sha256:"))
	if !strings.HasPrefix(found.Digest, "sha256:") || err != nil || len(digest) != sha256.Size {
		return found, fmt.Errorf("发布缺少有效 SHA-256 校验值；未修改原程序")
	}
	if found.Size <= 0 || found.Size > updateMaxBinary {
		return found, fmt.Errorf("发布程序大小无效")
	}
	return found, nil
}
func compareUpdateVersions(a, b string) (int, error) {
	parse := func(v string) ([3]uint64, error) {
		var out [3]uint64
		parts := strings.Split(v, ".")
		if len(parts) != 3 {
			return out, fmt.Errorf("无效的稳定版本号：%q", v)
		}
		for i, part := range parts {
			if part == "" || (len(part) > 1 && part[0] == '0') {
				return out, fmt.Errorf("无效的稳定版本号：%q", v)
			}
			for _, c := range part {
				if c < '0' || c > '9' {
					return out, fmt.Errorf("无效的稳定版本号：%q", v)
				}
			}
			number, err := strconv.ParseUint(part, 10, 64)
			if err != nil {
				return out, fmt.Errorf("无效的稳定版本号：%q", v)
			}
			out[i] = number
		}
		return out, nil
	}
	av, err := parse(a)
	if err != nil {
		return 0, err
	}
	bv, err := parse(b)
	if err != nil {
		return 0, err
	}
	for i := range av {
		if av[i] > bv[i] {
			return 1, nil
		}
		if av[i] < bv[i] {
			return -1, nil
		}
	}
	return 0, nil
}

func (u updateRuntime) response(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "shimo-net/"+u.current)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取更新失败：%w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("获取更新失败：HTTP %d", resp.StatusCode)
	}
	return resp, nil
}
func (u updateRuntime) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	resp, err := u.response(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("更新响应超过大小限制")
	}
	return data, nil
}
func (u updateRuntime) stage(ctx context.Context, target string, asset updateAsset, mode os.FileMode) (string, error) {
	// Keep the executable extension on Windows so the smoke test can launch it.
	pattern := ".shimo-net-update-*"
	if u.goos == "windows" {
		pattern += ".exe"
	}
	file, err := os.CreateTemp(filepath.Dir(target), pattern)
	if err != nil {
		return "", err
	}
	name := file.Name()
	keep := false
	defer func() {
		file.Close()
		if !keep {
			os.Remove(name)
		}
	}()
	resp, err := u.response(ctx, asset.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, asset.Size+1))
	if err != nil {
		return "", err
	}
	if n != asset.Size {
		return "", fmt.Errorf("下载大小不符；未修改原程序")
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), strings.TrimPrefix(asset.Digest, "sha256:")) {
		return "", fmt.Errorf("下载 SHA-256 不符；未修改原程序")
	}
	if err = file.Chmod(mode); err != nil {
		return "", err
	}
	if err = file.Sync(); err != nil {
		return "", err
	}
	if err = file.Close(); err != nil {
		return "", err
	}
	keep = true
	return name, nil
}
func smokeUpdateBinary(path, expected string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "version", "--json")
	// Limit even the verified binary's output; version must be one small JSON object.
	var output updateOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return err
	}
	var result struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(output.data, &result); err != nil {
		return err
	}
	if result.Version != expected {
		return fmt.Errorf("版本号不符：%q，期望 %q", result.Version, expected)
	}
	return nil
}

type updateOutput struct{ data []byte }

func (w *updateOutput) Write(p []byte) (int, error) {
	if len(w.data)+len(p) > 4096 {
		return 0, fmt.Errorf("版本输出过大")
	}
	w.data = append(w.data, p...)
	return len(p), nil
}

func replaceUpdateBinary(staged, target string) (string, error) {
	if runtime.GOOS != "windows" {
		return "", os.Rename(staged, target)
	}
	return replaceWindowsUpdateBinary(staged, target)
}

func replaceWindowsUpdateBinary(staged, target string) (string, error) {
	// Windows cannot overwrite a running executable. Rename it out of the way,
	// restore it on failure, and retain its path if it cannot yet be deleted.
	backup, err := os.CreateTemp(filepath.Dir(target), ".shimo-net-previous-*.exe")
	if err != nil {
		return "", err
	}
	name := backup.Name()
	if err = backup.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	if err = os.Remove(name); err != nil {
		return "", err
	}
	if err = os.Rename(target, name); err != nil {
		return "", err
	}
	if err = os.Rename(staged, target); err != nil {
		if rollback := os.Rename(name, target); rollback != nil {
			return name, fmt.Errorf("%w；恢复失败 %v；旧程序保留在 %s", err, rollback, name)
		}
		return "", err
	}
	if err = os.Remove(name); err != nil {
		return name, nil
	}
	return "", nil
}
