package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func phase3OfflineScreenshotPeer(t *testing.T, data []byte) *phase3EditorBrowser {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() { client.Close(); server.Close() })
	go func() {
		defer server.Close()
		peer := &WSConn{conn: server, rd: bufio.NewReader(server)}
		raw, err := peer.readMessage()
		if err != nil {
			return
		}
		var req map[string]any
		if json.Unmarshal(raw, &req) != nil {
			return
		}
		if req["method"] != "Page.captureScreenshot" {
			t.Errorf("unexpected method %v", req["method"])
			return
		}
		_ = peer.sendJSON(map[string]any{"id": req["id"], "result": map[string]any{"data": base64.StdEncoding.EncodeToString(data)}})
	}()
	return &phase3EditorBrowser{ws: &WSConn{conn: client, rd: bufio.NewReader(client)}}
}

func TestPhase3GlobalFormWhiteScreenshotRejectedAndRetained(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1440, 1400))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	if encoded.Len() < 1000 {
		t.Fatal("fixture must exceed old byte-only check")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "ready-but-white.png")
	err := phase3FormScreenshot(phase3OfflineScreenshotPeer(t, encoded.Bytes()), path)
	if err == nil || !strings.Contains(err.Error(), "空白") {
		t.Fatalf("valid all-white PNG accepted: %v", err)
	}
	actual, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(actual, encoded.Bytes()) {
		t.Fatalf("original diagnostic image not retained: %v", e)
	}
	archivePhase3Negative(t, dir, map[string]any{"source": "offline-production-screenshot-helper-with-fake-CDP", "action": "form-screenshot", "fault": "ready-white-valid-PNG", "appRunReturnedError": nil, "helperReturnedError": err.Error(), "mutationRequests": 0, "bytes": len(actual), "imageSHA256": fmt.Sprintf("%x", sha256.Sum256(actual)), "syntheticScreenshot": true})
}

func TestPhase3GlobalFormHistoricalScreenshotControls(t *testing.T) {
	paths := []string{
		"../tmp/phase3-live/form-edit/phase3-336122807/preview.png",
		"../tmp/phase3-live/form-edit/phase3-336122807/branch-02.png",
		"../tmp/phase3-live/form-edit/phase3-3587306806/preview.png",
		"../tmp/phase3-live/form-edit/phase3-910904031/confirmation.png",
		"../tmp/phase3-live/form-edit/phase3-61487164/reload.png",
	}
	for _, path := range paths {
		t.Run(filepath.Base(filepath.Dir(path))+"-"+filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if os.IsNotExist(err) {
				t.Skip("optional historical live PNG not available in checkout")
			}
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			copyPath := filepath.Join(dir, "historical-control.png")
			if err = phase3FormScreenshot(phase3OfflineScreenshotPeer(t, data), copyPath); err != nil {
				t.Fatalf("real historical form text rejected: %v", err)
			}
			copy, err := os.ReadFile(copyPath)
			if err != nil || !bytes.Equal(data, copy) {
				t.Fatal("historical screenshot bytes changed")
			}
			// Archive metadata only; existing real screenshots remain under their
			// original privacy-reviewed evidence path, not synthetic screenshot dirs.
			if err = os.Remove(copyPath); err != nil {
				t.Fatal(err)
			}
			archivePhase3Negative(t, dir, map[string]any{"source": "offline-production-screenshot-helper-on-historical-real-PNG", "action": "form-screenshot", "fault": "historical-control", "helperReturnedError": nil, "mutationRequests": 0, "originalHistoricalPath": path, "originalSHA256": fmt.Sprintf("%x", sha256.Sum256(data)), "comparison": "existing real PNG passed unchanged through current helper and mocked CDP; no new live capture"})
		})
	}
}
