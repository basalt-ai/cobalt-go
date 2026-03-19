package experiment

import "fmt"

// ThresholdMetric defines pass/fail criteria for a single evaluator's scores.
// All non-nil fields must be satisfied for the threshold to pass.
type ThresholdMetric struct {
	// Min requires the minimum score to be at least this value.
	Min *float64
	// Avg requires the average score to be at least this value.
	Avg *float64
	// P95 requires the 95th percentile score to be at least this value.
	P95 *float64
	// PassRate requires at least this fraction of items to score >= 0.5.
	PassRate *float64
}

// CIPass checks whether the report satisfies the given thresholds.
// Returns (true, nil) if all thresholds pass, or (false, failures) with a list
// of human-readable failure messages.
func (r *Report) CIPass(thresholds map[string]ThresholdMetric) (bool, []string) {
	var failures []string

	for evalName, thresh := range thresholds {
		stats, ok := r.Scores[evalName]
		if !ok {
			failures = append(failures, fmt.Sprintf("evaluator %q not found in results", evalName))
			continue
		}

		if thresh.Min != nil && stats.Min < *thresh.Min {
			failures = append(failures, fmt.Sprintf(
				"%s: min score %.3f < required %.3f", evalName, stats.Min, *thresh.Min,
			))
		}
		if thresh.Avg != nil && stats.Avg < *thresh.Avg {
			failures = append(failures, fmt.Sprintf(
				"%s: avg score %.3f < required %.3f", evalName, stats.Avg, *thresh.Avg,
			))
		}
		if thresh.P95 != nil && stats.P95 < *thresh.P95 {
			failures = append(failures, fmt.Sprintf(
				"%s: p95 score %.3f < required %.3f", evalName, stats.P95, *thresh.P95,
			))
		}
		if thresh.PassRate != nil {
			// Compute pass rate: fraction of items scoring >= 0.5.
			pass := 0
			for _, item := range r.Items {
				if score, ok := item.Scores[evalName]; ok && score >= 0.5 {
					pass++
				}
			}
			rate := 0.0
			if r.TotalItems > 0 {
				rate = float64(pass) / float64(r.TotalItems)
			}
			if rate < *thresh.PassRate {
				failures = append(failures, fmt.Sprintf(
					"%s: pass rate %.3f < required %.3f", evalName, rate, *thresh.PassRate,
				))
			}
		}
	}

	return len(failures) == 0, failures
}

// float64Ptr is a helper for creating *float64 threshold values.
func float64Ptr(v float64) *float64 { return &v }
