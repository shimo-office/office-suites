package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type updateTestTransport func(*http.Request) (*http.Response, error)

func (f updateTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func updateFixture(t *testing.T) (updateRuntime, *updateRelease, *int) {
	t.Helper()
	target := filepath.Join(t.TempDir(), "shimo-net")
	if err := os.WriteFile(target, []byte("old binary"), 0751); err != nil {
		t.Fatal(err)
	}
	payload := "new binary"
	digest := sha256.Sum256([]byte(payload))
	release := &updateRelease{Tag: "v1.2.4", Assets: []updateAsset{{Name: "shimo-net_linux_amd64", URL: "https://github.com/shimo-office/office-suites/releases/download/v1.2.4/shimo-net_linux_amd64", Digest: "sha256:" + hex.EncodeToString(digest[:]), Size: int64(len(payload))}}}
	requests := new(int)
	u := updateRuntime{current: "1.2.3", goos: "linux", goarch: "amd64", executable: func() (string, error) { return target, nil }, smoke: func(path, version string) error {
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		if string(b) != payload || version != "1.2.4" {
			return errors.New("bad smoke")
		}
		return nil
	}, replace: replaceUpdateBinary}
	u.client = &http.Client{Transport: updateTestTransport(func(r *http.Request) (*http.Response, error) {
		*requests++
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Fatal("credentials leaked")
		}
		body := payload
		if r.URL.String() == updateReleaseURL {
			b, _ := json.Marshal(release)
			body = string(b)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	return u, release, requests
}

func TestUpdateSuccessAndCheck(t *testing.T) {
	for _, check := range []bool{false, true} {
		t.Run(fmt.Sprint(check), func(t *testing.T) {
			u, _, requests := updateFixture(t)
			result, err := u.run(check)
			if err != nil {
				t.Fatal(err)
			}
			path, _ := u.executable()
			b, _ := os.ReadFile(path)
			want := "new binary"
			if check {
				want = "old binary"
			}
			if string(b) != want || result.Updated == check || !result.UpdateAvailable || result.LatestVersion != "1.2.4" {
				t.Fatalf("result=%+v file=%q", result, b)
			}
			if check && *requests != 1 {
				t.Fatalf("check downloaded binary: %d requests", *requests)
			}
			entries, _ := os.ReadDir(filepath.Dir(path))
			if len(entries) != 1 {
				t.Fatalf("temporary files: %v", entries)
			}
		})
	}
}

func TestUpdateNoDowngrade(t *testing.T) {
	for _, tag := range []string{"v1.2.3", "v1.2.2"} {
		u, r, requests := updateFixture(t)
		r.Tag = tag
		result, err := u.run(false)
		if err != nil {
			t.Fatal(err)
		}
		if result.Updated || result.UpdateAvailable || *requests != 1 {
			t.Fatalf("%+v requests=%d", result, *requests)
		}
	}
}

func TestUpdateFailurePreservesOriginal(t *testing.T) {
	cases := []struct {
		name   string
		change func(*updateRuntime, *updateRelease)
	}{
		{"hash", func(u *updateRuntime, r *updateRelease) { r.Assets[0].Digest = "sha256:" + strings.Repeat("0", 64) }},
		{"missing digest", func(u *updateRuntime, r *updateRelease) { r.Assets[0].Digest = "" }},
		{"truncated", func(u *updateRuntime, r *updateRelease) { r.Assets[0].Size++ }},
		{"prerelease", func(u *updateRuntime, r *updateRelease) { r.Prerelease = true }},
		{"draft", func(u *updateRuntime, r *updateRelease) { r.Draft = true }},
		{"invalid version", func(u *updateRuntime, r *updateRelease) { r.Tag = "v1.2.4;bad" }},
		{"foreign url", func(u *updateRuntime, r *updateRelease) { r.Assets[0].URL = "https://evil.example/binary" }},
		{"missing asset", func(u *updateRuntime, r *updateRelease) { r.Assets = nil }},
		{"duplicate asset", func(u *updateRuntime, r *updateRelease) { r.Assets = append(r.Assets, r.Assets[0]) }},
		{"huge asset", func(u *updateRuntime, r *updateRelease) { r.Assets[0].Size = updateMaxBinary + 1 }},
		{"platform", func(u *updateRuntime, r *updateRelease) { u.goarch = "386" }},
		{"network", func(u *updateRuntime, r *updateRelease) {
			u.client = &http.Client{Transport: updateTestTransport(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })}
		}},
		{"smoke", func(u *updateRuntime, r *updateRelease) {
			u.smoke = func(string, string) error { return errors.New("wrong version") }
		}},
		{"replace", func(u *updateRuntime, r *updateRelease) {
			u.replace = func(string, string) (string, error) { return "", errors.New("permission denied") }
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, r, _ := updateFixture(t)
			tc.change(&u, r)
			if _, err := u.run(false); err == nil {
				t.Fatal("expected failure")
			}
			path, _ := u.executable()
			b, _ := os.ReadFile(path)
			if string(b) != "old binary" {
				t.Fatalf("old changed: %q", b)
			}
			entries, _ := os.ReadDir(filepath.Dir(path))
			if len(entries) != 1 {
				t.Fatalf("leftovers: %v", entries)
			}
		})
	}
}

func TestUpdateVersionComparison(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{{"1.10.0", "1.9.9", 1}, {"2.0.0", "10.0.0", -1}, {"1.2.3", "1.2.3", 0}} {
		got, err := compareUpdateVersions(tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Fatalf("%+v got %d %v", tc, got, err)
		}
	}
	for _, v := range []string{"", "v1.2.3", "1.2", "1.2.3-beta", "01.2.3", "1.2.3/../../bad", "999999999999999999999.0.0"} {
		if _, err := compareUpdateVersions(v, "1.0.0"); err == nil {
			t.Fatalf("accepted %q", v)
		}
	}
}

func TestUpdateRejectsNPMPackage(t *testing.T) {
	u, _, requests := updateFixture(t)
	root := t.TempDir()
	dir := filepath.Join(root, "binaries")
	os.Mkdir(dir, 0755)
	os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"shimo-net-cli"}`), 0644)
	target := filepath.Join(dir, "shimo-net_linux_amd64")
	os.WriteFile(target, []byte("npm binary"), 0755)
	u.executable = func() (string, error) { return target, nil }
	if _, err := u.run(false); err == nil || !strings.Contains(err.Error(), "npm") {
		t.Fatalf("error=%v", err)
	}
	if *requests != 0 {
		t.Fatal("npm attempted network")
	}
}

func TestUpdateArgumentValidation(t *testing.T) {
	for _, args := range [][]string{{"update", "extra"}, {"update", "--check=true"}, {"update", "--force"}, {"update", "--origin", "https://example.com"}} {
		if err := newApp(args).Run(); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	a := newApp([]string{"update", "--check", "--json"})
	if err := a.validateOptions(); err != nil || !a.has("--check") || !a.JSON {
		t.Fatalf("parse: %+v %v", a, err)
	}
}

func TestUpdateHTTPFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{{"status", 403, "forbidden"}, {"malformed", 200, "{"}, {"oversized", 200, strings.Repeat("x", (2<<20)+1)}} {
		t.Run(tc.name, func(t *testing.T) {
			u, _, _ := updateFixture(t)
			u.client = &http.Client{Transport: updateTestTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			if _, err := u.run(false); err == nil {
				t.Fatal("expected failure")
			}
			path, _ := u.executable()
			b, _ := os.ReadFile(path)
			if string(b) != "old binary" {
				t.Fatal("modified original")
			}
		})
	}
}

func TestUpdateRedirectBoundary(t *testing.T) {
	for _, url := range []string{"http://github.com/a", "https://evil.example/a", "https://github.com.evil.example/a", "https://user@github.com/a", "https://github.com:444/a"} {
		req, _ := http.NewRequest("GET", url, nil)
		if err := updateRedirect(req, nil); err == nil {
			t.Fatalf("accepted %s", url)
		}
	}
	for _, host := range []string{"github.com", "api.github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com"} {
		req, _ := http.NewRequest("GET", "https://"+host+"/asset", nil)
		if err := updateRedirect(req, nil); err != nil {
			t.Fatal(err)
		}
		if err := updateRedirect(req, make([]*http.Request, 5)); err == nil {
			t.Fatal("redirect loop allowed")
		}
	}
}

func TestUpdateSymlinkAndLock(t *testing.T) {
	u, _, _ := updateFixture(t)
	target, _ := u.executable()
	link := filepath.Join(t.TempDir(), "shimo-net")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	u.executable = func() (string, error) { return link, nil }
	result, err := u.run(false)
	if err != nil {
		t.Fatal(err)
	}
	resolvedTarget, _ := filepath.EvalSymlinks(target)
	if result.Path != resolvedTarget {
		t.Fatalf("path %s", result.Path)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("replaced symlink")
	}
	u, _, _ = updateFixture(t)
	target, _ = u.executable()
	if err := os.WriteFile(target+".update.lock", []byte("other updater"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = u.run(false); err == nil {
		t.Fatal("lock ignored")
	}
	b, _ := os.ReadFile(target)
	if string(b) != "old binary" {
		t.Fatal("lock failed to protect original")
	}
}

func TestUpdateWindowsReplacementAndRollback(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "shimo-net.exe")
			staged := filepath.Join(dir, "staged.exe")
			os.WriteFile(target, []byte("old"), 0755)
			if !missing {
				os.WriteFile(staged, []byte("new"), 0755)
			}
			backup, err := replaceWindowsUpdateBinary(staged, target)
			if (err != nil) != missing {
				t.Fatalf("err=%v", err)
			}
			if backup != "" {
				t.Fatalf("backup=%s", backup)
			}
			want := "new"
			if missing {
				want = "old"
			}
			b, _ := os.ReadFile(target)
			if string(b) != want {
				t.Fatalf("binary=%q", b)
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 1 {
				t.Fatalf("leftovers=%v", entries)
			}
		})
	}
}

func TestUpdateSmokeBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixtures")
	}
	for _, tc := range []struct {
		name, body string
		fail       bool
	}{{"good", "printf '%s' '{\"version\":\"1.2.4\"}'", false}, {"wrong", "printf '%s' '{\"version\":\"1.2.3\"}'", true}, {"json", "printf bad", true}, {"failed", "exit 1", true}, {"large", "i=0; while [ $i -lt 5000 ]; do printf x; i=$((i+1)); done", true}} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture")
			os.WriteFile(path, []byte("#!/bin/sh\n"+tc.body+"\n"), 0755)
			if err := smokeUpdateBinary(path, "1.2.4"); (err != nil) != tc.fail {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestUpdatePlatformNames(t *testing.T) {
	for _, goos := range []string{"linux", "darwin", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			name, err := updateAssetName(goos, arch)
			if err != nil || !strings.Contains(name, goos+"_"+arch) {
				t.Fatalf("%s %v", name, err)
			}
			if (filepath.Ext(name) == ".exe") != (goos == "windows") {
				t.Fatal(name)
			}
		}
	}
}
