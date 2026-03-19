package dataset

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
)

// Item represents a single test data item (arbitrary key-value map).
type Item map[string]any

// Dataset is an immutable, chainable container for test data items.
type Dataset struct {
	items []Item
}

// New creates a Dataset from a slice of Items.
func New(items []Item) *Dataset {
	cp := make([]Item, len(items))
	copy(cp, items)
	return &Dataset{items: cp}
}

// FromJSON loads a Dataset from a JSON file.
// The file must contain a JSON array of objects.
func FromJSON(path string) (*Dataset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cobalt/dataset: read %s: %w", path, err)
	}
	var items []Item
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("cobalt/dataset: parse %s: %w", path, err)
	}
	return New(items), nil
}

// FromJSONL loads a Dataset from a JSON Lines file (one JSON object per line).
func FromJSONL(path string) (*Dataset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cobalt/dataset: read %s: %w", path, err)
	}
	var items []Item
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var item Item
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("cobalt/dataset: parse %s line %d: %w", path, i+1, err)
		}
		items = append(items, item)
	}
	return New(items), nil
}

// FromCSV loads a Dataset from a CSV file.
// The first row is treated as headers; each subsequent row becomes an Item.
func FromCSV(path string) (*Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cobalt/dataset: open %s: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("cobalt/dataset: parse %s: %w", path, err)
	}
	if len(records) == 0 {
		return New(nil), nil
	}
	headers := records[0]
	items := make([]Item, 0, len(records)-1)
	for _, row := range records[1:] {
		item := make(Item, len(headers))
		for i, h := range headers {
			if i < len(row) {
				item[h] = row[i]
			}
		}
		items = append(items, item)
	}
	return New(items), nil
}

// Items returns a copy of the dataset's items.
func (d *Dataset) Items() []Item {
	cp := make([]Item, len(d.items))
	copy(cp, d.items)
	return cp
}

// Len returns the number of items in the dataset.
func (d *Dataset) Len() int {
	return len(d.items)
}

// Filter returns a new Dataset containing only items for which fn returns true.
func (d *Dataset) Filter(fn func(Item) bool) *Dataset {
	var filtered []Item
	for _, item := range d.items {
		if fn(item) {
			filtered = append(filtered, item)
		}
	}
	return New(filtered)
}

// Map returns a new Dataset with each item transformed by fn.
func (d *Dataset) Map(fn func(Item) Item) *Dataset {
	mapped := make([]Item, len(d.items))
	for i, item := range d.items {
		mapped[i] = fn(item)
	}
	return New(mapped)
}

// Sample returns a new Dataset with at most n randomly selected items.
// If n >= d.Len(), a shuffled copy of all items is returned.
func (d *Dataset) Sample(n int) *Dataset {
	if n <= 0 {
		return New(nil)
	}
	cp := make([]Item, len(d.items))
	copy(cp, d.items)
	rand.Shuffle(len(cp), func(i, j int) { cp[i], cp[j] = cp[j], cp[i] })
	if n > len(cp) {
		n = len(cp)
	}
	return New(cp[:n])
}

// Slice returns a new Dataset with items from index start (inclusive) to end (exclusive).
func (d *Dataset) Slice(start, end int) *Dataset {
	if start < 0 {
		start = 0
	}
	if end > len(d.items) {
		end = len(d.items)
	}
	if start >= end {
		return New(nil)
	}
	return New(d.items[start:end])
}
