package experiment

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// historyDir returns the path to the cobalt results directory.
func historyDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".cobalt", "results")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// SaveReport saves a Report as JSON to ~/.cobalt/results/<timestamp>_<name>.json.
func SaveReport(report *Report) error {
	dir, err := historyDir()
	if err != nil {
		return fmt.Errorf("history: %w", err)
	}

	ts := report.Timestamp.UTC().Format("2006-01-02T15-04-05")
	// Sanitize name for filesystem.
	safeName := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, report.Name)

	filename := fmt.Sprintf("%s_%s_%s.json", ts, safeName, report.ID[:8])
	path := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("history: marshal: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// LoadReport loads a Report from a JSON file by ID prefix or filename.
func LoadReport(idOrPath string) (*Report, error) {
	// If it looks like a full path, load directly.
	if strings.HasSuffix(idOrPath, ".json") {
		return loadReportFile(idOrPath)
	}

	dir, err := historyDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if strings.Contains(e.Name(), idOrPath) {
			return loadReportFile(filepath.Join(dir, e.Name()))
		}
	}
	return nil, fmt.Errorf("history: report %q not found", idOrPath)
}

func loadReportFile(path string) (*Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("history: read %s: %w", path, err)
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("history: parse %s: %w", path, err)
	}
	return &r, nil
}

// HistoryEntry is a lightweight summary of a past run.
type HistoryEntry struct {
	ID         string
	Name       string
	Timestamp  time.Time
	TotalItems int
	Passed     int
	Failed     int
	DurationMs int64
	Filename   string
}

// ListHistory returns the most recent history entries (up to limit).
// Pass limit=0 for all entries.
func ListHistory(limit int) ([]HistoryEntry, error) {
	dir, err := historyDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Sort newest first.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() > entries[j].Name()
	})

	var result []HistoryEntry
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		report, err := loadReportFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue // skip corrupt files
		}
		result = append(result, HistoryEntry{
			ID:         report.ID,
			Name:       report.Name,
			Timestamp:  report.Timestamp,
			TotalItems: report.TotalItems,
			Passed:     report.Passed,
			Failed:     report.Failed,
			DurationMs: report.DurationMs,
			Filename:   e.Name(),
		})
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}
