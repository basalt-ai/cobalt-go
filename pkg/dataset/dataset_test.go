package dataset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	items := []Item{{"a": 1}, {"b": 2}}
	d := New(items)
	if d.Len() != 2 {
		t.Fatalf("expected 2 items, got %d", d.Len())
	}
}

func TestItems(t *testing.T) {
	items := []Item{{"x": "hello"}}
	d := New(items)
	got := d.Items()
	if len(got) != 1 || got[0]["x"] != "hello" {
		t.Fatal("items mismatch")
	}
}

func TestFilter(t *testing.T) {
	items := []Item{{"n": 1}, {"n": 2}, {"n": 3}}
	d := New(items)
	even := d.Filter(func(i Item) bool {
		v, _ := i["n"].(int)
		return v%2 == 0
	})
	if even.Len() != 1 {
		t.Fatalf("expected 1 item, got %d", even.Len())
	}
}

func TestMap(t *testing.T) {
	items := []Item{{"val": "hello"}}
	d := New(items)
	upper := d.Map(func(i Item) Item {
		return Item{"val": "HELLO"}
	})
	got := upper.Items()
	if got[0]["val"] != "HELLO" {
		t.Fatal("map transform failed")
	}
}

func TestSample(t *testing.T) {
	items := []Item{{"a": 1}, {"b": 2}, {"c": 3}, {"d": 4}, {"e": 5}}
	d := New(items)
	s := d.Sample(3)
	if s.Len() != 3 {
		t.Fatalf("expected 3 items, got %d", s.Len())
	}
	// Sample more than available
	s2 := d.Sample(100)
	if s2.Len() != 5 {
		t.Fatalf("expected 5 items, got %d", s2.Len())
	}
}

func TestSlice(t *testing.T) {
	items := []Item{{"a": 1}, {"b": 2}, {"c": 3}, {"d": 4}}
	d := New(items)
	s := d.Slice(1, 3)
	if s.Len() != 2 {
		t.Fatalf("expected 2 items, got %d", s.Len())
	}
	// Out of bounds
	s2 := d.Slice(3, 100)
	if s2.Len() != 1 {
		t.Fatalf("expected 1 item, got %d", s2.Len())
	}
	// Empty
	s3 := d.Slice(2, 2)
	if s3.Len() != 0 {
		t.Fatalf("expected 0 items, got %d", s3.Len())
	}
}

func TestFromJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	data := []map[string]any{{"input": "hello", "expected": "world"}}
	b, _ := json.Marshal(data)
	os.WriteFile(path, b, 0600)

	d, err := FromJSON(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Len() != 1 {
		t.Fatalf("expected 1 item, got %d", d.Len())
	}
	if d.Items()[0]["input"] != "hello" {
		t.Fatal("input mismatch")
	}
}

func TestFromJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	content := `{"input":"a"}` + "\n" + `{"input":"b"}` + "\n"
	os.WriteFile(path, []byte(content), 0600)

	d, err := FromJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Len() != 2 {
		t.Fatalf("expected 2 items, got %d", d.Len())
	}
}

func TestFromCSV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")
	content := "input,expected\nhello,world\nfoo,bar\n"
	os.WriteFile(path, []byte(content), 0600)

	d, err := FromCSV(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.Len() != 2 {
		t.Fatalf("expected 2 items, got %d", d.Len())
	}
	items := d.Items()
	if items[0]["input"] != "hello" || items[0]["expected"] != "world" {
		t.Fatalf("CSV parse mismatch: %+v", items[0])
	}
}
