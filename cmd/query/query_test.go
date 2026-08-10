package query

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dominicluechinger/esqlorer/pkg/elastic"
)

func TestValidateTimeRangeAcceptsAscendingRelativeRange(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	if err := validateTimeRange("now-2h", "now", now); err != nil {
		t.Fatalf("expected valid range, got error: %v", err)
	}
}

func TestValidateTimeRangeRejectsDescendingRelativeRange(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	err := validateTimeRange("now", "now-2h", now)
	if err == nil {
		t.Fatal("expected inverted range to fail")
	}
	if !strings.Contains(err.Error(), "--from") {
		t.Fatalf("expected clear flag error, got: %v", err)
	}
}

func TestValidateTimeRangeAcceptsAscendingRFC3339Range(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	if err := validateTimeRange("2026-05-01T10:00:00Z", "2026-05-01T12:00:00Z", now); err != nil {
		t.Fatalf("expected valid range, got error: %v", err)
	}
}

func TestValidateTimeRangeSkipsUnknownFormats(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	if err := validateTimeRange("now/d", "now", now); err != nil {
		t.Fatalf("expected unsupported but pass-through format to skip validation, got: %v", err)
	}
}

func TestPrintJSONEmitsRowsWithColumnKeys(t *testing.T) {
	result := &elastic.QueryResult{
		Columns: []elastic.Column{{Name: "service"}, {Name: "count"}},
		Values: [][]interface{}{
			{"api", 3},
		},
	}

	output := captureStdout(t, func() {
		if err := printJSON(result); err != nil {
			t.Fatalf("printJSON returned error: %v", err)
		}
	})

	var rows []map[string]any
	if err := json.Unmarshal([]byte(output), &rows); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, output)
	}

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0]["service"] != "api" {
		t.Fatalf("expected service key to map to api, got %#v", rows[0]["service"])
	}
	if rows[0]["count"] != float64(3) {
		t.Fatalf("expected count key to map to 3, got %#v", rows[0]["count"])
	}
}

func TestPrintCSVEmitsRealCSV(t *testing.T) {
	result := &elastic.QueryResult{
		Columns: []elastic.Column{{Name: "service"}, {Name: "message"}},
		Values: [][]interface{}{
			{"api", "hello, world"},
		},
	}

	output := captureStdout(t, func() {
		if err := printCSV(result); err != nil {
			t.Fatalf("printCSV returned error: %v", err)
		}
	})

	reader := csv.NewReader(strings.NewReader(output))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("output is not valid CSV: %v\n%s", err, output)
	}

	if len(rows) != 2 {
		t.Fatalf("expected header plus one data row, got %d", len(rows))
	}
	if rows[0][0] != "service" || rows[0][1] != "message" {
		t.Fatalf("unexpected CSV headers: %#v", rows[0])
	}
	if rows[1][0] != "api" || rows[1][1] != "hello, world" {
		t.Fatalf("unexpected CSV row: %#v", rows[1])
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	_ = w.Close()
	os.Stdout = old
	return <-done
}
