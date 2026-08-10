package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQueryHistoryLoadAndRemember(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "query-history.jsonl")

	history, err := loadQueryHistory(path)
	if err != nil {
		t.Fatalf("loadQueryHistory() error = %v", err)
	}

	if err := history.remember("FROM logs-*"); err != nil {
		t.Fatalf("remember() error = %v", err)
	}
	if err := history.remember("FROM traces-*"); err != nil {
		t.Fatalf("remember() error = %v", err)
	}

	reloaded, err := loadQueryHistory(path)
	if err != nil {
		t.Fatalf("reload error = %v", err)
	}
	if len(reloaded.entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(reloaded.entries))
	}
	if got := reloaded.entries[1].Query; got != "FROM traces-*" {
		t.Fatalf("last query = %q, want %q", got, "FROM traces-*")
	}
}

func TestQueryHistorySkipsInvalidLines(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "query-history.jsonl")
	content := []byte("{bad json}\n{\"query\":\"FROM logs-*\"}\n{\"query\":\"\"}\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	history, err := loadQueryHistory(path)
	if err != nil {
		t.Fatalf("loadQueryHistory() error = %v", err)
	}
	if len(history.entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(history.entries))
	}
	if got := history.entries[0].Query; got != "FROM logs-*" {
		t.Fatalf("query = %q, want %q", got, "FROM logs-*")
	}
}

func TestQueryHistoryNavigationWraps(t *testing.T) {
	t.Parallel()

	history := &queryHistoryManager{
		entries: []queryHistoryEntry{
			{Query: "one"},
			{Query: "two"},
			{Query: "three"},
		},
		browseIndex: -1,
	}

	if got := history.previous("draft"); got != "three" {
		t.Fatalf("previous() = %q, want %q", got, "three")
	}
	if got := history.previous("draft"); got != "two" {
		t.Fatalf("previous() second = %q, want %q", got, "two")
	}
	if got := history.previous("draft"); got != "one" {
		t.Fatalf("previous() third = %q, want %q", got, "one")
	}
	if got := history.previous("draft"); got != "one" {
		t.Fatalf("previous() clamp = %q, want %q", got, "one")
	}
	if got := history.next("draft"); got != "two" {
		t.Fatalf("next() from oldest = %q, want %q", got, "two")
	}
	if got := history.next("draft"); got != "three" {
		t.Fatalf("next() from middle = %q, want %q", got, "three")
	}
	if got := history.next("draft"); got != "draft" {
		t.Fatalf("next() after newest = %q, want %q", got, "draft")
	}
}

func TestQueryHistoryPreservesDraft(t *testing.T) {
	t.Parallel()

	history := &queryHistoryManager{
		entries: []queryHistoryEntry{
			{Query: "one"},
			{Query: "two"},
		},
		browseIndex: -1,
	}

	if got := history.previous("draft"); got != "two" {
		t.Fatalf("previous() = %q, want %q", got, "two")
	}
	history.setDraft("draft-edited")
	if got := history.next("draft-edited"); got != "draft-edited" {
		t.Fatalf("next() = %q, want %q", got, "draft-edited")
	}
}

func TestQueryHistoryPreservesEmptyDraft(t *testing.T) {
	t.Parallel()

	history := &queryHistoryManager{
		entries: []queryHistoryEntry{
			{Query: "one"},
		},
		browseIndex: -1,
	}

	if got := history.previous(""); got != "one" {
		t.Fatalf("previous() = %q, want %q", got, "one")
	}
	if got := history.next(""); got != "" {
		t.Fatalf("next() = %q, want empty draft", got)
	}
}

func TestQueryHistoryFuzzySearch(t *testing.T) {
	t.Parallel()

	history := &queryHistoryManager{
		entries: []queryHistoryEntry{
			{Query: "FROM metrics-*"},
			{Query: "FROM traces-*"},
			{Query: "FROM logs-api-* | LIMIT 10"},
		},
		browseIndex: -1,
	}

	history.beginSearch("draft")
	value := history.updateSearchTerm("logs")
	if value != "FROM logs-api-* | LIMIT 10" {
		t.Fatalf("updateSearchTerm() = %q, want %q", value, "FROM logs-api-* | LIMIT 10")
	}
	if got := history.acceptSearch(); got != "FROM logs-api-* | LIMIT 10" {
		t.Fatalf("acceptSearch() = %q, want %q", got, "FROM logs-api-* | LIMIT 10")
	}
}
