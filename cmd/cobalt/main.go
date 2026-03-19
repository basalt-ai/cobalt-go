package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/basalt-ai/cobalt-go/pkg/experiment"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "cobalt",
	Short: "Cobalt — AI evaluation framework for Go",
	Long: `Cobalt is a unit-testing framework for AI agents and LLM-powered applications.

Run experiments, score outputs, track history, and enforce CI thresholds.`,
}

func main() {
	rootCmd.AddCommand(runCmd())
	rootCmd.AddCommand(initCmd())
	rootCmd.AddCommand(historyCmd())
	rootCmd.AddCommand(compareCmd())
	rootCmd.AddCommand(serveCmd())
	rootCmd.AddCommand(mcpCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// runCmd builds and runs experiment Go files.
func runCmd() *cobra.Command {
	var ciMode bool
	cmd := &cobra.Command{
		Use:   "run [path]",
		Short: "Build and run experiment files",
		Long: `Run Cobalt experiment files (*.go).

The experiment file must be a valid Go program (package main) that calls
experiment.Run() in its main() function.

Example:
  cobalt run ./experiments/quality.go`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pattern := "*.go"
			if len(args) > 0 {
				pattern = args[0]
			}

			// Resolve files matching the pattern.
			matches, err := filepath.Glob(pattern)
			if err != nil || len(matches) == 0 {
				// Try as a direct path.
				if _, err := os.Stat(pattern); err == nil {
					matches = []string{pattern}
				} else {
					return fmt.Errorf("no files matching %q", pattern)
				}
			}

			for _, file := range matches {
				fmt.Printf("Running %s...\n", file)
				goCmd := exec.Command("go", "run", file)
				goCmd.Stdout = os.Stdout
				goCmd.Stderr = os.Stderr
				goCmd.Env = os.Environ()
				if err := goCmd.Run(); err != nil {
					if ciMode {
						return fmt.Errorf("experiment failed: %w", err)
					}
					fmt.Fprintf(os.Stderr, "error: %v\n", err)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&ciMode, "ci", false, "Exit non-zero if any CI threshold fails")
	return cmd
}

// initCmd creates a starter cobalt.yaml and SKILLS.md.
func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a new Cobalt project",
		RunE: func(cmd *cobra.Command, args []string) error {
			cobaltYAML := `# cobalt.yaml — Cobalt AI evaluation configuration

judge:
  model: gpt-4o-mini
  provider: openai

concurrency: 3
timeout: 30s
testDir: ./experiments
testMatch:
  - "**/*.cobalt.go"
`
			if err := os.WriteFile("cobalt.yaml", []byte(cobaltYAML), 0644); err != nil {
				return err
			}
			fmt.Println("Created cobalt.yaml")

			skillsMD := `# Cobalt Skills

## Running Experiments

` + "```" + `bash
cobalt run ./experiments/my_experiment.go
` + "```" + `

## Viewing History

` + "```" + `bash
cobalt history --limit 10
` + "```" + `

## Comparing Runs

` + "```" + `bash
cobalt compare <id1> <id2>
` + "```" + `

## Serving Dashboard

` + "```" + `bash
cobalt serve --port 3001
` + "```" + `
`
			if err := os.WriteFile("SKILLS.md", []byte(skillsMD), 0644); err != nil {
				return err
			}
			fmt.Println("Created SKILLS.md")
			fmt.Println("\nCobalt project initialized! Set OPENAI_API_KEY and run your first experiment.")
			return nil
		},
	}
}

// historyCmd lists recent experiment runs.
func historyCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "history",
		Short: "List recent experiment runs",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := experiment.ListHistory(limit)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No experiment history found.")
				return nil
			}

			headers := []string{"ID", "Name", "Timestamp", "Items", "Passed", "Failed", "Duration"}
			rows := make([][]string, 0, len(entries))
			for _, e := range entries {
				idShort := e.ID
				if len(idShort) > 8 {
					idShort = idShort[:8]
				}
				rows = append(rows, []string{
					idShort,
					e.Name,
					e.Timestamp.Format(time.RFC3339),
					strconv.Itoa(e.TotalItems),
					strconv.Itoa(e.Passed),
					strconv.Itoa(e.Failed),
					fmt.Sprintf("%dms", e.DurationMs),
				})
			}
			printCLITable(headers, rows)
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum number of runs to show")
	return cmd
}

// compareCmd compares two experiment runs side by side.
func compareCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "compare <id1> <id2>",
		Short: "Compare two experiment runs",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			r1, err := experiment.LoadReport(args[0])
			if err != nil {
				return fmt.Errorf("load %s: %w", args[0], err)
			}
			r2, err := experiment.LoadReport(args[1])
			if err != nil {
				return fmt.Errorf("load %s: %w", args[1], err)
			}

			fmt.Printf("\nComparing experiments:\n")
			fmt.Printf("  A: %s (%s) — %s\n", r1.Name, r1.ID[:8], r1.Timestamp.Format(time.RFC3339))
			fmt.Printf("  B: %s (%s) — %s\n\n", r2.Name, r2.ID[:8], r2.Timestamp.Format(time.RFC3339))

			// Collect all evaluator names.
			evalNames := make(map[string]struct{})
			for k := range r1.Scores {
				evalNames[k] = struct{}{}
			}
			for k := range r2.Scores {
				evalNames[k] = struct{}{}
			}
			names := make([]string, 0, len(evalNames))
			for k := range evalNames {
				names = append(names, k)
			}
			sort.Strings(names)

			headers := []string{"Evaluator", "A (avg)", "B (avg)", "Delta"}
			rows := make([][]string, 0, len(names))
			for _, name := range names {
				sa := r1.Scores[name]
				sb := r2.Scores[name]
				rows = append(rows, []string{
					name,
					fmt.Sprintf("%.3f", sa.Avg),
					fmt.Sprintf("%.3f", sb.Avg),
					fmt.Sprintf("%+.3f", sb.Avg-sa.Avg),
				})
			}
			printCLITable(headers, rows)
			return nil
		},
	}
}

// serveCmd starts a local HTTP dashboard.
func serveCmd() *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start local results dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			addr := fmt.Sprintf(":%d", port)
			fmt.Printf("Cobalt dashboard running at http://localhost%s\n", addr)
			fmt.Println("Press Ctrl+C to stop.")

			http.HandleFunc("/", handleDashboard)
			http.HandleFunc("/api/results", handleAPIResults)
			http.HandleFunc("/api/results/", handleAPIResult)

			return http.ListenAndServe(addr, nil)
		},
	}
	cmd.Flags().IntVar(&port, "port", 3001, "Port to listen on")
	return cmd
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	entries, _ := experiment.ListHistory(50)
	funcs := template.FuncMap{
		"shortID": func(s string) string {
			if len(s) > 8 {
				return s[:8]
			}
			return s
		},
	}
	tmpl := template.Must(template.New("dash").Funcs(funcs).Parse(dashboardHTML))
	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, entries)
}

func handleAPIResults(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if n, err := strconv.Atoi(limitStr); err == nil {
		limit = n
	}
	entries, _ := experiment.ListHistory(limit)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

func handleAPIResult(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/results/")
	report, err := experiment.LoadReport(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

// mcpCmd launches the MCP server.
func mcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP server over stdio",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Import and run the MCP server.
			runMCPServer()
			return nil
		},
	}
}

// printCLITable prints a simple ASCII table to stdout.
func printCLITable(headers []string, rows [][]string) {
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
	sep := buildSep(widths)
	fmt.Println(sep)
	fmt.Println(buildRow(headers, widths))
	fmt.Println(sep)
	for _, row := range rows {
		fmt.Println(buildRow(row, widths))
	}
	fmt.Println(sep)
}

func buildSep(widths []int) string {
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

// dashboardHTML is the minimal dashboard template.
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Cobalt Dashboard</title>
<style>
  body { font-family: system-ui, sans-serif; margin: 2rem; background: #0f0f0f; color: #e5e5e5; }
  h1 { color: #a78bfa; }
  table { width: 100%; border-collapse: collapse; margin-top: 1rem; }
  th { background: #1a1a2e; color: #a78bfa; padding: 0.75rem 1rem; text-align: left; }
  td { padding: 0.6rem 1rem; border-bottom: 1px solid #2a2a3a; }
  tr:hover td { background: #1a1a2e; }
  .pass { color: #4ade80; }
  .fail { color: #f87171; }
  a { color: #a78bfa; text-decoration: none; }
  .empty { color: #6b7280; text-align: center; padding: 2rem; }
</style>
</head>
<body>
<h1>Cobalt Dashboard</h1>
<p>Recent experiment runs. Use <code>cobalt history</code> for CLI view.</p>
{{if .}}
<table>
  <thead>
    <tr><th>ID</th><th>Name</th><th>Timestamp</th><th>Items</th><th>Passed</th><th>Failed</th><th>Duration</th></tr>
  </thead>
  <tbody>
  {{range .}}
    <tr>
      <td><a href="/api/results/{{.ID}}">{{shortID .ID}}</a></td>
      <td>{{.Name}}</td>
      <td>{{.Timestamp.Format "2006-01-02 15:04:05"}}</td>
      <td>{{.TotalItems}}</td>
      <td class="pass">{{.Passed}}</td>
      <td class="fail">{{.Failed}}</td>
      <td>{{.DurationMs}}ms</td>
    </tr>
  {{end}}
  </tbody>
</table>
{{else}}
<p class="empty">No experiment runs found. Run <code>cobalt run ./your_experiment.go</code> first.</p>
{{end}}
</body>
</html>`
