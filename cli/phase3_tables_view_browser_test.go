package main

import "testing"

func TestPhase3NativeViewProofRejectsWrongOrStaleBinding(t *testing.T) {
	spec := map[string]any{"tableGuid": "t1", "viewGuid": "v1", "viewType": "Kanban"}
	actual := phase3TableCopy(spec)
	actual["timeOrigin"] = float64(20)
	actual["visibleNativeNodes"] = float64(2)
	if err := verifyPhase3NativeViewProof(actual, spec, 10); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tableGuid", "viewGuid", "viewType", "timeOrigin", "visibleNativeNodes"} {
		bad := phase3TableCopy(actual)
		bad[key] = "wrong"
		if err := verifyPhase3NativeViewProof(bad, spec, 10); err == nil {
			t.Fatalf("invalid %s accepted", key)
		}
	}
	actual["timeOrigin"] = float64(10)
	if err := verifyPhase3NativeViewProof(actual, spec, 10); err == nil {
		t.Fatal("stale view accepted")
	}
}

func TestPhase3NativeViewRecordsRequireExactVisibleIdentityAndData(t *testing.T) {
	spec := map[string]any{"tableGuid": "t1", "viewGuid": "v1", "viewType": "Calendar", "documentTimezone": "UTC", "browserTimezone": "Asia/Shanghai", "expectedRecords": map[string]any{"r1": map[string]any{"title": "P3-001", "startDay": "2026-11-18", "endDay": "2026-11-19", "startOA": float64(46344), "endOA": float64(46345)}}}
	actual := phase3TableCopy(spec)
	actual["timeOrigin"], actual["visibleNativeNodes"] = float64(20), float64(1)
	actual["renderedRecords"] = phase3TableCopy(spec["expectedRecords"].(map[string]any))
	if err := verifyPhase3NativeViewProof(actual, spec, 10); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"title", "startDay", "endDay", "startOA", "endOA"} {
		bad := phase3TableCopy(actual)
		bad["renderedRecords"].(map[string]any)["r1"].(map[string]any)[key] = "wrong"
		if err := verifyPhase3NativeViewProof(bad, spec, 10); err == nil {
			t.Fatalf("wrong calendar %s accepted", key)
		}
	}
	for _, records := range []map[string]any{{}, {"wrong-id": spec["expectedRecords"].(map[string]any)["r1"]}} {
		actual["renderedRecords"] = records
		if err := verifyPhase3NativeViewProof(actual, spec, 10); err == nil {
			t.Fatal("missing/incorrect record binding accepted")
		}
	}
}

func TestPhase3NativeEmptyViewRequiresIndependentModelProof(t *testing.T) {
	spec := map[string]any{"tableGuid": "t1", "viewGuid": "v1", "viewType": "Grid", "expectedRecords": map[string]any{}, "excludedWhollyEmptyRecordGuids": map[string]any{"blank": true}}
	for _, tc := range []struct {
		name     string
		ids      any
		rendered map[string]any
		valid    bool
	}{
		{"empty", []any{}, map[string]any{}, true},
		{"initial-blank-row", []any{"blank"}, map[string]any{}, true},
		{"no-model-proof", nil, map[string]any{}, false},
		{"unexpected-model-record", []any{"populated"}, map[string]any{}, false},
		{"duplicate-model-record", []any{"blank", "blank"}, map[string]any{}, false},
		{"unexpected-rendered-record", []any{}, map[string]any{"r1": map[string]any{"title": "Unexpected"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual := phase3TableCopy(spec)
			actual["timeOrigin"], actual["visibleNativeNodes"] = float64(20), float64(1)
			actual["nativeRecordGuids"], actual["renderedRecords"] = tc.ids, tc.rendered
			if err := verifyPhase3NativeViewProof(actual, spec, 10); (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
}
