package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

type phase3TableRecordOutcome struct {
	ID       string         `json:"id"`
	Index    int            `json:"inputIndex"`
	Status   string         `json:"status"`
	Source   string         `json:"source"`
	Reason   string         `json:"reason"`
	Expected any            `json:"expected"`
	Observed map[string]any `json:"observed,omitempty"`
}

type phase3TablePartialReport struct {
	OK                     bool                       `json:"ok"`
	Verified               bool                       `json:"verified"`
	MutationAttempted      bool                       `json:"mutationAttempted"`
	MayHaveApplied         bool                       `json:"mayHaveApplied"`
	AutomaticReplayAllowed bool                       `json:"automaticReplayAllowed"`
	Action                 string                     `json:"action"`
	TableGuid              string                     `json:"tableGuid"`
	Stage                  string                     `json:"stage"`
	OriginalError          string                     `json:"originalError"`
	ReadbackError          string                     `json:"readbackError,omitempty"`
	WholeTableError        string                     `json:"wholeTableError,omitempty"`
	ObservedRevision       float64                    `json:"observedRevision"`
	ObservedAt             string                     `json:"observedAt"`
	Scope                  string                     `json:"scope"`
	Records                []phase3TableRecordOutcome `json:"records"`
}

func phase3TableRecordOutcomes(before map[string]any, plan phase3TableEditPlan, after map[string]any, revision, baseRev float64, readErr, originalErr error) phase3TablePartialReport {
	r := phase3TablePartialReport{
		Action: plan.Action, TableGuid: plan.TableGuid, Stage: "failure-readback",
		MutationAttempted: true, MayHaveApplied: true, OriginalError: originalErr.Error(),
		ObservedRevision: revision, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Scope:   "逐条状态仅描述稳定且已推进 revision 的一次只读快照；not_applied 不代表安全重试。请求可能仍在处理，禁止自动重放；更新项 inputIndex 为匹配 ID 排序。",
		Records: []phase3TableRecordOutcome{},
	}
	if readErr == nil {
		readErr = phase3ValidateTableSnapshot(after, str(before, "baseGuid"), plan.TableGuid)
	}
	if readErr == nil && (math.IsNaN(revision) || math.IsInf(revision, 0) || revision <= baseRev) {
		readErr = errors.New("revision 未明确推进，可能仍在处理；不能判断未写入")
	}
	if readErr != nil {
		r.ReadbackError = readErr.Error()
	} else if err := verifyPhase3TableEdit(before, after, plan); err != nil {
		r.WholeTableError = err.Error()
	}
	ids := plan.CreatedIDs
	if plan.Action == "table-record-update" {
		ids = plan.MatchedIDs
	}
	expected, _ := plan.Expected["records"].(map[string]any)
	oldRows, rows := phase3TableMap(before, "records"), phase3TableMap(after, "records")
	for i, id := range ids {
		item := phase3TableRecordOutcome{ID: id, Index: i + 1, Status: "unknown", Source: "unknown", Reason: "读回不完整或 revision 不确定", Expected: expected[id]}
		if readErr == nil {
			item.Source = "stable_readback"
			item = phase3ClassifyTableRecord(item, plan.Action, oldRows[id], rows[id])
		}
		r.Records = append(r.Records, item)
	}
	return r
}

func phase3ClassifyTableRecord(item phase3TableRecordOutcome, action string, before, after map[string]any) phase3TableRecordOutcome {
	if after != nil {
		item.Observed, _ = after["valueMap"].(map[string]any)
	}
	expected, valid := item.Expected.(map[string]any)
	if !valid {
		item.Reason = "请求计划缺少该记录的完整预期值"
	} else if after != nil && reflect.DeepEqual(item.Observed, expected) {
		item.Status, item.Reason = "applied", "该 ID 的完整 valueMap 与请求后预期一致；不代表整体动作验收通过"
	} else if action == "table-record-add" && after == nil && before == nil {
		item.Status, item.Reason = "not_applied", "该预定新增 ID 在本次完整快照中不存在；禁止据此重放"
	} else if action == "table-record-update" && before != nil && after != nil && reflect.DeepEqual(item.Observed, before["valueMap"]) {
		item.Status, item.Reason = "not_applied", "该 ID 仍与写前完整值一致；禁止据此重放"
	} else {
		item.Reason = "记录缺失或值既不等于完整预期也不等于写前状态，可能部分写入或并发改变"
	}
	return item
}

// This diagnostic never calls authRequestPath: neither a 401 nor a timeout can
// refresh browser credentials, retry a compose, or repeat the enclosing command.
func (s *phase3SDKSession) phase3TablePartialSnapshot(ctx context.Context, tableGuid string) (map[string]any, float64, error) {
	cookie, err := readCookie()
	if err != nil {
		return nil, 0, err
	}
	get := func(path string) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		r, err := s.app.requestURLContext(ctx, strings.TrimRight(s.app.Origin, "/")+s.prefix+path, "GET", nil, nil, s.headers, cookie)
		if err != nil {
			// Network errors can contain signed request URLs. Keep diagnostics safe.
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, errors.New("逐条结果只读网络请求失败；未刷新认证或重试写入")
		}
		return phase3SDKJSON(r)
	}
	revision := func() (float64, error) {
		data, err := get("table-api/bases/" + url.PathEscape(s.guid) + "/rev")
		if err != nil {
			return 0, err
		}
		m, _ := data.(map[string]any)
		value, ok := m["rev"].(float64)
		permission, _ := m["filePermission"].(map[string]any)
		if !ok || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value != math.Trunc(value) || !boolv(permission, "canRead") {
			return 0, errors.New("逐条结果缺少有效 revision 或读取权限")
		}
		return value, nil
	}
	startRev, err := revision()
	if err != nil {
		return nil, 0, err
	}
	data, err := get("table-api/tables/" + url.PathEscape(tableGuid) + "?withData=true")
	if err != nil {
		return nil, 0, err
	}
	table, _ := data.(map[string]any)
	if err = phase3ValidateTableSnapshot(table, s.guid, tableGuid); err != nil {
		return nil, 0, err
	}
	endRev, err := revision()
	if err != nil {
		return nil, 0, err
	}
	if startRev != endRev {
		return nil, endRev, errors.New("逐条结果读取期间 revision 改变，结果未知")
	}
	return table, endRev, nil
}

func (s *phase3SDKSession) phase3WriteTableRecordFailure(dir string, before map[string]any, plan phase3TableEditPlan, baseRev float64, originalErr error) (string, error) {
	if plan.Action != "table-record-add" && plan.Action != "table-record-update" {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	after, rev, readErr := s.phase3TablePartialSnapshot(ctx, plan.TableGuid)
	report := phase3TableRecordOutcomes(before, plan, after, rev, baseRev, readErr, originalErr)
	path := filepath.Join(dir, "record-outcomes.json")
	if err := phase3WriteEvidence(path, report); err != nil {
		return "", fmt.Errorf("逐条失败报告无法保存：%w", err)
	}
	return path, nil
}
