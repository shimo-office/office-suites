package main

import (
	"context"
	"errors"
	"reflect"
	"time"
)

const phase3FormReadbackBudget = 120 * time.Second

// Only index propagation is retried. All pages and pauses share this budget;
// visible contradictory data fails even when the index snapshot is incomplete.
func phase3WaitFormSubmission(ctx context.Context, read func(context.Context) ([]map[string]any, int, error), before []map[string]any, total int, id string, interval time.Duration) ([]map[string]any, int, []int, error) {
	ctx, cancel := context.WithTimeout(ctx, phase3FormReadbackBudget)
	defer cancel()
	prior := map[string]map[string]any{}
	for _, r := range before {
		prior[str(r, "submitId")] = r
	}
	totals := []int{}
	observedNewID := id
	for attempt := 0; attempt < 15; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, 0, totals, err
		}
		rows, n, err := read(ctx)
		if e := ctx.Err(); e != nil {
			return nil, 0, totals, e
		}
		partial := errors.Is(err, errPhase3FormResultTotalLag)
		if err != nil && !partial {
			return nil, 0, totals, err
		}
		if partial {
			totals = append(totals, -1)
		} else {
			totals = append(totals, n)
		}
		if n > total+1 || (!partial && n < total) {
			return rows, n, totals, errors.New("答卷总数发生非预期变化")
		}
		added, nextID, e := phase3CheckSubmissionRows(rows, prior, observedNewID, partial)
		if e != nil {
			return rows, n, totals, e
		}
		observedNewID = nextID
		if !partial && n == total+1 && added == 1 {
			return rows, n, totals, nil
		}
		if attempt < 14 {
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, 0, totals, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return nil, total, totals, errors.New("有界只读等待结束，新答卷仍未完整可见；禁止重放提交")
}

func phase3CheckSubmissionRows(rows []map[string]any, prior map[string]map[string]any, id string, partial bool) (int, string, error) {
	found := map[string]bool{}
	added := 0
	for _, r := range rows {
		key := str(r, "submitId")
		if key == "" || found[key] {
			return 0, id, errors.New("读回答卷缺少或重复ID")
		}
		found[key] = true
		if old, exists := prior[key]; exists {
			if !reflect.DeepEqual(old, r) {
				return 0, id, errors.New("核验期间已有答卷发生变化")
			}
		} else {
			added++
			if added > 1 || (id != "" && id != key) {
				return 0, id, errors.New("出现非预期的新答卷，停止核验")
			}
			id = key
		}
	}
	if !partial {
		for key := range prior {
			if !found[key] {
				return 0, id, errors.New("核验期间已有答卷缺失")
			}
		}
	}
	return added, id, nil
}
