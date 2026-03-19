// Package experiment provides the core experiment runner for Cobalt.
package experiment

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/basalt-ai/cobalt-go/pkg/dataset"
	"github.com/basalt-ai/cobalt-go/pkg/evaluator"
)

// RunnerFunc is the user-provided function that processes a single dataset item.
// It receives the item map and its index and must return the string output.
type RunnerFunc func(ctx context.Context, item map[string]any, index int) (string, error)

// Options configures an experiment run.
type Options struct {
	// Evaluators is the list of evaluators to score each item.
	Evaluators []evaluator.Evaluator
	// Concurrency is the number of items to process in parallel (default: 3).
	Concurrency int
	// Timeout is the per-item deadline (default: 30s).
	Timeout time.Duration
	// Thresholds maps evaluator names to CI pass/fail criteria.
	Thresholds map[string]ThresholdMetric
	// Tags are arbitrary labels attached to the report.
	Tags []string
	// Runs is the number of times to run each item (default: 1).
	// When > 1, scores are averaged across runs.
	Runs int
}

// Run executes an experiment over all items in ds using runner, then scores
// each output with the configured evaluators and returns a Report.
func Run(ctx context.Context, name string, ds *dataset.Dataset, runner RunnerFunc, opts Options) (*Report, error) {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 3
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.Runs <= 0 {
		opts.Runs = 1
	}

	runID, err := newID()
	if err != nil {
		return nil, err
	}

	items := ds.Items()
	total := len(items)
	results := make([]ItemResult, total)
	start := time.Now()

	// Semaphore for concurrency control.
	sem := make(chan struct{}, opts.Concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, item := range items {
		i, item := i, item
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			result := runItem(ctx, i, item, runner, opts)
			mu.Lock()
			results[i] = result
			mu.Unlock()
		}()
	}
	wg.Wait()

	durationMs := time.Since(start).Milliseconds()

	// Build per-evaluator score slices for stats.
	scoresByEval := make(map[string][]float64)
	for _, evalr := range opts.Evaluators {
		scoresByEval[evalr.Name()] = make([]float64, 0, total)
	}
	passed, failed := 0, 0
	for _, res := range results {
		if res.Error != "" {
			failed++
		} else {
			passed++
		}
		for evalName, score := range res.Scores {
			scoresByEval[evalName] = append(scoresByEval[evalName], score)
		}
	}

	scoreStats := make(map[string]ScoreStats, len(scoresByEval))
	for evalName, scores := range scoresByEval {
		scoreStats[evalName] = calcStats(scores)
	}

	report := &Report{
		ID:         runID,
		Name:       name,
		Timestamp:  time.Now(),
		Tags:       opts.Tags,
		TotalItems: total,
		Passed:     passed,
		Failed:     failed,
		DurationMs: durationMs,
		Scores:     scoreStats,
		Items:      results,
	}

	return report, nil
}

// runItem executes a single dataset item across all configured runs and evaluators.
func runItem(ctx context.Context, index int, item map[string]any, runner RunnerFunc, opts Options) ItemResult {
	result := ItemResult{
		Index:   index,
		Input:   fmt.Sprintf("%v", item),
		Scores:  make(map[string]float64),
		Reasons: make(map[string]string),
	}

	// Get string representation of input for display.
	if v, ok := item["input"]; ok {
		result.Input = fmt.Sprintf("%v", v)
	}

	start := time.Now()

	// Run the agent with timeout.
	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	var allScores map[string][]float64
	if opts.Runs > 1 {
		allScores = make(map[string][]float64)
	}

	var lastOutput string
	var runErr error

	for run := 0; run < opts.Runs; run++ {
		output, err := runner(runCtx, item, index)
		if err != nil {
			runErr = err
			break
		}
		lastOutput = output

		if opts.Runs > 1 {
			// Evaluate this run and accumulate scores.
			for _, evalr := range opts.Evaluators {
				ec := evaluator.EvalContext{
					Item:   item,
					Output: output,
				}
				res, err := evalr.Evaluate(runCtx, ec)
				if err != nil {
					// Record error score of 0.
					allScores[evalr.Name()] = append(allScores[evalr.Name()], 0)
					continue
				}
				allScores[evalr.Name()] = append(allScores[evalr.Name()], res.Score)
			}
		}
	}

	result.DurationMs = time.Since(start).Milliseconds()

	if runErr != nil {
		result.Error = runErr.Error()
		return result
	}

	result.Output = lastOutput

	if opts.Runs > 1 {
		// Average scores across runs.
		for name, scores := range allScores {
			sum := 0.0
			for _, s := range scores {
				sum += s
			}
			result.Scores[name] = sum / float64(len(scores))
		}
		return result
	}

	// Single run: evaluate once.
	for _, evalr := range opts.Evaluators {
		ec := evaluator.EvalContext{
			Item:   item,
			Output: lastOutput,
		}
		res, err := evalr.Evaluate(runCtx, ec)
		if err != nil {
			result.Scores[evalr.Name()] = 0
			result.Reasons[evalr.Name()] = fmt.Sprintf("error: %v", err)
			continue
		}
		result.Scores[evalr.Name()] = res.Score
		if res.Reason != "" {
			result.Reasons[evalr.Name()] = res.Reason
		}
	}

	return result
}

// newID generates a random 16-char hex ID.
func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
