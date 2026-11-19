package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPhase3DocumentContentURLDiagnostic(t *testing.T) {
	if os.Getenv("SHIMO_PHASE3_CONTENT_URL_DIAGNOSTIC") != "1" {
		t.Skip("opt-in read-only content signature diagnosis")
	}
	a := newApp([]string{"phase3"})
	guid := "4WDzngzwAnCJYzqm"
	dir := "../tmp/phase3-live/docx-slides/content-url-diagnostic-amz-window"
	_ = os.MkdirAll(dir, 0700)
	inspect := func(stage string) {
		t.Helper()
		r, err := a.authRequestPath("/docx/"+guid, "GET", nil, nil, nil)
		if err != nil {
			t.Fatal("editor HTML request failed")
		}
		summary := map[string]any{"stage": stage, "pageStatus": r.Status, "pageDate": r.Header.Get("Date"), "localUTC": time.Now().UTC()}
		marker := []byte("const envs = ")
		i := bytes.Index(r.Raw, marker)
		if i < 0 {
			t.Fatal("runtime absent")
		}
		var env any
		if json.NewDecoder(bytes.NewReader(r.Raw[i+len(marker):])).Decode(&env) != nil {
			t.Fatal("runtime decode failed")
		}
		urls := map[string]string{}
		keys := []string{}
		var walk func(any, string)
		walk = func(v any, p string) {
			switch x := v.(type) {
			case map[string]any:
				for k, c := range x {
					walk(c, p+"/"+k)
				}
			case []any:
				for i, c := range x {
					walk(c, p+"/"+strconv.Itoa(i))
				}
			case string:
				if strings.Contains(x, "cos.ap-beijing.myqcloud.com/") {
					u, e := url.Parse(x)
					if e == nil && strings.HasSuffix(u.Host, ".cos.ap-beijing.myqcloud.com") && u.Path == "/"+guid {
						urls[p] = x
						keys = append(keys, p)
					}
				}
			}
		}
		walk(env, "")
		summary["contentURLKeys"] = keys
		results := []map[string]any{}
		for key, raw := range urls {
			u, _ := url.Parse(raw)
			q := u.Query()
			keys := []string{}
			for key := range q {
				keys = append(keys, key)
			}
			if signed, e := base64.StdEncoding.DecodeString(q.Get("sign")); e == nil && len(signed) > 20 {
				if v, e := url.ParseQuery(string(signed[20:])); e == nil {
					if start, e := strconv.ParseInt(v.Get("t"), 10, 64); e == nil {
						q.Set("diagnostic-start", strconv.FormatInt(start, 10))
					}
					if end, e := strconv.ParseInt(v.Get("e"), 10, 64); e == nil {
						q.Set("diagnostic-end", strconv.FormatInt(end, 10))
					}
				}
			}
			result := map[string]any{"sourceKey": key, "origin": u.Scheme + "://" + u.Host, "path": u.Path}
			result["parameterNames"] = keys
			if start, e := time.Parse("20060102T150405Z", q.Get("X-Amz-Date")); e == nil {
				if expiry, e := strconv.Atoi(q.Get("X-Amz-Expires")); e == nil {
					end := start.Add(time.Duration(expiry) * time.Second)
					result["signatureWindow"] = map[string]any{"startUTC": start, "endUTC": end, "remainingSeconds": time.Until(end).Seconds(), "signedHeaderNames": q.Get("X-Amz-SignedHeaders")}
				}
			}
			if q.Get("diagnostic-start") != "" && q.Get("diagnostic-end") != "" {
				q.Set("q-sign-time", q.Get("diagnostic-start")+";"+q.Get("diagnostic-end"))
			}
			for _, k := range []string{"q-sign-time", "q-key-time"} {
				if value := q.Get(k); value != "" {
					parts := strings.Split(value, ";")
					if len(parts) == 2 {
						start, e1 := strconv.ParseInt(parts[0], 10, 64)
						end, e2 := strconv.ParseInt(parts[1], 10, 64)
						if e1 == nil && e2 == nil {
							result[k] = map[string]any{"startUTC": time.Unix(start, 0).UTC(), "endUTC": time.Unix(end, 0).UTC(), "remainingSeconds": end - time.Now().Unix()}
						}
					}
				}
			}
			req, _ := http.NewRequest("GET", raw, nil)
			req.Header.Set("Origin", a.Origin)
			req.Header.Set("Referer", a.Origin+"/")
			client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			response, e := client.Do(req)
			if e != nil {
				result["transportError"] = true
			} else {
				result["status"] = response.StatusCode
				result["serverDate"] = response.Header.Get("Date")
				if date, e := http.ParseTime(response.Header.Get("Date")); e == nil {
					result["clockDifferenceSeconds"] = time.Since(date).Seconds()
				}
				if response.StatusCode != 200 {
					body, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
					var failure struct {
						Code string `xml:"Code"`
					}
					if xml.Unmarshal(body, &failure) == nil {
						result["cosCode"] = failure.Code
					}
				}
				response.Body.Close()
			}
			results = append(results, result)
		}
		summary["contentRequests"] = results
		_ = phase3WriteEvidence(filepath.Join(dir, stage+".json"), summary)
		t.Log(stage, "contentURLs", len(urls))
	}
	inspect("before-export")
	if _, err := a.phase3OfficeExport(guid, filepath.Join(dir, "export"), "docx"); err != nil {
		t.Fatal("export failed")
	}
	inspect("after-export")
}
