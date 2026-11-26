package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

type phase3TableChange struct {
	Action string `json:"action"`
	UUID   string `json:"uuid"`
	Data   string `json:"data"`
}
type phase3TableEditPlan struct {
	Action          string                    `json:"action"`
	TableGuid       string                    `json:"tableGuid"`
	Changes         []phase3TableChange       `json:"changes"`
	Expected        map[string]any            `json:"expected"`
	CreatedIDs      []string                  `json:"createdIds,omitempty"`
	DeletedIDs      []string                  `json:"deletedIds,omitempty"`
	MatchedIDs      []string                  `json:"matchedIds,omitempty"`
	ViewLayouts     map[string]map[string]any `json:"viewLayouts,omitempty"`
	NewViews        map[string]map[string]any `json:"newViews,omitempty"`
	DeletedViewIDs  []string                  `json:"deletedViewIds,omitempty"`
	UnchangedTables map[string]map[string]any `json:"unchangedTables,omitempty"`
}

// A connected, initialized presence observer must remain alive through the
// mutation. The default fails closed until the editor bridge supplies one.
type phase3TableGuard interface {
	CheckPresence() error
	Close()
}

var phase3OpenTableGuard = func(target string) (phase3TableGuard, error) {
	return nil, errors.New("应用表格协作在线名单尚未初始化，拒绝写入")
}

func phase3TableID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func phase3TableCopy(m map[string]any) map[string]any {
	b, _ := json.Marshal(m)
	out := map[string]any{}
	_ = json.Unmarshal(b, &out)
	return out
}
func (p *phase3TableEditPlan) add(action string, data map[string]any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	id, err := phase3TableID()
	if err != nil {
		return err
	}
	p.Changes = append(p.Changes, phase3TableChange{action, id, string(b)})
	return nil
}
func phase3TableRows(table map[string]any, key string) []any { x, _ := table[key].([]any); return x }
func phase3TableMap(table map[string]any, key string) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, v := range phase3TableRows(table, key) {
		m, _ := v.(map[string]any)
		if m != nil {
			out[str(m, "guid")] = m
		}
	}
	return out
}
func phase3TableSemantic(table map[string]any) map[string]any {
	fields := map[string]any{}
	records := map[string]any{}
	views := map[string]any{}
	for id, m := range phase3TableMap(table, "fields") {
		fields[id] = map[string]any{"name": m["name"], "type": m["type"], "isPrimary": boolv(m, "isPrimary"), "props": m["props"], "desc": str(m, "desc"), "formQuestionGuid": str(m, "formQuestionGuid")}
	}
	for id, m := range phase3TableMap(table, "records") {
		records[id] = m["valueMap"]
	}
	for id, m := range phase3TableMap(table, "views") {
		view := phase3TableCopy(m)
		for _, key := range []string{"createdAt", "updatedAt", "creator", "fields", "records", "fieldInfo"} {
			delete(view, key)
		}
		views[id] = view
	}
	return map[string]any{"guid": table["guid"], "baseGuid": table["baseGuid"], "name": table["name"], "desc": table["desc"], "fields": fields, "records": records, "views": views}
}
func phase3TablePayloadKeys(payload map[string]any, allowed ...string) error {
	keys := map[string]bool{}
	for _, k := range allowed {
		keys[k] = true
	}
	for k := range payload {
		if !keys[k] {
			return fmt.Errorf("未知参数 %q；未执行写入", k)
		}
	}
	return nil
}
func preparePhase3TableEdit(action string, payload, before map[string]any) (phase3TableEditPlan, error) {
	p := phase3TableEditPlan{Action: action, TableGuid: str(payload, "tableGuid"), Expected: phase3TableSemantic(before), Changes: []phase3TableChange{}}
	if p.TableGuid == "" || p.TableGuid != str(before, "guid") {
		return p, errors.New("tableGuid 与当前数据表不一致")
	}
	views := phase3TableRows(before, "views")
	if len(views) == 0 {
		return p, errors.New("数据表无可验证视图")
	}
	view, _ := views[0].(map[string]any)
	for _, v := range views {
		if candidate, ok := v.(map[string]any); ok && str(candidate, "type") == "Grid" {
			view = candidate
			break
		}
	}
	viewGuid := str(view, "guid")
	var err error
	switch action {
	case "table-update":
		if err = phase3TablePayloadKeys(payload, "tableGuid", "name", "desc"); err != nil {
			break
		}
		for _, key := range []string{"name", "desc"} {
			v, exists := payload[key]
			if !exists {
				continue
			}
			text, ok := v.(string)
			if !ok || (key == "name" && strings.TrimSpace(text) == "") || len(text) > 10000 {
				return p, errors.New("名称/说明参数无效")
			}
			p.Expected[key] = text
			serverAction := "UpdateTableName"
			if key == "desc" {
				serverAction = "UpdateTableDesc"
			}
			if err = p.add(serverAction, map[string]any{"tableGuid": p.TableGuid, "viewGuid": viewGuid, key: text}); err != nil {
				break
			}
		}
	case "table-field-add":
		if err = phase3TablePayloadKeys(payload, "tableGuid", "fields"); err == nil {
			err = preparePhase3TableFields(&p, payload, before, viewGuid)
		}
	case "table-field-update":
		err = preparePhase3TableFieldUpdate(&p, payload, before, viewGuid)
	case "table-field-delete":
		err = preparePhase3TableFieldDelete(&p, payload, before, viewGuid)
	case "table-field-convert":
		err = preparePhase3TableConversion(&p, payload, before, viewGuid)
	case "table-view-update", "table-view-create", "table-view-delete":
		err = preparePhase3TableView(&p, payload, before)
	case "table-record-add":
		if err = phase3TablePayloadKeys(payload, "tableGuid", "records"); err == nil {
			err = preparePhase3TableRecords(&p, payload, before, viewGuid)
		}
	case "table-record-update":
		if err = phase3TablePayloadKeys(payload, "tableGuid", "where", "values"); err == nil {
			err = preparePhase3TableConditional(&p, payload, before, viewGuid)
		}
	default:
		return p, errors.New("不支持的应用表格写动作")
	}
	if err != nil {
		return p, err
	}
	if len(p.Changes) == 0 {
		return p, errors.New("没有可执行变更；未执行写入")
	}
	return p, nil
}
func preparePhase3TableFields(p *phase3TableEditPlan, payload, before map[string]any, viewGuid string) error {
	entries, ok := payload["fields"].([]any)
	if !ok || len(entries) == 0 || len(entries) > 20 {
		return errors.New("fields 必须包含 1 至 20 个字段")
	}
	fields := p.Expected["fields"].(map[string]any)
	cols := []any{}
	names := map[string]bool{}
	for _, m := range phase3TableMap(before, "fields") {
		names[str(m, "name")] = true
	}
	for _, v := range entries {
		field, ok := v.(map[string]any)
		if !ok {
			return errors.New("字段必须是对象")
		}
		if err := phase3TablePayloadKeys(field, "name", "type", "options"); err != nil {
			return err
		}
		name, nameOK := field["name"].(string)
		typ, typeOK := field["type"].(string)
		if !nameOK || !typeOK {
			return errors.New("字段 name/type 必须是字符串")
		}
		if name == "" || names[name] || len(name) > 200 {
			return errors.New("字段名称为空、重复或过长")
		}
		if typ != "SingleLineText" && typ != "Number" && typ != "SingleChoice" && typ != "Date" {
			return errors.New("当前新增字段仅支持 SingleLineText/Number/SingleChoice/Date；其他字段未验收")
		}
		props, err := phase3TableFieldProps(typ, field)
		if err != nil {
			return err
		}
		id, err := phase3TableID()
		if err != nil {
			return err
		}
		id = id[:11]
		names[name] = true
		p.CreatedIDs = append(p.CreatedIDs, id)
		cols = append(cols, map[string]any{"guid": id, "name": name, "type": typ, "isPrimary": false, "props": props})
		fields[id] = map[string]any{"name": name, "type": typ, "isPrimary": false, "props": props, "desc": "", "formQuestionGuid": ""}
	}
	return p.add("CreateColumn", map[string]any{"tableGuid": p.TableGuid, "viewGuid": viewGuid, "viewTargetGuids": phase3TableAppendTargets(before, "fields", viewGuid, p.CreatedIDs), "cols": cols})
}
// phase3TableAppendTargets returns the single viewTargetGuids entry the SDK
// expects for CreateColumn/CreateRow: a map keyed only by the active view,
// whose value anchors the new columns/rows at the end of that view's member
// order. The server rejects a map that lists several views here (HTTP 400
// "invalid viewTargetGuids"), so we emit exactly the active view rather than
// every grid view; a Kanban/Gantt active view is never used for these writes
// because the caller already resolves the active view to a grid.
func phase3TableAppendTargets(table map[string]any, key, viewGuid string, ids []string) map[string]any {
	items := []any{}
	for _, v := range phase3TableRows(table, "views") {
		view, _ := v.(map[string]any)
		if view == nil || str(view, "guid") != viewGuid {
			continue
		}
		items, _ = view[key].([]any)
		break
	}
	last := ""
	if len(items) > 0 {
		last, _ = items[len(items)-1].(string)
	}
	anchors := make([]string, len(ids))
	for i := range anchors {
		if i == 0 {
			anchors[i] = last
		} else {
			anchors[i] = ids[i-1]
		}
	}
	return map[string]any{viewGuid: anchors}
}
func phase3TableValues(values map[string]any, fields map[string]map[string]any) error {
	if len(values) == 0 {
		return errors.New("记录值不能为空")
	}
	for id, v := range values {
		f, ok := fields[id]
		if !ok {
			return fmt.Errorf("未知字段 %q", id)
		}
		switch str(f, "type") {
		case "SingleLineText":
			if _, ok := v.(string); !ok && v != nil {
				return fmt.Errorf("字段 %s 需要文本", id)
			}
		case "Number":
			if n, ok := v.(float64); v != nil && (!ok || math.IsNaN(n) || math.IsInf(n, 0)) {
				return fmt.Errorf("字段 %s 需要有限数值", id)
			}
		case "Date":
			if n, ok := v.(float64); v != nil && (!ok || math.IsNaN(n) || math.IsInf(n, 0) || n < -693593 || n > 2958465) {
				return fmt.Errorf("日期字段 %s 的原生 OA 日期值无效", id)
			}
		case "SingleChoice":
			props, _ := f["props"].(map[string]any)
			choices, _ := props["choices"].(map[string]any)
			option, ok := v.(string)
			if v != nil && (!ok || choices[option] == nil) {
				return fmt.Errorf("字段 %s 必须使用已存在的单选 option guid", id)
			}
		default:
			return fmt.Errorf("字段 %s 的 %s 类型写入尚未验收", id, str(f, "type"))
		}
	}
	return nil
}
func preparePhase3TableRecords(p *phase3TableEditPlan, payload, before map[string]any, viewGuid string) error {
	values, ok := payload["records"].([]any)
	if !ok || len(values) == 0 || len(values) > 100 {
		return errors.New("records 必须包含 1 至 100 条记录")
	}
	fields := phase3TableMap(before, "fields")
	expected := p.Expected["records"].(map[string]any)
	rows := []any{}
	for _, v := range values {
		m, ok := v.(map[string]any)
		if !ok {
			return errors.New("记录必须是字段 guid 到值的对象")
		}
		m, err := phase3NormalizeTableValues(m, fields)
		if err != nil {
			return err
		}
		if err := phase3TableValues(m, fields); err != nil {
			return err
		}
		id, err := phase3TableID()
		if err != nil {
			return err
		}
		id = id[:11]
		p.CreatedIDs = append(p.CreatedIDs, id)
		row := map[string]any{"guid": id, "valueMap": phase3TableCopy(m)}
		rows = append(rows, row)
		expected[id] = phase3TableCopy(m)
	}
	return p.add("CreateRow", map[string]any{"tableGuid": p.TableGuid, "viewGuid": viewGuid, "viewTargetGuids": phase3TableAppendTargets(before, "records", viewGuid, p.CreatedIDs), "rows": rows})
}
func preparePhase3TableConditional(p *phase3TableEditPlan, payload, before map[string]any, viewGuid string) error {
	where, ok := payload["where"].(map[string]any)
	if !ok || len(where) == 0 {
		return errors.New("where 必须是非空 AND 等值条件，禁止隐式全表更新")
	}
	values, ok := payload["values"].(map[string]any)
	if !ok {
		return errors.New("values 必须是对象")
	}
	fields := phase3TableMap(before, "fields")
	var err error
	where, err = phase3NormalizeTableValues(where, fields)
	if err != nil {
		return err
	}
	values, err = phase3NormalizeTableValues(values, fields)
	if err != nil {
		return err
	}
	if err := phase3TableValues(where, fields); err != nil {
		return err
	}
	if err := phase3TableValues(values, fields); err != nil {
		return err
	}
	changed := map[string]any{}
	expected := p.Expected["records"].(map[string]any)
	for id, row := range phase3TableMap(before, "records") {
		current, _ := row["valueMap"].(map[string]any)
		matches := true
		for key, v := range where {
			if !reflect.DeepEqual(current[key], v) {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		next := phase3TableCopy(current)
		for key, v := range values {
			next[key] = v
		}
		expected[id] = next
		changed[id] = phase3TableCopy(values)
		p.MatchedIDs = append(p.MatchedIDs, id)
	}
	if len(changed) == 0 {
		return errors.New("条件未匹配记录；未执行写入")
	}
	sort.Strings(p.MatchedIDs)
	return p.add("UpdateCell", map[string]any{"tableGuid": p.TableGuid, "viewGuid": viewGuid, "rows": changed})
}
func verifyPhase3TableEdit(before, after map[string]any, p phase3TableEditPlan) error {
	if err := phase3ValidateTableSnapshot(after, str(before, "baseGuid"), p.TableGuid); err != nil {
		return err
	}
	actual := phase3TableSemantic(after)
	if err := verifyPhase3NewViews(after, p); err != nil {
		return err
	}
	for id := range p.NewViews {
		delete(actual["views"].(map[string]any), id)
	}
	if !reflect.DeepEqual(actual, p.Expected) {
		return errors.New("应用表格持久化读回与请求后的完整字段/记录/视图状态不一致")
	}
	beforeViews, afterViews := phase3TableMap(before, "views"), phase3TableMap(after, "views")
	for id, view := range beforeViews {
		next := afterViews[id]
		removedView := false
		for _, deleted := range p.DeletedViewIDs {
			if id == deleted {
				removedView = true
			}
		}
		if removedView {
			continue
		}
		for _, key := range []string{"fields", "records"} {
			old, ok := view[key].([]any)
			if !ok {
				continue
			}
			expected := []any{}
			for _, item := range old {
				removed := false
				if key == "fields" {
					for _, deleted := range p.DeletedIDs {
						if item == deleted {
							removed = true
						}
					}
				}
				if !removed {
					expected = append(expected, item)
				}
			}
			if (p.Action == "table-field-add" && key == "fields") || (p.Action == "table-record-add" && key == "records") {
				for _, created := range p.CreatedIDs {
					expected = append(expected, created)
				}
			}
			if layout := p.ViewLayouts[id]; layout != nil {
				if value, exists := layout[key]; exists {
					expected = value.([]any)
				}
			}
			if !reflect.DeepEqual(expected, next[key]) {
				return fmt.Errorf("视图 %s 的 %s 顺序/成员与请求不一致", id, key)
			}
		}
		oldInfo, _ := view["fieldInfo"].(map[string]any)
		newInfo, _ := next["fieldInfo"].(map[string]any)
		for field, attrs := range oldInfo {
			removed := false
			for _, deleted := range p.DeletedIDs {
				if field == deleted {
					removed = true
				}
			}
			if removed {
				continue
			}
			if !reflect.DeepEqual(attrs, newInfo[field]) {
				return errors.New("非目标字段的视图显示设置发生变化")
			}
		}
	}
	return nil
}
func (s *phase3SDKSession) tableRevision() (float64, error) {
	data, err := s.get("table-api/bases/" + url.PathEscape(s.guid) + "/rev")
	if err != nil {
		return 0, err
	}
	m, ok := data.(map[string]any)
	if !ok {
		return 0, errors.New("revision 响应无效")
	}
	rev, ok := m["rev"].(float64)
	permission, _ := m["filePermission"].(map[string]any)
	if !ok || rev < 0 || rev != math.Trunc(rev) || !boolv(permission, "canRead") || !boolv(permission, "canEdit") {
		return 0, errors.New("缺少明确读写权限或有效 revision，拒绝写入")
	}
	return rev, nil
}
func (s *phase3SDKSession) composeTable(plan phase3TableEditPlan, rev float64) (any, error) {
	clientID, err := phase3TableID()
	if err != nil {
		return nil, err
	}
	r, err := s.app.authRequestPath(s.prefix+"table-api/bases/"+url.PathEscape(s.guid)+"/compose", "POST", map[string]any{"clientId": clientID, "baseRev": rev, "change": plan.Changes}, nil, s.headers)
	if err != nil {
		return nil, err
	}
	data, err := phase3SDKJSON(r)
	if err != nil {
		return nil, err
	}
	m, ok := data.(map[string]any)
	if !ok || str(m, "status") != "normal" {
		return nil, errors.New("compose 未明确确认 normal；可能已部分生效，禁止自动重试")
	}
	return data, nil
}
func (a *App) cmdPhase3TableEdit() (runErr error) {
	if len(a.Pos) != 1 {
		return errors.New("用法：phase3 <应用表格链接> --action table-update|table-field-add|table-field-update|table-field-delete|table-record-add|table-record-update|table-view-create|table-view-update|table-view-delete --payload JSON [--out 目录]")
	}
	args, err := phase2Args(a.opt("--payload"))
	if err != nil {
		return err
	}
	payload, err := phase2ObjectArg(args)
	if err != nil {
		return err
	}
	target, err := a.phase2Target(a.Pos[0])
	if err != nil {
		return err
	}
	guard, err := phase3OpenTableGuard(target)
	if err != nil {
		return err
	}
	defer guard.Close()
	if err = guard.CheckPresence(); err != nil {
		return err
	}
	s, err := a.phase3SDK(target, "table")
	if err != nil {
		return err
	}
	rev, err := s.tableRevision()
	if err != nil {
		return err
	}
	tableGuid := str(payload, "tableGuid")
	if tableGuid == "" {
		return errors.New("缺少 tableGuid")
	}
	before, err := s.tableSnapshot(tableGuid)
	if err != nil {
		return err
	}
	plan, err := preparePhase3TableEdit(a.opt("--action"), payload, before)
	if err != nil {
		return err
	}
	var viewSpec map[string]any
	if plan.Action == "table-view-create" || plan.Action == "table-view-update" {
		if _, ok := guard.(interface {
			ViewProof(map[string]any) (any, error)
		}); !ok {
			return errors.New("原生视图验收桥未安装，未执行写入")
		}
		viewID := str(payload, "viewGuid")
		view := plan.Expected["views"].(map[string]any)[viewID].(map[string]any)
		if plan.Action == "table-view-create" {
			for id, v := range plan.NewViews {
				viewID, view = id, v
			}
		}
		timezone := ""
		browserTimezone := ""
		if str(view, "type") == "Calendar" {
			provider, ok := guard.(interface{ BrowserTimezone() (string, error) })
			if !ok {
				return errors.New("日历浏览器时区桥未安装，未执行写入")
			}
			browserTimezone, err = provider.BrowserTimezone()
			if err != nil {
				return err
			}
			raw, e := s.get("table-api/v2/bases/" + url.PathEscape(s.guid))
			if e != nil {
				return e
			}
			base, ok := raw.(map[string]any)
			if !ok {
				return errors.New("日历文档时区无法读回")
			}
			timezone = str(base, "timezone")
		}
		viewSpec, err = phase3ViewRecordSpec(before, view, timezone, browserTimezone)
		if err != nil {
			return err
		}
		viewSpec["viewGuid"] = viewID
	}
	if plan.Action == "table-field-delete" || plan.Action == "table-field-convert" {
		tables, e := s.baseSnapshot()
		if e != nil {
			return e
		}
		dependencyPayload := payload
		if plan.Action == "table-field-convert" {
			dependencyPayload = map[string]any{"fieldGuids": []any{str(payload, "fieldGuid")}}
		}
		plan.UnchangedTables, e = phase3FieldDeleteDependencies(tables, tableGuid, dependencyPayload)
		if e != nil {
			return e
		}
	}
	if plan.Action == "table-view-delete" {
		tables, e := s.baseSnapshot()
		if e != nil {
			return e
		}
		plan.UnchangedTables, e = phase3ViewDeleteDependencies(tables, tableGuid, str(payload, "viewGuid"))
		if e != nil {
			return e
		}
	}
	started := time.Now()
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		return err
	}
	commandRecord := map[string]any{"argv": os.Args, "targetGuid": s.guid, "tableGuid": tableGuid, "startedUTC": started.UTC().Format(time.RFC3339), "startedShanghai": started.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format(time.RFC3339), "binarySHA256": fmt.Sprintf("%x", sha256.Sum256(binary))}
	dir, err := a.phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "command.json"), commandRecord); err != nil {
		return err
	}
	attempted := false
	defer func() {
		if runErr != nil {
			failure := map[string]any{"ok": false, "mutationAttempted": attempted, "stage": "pre-mutation"}
			if attempted {
				failure["stage"], failure["mayHaveApplied"] = "post-mutation", true
				failure["originalError"] = runErr.Error()
				// Persist the original failure before the bounded diagnostic read.
				failure["error"] = fmt.Sprintf("写请求已提交或可能已提交；禁止自动重放：%v", runErr)
				if evidenceErr := phase3WriteEvidence(filepath.Join(dir, "failure.json"), failure); evidenceErr != nil {
					runErr = fmt.Errorf("%w；原始失败证据无法保存：%v", runErr, evidenceErr)
				}
				if report, reportErr := s.phase3WriteTableRecordFailure(dir, before, plan, rev, runErr); reportErr != nil {
					runErr = fmt.Errorf("%w；%v", runErr, reportErr)
				} else if report != "" {
					failure["recordOutcomes"] = report
					runErr = fmt.Errorf("%w；逐条结果报告：%s", runErr, report)
				}
				runErr = fmt.Errorf("写请求已提交或可能已提交；禁止自动重放；请只读检查。证据目录 %s：%w", dir, runErr)
			}
			failure["error"] = runErr.Error()
			if evidenceErr := phase3WriteEvidence(filepath.Join(dir, "failure.json"), failure); evidenceErr != nil {
				runErr = fmt.Errorf("%w；失败证据无法保存：%v", runErr, evidenceErr)
			}
		}
	}()
	if err = phase3WriteEvidence(filepath.Join(dir, "before-structure.json"), before); err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "plan.json"), plan); err != nil {
		return err
	}
	if navigation, ok := guard.(interface{ NavigateTable(string) error }); ok && str(payload, "tableGuid") != "" {
		if err = navigation.NavigateTable(str(payload, "tableGuid")); err != nil {
			return err
		}
	}
	if proof, ok := guard.(interface{ Capture(string) error }); ok {
		if err = proof.Capture(filepath.Join(dir, "before.png")); err != nil {
			return err
		}
	}
	if err = guard.CheckPresence(); err != nil {
		return err
	}
	current, err := s.tableRevision()
	if err != nil {
		return err
	}
	if current != rev {
		return errors.New("准备期间 revision 已变化，拒绝写入")
	}
	if err = guard.CheckPresence(); err != nil {
		return err
	}
	attempted = true
	submitted, err := s.composeTable(plan, rev)
	if err != nil {
		return fmt.Errorf("写请求结果未通过；不要自动重试：%w", err)
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "submission.json"), submitted); err != nil {
		return err
	}
	var after map[string]any
	var verifyErr error
	for i := 0; i < 8; i++ {
		if err = guard.CheckPresence(); err != nil {
			return fmt.Errorf("已提交写入，协作状态变化，必须人工检查：%w", err)
		}
		after, err = s.tableSnapshot(tableGuid)
		if err == nil {
			verifyErr = verifyPhase3TableEdit(before, after, plan)
			if verifyErr == nil {
				verifyErr = s.verifyPhase3UnchangedTables(plan.UnchangedTables, tableGuid)
			}
			if verifyErr == nil {
				break
			}
		} else {
			verifyErr = err
		}
		time.Sleep(time.Second)
	}
	if after != nil {
		if err = phase3WriteEvidence(filepath.Join(dir, "after-structure.json"), after); err != nil {
			return err
		}
	}
	if verifyErr != nil {
		return fmt.Errorf("已提交但持久化验收失败；禁止自动重试：%w", verifyErr)
	}
	finalRev, err := s.tableRevision()
	if err != nil {
		return err
	}
	if finalRev <= rev {
		return errors.New("内容已读回但 revision 未推进，不能确认持久化")
	}
	if viewSpec != nil {
		v, e := guard.(interface {
			ViewProof(map[string]any) (any, error)
		}).ViewProof(viewSpec)
		if e != nil {
			return fmt.Errorf("视图已保存但原生展示未验收：%w", e)
		}
		if e = phase3WriteEvidence(filepath.Join(dir, "rendered-view.json"), v); e != nil {
			return e
		}
	}
	if proof, ok := guard.(interface {
		Capture(string) error
		FreshCapture(string) error
	}); ok {
		if err = proof.Capture(filepath.Join(dir, "after.png")); err != nil {
			return fmt.Errorf("写入已验收但截图失败：%w", err)
		}
		if err = proof.FreshCapture(filepath.Join(dir, "reload.png")); err != nil {
			return fmt.Errorf("写入已验收但全新页面验证失败：%w", err)
		}
		reopened, e := s.tableSnapshot(tableGuid)
		if e != nil {
			return e
		}
		if e = verifyPhase3TableEdit(before, reopened, plan); e != nil {
			return e
		}
		if e = s.verifyPhase3UnchangedTables(plan.UnchangedTables, tableGuid); e != nil {
			return e
		}
		if err = phase3WriteEvidence(filepath.Join(dir, "reopened-structure.json"), reopened); err != nil {
			return err
		}
		if viewSpec != nil {
			v, e := guard.(interface {
				ViewProof(map[string]any) (any, error)
			}).ViewProof(viewSpec)
			if e != nil {
				return e
			}
			if e = phase3WriteEvidence(filepath.Join(dir, "reopened-view.json"), v); e != nil {
				return e
			}
			if e = proof.Capture(filepath.Join(dir, "reload.png")); e != nil {
				return e
			}
		}
	}
	result := map[string]any{"ok": true, "verified": true, "action": plan.Action, "url": s.target, "tableGuid": tableGuid, "beforeRev": rev, "afterRev": finalRev, "createdIds": plan.CreatedIDs, "matchedIds": plan.MatchedIDs, "evidenceDir": dir, "scope": "requested table operation only; not entire phase3 scenario"}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printJSON(result)
}

func phase3TableFieldProps(typ string, field map[string]any) (any, error) {
	if typ != "SingleChoice" {
		if _, exists := field["options"]; exists {
			return nil, errors.New("仅 SingleChoice 接受 options")
		}
		if typ == "Date" {
			return map[string]any{"dateFormat": "yyyy/M/d", "isAutoFill": false}, nil
		}
		return nil, nil
	}
	options, ok := field["options"].([]any)
	if !ok || len(options) == 0 || len(options) > 100 {
		return nil, errors.New("单选字段 options 必须含 1 至 100 个选项名称")
	}
	choices := map[string]any{}
	order := []any{}
	names := map[string]bool{}
	for _, v := range options {
		name, ok := v.(string)
		if !ok || strings.TrimSpace(name) == "" || names[name] {
			return nil, errors.New("选项名称为空或重复")
		}
		names[name] = true
		id, err := phase3TableID()
		if err != nil {
			return nil, err
		}
		id = id[:11]
		order = append(order, id)
		choices[id] = map[string]any{"guid": id, "name": name, "hidden": false, "locked": false, "statusType": float64(0), "style": nil}
	}
	return map[string]any{"choices": choices, "choiceOrder": order, "disableColors": true, "allowAutoAdd": false, "defaultChoices": nil}, nil
}
