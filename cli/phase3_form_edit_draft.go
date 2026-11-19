package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"time"
)

const phase3FormReady = `(()=>{const q=window.logged?.edit?.query;if(!q?.getForm||!window.modoc?.Delta)return false;const f=q.getForm(),body=document.body?.innerText||'';if(!f||!body.includes(f.title))return false;const route=q.getRouterPage?.();if(route==='result')return body.includes('结果统计')&&body.includes('数据汇总');if(route!=='questions')return false;const values=[...document.querySelectorAll('input,textarea')].filter(e=>[...e.getClientRects()].some(r=>r.width>0&&r.height>0)).map(e=>e.value);return f.questions?.length?f.questions.some(x=>typeof x.subject==='string'&&x.subject.length>0&&(body.includes(x.subject)||values.includes(x.subject))):body.includes('题型')})()`

// Only the browser startup boundary is replaceable in offline integration tests.
// Production uses the full isolated-browser readiness and authentication path.
var phase3OpenFormEditor = openPhase3EditorWhen

func phase3FormShowQuestions(b *phase3EditorBrowser) error {
	_, err := b.Eval(`(()=>{const e=window.logged.edit;if(e.query.getRouterPage()!=='questions')e.command.changeRouterPage('questions');return {ok:true}})()`)
	if err != nil {
		return err
	}
	return b.WaitReady(phase3FormReady, phase3EditorReadyTimeout)
}

// Decodes the SDK's documented-by-runtime insert snapshot representation. The
// transport's compressed Delta is decoded by its own SDK, never by editor state.
func phase3DecodeFormDelta(raw any) (map[string]any, error) {
	list, ok := raw.([]any)
	if !ok || len(list) != 1 {
		return nil, errors.New("服务端草稿不是单一 form delta")
	}
	op, ok := list[0].(map[string]any)
	if !ok || str(op, "action") != "insert" {
		return nil, errors.New("草稿不是完整 insert 快照")
	}
	data, ok := op["data"].(map[string]any)
	if !ok {
		return nil, errors.New("草稿缺少 data")
	}
	parts, ok := data["form"].([]any)
	if !ok || len(parts) != 2 {
		return nil, errors.New("草稿缺少 questions/rules 分区")
	}
	out := map[string]any{"title": "", "desc": "", "descAlign": "left", "viewMode": "classic", "questions": []any{}, "rules": []any{}}
	attrs, _ := op["attributes"].(map[string]any)
	for key, v := range attrs {
		if v == nil {
			continue
		}
		switch key {
		case "title", "desc", "descAlign", "viewMode", "confirmation":
			out[key] = v
		case "widgets":
			s, ok := v.(string)
			if !ok {
				return nil, errors.New("草稿 widgets 不是 JSON 字符串")
			}
			var x any
			if json.Unmarshal([]byte(s), &x) != nil {
				return nil, errors.New("草稿 widgets 无效")
			}
			out[key] = x
		default:
			return nil, errors.New("草稿包含未知元数据字段，无法完整验证")
		}
	}
	for idx, part := range parts {
		if part == nil {
			continue
		}
		ops, ok := part.([]any)
		if !ok {
			return nil, errors.New("草稿分区不是数组")
		}
		rows := []any{}
		for _, v := range ops {
			entry, ok := v.(map[string]any)
			if !ok || str(entry, "action") != "insert" {
				return nil, errors.New("草稿分区不是 insert 快照")
			}
			d, _ := entry["data"].(map[string]any)
			obj, ok := d["object"].(map[string]any)
			if !ok || str(obj, "guid") == "" {
				return nil, errors.New("草稿对象缺少 guid")
			}
			row := map[string]any{"guid": obj["guid"]}
			key := "question"
			if idx == 1 {
				key = "rule"
			}
			detail, ok := obj[key].(map[string]any)
			if !ok || str(detail, "type") == "" {
				return nil, errors.New("草稿对象缺少 type")
			}
			body := map[string]any{}
			for k, x := range detail {
				if k != "type" {
					body[k] = x
				}
			}
			row["type"] = detail["type"]
			row[key] = body
			if idx == 0 {
				row["required"] = obj["required"]
				row["subject"] = obj["subject"]
				for _, k := range []string{"description", "tableFieldGuid"} {
					if x, ok := obj[k]; ok && x != nil {
						row[k] = x
					}
				}
			}
			rows = append(rows, row)
		}
		if idx == 0 {
			out["questions"] = rows
		} else {
			out["rules"] = rows
		}
	}
	return out, nil
}
func (s *phase3SDKSession) formDraft(b *phase3EditorBrowser) (map[string]any, error) {
	v, err := s.get("forms/" + url.PathEscape(s.guid) + "/content")
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("草稿地址响应无效")
	}
	u, err := url.Parse(str(m, "contentUrl"))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil, errors.New("草稿下载地址无效")
	}
	// Signed content URLs are fetched without the account cookie or SDK headers.
	// Errors deliberately omit the URL, whose query may carry scoped credentials.
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || len(via) > 3 {
			return errors.New("unsafe content redirect")
		}
		return nil
	}}
	resp, err := client.Get(u.String())
	if err != nil {
		return nil, errors.New("读取服务端草稿失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New("服务端草稿 HTTP 状态异常")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024+1))
	if err != nil || len(data) > 8*1024*1024 {
		return nil, errors.New("服务端草稿超出读取上限或未读完")
	}
	encoded, _ := json.Marshal(string(data))
	expanded, err := b.Eval(`new window.modoc.Delta(` + string(encoded) + `).toJSON()`)
	if err != nil {
		return nil, err
	}
	return phase3DecodeFormDelta(expanded)
}
func (a *App) cmdPhase3FormDraft() error {
	if len(a.Pos) != 1 {
		return errors.New("用法：phase3 <表单链接> --action form-draft [--out 证据目录]")
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	s, err := a.phase3SDK(target, "form")
	if err != nil {
		return err
	}
	b, err := phase3OpenFormEditor(s.target+"?debug=1", phase3FormReady)
	if err != nil {
		return err
	}
	defer b.Close()
	draft, err := s.formDraft(b)
	if err != nil {
		return err
	}
	result := map[string]any{"ok": true, "action": "form-draft", "guid": s.guid, "url": s.target, "form": draft, "source": "signed server draft content; SDK Delta decoder only", "mutationAttempted": false}
	if a.opt("--out") != "" {
		dir, err := phase3EvidenceDir(a.opt("--out"))
		if err != nil {
			return err
		}
		result["evidenceDir"] = dir
		if err = phase3WriteEvidence(filepath.Join(dir, "draft.json"), result); err != nil {
			return err
		}
		if err = phase3FormScreenshot(b, filepath.Join(dir, "draft.png")); err != nil {
			return err
		}
	}
	return a.printJSON(result)
}
