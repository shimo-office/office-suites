package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestReleaseFormQuestionBoundaryValidation(t *testing.T) {
	base := map[string]any{"guid": "validQuestion", "subject": "Question", "required": true, "type": "text", "question": map[string]any{}}
	for _, tc := range []struct {
		name  string
		patch map[string]any
	}{
		{"unknown-field", map[string]any{"privateSideEffect": true}},
		{"bad-id", map[string]any{"guid": "!"}},
		{"missing-title", map[string]any{"subject": " "}},
		{"large-title", map[string]any{"subject": strings.Repeat("x", 20001)}},
		{"invalid-required", map[string]any{"required": "yes"}},
		{"invalid-description", map[string]any{"description": float64(4)}},
		{"body-not-object", map[string]any{"question": []any{}}},
		{"quiz-score", map[string]any{"question": map[string]any{"total": float64(1)}}},
		{"unsupported-type", map[string]any{"type": "payment"}},
		{"unknown-rating-config", map[string]any{"type": "rate", "question": map[string]any{"shape": "star", "sum": float64(5), "allowHalf": true}}},
		{"fractional-rating-scale", map[string]any{"type": "rate", "question": map[string]any{"shape": "star", "sum": float64(4.5)}}},
		{"unknown-choice-config", map[string]any{"type": "single_choice", "question": map[string]any{"shuffle": true}}},
		{"limited-multiple", map[string]any{"type": "multiple_choice", "question": map[string]any{"limitCount": true}}},
		{"duplicate-options", map[string]any{"type": "dropdown_list", "question": map[string]any{"options": []any{map[string]any{"type": "normal", "value": "optA", "label": "A"}, map[string]any{"type": "normal", "value": "optA", "label": "Again"}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := phase3TableCopy(base)
			for k, v := range tc.patch {
				q[k] = v
			}
			if _, err := phase3FormExpected(formEditFixture(), map[string]any{"operation": "question-add", "question": q}); err == nil {
				t.Fatal("invalid question accepted")
			}
		})
	}
}

func TestReleaseFormEditAndRuleContracts(t *testing.T) {
	for _, req := range []map[string]any{
		{"operation": "question-add"}, {"operation": "question-add", "question": map[string]any{"guid": "newQuestion", "type": "text", "required": false, "subject": "Valid", "question": map[string]any{}}, "index": float64(100)},
		{"operation": "question-update", "guid": "missing", "patch": map[string]any{"subject": "New"}},
		{"operation": "question-update", "guid": "questionA", "patch": map[string]any{}},
		{"operation": "question-reorder", "from": float64(0), "to": float64(100)},
		{"operation": "meta", "patch": map[string]any{}},
		{"operation": "meta", "patch": map[string]any{"confirmation": map[string]any{"type": "custom", "confirmation": map[string]any{"showTitle": "yes", "content": "Thank you"}}}},
		{"operation": "meta", "patch": map[string]any{"confirmation": map[string]any{"type": "custom", "confirmation": map[string]any{"showTitle": true, "content": " "}}}},
	} {
		if _, err := phase3FormExpected(formEditFixture(), req); err == nil {
			t.Fatal("invalid edit accepted", req)
		}
	}
	base, _ := phase3DecodeFormDelta(releaseFormRuleDelta(t, false))
	for _, req := range []map[string]any{
		{"operation": "rule-update", "guid": "missing"}, {"operation": "rule-delete", "guid": "missing"},
		{"operation": "rule-add", "trigger": "questionA", "options": []any{}},
		{"operation": "rule-add", "trigger": "questionA", "options": []any{"optA", "optA"}, "targets": []any{"questionB"}},
		{"operation": "rule-add", "trigger": "questionA", "options": []any{true}, "targets": []any{"questionB"}},
		{"operation": "rule-add", "trigger": "questionA", "options": []any{"optA"}, "targets": []any{}},
		{"operation": "rule-add", "trigger": "missing", "options": []any{"optA"}, "targets": []any{"questionB"}},
		{"operation": "rule-add", "trigger": "questionB", "options": []any{"optA"}, "targets": []any{"questionA"}},
	} {
		if _, err := phase3FormExpected(base, req); err == nil {
			t.Fatal("invalid rule accepted", req)
		}
	}
	update, err := phase3FormExpected(base, map[string]any{"operation": "rule-update", "guid": "existingRule", "trigger": "questionA", "options": []any{"optA"}, "targets": []any{"questionB"}})
	if err != nil || len(update["rules"].([]any)) != 1 {
		t.Fatal("valid rule update failed", err)
	}
	deleted, err := phase3FormExpected(base, map[string]any{"operation": "rule-delete", "guid": "existingRule"})
	if err != nil || len(deleted["rules"].([]any)) != 0 {
		t.Fatal("valid rule deletion failed", err)
	}
	original, _ := json.Marshal(base)
	after, _ := json.Marshal(update)
	if string(original) != string(after) {
		t.Fatal("identical rule update altered unrelated metadata")
	}
}

func TestReleaseFormDeadlineZoneAndConfirmationBoundaries(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, req := range []map[string]any{{"deadline": "2027-01-01T00:00:00Z", "timezone": float64(0)}, {"deadline": "2027-01-01T00:00:00Z", "timezone": "Asia/Shanghai"}} {
		if _, err := phase3FormSettingsPayload(req, now); err == nil {
			t.Fatal("incorrect timezone accepted", req)
		}
	}
	for _, v := range []any{nil, map[string]any{"type": "redirect", "url": "https://evil.test"}, map[string]any{"type": "custom", "confirmation": "bad"}} {
		if err := phase3ValidateConfirmation(v); err == nil {
			t.Fatal("unsafe confirmation accepted", v)
		}
	}
}
