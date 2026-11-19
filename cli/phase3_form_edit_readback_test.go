package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestPhase3FormSubmissionReadbackWaitsOnlyForIndex(t *testing.T) {
	calls := 0
	row := map[string]any{"submitId": "new", "content": "[]"}
	read := func(context.Context) ([]map[string]any, int, error) {
		calls++
		if calls < 3 {
			return nil, 0, nil
		}
		return []map[string]any{row}, 1, nil
	}
	rows, n, totals, err := phase3WaitFormSubmission(context.Background(), read, nil, 0, "new", 0)
	if err != nil || n != 1 || len(rows) != 1 || !reflect.DeepEqual(totals, []int{0, 0, 1}) || calls != 3 {
		t.Fatal(rows, n, totals, calls, err)
	}
}
func TestPhase3FormSubmissionReadbackRejectsUnsafeChanges(t *testing.T) {
	old := map[string]any{"submitId": "old", "content": "unchanged"}
	for _, scenario := range []string{"never-visible", "old-changed", "old-missing", "extra-response", "unexpected-id", "read-error"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			read := func(context.Context) ([]map[string]any, int, error) {
				calls++
				switch scenario {
				case "never-visible":
					return []map[string]any{old}, 1, nil
				case "old-changed":
					return []map[string]any{{"submitId": "old", "content": "changed"}}, 1, nil
				case "old-missing":
					return nil, 0, nil
				case "extra-response":
					return []map[string]any{old, {"submitId": "new"}, {"submitId": "other"}}, 3, nil
				case "unexpected-id":
					return []map[string]any{old, {"submitId": "other"}}, 2, nil
				default:
					return nil, 0, errors.New("read failed")
				}
			}
			_, _, _, err := phase3WaitFormSubmission(context.Background(), read, []map[string]any{old}, 1, "new", 0)
			if err == nil {
				t.Fatal("unsafe result accepted")
			}
			if scenario == "never-visible" {
				if calls != 15 {
					t.Fatal(calls)
				}
			} else if calls != 1 {
				t.Fatal("unsafe change retried", calls)
			}
		})
	}
}

func TestPhase3FormSubmissionReadbackRetriesOnlyCountLag(t *testing.T) {
	calls := 0
	read := func(context.Context) ([]map[string]any, int, error) {
		calls++
		if calls == 1 {
			return nil, 0, errPhase3FormResultTotalLag
		}
		return []map[string]any{{"submitId": "new"}}, 1, nil
	}
	rows, n, totals, err := phase3WaitFormSubmission(context.Background(), read, nil, 0, "new", 0)
	if err != nil || n != 1 || len(rows) != 1 || calls != 2 || !reflect.DeepEqual(totals, []int{-1, 1}) {
		t.Fatal(rows, n, totals, calls, err)
	}
}

func TestPhase3FormSubmissionReadbackStopsBeforeReadWhenExpired(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	read := func(context.Context) ([]map[string]any, int, error) {
		calls++
		return []map[string]any{{"submitId": "new"}}, 1, nil
	}
	_, _, _, err := phase3WaitFormSubmission(ctx, read, nil, 0, "new", 0)
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("expired budget reached network", err, calls)
	}
}
func TestPhase3FormSubmissionReadbackCancelsPause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	read := func(context.Context) ([]map[string]any, int, error) { calls++; cancel(); return nil, 0, nil }
	start := time.Now()
	_, _, _, err := phase3WaitFormSubmission(ctx, read, nil, 0, "new", time.Millisecond)
	if !errors.Is(err, context.Canceled) || calls != 1 || time.Since(start) > 100*time.Millisecond {
		t.Fatal(err, calls, time.Since(start))
	}
}
func TestPhase3FormSubmissionReadbackInspectsLagRows(t *testing.T) {
	old := map[string]any{"submitId": "old", "content": "unchanged"}
	for _, mode := range []string{"old-changed", "unknown-new", "extra-new", "partial-old-missing"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			read := func(context.Context) ([]map[string]any, int, error) {
				calls++
				if calls > 1 {
					return []map[string]any{old, {"submitId": "new"}}, 2, nil
				}
				switch mode {
				case "old-changed":
					return []map[string]any{{"submitId": "old", "content": "changed"}}, 0, errPhase3FormResultTotalLag
				case "unknown-new":
					return []map[string]any{{"submitId": "other"}}, 0, errPhase3FormResultTotalLag
				case "extra-new":
					return []map[string]any{{"submitId": "new"}, {"submitId": "other"}}, 0, errPhase3FormResultTotalLag
				default:
					return []map[string]any{{"submitId": "new"}}, 0, errPhase3FormResultTotalLag
				}
			}
			_, _, _, err := phase3WaitFormSubmission(context.Background(), read, []map[string]any{old}, 1, "new", 0)
			if mode == "partial-old-missing" {
				if err != nil || calls != 2 {
					t.Fatal(err, calls)
				}
			} else if err == nil || calls != 1 {
				t.Fatal("unsafe partial rows retried", err, calls)
			}
		})
	}
}

func TestPhase3FormSubmissionReadbackCapsSharedBudgetAndRejectsLateSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	read := func(budget context.Context) ([]map[string]any, int, error) {
		deadline, ok := budget.Deadline()
		if !ok || time.Until(deadline) > phase3FormReadbackBudget {
			t.Fatal("no shared budget", deadline)
		}
		cancel()
		return []map[string]any{{"submitId": "new"}}, 1, nil
	}
	_, _, _, err := phase3WaitFormSubmission(ctx, read, nil, 0, "new", 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("late response accepted", err)
	}
}
func TestPhase3FormSubmissionReadbackTracksNewIDAcrossPartialReads(t *testing.T) {
	calls := 0
	read := func(context.Context) ([]map[string]any, int, error) {
		calls++
		id := "new-one"
		if calls > 1 {
			id = "new-two"
		}
		return []map[string]any{{"submitId": id}}, 0, errPhase3FormResultTotalLag
	}
	_, _, _, err := phase3WaitFormSubmission(context.Background(), read, nil, 0, "", 0)
	if err == nil || calls != 2 {
		t.Fatal("UI new ID changed during lag", err, calls)
	}
}
