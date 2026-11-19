package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestPhase3SDKRuntimeRejectsUnsafeAndMissingConfig(t *testing.T) {
	for _, html := range []string{
		`<html>login</html>`,
		`const envs = [{"__RUNTIME_ENV__":{"SDK_V2_PATH_PREFIX":"https://evil.test/","SDK_V2_SIGNATURE":"s","SDK_V2_TOKEN":"t"}}];`,
		`const envs = [{"__RUNTIME_ENV__":{"SDK_V2_PATH_PREFIX":"//evil.test/","SDK_V2_SIGNATURE":"s","SDK_V2_TOKEN":"t"}}];`,
		`const envs = [{"__RUNTIME_ENV__":{"SDK_V2_PATH_PREFIX":"/sdk/v2/api"}}];`,
	} {
		if _, err := phase3ParseSDKRuntime([]byte(html)); err == nil {
			t.Fatal("accepted invalid runtime")
		}
	}
	r, err := phase3ParseSDKRuntime([]byte(`const envs = [{"__RUNTIME_ENV__":{"SDK_V2_PATH_PREFIX":"/sdk/v2/api","SDK_V2_SIGNATURE":"private-signature","SDK_V2_TOKEN":"private-token"}}];`))
	if err != nil || r.prefix != "/sdk/v2/api/" {
		t.Fatalf("runtime parse: %v", err)
	}
}

func TestPhase3SDKJSONRejectsHTMLAndServerErrors(t *testing.T) {
	for _, r := range []*Response{
		{Status: 200, Header: http.Header{}, Raw: []byte("<html>login</html>")},
		{Status: 200, Data: map[string]any{"errorCode": float64(20221), "error": "unpublished"}},
		{Status: 403, Data: map[string]any{"error": "forbidden"}},
	} {
		if _, err := phase3SDKJSON(r); err == nil {
			t.Fatal("accepted false success")
		}
	}
}

func TestPhase3TableSnapshotRejectsUnverifiableData(t *testing.T) {
	base := map[string]any{"guid": "table1", "baseGuid": "base1", "fields": []any{}, "views": []any{}, "records": []any{}}
	if err := phase3ValidateTableSnapshot(base, "base1", "table1"); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"fields", "views", "records"} {
		x := map[string]any{}
		for a, b := range base {
			x[a] = b
		}
		delete(x, k)
		if err := phase3ValidateTableSnapshot(x, "base1", "table1"); err == nil {
			t.Fatalf("missing %s accepted", k)
		}
	}
	base["guid"] = "different"
	if err := phase3ValidateTableSnapshot(base, "base1", "table1"); err == nil {
		t.Fatal("wrong table accepted")
	}
}

func TestPhase3TableSafeSummaryOmitsSignedURLs(t *testing.T) {
	input := map[string]any{"guid": "t", "name": "table", "urls": []any{"https://private/?signature=secret"}, "createdBy": 1}
	out := phase3TableSummary(input)
	if strings.Contains(strings.Join(mapKeysForTableTest(out), ","), "urls") {
		t.Fatal("signed URL included")
	}
	if out["guid"] != "t" {
		t.Fatal("lost guid")
	}
}
func mapKeysForTableTest(x map[string]any) []string {
	r := []string{}
	for k := range x {
		r = append(r, k)
	}
	return r
}

func TestPhase3TableSnapshotRejectsDuplicateIdentity(t *testing.T) {
	x := tableFixtureForEdit()
	rows := x["records"].([]any)
	x["records"] = append(rows, rows[0])
	if err := phase3ValidateTableSnapshot(x, "b1", "t1"); err == nil {
		t.Fatal("duplicate rows accepted")
	}
}
