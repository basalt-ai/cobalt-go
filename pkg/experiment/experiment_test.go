package experiment

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/basalt-ai/cobalt-go/pkg/dataset"
	"github.com/basalt-ai/cobalt-go/pkg/evaluator"
)

func makeDataset(n int) *dataset.Dataset {
	items := make([]dataset.Item, n)
	for i := range items {
		items[i] = dataset.Item{"input": "hello", "expectedOutput": "world"}
	}
	return dataset.New(items)
}

func echoRunner(ctx context.Context, item map[string]any, index int) (string, error) {
	return "world", nil
}

func TestRun_Basic(t *testing.T) {
	ds := makeDataset(3)
	eval := evaluator.NewFunction("exact", func(ec evaluator.EvalContext) (evaluator.EvalResult, error) {
		if ec.Output == "world" {
			return evaluator.EvalResult{Score: 1}, nil
		}
		return evaluator.EvalResult{Score: 0}, nil
	})

	report, err := Run(context.Background(), "test-run", ds, echoRunner, Options{
		Evaluators: []evaluator.Evaluator{eval},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalItems != 3 {
		t.Fatalf("expected 3 items, got %d", report.TotalItems)
	}
	if report.Passed != 3 {
		t.Fatalf("expected 3 passed, got %d", report.Passed)
	}
	stats, ok := report.Scores["exact"]
	if !ok {
		t.Fatal("missing 'exact' evaluator scores")
	}
	if stats.Avg != 1.0 {
		t.Fatalf("expected avg 1.0, got %f", stats.Avg)
	}
}

func TestRun_Concurrency(t *testing.T) {
	ds := makeDataset(10)
	report, err := Run(context.Background(), "concurrency-test", ds, echoRunner, Options{
		Concurrency: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalItems != 10 {
		t.Fatalf("expected 10 items, got %d", report.TotalItems)
	}
}

func TestRun_Timeout(t *testing.T) {
	ds := makeDataset(1)
	slowRunner := func(ctx context.Context, item map[string]any, index int) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
			return "done", nil
		}
	}

	report, err := Run(context.Background(), "timeout-test", ds, slowRunner, Options{
		Timeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Failed != 1 {
		t.Fatalf("expected 1 failed item, got %d", report.Failed)
	}
}

func TestRun_MultipleRuns(t *testing.T) {
	ds := makeDataset(2)
	var callCount int64
	runner := func(ctx context.Context, item map[string]any, index int) (string, error) {
		atomic.AddInt64(&callCount, 1)
		return "world", nil
	}

	eval := evaluator.NewFunction("exact", func(ec evaluator.EvalContext) (evaluator.EvalResult, error) {
		return evaluator.EvalResult{Score: 1}, nil
	})

	_, err := Run(context.Background(), "multi-run", ds, runner, Options{
		Evaluators: []evaluator.Evaluator{eval},
		Runs:       3,
	})
	if err != nil {
		t.Fatal(err)
	}
	// 2 items × 3 runs = 6 calls
	if atomic.LoadInt64(&callCount) != 6 {
		t.Fatalf("expected 6 runner calls, got %d", atomic.LoadInt64(&callCount))
	}
}

// --- ScoreStats tests ---

func TestCalcStats_Empty(t *testing.T) {
	st := calcStats(nil)
	if st.Avg != 0 || st.Min != 0 || st.Max != 0 {
		t.Fatal("empty stats should be all zeros")
	}
}

func TestCalcStats_Single(t *testing.T) {
	st := calcStats([]float64{0.7})
	if st.Avg != 0.7 || st.Min != 0.7 || st.Max != 0.7 {
		t.Fatalf("unexpected stats for single value: %+v", st)
	}
}

func TestCalcStats_Multiple(t *testing.T) {
	scores := []float64{0.2, 0.4, 0.6, 0.8, 1.0}
	st := calcStats(scores)
	if st.Avg != 0.6 {
		t.Fatalf("expected avg 0.6, got %f", st.Avg)
	}
	if st.Min != 0.2 {
		t.Fatalf("expected min 0.2, got %f", st.Min)
	}
	if st.Max != 1.0 {
		t.Fatalf("expected max 1.0, got %f", st.Max)
	}
}

func TestPercentile(t *testing.T) {
	sorted := []float64{0.0, 0.25, 0.5, 0.75, 1.0}
	p50 := percentile(sorted, 50)
	if p50 != 0.5 {
		t.Fatalf("expected p50=0.5, got %f", p50)
	}
}

// --- CIPass tests ---

func TestCIPass_AllPass(t *testing.T) {
	report := &Report{
		TotalItems: 3,
		Items: []ItemResult{
			{Scores: map[string]float64{"acc": 1.0}},
			{Scores: map[string]float64{"acc": 0.8}},
			{Scores: map[string]float64{"acc": 0.9}},
		},
		Scores: map[string]ScoreStats{
			"acc": {Avg: 0.9, Min: 0.8, Max: 1.0, P95: 1.0},
		},
	}

	avg := 0.8
	ok, failures := report.CIPass(map[string]ThresholdMetric{
		"acc": {Avg: &avg},
	})
	if !ok {
		t.Fatalf("expected CI pass, got failures: %v", failures)
	}
}

func TestCIPass_AvgFail(t *testing.T) {
	report := &Report{
		TotalItems: 2,
		Items: []ItemResult{
			{Scores: map[string]float64{"acc": 0.3}},
			{Scores: map[string]float64{"acc": 0.4}},
		},
		Scores: map[string]ScoreStats{
			"acc": {Avg: 0.35},
		},
	}

	avg := 0.8
	ok, failures := report.CIPass(map[string]ThresholdMetric{
		"acc": {Avg: &avg},
	})
	if ok {
		t.Fatal("expected CI fail")
	}
	if len(failures) == 0 {
		t.Fatal("expected failure messages")
	}
}

func TestCIPass_MissingEvaluator(t *testing.T) {
	report := &Report{
		Scores: map[string]ScoreStats{},
	}
	avg := 0.8
	ok, failures := report.CIPass(map[string]ThresholdMetric{
		"missing": {Avg: &avg},
	})
	if ok {
		t.Fatal("expected CI fail for missing evaluator")
	}
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d: %v", len(failures), failures)
	}
}

// --- History tests ---

func TestSaveLoadReport(t *testing.T) {
	report := &Report{
		ID:         "abcd1234",
		Name:       "test-history",
		Timestamp:  time.Now(),
		TotalItems: 5,
		Passed:     4,
		Failed:     1,
		DurationMs: 1234,
		Scores:     map[string]ScoreStats{"acc": {Avg: 0.9}},
		Items:      []ItemResult{{Index: 0, Input: "hi", Output: "hello", Scores: map[string]float64{"acc": 0.9}}},
	}

	if err := SaveReport(report); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadReport("abcd1234")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "test-history" {
		t.Fatalf("expected name 'test-history', got %q", loaded.Name)
	}
	if loaded.TotalItems != 5 {
		t.Fatalf("expected 5 items, got %d", loaded.TotalItems)
	}
}

func TestListHistory(t *testing.T) {
	// Save a report to ensure there's at least one entry.
	report := &Report{
		ID:        "list-test-id",
		Name:      "list-test",
		Timestamp: time.Now(),
		Scores:    map[string]ScoreStats{},
	}
	if err := SaveReport(report); err != nil {
		t.Fatal(err)
	}

	entries, err := ListHistory(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one history entry")
	}
}
