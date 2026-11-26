package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Only the submission observer may retry this explicit eventually consistent count lag.
var errPhase3FormResultTotalLag = errors.New("答卷页条数超过总数")

func phase3FormResponseJSON(r *Response) (map[string]any, error) {
	if strings.EqualFold(r.Header.Get("X-Content-Encoding"), "base64") {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(r.Raw)))
		if err != nil {
			return nil, errors.New("表单结果 base64 编码无效")
		}
		copy := *r
		copy.Raw = decoded
		if err := json.Unmarshal(decoded, &copy.Data); err != nil {
			return nil, errors.New("表单结果不是有效 JSON")
		}
		r = &copy
	}
	data, err := phase3SDKJSON(r)
	if err != nil {
		return nil, err
	}
	m, ok := data.(map[string]any)
	if !ok {
		return nil, errors.New("表单响应不是对象")
	}
	return m, nil
}
func phase3FormAnswers(row map[string]any) ([]any, error) {
	value := row["content"]
	if s, ok := value.(string); ok {
		if err := json.Unmarshal([]byte(s), &value); err != nil {
			return nil, errors.New("答卷 content 不是有效 JSON")
		}
	}
	answers, ok := value.([]any)
	if !ok {
		return nil, errors.New("答卷缺少 content 数组")
	}
	seen := map[string]bool{}
	for _, v := range answers {
		m, ok := v.(map[string]any)
		if !ok || str(m, "guid") == "" {
			return nil, errors.New("答案缺少问题 guid")
		}
		id := str(m, "guid")
		if seen[id] {
			return nil, errors.New("同一答卷包含重复问题")
		}
		seen[id] = true
	}
	return answers, nil
}
func phase3FormPage(m map[string]any) ([]map[string]any, int, error) {
	n, ok := m["total"].(float64)
	if !ok || n < 0 || n != math.Trunc(n) || n > 1000000 {
		return nil, 0, errors.New("答卷总数无效或超过安全读取上限")
	}
	entries, ok := m["responses"].([]any)
	if !ok {
		return nil, 0, errors.New("表单结果缺少 responses 数组")
	}
	seen := map[string]bool{}
	rows := []map[string]any{}
	for _, v := range entries {
		row, ok := v.(map[string]any)
		if !ok || str(row, "submitId") == "" {
			return nil, 0, errors.New("答卷缺少 submitId")
		}
		id := str(row, "submitId")
		if seen[id] {
			return nil, 0, errors.New("答卷 ID 重复")
		}
		seen[id] = true
		if _, err := phase3FormAnswers(row); err != nil {
			return nil, 0, err
		}
		rows = append(rows, row)
	}
	if len(rows) > int(n) {
		return rows, int(n), errPhase3FormResultTotalLag
	}
	return rows, int(n), nil
}

// Values remain lossless JSON in per-question columns; headers are stable
// question GUIDs, avoiding ambiguity when multiple questions have equal titles.
func phase3FormCSV(rows []map[string]any, questionIDs ...string) ([]byte, map[string]map[string]int, error) {
	keys := map[string]bool{}
	for _, id := range questionIDs {
		if id == "" {
			return nil, nil, errors.New("问题 guid 为空")
		}
		keys[id] = true
	}
	mapped := []map[string]string{}
	counts := map[string]map[string]int{}
	for _, row := range rows {
		answers, err := phase3FormAnswers(row)
		if err != nil {
			return nil, nil, err
		}
		values := map[string]string{}
		for _, v := range answers {
			m := v.(map[string]any)
			id := str(m, "guid")
			keys[id] = true
			answer := map[string]any{}
			for k, value := range m {
				if k != "guid" && k != "type" {
					answer[k] = value
				}
			}
			b, err := json.Marshal(answer)
			if err != nil {
				return nil, nil, err
			}
			values[id] = string(b)
			if counts[id] == nil {
				counts[id] = map[string]int{}
			}
			counts[id][string(b)]++
		}
		mapped = append(mapped, values)
	}
	ids := []string{}
	for id := range keys {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	header := []string{"submitId", "createdAt", "score"}
	for _, id := range ids {
		header = append(header, phase3SafeCSV(id))
	}
	if err := w.Write(header); err != nil {
		return nil, nil, err
	}
	for i, row := range rows {
		record := []string{phase3SafeCSV(str(row, "submitId")), phase3SafeCSV(str(row, "createdAt")), phase3SafeCSV(str(row, "score"))}
		for _, id := range ids {
			record = append(record, mapped[i][id])
		}
		if err := w.Write(record); err != nil {
			return nil, nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), counts, nil
}
func phase3SafeCSV(s string) string {
	trimmed := strings.TrimLeft(s, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + s
	}
	return s
}

func (s *phase3SDKSession) formPage(offset int) ([]map[string]any, int, error) {
	path := "forms/" + url.PathEscape(s.guid) + "/responses?formGuid=" + url.QueryEscape(s.guid) + "&limit=100&offset=" + strconv.Itoa(offset)
	r, err := s.app.authRequestPath(s.prefix+path, "GET", nil, nil, s.headers)
	if err != nil {
		return nil, 0, err
	}
	m, err := phase3FormResponseJSON(r)
	if err != nil {
		return nil, 0, err
	}
	return phase3FormPage(m)
}

// Context reads are used after a submission: they never refresh credentials or
// replay a write. Existing standalone reads retain authRequestPath behavior.
func (s *phase3SDKSession) formPageContext(ctx context.Context, offset int) ([]map[string]any, int, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	cookie, err := authCookie()
	if err != nil {
		return nil, 0, err
	}
	path := "forms/" + url.PathEscape(s.guid) + "/responses?formGuid=" + url.QueryEscape(s.guid) + "&limit=100&offset=" + strconv.Itoa(offset)
	r, err := s.app.requestURLContext(ctx, strings.TrimRight(s.app.Origin, "/")+s.prefix+path, "GET", nil, nil, s.headers, cookie)
	if err != nil {
		return nil, 0, err
	}
	m, err := phase3FormResponseJSON(r)
	if err != nil {
		return nil, 0, err
	}
	return phase3FormPage(m)
}
func (s *phase3SDKSession) formResultsContext(ctx context.Context) ([]map[string]any, int, error) {
	return phase3FormResultPages(func(offset int) ([]map[string]any, int, error) { return s.formPageContext(ctx, offset) })
}
func (s *phase3SDKSession) formResults() ([]map[string]any, int, error) {
	return phase3FormResultPages(s.formPage)
}
func phase3FormResultPages(read func(int) ([]map[string]any, int, error)) ([]map[string]any, int, error) {
	rows := []map[string]any{}
	seen := map[string]bool{}
	expected := -1
	for {
		page, total, err := read(len(rows))
		partial := errors.Is(err, errPhase3FormResultTotalLag)
		if err != nil && !partial {
			return nil, 0, err
		}
		if expected < 0 {
			expected = total
		} else if total != expected {
			return nil, 0, errors.New("读取期间答卷总数变化，请重新读取")
		}
		for _, row := range page {
			id := str(row, "submitId")
			if seen[id] {
				return nil, 0, errors.New("跨页出现重复答卷，无法保证完整导出")
			}
			seen[id] = true
			rows = append(rows, row)
		}
		if partial {
			return rows, total, errPhase3FormResultTotalLag
		}
		if len(rows) == expected {
			return rows, expected, nil
		}
		if len(page) == 0 || len(rows) > expected {
			return nil, 0, errors.New("答卷分页不完整")
		}
	}
}

func (a *App) cmdPhase3FormRead() error {
	if len(a.Pos) != 1 {
		return errors.New("用法：phase3 <表单链接> --action form-inspect|form-results [--out 证据目录]")
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	s, err := a.phase3SDK(target, "form")
	if err != nil {
		return err
	}
	action := a.opt("--action")
	if action == "form-inspect" {
		r, err := a.authRequestPath(s.prefix+"forms/"+url.PathEscape(s.guid)+"/publications", "GET", nil, nil, s.headers)
		if err != nil {
			return err
		}
		if m, ok := r.Data.(map[string]any); ok && r.Status == 200 && str(m, "errorCode") == "20221" {
			return a.printJSON(map[string]any{"ok": true, "action": action, "url": s.target, "published": false, "mutationAttempted": false, "scope": "publication status only; draft content not decoded"})
		}
		m, err := phase3FormResponseJSON(r)
		if err != nil {
			return err
		}
		result, err := phase3FormPublication(m)
		if err != nil {
			return err
		}
		result["ok"] = true
		result["action"] = action
		result["url"] = s.target
		result["mutationAttempted"] = false
		return a.printJSON(result)
	}
	if action != "form-results" {
		return errors.New("不支持的表单读取动作")
	}
	publication, err := s.get("forms/" + url.PathEscape(s.guid) + "/publications")
	if err != nil {
		return err
	}
	pub, ok := publication.(map[string]any)
	if !ok {
		return errors.New("表单 publication 响应无效")
	}
	parsed, err := phase3FormPublication(pub)
	if err != nil {
		return err
	}
	form := parsed["form"].(map[string]any)
	questions, ok := form["questions"].([]any)
	if !ok {
		return errors.New("表单 publication 缺少 questions")
	}
	questionIDs := []string{}
	questionTitles := map[string]any{}
	for _, v := range questions {
		q, ok := v.(map[string]any)
		if !ok || str(q, "guid") == "" {
			return errors.New("表单 question 无效")
		}
		id := str(q, "guid")
		if _, exists := questionTitles[id]; exists {
			return errors.New("表单 question guid 重复")
		}
		questionIDs = append(questionIDs, id)
		questionTitles[id] = q["subject"]
	}
	rows, total, err := s.formResults()
	if err != nil {
		return err
	}
	csvData, counts, err := phase3FormCSV(rows, questionIDs...)
	if err != nil {
		return err
	}
	stats, err := s.get("forms/" + url.PathEscape(s.guid) + "/statistics")
	if err != nil {
		return err
	}
	statMap, ok := stats.(map[string]any)
	if !ok {
		return errors.New("表单 statistics 响应无效")
	}
	count, ok := statMap["responses"].(float64)
	if !ok || count != float64(total) {
		return errors.New("表单 statistics 答卷总数与分页结果不一致")
	}
	if views, ok := statMap["views"].(float64); !ok || views < 0 {
		return errors.New("表单 statistics 缺少有效访问数")
	}
	dir, err := a.phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	if err := phase3WriteEvidence(filepath.Join(dir, "responses.json"), rows); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "results.csv"), csvData, 0600); err != nil {
		return err
	}
	result := map[string]any{"ok": true, "action": action, "url": s.target, "total": total, "statistics": stats, "answerCounts": counts, "questionTitles": questionTitles, "csv": filepath.Join(dir, "results.csv"), "responses": filepath.Join(dir, "responses.json"), "mutationAttempted": false, "scope": "complete API result pagination and CSV export; not full scenario acceptance"}
	if err := phase3WriteEvidence(filepath.Join(dir, "results.json"), result); err != nil {
		return err
	}
	if total == 0 {
		result["acceptanceNote"] = "当前没有答卷；空导出不能证明统计场景已验收"
	}
	return a.printJSON(result)
}

func phase3FormPublication(m map[string]any) (map[string]any, error) {
	status := str(m, "status")
	if status != "unpublished" && status != "normal" && status != "stopped" {
		return nil, errors.New("表单发布状态缺失或未知，不能推断已发布")
	}
	var form map[string]any
	if json.Unmarshal([]byte(str(m, "publishedContent")), &form) != nil || form == nil {
		return nil, errors.New("表单 publication 内容无效")
	}
	return map[string]any{"published": status != "unpublished", "collecting": status == "normal", "status": status, "publishedRev": m["publishedRev"], "form": form, "scope": "publication content; initialization is not publication and may differ from draft"}, nil
}
