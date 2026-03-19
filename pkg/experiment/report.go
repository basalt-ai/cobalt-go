package experiment

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

// ScoreStats holds aggregate statistics for a single evaluator's scores.
type ScoreStats struct {
	Avg float64
	Min float64
	Max float64
	P50 float64
	P95 float64
	P99 float64
}

// ItemResult holds the result of running a single dataset item.
type ItemResult struct {
	Index      int
	Input      string
	Output     string
	Scores     map[string]float64
	Reasons    map[string]string
	DurationMs int64
	Error      string
}

// Report is the full output of an experiment run.
type Report struct {
	ID         string
	Name       string
	Timestamp  time.Time
	Tags       []string
	TotalItems int
	Passed     int
	Failed     int
	DurationMs int64
	Scores     map[string]ScoreStats
	Items      []ItemResult
}

// calcStats computes aggregate statistics over a slice of scores.
func calcStats(scores []float64) ScoreStats {
	if len(scores) == 0 {
		return ScoreStats{}
	}
	sorted := make([]float64, len(scores))
	copy(sorted, scores)
	sort.Float64s(sorted)

	sum := 0.0
	for _, s := range sorted {
		sum += s
	}

	return ScoreStats{
		Avg: sum / float64(len(sorted)),
		Min: sorted[0],
		Max: sorted[len(sorted)-1],
		P50: percentile(sorted, 50),
		P95: percentile(sorted, 95),
		P99: percentile(sorted, 99),
	}
}

// percentile returns the pth percentile of a sorted slice using linear interpolation.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := (p / 100) * float64(len(sorted)-1)
	lower := int(math.Floor(rank))
	upper := int(math.Ceil(rank))
	if lower == upper {
		return sorted[lower]
	}
	frac := rank - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

// Print outputs a formatted report to stdout.
func (r *Report) Print() {
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "Experiment: %s\n", r.Name)
	fmt.Fprintf(os.Stdout, "Run ID:     %s\n", r.ID)
	fmt.Fprintf(os.Stdout, "Timestamp:  %s\n", r.Timestamp.Format(time.RFC3339))
	if len(r.Tags) > 0 {
		fmt.Fprintf(os.Stdout, "Tags:       %s\n", strings.Join(r.Tags, ", "))
	}
	fmt.Fprintf(os.Stdout, "Duration:   %dms\n", r.DurationMs)
	fmt.Fprintf(os.Stdout, "Items:      %d total, %d passed, %d failed\n\n",
		r.TotalItems, r.Passed, r.Failed)

	// Score summary table.
	if len(r.Scores) > 0 {
		names := sortedKeys(r.Scores)
		headers := []string{"Evaluator", "Avg", "Min", "Max", "P50", "P95", "P99"}
		rows := make([][]string, 0, len(names))
		for _, name := range names {
			st := r.Scores[name]
			rows = append(rows, []string{
				name,
				fmt.Sprintf("%.3f", st.Avg),
				fmt.Sprintf("%.3f", st.Min),
				fmt.Sprintf("%.3f", st.Max),
				fmt.Sprintf("%.3f", st.P50),
				fmt.Sprintf("%.3f", st.P95),
				fmt.Sprintf("%.3f", st.P99),
			})
		}
		fmt.Fprintln(os.Stdout, "Score Summary:")
		printTable(os.Stdout, headers, rows)
		fmt.Fprintln(os.Stdout)
	}

	// Per-item results table.
	if len(r.Items) > 0 {
		var evalNames []string
		for k := range r.Items[0].Scores {
			evalNames = append(evalNames, k)
		}
		sort.Strings(evalNames)

		headers := append([]string{"#", "Input", "Output", "Duration"}, evalNames...)
		headers = append(headers, "Error")

		rows := make([][]string, 0, len(r.Items))
		for _, item := range r.Items {
			row := []string{
				fmt.Sprintf("%d", item.Index+1),
				truncate(item.Input, 40),
				truncate(item.Output, 40),
				fmt.Sprintf("%dms", item.DurationMs),
			}
			for _, name := range evalNames {
				row = append(row, fmt.Sprintf("%.2f", item.Scores[name]))
			}
			row = append(row, truncate(item.Error, 30))
			rows = append(rows, row)
		}
		fmt.Fprintln(os.Stdout, "Item Results:")
		printTable(os.Stdout, headers, rows)
	}
}

// printTable renders a simple ASCII table to w.
func printTable(w *os.File, headers []string, rows [][]string) {
	if len(headers) == 0 {
		return
	}
	// Compute column widths.
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	sep := buildSeparator(widths)
	fmt.Fprintln(w, sep)
	fmt.Fprintln(w, buildRow(headers, widths))
	fmt.Fprintln(w, sep)
	for _, row := range rows {
		fmt.Fprintln(w, buildRow(row, widths))
	}
	fmt.Fprintln(w, sep)
}

func buildSeparator(widths []int) string {
	parts := make([]string, len(widths))
	for i, w := range widths {
		parts[i] = strings.Repeat("-", w+2)
	}
	return "+" + strings.Join(parts, "+") + "+"
}

func buildRow(cells []string, widths []int) string {
	parts := make([]string, len(widths))
	for i, w := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		parts[i] = " " + cell + strings.Repeat(" ", w-len(cell)) + " "
	}
	return "|" + strings.Join(parts, "|") + "|"
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
