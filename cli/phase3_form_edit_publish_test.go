package main

import (
	"testing"
	"time"
)

func TestPhase3FormSettingsPayload(t *testing.T) {
	now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	got, err := phase3FormSettingsPayload(map[string]any{"deadline": "2026-11-20T18:00:00+08:00", "timezone": "Asia/Shanghai"}, now)
	if err != nil || got["deadline"] != "2026-11-20T18:00:00+08:00" || got["timezone"] != float64(8) {
		t.Fatal(got, err)
	}
	for _, req := range []map[string]any{{"deadline": "2026-11-20 18:00"}, {"deadline": "2020-01-01T00:00:00Z"}, {"deadline": "2026-11-20T18:00:00+08:00", "submitterCondition": ""}, {"deadline": "2026-11-20T18:00:00+08:00", "timezone": "bad"}} {
		if _, err := phase3FormSettingsPayload(req, now); err == nil {
			t.Fatalf("accepted %v", req)
		}
	}
}
func TestPhase3FormPublicationVerification(t *testing.T) {
	f := formEditFixture()
	b := map[string]any{"status": "unpublished", "submitterCondition": "enterprise_member", "isQuiz": float64(0)}
	p, err := phase3FormPublishPayload(b, f, float64(42))
	if err != nil {
		t.Fatal(err)
	}
	if p["status"] != "normal" || p["submitterCondition"] != "enterprise_member" {
		t.Fatal(p)
	}
	if err = phase3VerifyPublication(b, p, f); err != nil {
		t.Fatal(err)
	}
	p["publishedRev"] = 2
	if phase3VerifyPublication(b, p, f) == nil {
		t.Fatal("accepted unexpected publication revision")
	}
	delete(p, "publishedRev")
	if phase3VerifyPublication(b, p, f) == nil {
		t.Fatal("accepted missing publication revision")
	}
	p["publishedRev"] = phase3FormPublicationRevision
	p["status"] = "unpublished"
	if phase3VerifyPublication(b, p, f) == nil {
		t.Fatal("initialization counted as publication")
	}
	p["status"] = "normal"
	p["publishedContent"] = `{"title":"stale"}`
	if phase3VerifyPublication(b, p, f) == nil {
		t.Fatal("accepted old published content")
	}
	p, _ = phase3FormPublishPayload(b, f, float64(42))
	p["submitterCondition"] = ""
	if phase3VerifyPublication(b, p, f) == nil {
		t.Fatal("accepted expanded permission")
	}
}

func TestPhase3FormMutationAckNeedsReadback(t *testing.T) {
	for _, status := range []int{200, 204} {
		ack, err := phase3FormMutationAck(&Response{Status: status})
		if err != nil || ack["actionVerified"] != false || ack["requiresIndependentReadback"] != true {
			t.Fatal(ack, err)
		}
	}
	for _, r := range []*Response{{Status: 401}, {Status: 200, Raw: []byte("<html>login</html>")}, {Status: 500}} {
		if _, err := phase3FormMutationAck(r); err == nil {
			t.Fatal("unsafe transport accepted", r.Status)
		}
	}
}
