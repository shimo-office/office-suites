package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

type phase3BaseEditPlan struct {
	phase3TableEditPlan
	SourceGuid         string                    `json:"sourceGuid,omitempty"`
	Name               string                    `json:"name,omitempty"`
	Desc               string                    `json:"desc,omitempty"`
	FieldGuid          string                    `json:"fieldGuid,omitempty"`
	ViewGuid           string                    `json:"viewGuid,omitempty"`
	RowGuid            string                    `json:"rowGuid,omitempty"`
	Dashboard          map[string]any            `json:"dashboard,omitempty"`
	ReferenceTableGuid string                    `json:"referenceTableGuid,omitempty"`
	ReferenceFieldGuid string                    `json:"referenceFieldGuid,omitempty"`
	LinkedExpected     map[string]map[string]any `json:"linkedExpected,omitempty"`
}

func preparePhase3BaseEdit(action string, payload map[string]any, baseGuid string, tables map[string]map[string]any) (phase3BaseEditPlan, error) {
	p := phase3BaseEditPlan{phase3TableEditPlan: phase3TableEditPlan{Action: action, TableGuid: str(payload, "tableGuid")}}
	if len(tables) == 0 {
		return p, errors.New("没有可验证的数据表")
	}
	if action == "table-chart-style" {
		return preparePhase3ChartStyle(payload, baseGuid, tables)
	}
	if action == "table-chart-create" {
		return preparePhase3TableChart(payload, baseGuid, tables)
	}
	if action == "table-reference-add" {
		return preparePhase3TableReference(payload, baseGuid, tables)
	}
	if action == "table-reference-set" {
		return preparePhase3ReferenceSet(payload, baseGuid, tables)
	}
	if action == "table-delete" {
		if err := phase3TablePayloadKeys(payload, "tableGuid"); err != nil {
			return p, err
		}
		dataTables := 0
		for _, table := range tables {
			if num(table, "tableType") != 4 {
				dataTables++
			}
		}
		if dataTables < 2 {
			return p, errors.New("不能删除最后一个数据表")
		}
		if tables[p.TableGuid] == nil {
			return p, errors.New("待删除数据表不存在")
		}
		if num(tables[p.TableGuid], "tableType") == 4 {
			return p, errors.New("仪表盘不能作为数据表删除")
		}
		for id, t := range tables {
			if id == p.TableGuid {
				if str(t, "formGuid") != "" || t["syncedConfig"] != nil {
					return p, errors.New("关联表单或同步数据表不允许删除")
				}
			} else if phase3ContainsTableReference(t, p.TableGuid) {
				return p, errors.New("其他数据表引用此表，拒绝删除")
			}
		}
		return p, p.add("DeleteTable", map[string]any{"tableGuid": p.TableGuid})
	}
	if err := phase3TablePayloadKeys(payload, "name", "desc", "tableGuid"); err != nil {
		return p, err
	}
	name, ok := payload["name"].(string)
	if !ok || strings.TrimSpace(name) == "" || len(name) > 200 {
		return p, errors.New("数据表名称为空或过长")
	}
	for _, t := range tables {
		if str(t, "name") == name {
			return p, errors.New("数据表名称已存在")
		}
	}
	p.Name = name
	if raw, exists := payload["desc"]; exists {
		var ok bool
		p.Desc, ok = raw.(string)
		if !ok || len(p.Desc) > 10000 {
			return p, errors.New("说明必须为字符串且不超过10000字节")
		}
	}
	id, err := phase3TableID()
	if err != nil {
		return p, err
	}
	p.TableGuid = id[:11]
	if action == "table-duplicate" {
		p.SourceGuid = str(payload, "tableGuid")
		source := tables[p.SourceGuid]
		if source == nil {
			return p, errors.New("待复制数据表不存在")
		}
		if num(source, "tableType") == 4 {
			return p, errors.New("仪表盘不能作为数据表复制")
		}
		if _, exists := payload["desc"]; exists {
			return p, errors.New("复制仅接受原表与新名称，说明保留源值")
		}
		if str(source, "formGuid") != "" || source["syncedConfig"] != nil {
			return p, errors.New("暂不复制关联表单或同步表")
		}
		for _, f := range phase3TableMap(source, "fields") {
			switch str(f, "type") {
			case "SingleLineText", "Number", "SingleChoice", "LongText", "CheckBox":
			default:
				return p, errors.New("复制包含尚未验证的复杂字段类型")
			}
		}
		p.Desc = str(source, "desc")
		return p, nil
	}
	if action != "table-create" {
		return p, errors.New("不支持的数据表生命周期动作")
	}
	if _, exists := payload["tableGuid"]; exists {
		return p, errors.New("新建不接受已有 tableGuid")
	}
	ids := make([]string, 3)
	for i := range ids {
		v, e := phase3TableID()
		if e != nil {
			return p, e
		}
		ids[i] = v[:11]
	}
	p.FieldGuid, p.ViewGuid, p.RowGuid = ids[0], ids[1], ids[2]
	ordered := make([]string, 0, len(tables))
	for id := range tables {
		ordered = append(ordered, id)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		a := num(tables[ordered[i]], "order")
		b := num(tables[ordered[j]], "order")
		if a == b {
			return ordered[i] < ordered[j]
		}
		return a < b
	})
	data := map[string]any{"baseGuid": baseGuid, "name": p.Name, "tableGuid": p.TableGuid, "targetGuid": ordered[len(ordered)-1], "rowsGuid": []string{p.RowGuid}, "viewGuid": p.ViewGuid, "viewName": "默认视图", "desc": p.Desc, "cols": []any{map[string]any{"guid": p.FieldGuid, "name": "标题", "type": "SingleLineText", "props": nil, "isPrimary": true}}}
	return p, p.add("CreateTable", data)
}

func phase3ContainsTableReference(value any, id string) bool {
	switch v := value.(type) {
	case string:
		return v == id
	case []any:
		for _, x := range v {
			if phase3ContainsTableReference(x, id) {
				return true
			}
		}
	case map[string]any:
		for _, x := range v {
			if phase3ContainsTableReference(x, id) {
				return true
			}
		}
	}
	return false
}

func phase3BaseComparable(t map[string]any) map[string]any {
	if num(t, "tableType") == 4 {
		return phase3DashboardComparable(t)
	}
	out := phase3TableSemantic(t)
	layout := map[string]any{}
	for id, v := range phase3TableMap(t, "views") {
		layout[id] = map[string]any{"fields": v["fields"], "records": v["records"], "fieldInfo": v["fieldInfo"]}
	}
	out["viewLayout"] = layout
	return out
}

func verifyPhase3BaseEdit(before, after map[string]map[string]any, p phase3BaseEditPlan) error {
	if p.Action == "table-reference-set" || p.Action == "table-chart-style" {
		if len(after) != len(p.LinkedExpected) {
			return errors.New("写入改变数据表数量")
		}
		for id, want := range p.LinkedExpected {
			if after[id] == nil || !reflect.DeepEqual(phase3BaseComparable(after[id]), phase3BaseComparable(want)) {
				return errors.New("目标内容或其他数据未完整按预期持久化")
			}
		}
		return nil
	}
	wantCount := len(before) + 1
	if p.Action == "table-delete" {
		wantCount = len(before) - 1
	}
	if p.Action == "table-reference-add" {
		wantCount = len(before)
	}
	if len(after) != wantCount {
		return errors.New("数据表数量未达到预期")
	}
	for id, t := range before {
		if p.Action == "table-reference-add" && (id == p.TableGuid || id == p.ReferenceTableGuid) {
			continue
		}
		if p.Action == "table-delete" && id == p.TableGuid {
			if after[id] != nil {
				return errors.New("已删除数据表仍然存在")
			}
			continue
		}
		if after[id] == nil || !reflect.DeepEqual(phase3BaseComparable(t), phase3BaseComparable(after[id])) {
			return fmt.Errorf("原有数据表 %s 被意外改变", id)
		}
	}
	if p.Action == "table-delete" {
		return nil
	}
	if p.Action == "table-reference-add" {
		return verifyPhase3TableReference(before, after, p)
	}
	if p.Action == "table-chart-create" {
		return verifyPhase3Dashboard(after[p.TableGuid], p.Dashboard)
	}
	t := after[p.TableGuid]
	if t == nil || str(t, "name") != p.Name || str(t, "desc") != p.Desc {
		return errors.New("新数据表身份、名称或说明不符")
	}
	if p.Action == "table-duplicate" {
		return verifyPhase3TableDuplicate(before[p.SourceGuid], t)
	}
	fields, views, rows := phase3TableMap(t, "fields"), phase3TableMap(t, "views"), phase3TableMap(t, "records")
	if len(fields) != 1 || len(views) != 1 || len(rows) != 1 {
		return errors.New("新表字段、视图或记录数量不符")
	}
	f, v, r := fields[p.FieldGuid], views[p.ViewGuid], rows[p.RowGuid]
	if f == nil || v == nil || r == nil || str(f, "name") != "标题" || str(f, "type") != "SingleLineText" || !boolv(f, "isPrimary") || str(v, "name") != "默认视图" || str(v, "type") != "Grid" {
		return errors.New("新表结构不符")
	}
	values, ok := r["valueMap"].(map[string]any)
	if !ok || len(values) != 0 {
		return errors.New("新表空记录包含意外数据")
	}
	if !reflect.DeepEqual(v["fields"], []any{p.FieldGuid}) || !reflect.DeepEqual(v["records"], []any{p.RowGuid}) {
		return errors.New("新表视图字段/记录顺序不符")
	}
	return nil
}

// A copy may allocate new IDs. Match fields by unique name and preserve row and
// view order, then translate all internal references before comparing content.
func verifyPhase3TableDuplicate(source, target map[string]any) error {
	replacements := map[string]string{str(target, "guid"): str(source, "guid")}
	for _, key := range []string{"fields", "records", "views"} {
		a, b := phase3TableRows(source, key), phase3TableRows(target, key)
		if len(a) != len(b) {
			return fmt.Errorf("副本 %s 数量不一致", key)
		}
		for i := range a {
			am, _ := a[i].(map[string]any)
			bm, _ := b[i].(map[string]any)
			if key != "records" && str(am, "name") != str(bm, "name") {
				return errors.New("副本字段/视图名称或顺序不符")
			}
			replacements[str(bm, "guid")] = str(am, "guid")
		}
	}
	a, b := phase3DuplicateComparable(source, nil), phase3DuplicateComparable(target, replacements)
	if !reflect.DeepEqual(a, b) {
		return errors.New("副本内容、字段属性或视图设置不等价")
	}
	return nil
}

func (s *phase3SDKSession) baseSnapshot() (map[string]map[string]any, error) {
	raw, err := s.get("table-api/v2/bases/" + url.PathEscape(s.guid))
	if err != nil {
		return nil, err
	}
	base, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("应用表格 base 非对象")
	}
	tables, ok := base["tables"].([]any)
	if !ok || len(tables) == 0 {
		return nil, errors.New("应用表格 tables 无效")
	}
	out := map[string]map[string]any{}
	for _, v := range tables {
		m, ok := v.(map[string]any)
		id := str(m, "guid")
		if !ok || id == "" || out[id] != nil {
			return nil, errors.New("数据表身份缺失/重复")
		}
		n, ok := m["tableType"].(float64)
		if !ok || (n != 0 && n != 4) {
			return nil, errors.New("未知或特殊数据表类型，拒绝生命周期写入")
		}
		var t map[string]any
		var e error
		if n == 4 {
			t, e = s.dashboardSnapshot(m)
		} else {
			t, e = s.tableSnapshot(id)
		}
		if e != nil {
			return nil, e
		}
		out[id] = t
	}
	return out, nil
}

func (a *App) cmdPhase3BaseEdit() (runErr error) {
	if len(a.Pos) != 1 {
		return errors.New("用法：phase3 链接 --action table-create|table-duplicate|table-delete|table-reference-add|table-chart-create --payload JSON --out 目录")
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
	s, err := a.phase3SDK(target, "table")
	if err != nil {
		return err
	}
	rev, err := s.tableRevision()
	if err != nil {
		return err
	}
	before, err := s.baseSnapshot()
	if err != nil {
		return err
	}
	plan, err := preparePhase3BaseEdit(a.opt("--action"), payload, s.guid, before)
	if err != nil {
		return err
	}
	dir, err := a.phase3EvidenceDir(a.opt("--out"))
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	binary, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "command.json"), map[string]any{"argv": os.Args, "startedUTC": time.Now().UTC().Format(time.RFC3339), "binarySHA256": fmt.Sprintf("%x", sha256.Sum256(binary)), "targetGuid": s.guid}); err != nil {
		return err
	}
	attempted := false
	defer func() {
		if runErr != nil {
			if attempted {
				runErr = fmt.Errorf("写请求已提交或可能已提交；禁止自动重放；请只读检查。证据目录 %s：%w", dir, runErr)
			}
			_ = phase3WriteEvidence(filepath.Join(dir, "failure.json"), map[string]any{"ok": false, "mutationAttempted": attempted, "error": runErr.Error()})
		}
	}()
	for name, v := range map[string]any{"before-structure.json": before, "plan.json": plan} {
		if err = phase3WriteEvidence(filepath.Join(dir, name), v); err != nil {
			return err
		}
	}
	var previousChart map[string]any
	if plan.Action == "table-chart-style" {
		proof, ok := guard.(interface {
			ChartProof(map[string]any, map[string]int) (any, error)
		})
		if !ok {
			return errors.New("图表渲染验收桥未安装，未执行写入")
		}
		oldPlan := plan
		oldPlan.Dashboard = before[plan.TableGuid]
		expected, e := phase3ChartExpectedCounts(before[plan.SourceGuid], plan.FieldGuid)
		if e != nil {
			return e
		}
		v, e := proof.ChartProof(phase3ChartProofSpec(oldPlan), expected)
		if e != nil {
			return e
		}
		previousChart, ok = v.(map[string]any)
		if !ok {
			return errors.New("原图表样式证据无效")
		}
		if e = phase3WriteEvidence(filepath.Join(dir, "before-chart.json"), previousChart); e != nil {
			return e
		}
	}
	if navigation, ok := guard.(interface{ NavigateTable(string) error }); ok && str(payload, "tableGuid") != "" {
		if err = navigation.NavigateTable(str(payload, "tableGuid")); err != nil {
			return err
		}
	}
	if plan.Action == "table-reference-add" {
		capability, ok := guard.(interface {
			ReferenceCapability(string) (map[string]any, error)
		})
		if !ok {
			return errors.New("关联字段原生能力检查未安装，未执行写入")
		}
		actual, e := capability.ReferenceCapability(plan.TableGuid)
		if actual != nil {
			if writeErr := phase3WriteEvidence(filepath.Join(dir, "reference-capability.json"), actual); writeErr != nil {
				return writeErr
			}
		}
		if e != nil {
			return e
		}
		if e = verifyPhase3ReferenceCapability(actual, plan.TableGuid); e != nil {
			return e
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
	if plan.Action == "table-chart-create" || plan.Action == "table-chart-style" {
		if _, ok := guard.(interface {
			ChartProof(map[string]any, map[string]int) (any, error)
		}); !ok {
			return errors.New("图表渲染验收桥未安装，未执行写入")
		}
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
	var submitted any
	if plan.Action == "table-duplicate" {
		client, e := phase3TableID()
		if e != nil {
			return e
		}
		r, e := a.authRequestPath(s.prefix+"table-api/bases/"+url.PathEscape(s.guid)+"/duplicateTable", "POST", map[string]any{"sourceTableGuid": plan.SourceGuid, "tableName": plan.Name, "tableGuid": plan.TableGuid, "clientId": client}, nil, s.headers)
		if e != nil {
			return fmt.Errorf("复制请求结果未知；禁止自动重试：%w", e)
		}
		if e = phase3WriteEvidence(filepath.Join(dir, "duplicate-response-metadata.json"), map[string]any{"httpStatus": r.Status, "bodyBytes": len(r.Raw), "jsonBody": r.Data != nil}); e != nil {
			return e
		}
		submitted, err = phase3DuplicateResponse(r)
	} else {
		submitted, err = s.composeTable(plan.phase3TableEditPlan, rev)
	}
	if err != nil {
		return err
	}
	if err = phase3WriteEvidence(filepath.Join(dir, "submission.json"), submitted); err != nil {
		return err
	}
	var after map[string]map[string]any
	for i := 0; i < 8; i++ {
		if err = guard.CheckPresence(); err != nil {
			return err
		}
		after, err = s.baseSnapshot()
		if err == nil {
			err = verifyPhase3BaseEdit(before, after, plan)
			if err == nil {
				break
			}
		}
		time.Sleep(time.Second)
	}
	if after != nil {
		if e := phase3WriteEvidence(filepath.Join(dir, "after-structure.json"), after); e != nil {
			return e
		}
	}
	if err != nil {
		return fmt.Errorf("已提交但未通过持久化验证；禁止自动重试：%w", err)
	}
	finalRev, err := s.tableRevision()
	if err != nil {
		return err
	}
	if finalRev <= rev {
		return errors.New("revision 未推进，不能确认持久化")
	}
	if plan.Action == "table-chart-create" || plan.Action == "table-chart-style" {
		fieldID := plan.FieldGuid
		expected, e := phase3ChartExpectedCounts(before[plan.SourceGuid], fieldID)
		if e != nil {
			return e
		}
		proof, e := guard.(interface {
			ChartProof(map[string]any, map[string]int) (any, error)
		}).ChartProof(phase3ChartProofSpec(plan), expected)
		if e != nil {
			return fmt.Errorf("图表配置已保存但原生汇总未验收：%w", e)
		}
		if previousChart != nil {
			currentChart, ok := proof.(map[string]any)
			if !ok || reflect.DeepEqual(currentChart["palette"], previousChart["palette"]) {
				return errors.New("图表配置已保存但原生渲染调色板未改变")
			}
		}
		if e = phase3WriteEvidence(filepath.Join(dir, "rendered-chart.json"), map[string]any{"expected": expected, "actual": proof, "verified": true}); e != nil {
			return e
		}
	}
	if navigation, ok := guard.(interface{ NavigateTable(string) error }); ok {
		destination := ""
		switch plan.Action {
		case "table-create", "table-duplicate":
			destination = plan.TableGuid
		case "table-delete":
			ids := []string{}
			for id, table := range after {
				if num(table, "tableType") != 4 {
					ids = append(ids, id)
				}
			}
			sort.Strings(ids)
			if len(ids) > 0 {
				destination = ids[0]
			}
		}
		if destination != "" {
			if err = navigation.NavigateTable(destination); err != nil {
				return fmt.Errorf("写入已保存但目标表导航失败：%w", err)
			}
		}
	}
	if proof, ok := guard.(interface {
		Capture(string) error
		FreshCapture(string) error
	}); ok {
		if err = proof.Capture(filepath.Join(dir, "after.png")); err != nil {
			return err
		}
		if err = proof.FreshCapture(filepath.Join(dir, "reload.png")); err != nil {
			return err
		}
		reopened, e := s.baseSnapshot()
		if e != nil {
			return e
		}
		if e = verifyPhase3BaseEdit(before, reopened, plan); e != nil {
			return e
		}
		if e = phase3WriteEvidence(filepath.Join(dir, "reopened-structure.json"), reopened); e != nil {
			return e
		}
		if plan.Action == "table-chart-create" || plan.Action == "table-chart-style" {
			expected, e := phase3ChartExpectedCounts(before[plan.SourceGuid], plan.FieldGuid)
			if e != nil {
				return e
			}
			proof, e := guard.(interface {
				ChartProof(map[string]any, map[string]int) (any, error)
			}).ChartProof(phase3ChartProofSpec(plan), expected)
			if e != nil {
				return e
			}
			if e = phase3WriteEvidence(filepath.Join(dir, "reopened-chart.json"), proof); e != nil {
				return e
			}
			if capture, ok := guard.(interface{ Capture(string) error }); ok {
				if e = capture.Capture(filepath.Join(dir, "reload.png")); e != nil {
					return e
				}
			}
		}
	}
	result := map[string]any{"ok": true, "verified": true, "action": plan.Action, "tableGuid": plan.TableGuid, "beforeRev": rev, "afterRev": finalRev, "evidenceDir": dir}
	if err = phase3WriteEvidence(filepath.Join(dir, "result.json"), result); err != nil {
		return err
	}
	return a.printJSON(result)
}
