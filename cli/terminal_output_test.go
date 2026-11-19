package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestTerminalInfoEscapesRemoteMetadataWithoutChangingJSON(t *testing.T) {
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	t.Setenv("SHIMO_NET_COOKIE", "sid=synthetic")
	t.Setenv("SHIMO_NET_ORIGIN", "https://terminal.invalid")
	t.Setenv("SHIMO_NET_DISABLE_HARVEST", "1")
	payload := "正常\nFORGED\r\x1b[2J\x00\u0085\u202e\u2066\u2028\u2029"
	old := httpClient
	defer func() { httpClient = old }()
	httpClient = &http.Client{Transport: phase3RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		v := map[string]any{"guid": "abcdefgh", "name": payload, "type": payload, "shareMode": payload, "updatedAt": payload}
		if strings.HasSuffix(r.URL.Path, "/ancestors") {
			v = map[string]any{"ancestors": []any{}}
		}
		b, _ := json.Marshal(v)
		return phase3Response(200, string(b)), nil
	})}
	for _, jsonMode := range []bool{false, true} {
		a := newApp([]string{"info", "abcdefgh"})
		a.JSON = jsonMode
		out, err := issueCapture(t, a.Run)
		if err != nil {
			t.Fatal(err)
		}
		if jsonMode {
			var result map[string]any
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatal(err)
			}
			if result["name"] != payload || result["type"] != payload || result["shareMode"] != payload {
				t.Fatalf("JSON metadata changed: %q", out)
			}
		} else {
			for _, forbidden := range []string{"\nFORGED", "\r", "\x1b", "\x00", "\u0085", "\u202e", "\u2066", "\u2028", "\u2029"} {
				if strings.Contains(out, forbidden) {
					t.Errorf("raw control %q in terminal output: %q", forbidden, out)
				}
			}
			if !strings.Contains(out, `正常\nFORGED\r\x1b[2J`) {
				t.Errorf("metadata not visibly escaped: %q", out)
			}
		}
	}
}

func TestTerminalFileRowsEscapeEveryRemoteMetadataField(t *testing.T) {
	payload := "ROW\nFORGED\x1b[2J\u202e"
	row := map[string]any{"name": payload, "type": payload, "url": payload, "guid": payload, "path": payload, "shareMode": payload, "updatedAt": payload}
	got := fileRow(row, 0)
	if strings.Count(got, "\n") != 2 || strings.ContainsAny(got, "\x1b\u202e") {
		t.Fatalf("file row metadata escaped layout: %q", got)
	}
	if !strings.Contains(got, `ROW\nFORGED\x1b[2J\u202e`) {
		t.Fatalf("lost metadata: %q", got)
	}
}

func TestTerminalFavEscapesCachedRemoteMetadata(t *testing.T) {
	t.Setenv("SHIMO_NET_STATE_DIR", t.TempDir())
	payload := "cached\nFORGED\x1b[2J"
	if err := saveFav(map[string]map[string]any{"test": {"name": payload, "url": "https://example.invalid/" + payload, "note": payload}}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"fav", "list"}, {"fav", "show", "test"}} {
		out, err := issueCapture(t, newApp(args).Run)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, "\nFORGED") || strings.Contains(out, "\x1b") {
			t.Fatalf("cached metadata displayed raw: %q", out)
		}
	}
}

func TestTerminalControlBoundaries(t *testing.T) {
	for _, s := range []string{"", "普通中文 😀 é \\ path", "line\nnext\ttab"} {
		got := terminalContent(s)
		if got != s {
			t.Errorf("ordinary content changed: %q -> %q", s, got)
		}
	}
	for r := rune(0); r <= 0x9f; r++ {
		if r > 0x1f && r < 0x7f {
			continue
		}
		got := terminalText(string(r))
		if strings.ContainsRune(got, r) {
			t.Errorf("metadata control U+%04X remains", r)
		}
		content := terminalContent(string(r))
		if r != '\n' && r != '\t' && strings.ContainsRune(content, r) {
			t.Errorf("content control U+%04X remains", r)
		}
	}
	for _, r := range []rune{'\u061c', '\u200e', '\u200f', '\u202a', '\u202b', '\u202c', '\u202d', '\u202e', '\u2066', '\u2067', '\u2068', '\u2069', '\u2028', '\u2029'} {
		if strings.ContainsRune(terminalText(string(r)), r) || strings.ContainsRune(terminalContent(string(r)), r) {
			t.Errorf("formatting U+%04X remains", r)
		}
	}
}

func TestTerminalReadPreservesJSONAndMultilineContent(t *testing.T) {
	body := "第一行\n第二行\t列\x1b[2J\u202e末尾"
	issueFileTransport(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/api/v1/files/abcdefgh":
			return phase3Response(200, `{"guid":"abcdefgh","name":"正文.txt","isCloudFile":true}`), nil
		case "/api/v1/files/abcdefgh/download":
			return phase3Response(200, `{"resourceURL":"https://download.invalid/content.txt"}`), nil
		case "/content.txt":
			return phase3Response(200, body), nil
		}
		t.Errorf("unexpected request %s", r.URL.Path)
		return phase3Response(404, `{}`), nil
	})
	for _, jsonMode := range []bool{false, true} {
		a := newApp([]string{"read", "abcdefgh"})
		a.JSON = jsonMode
		out, err := issueCapture(t, a.Run)
		if err != nil {
			t.Fatal(err)
		}
		if jsonMode {
			var result map[string]any
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatal(err)
			}
			if result["text"] != body {
				t.Fatalf("JSON content changed: %q", out)
			}
		} else if !strings.Contains(out, "第一行\n第二行\t列\\x1b[2J\\u202e末尾") || strings.ContainsAny(out, "\x1b\u202e") {
			t.Fatalf("terminal content did not preserve lines/escape controls: %q", out)
		}
	}
}
