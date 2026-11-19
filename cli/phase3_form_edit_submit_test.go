package main

import (
	"encoding/json"
	"testing"
)

func TestPhase3FormAnswersValidation(t *testing.T) {
	var form map[string]any
	_ = json.Unmarshal([]byte(`{"questions":[{"guid":"qtext","subject":"测试标记","type":"text","required":true,"question":{}},{"guid":"qrate","subject":"评分","type":"rate","required":true,"question":{"shape":"star","sum":5}}],"rules":[]}`), &form)
	payload := map[string]any{"qtext": "test marker", "qrate": float64(4)}
	got, err := phase3FormSubmissionAnswers(form, payload)
	if err != nil || len(got) != 2 {
		t.Fatal(got, err)
	}
	for _, bad := range []map[string]any{{"qtext": "", "qrate": float64(4)}, {"qtext": "test", "qrate": float64(6)}, {"qtext": "test", "qrate": float64(4.5)}, {"qtext": "test", "qrate": float64(4), "bogus": "x"}} {
		if _, err = phase3FormSubmissionAnswers(form, bad); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
}

func TestPhase3FormAnswersRespectVisibleBranch(t *testing.T) {
	b := formEditFixture()
	q := b["questions"].([]any)[0].(map[string]any)
	q["type"] = "single_choice"
	q["question"] = map[string]any{"options": []any{map[string]any{"type": "normal", "value": "yes1", "label": "yes"}, map[string]any{"type": "normal", "value": "no01", "label": "no"}}}
	form, err := phase3FormExpected(b, map[string]any{"operation": "rule-add", "trigger": "questionA", "options": []any{"yes1"}, "targets": []any{"questionB"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = phase3FormSubmissionAnswers(form, map[string]any{"questionA": "no01"}); err != nil {
		t.Fatal(err)
	}
	if _, err = phase3FormSubmissionAnswers(form, map[string]any{"questionA": "yes1"}); err == nil {
		t.Fatal("visible required followup omitted")
	}
	if _, err = phase3FormSubmissionAnswers(form, map[string]any{"questionA": "no01", "questionB": "hidden answer"}); err == nil {
		t.Fatal("hidden answer accepted")
	}
	if _, err = phase3FormSubmissionAnswers(form, map[string]any{"questionA": "yes1", "questionB": "followup"}); err != nil {
		t.Fatal(err)
	}
}

func TestPhase3FormFillResponses(t *testing.T) {
	form := formEditFixture()
	answers := map[string]any{"questionA": "合成答卷", "questionB": "说明"}
	got, err := phase3FormFillResponses(form, answers)
	if err != nil || len(got) != 2 {
		t.Fatal(got, err)
	}
	a := got[0].(map[string]any)
	if a["guid"] != "questionA" || a["response"].(map[string]any)["content"] != "合成答卷" {
		t.Fatal(a)
	}
	if _, err = phase3FormFillResponses(form, map[string]any{"missing": "x"}); err == nil {
		t.Fatal("unknown answer accepted")
	}
}

func TestPhase3FormFillRatingIncludesScale(t *testing.T) {
	form := formEditFixture()
	form["questions"] = append(form["questions"].([]any), map[string]any{"guid": "rating", "type": "rate", "required": true, "subject": "评分", "question": map[string]any{"shape": "star", "sum": float64(5)}})
	got, err := phase3FormFillResponses(form, map[string]any{"questionB": "说明", "rating": float64(4)})
	if err != nil {
		t.Fatal(err)
	}
	r := got[len(got)-1].(map[string]any)["response"].(map[string]any)
	if r["value"] != float64(4) || r["sum"] != float64(5) {
		t.Fatal(r)
	}
}

func TestPhase3FormSubmissionRejectsUnsupportedQuestionRules(t *testing.T) {
	form := formEditFixture()
	q := form["questions"].([]any)[0].(map[string]any)
	q["question"] = map[string]any{"validation": "email"}
	if _, err := phase3FormSubmissionAnswers(form, map[string]any{"questionA": "invalid-email", "questionB": "test"}); err == nil {
		t.Fatal("API submission must not bypass unsupported question validation")
	}
}
