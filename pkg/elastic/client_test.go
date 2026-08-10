package elastic

import (
	"encoding/json"
	"testing"
)

func TestBuildESQLRequestBodyWithoutTimeRange(t *testing.T) {
	body, err := buildESQLRequestBody(QueryOptions{Query: "FROM logs-* | LIMIT 10"})
	if err != nil {
		t.Fatalf("buildESQLRequestBody returned error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	if payload["query"] != "FROM logs-* | LIMIT 10" {
		t.Fatalf("unexpected query payload: %#v", payload["query"])
	}
	if _, ok := payload["filter"]; ok {
		t.Fatal("did not expect filter in payload")
	}
}

func TestBuildESQLRequestBodyWithTimeRange(t *testing.T) {
	body, err := buildESQLRequestBody(QueryOptions{
		Query: "FROM logs-* | LIMIT 10",
		From:  "now-2h",
		To:    "now",
	})
	if err != nil {
		t.Fatalf("buildESQLRequestBody returned error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	filter, ok := payload["filter"].(map[string]any)
	if !ok {
		t.Fatalf("expected filter object, got %#v", payload["filter"])
	}
	boolFilter, ok := filter["bool"].(map[string]any)
	if !ok {
		t.Fatalf("expected bool filter, got %#v", filter["bool"])
	}
	filters, ok := boolFilter["filter"].([]any)
	if !ok || len(filters) != 1 {
		t.Fatalf("expected single filter entry, got %#v", boolFilter["filter"])
	}
	rangeFilter, ok := filters[0].(map[string]any)
	if !ok {
		t.Fatalf("expected range filter object, got %#v", filters[0])
	}
	rangeBody, ok := rangeFilter["range"].(map[string]any)
	if !ok {
		t.Fatalf("expected range body, got %#v", rangeFilter["range"])
	}
	timestampRange, ok := rangeBody["@timestamp"].(map[string]any)
	if !ok {
		t.Fatalf("expected @timestamp range, got %#v", rangeBody["@timestamp"])
	}
	if timestampRange["gte"] != "now-2h" || timestampRange["lte"] != "now" {
		t.Fatalf("unexpected time range: %#v", timestampRange)
	}
}

func TestBuildESQLRequestIncludesDropNullColumns(t *testing.T) {
	req := buildESQLRequest([]byte(`{"query":"FROM logs-*"}`), QueryOptions{
		Query:           "FROM logs-*",
		DropNullColumns: true,
	})

	if req.DropNullColumns == nil || !*req.DropNullColumns {
		t.Fatal("expected DropNullColumns to be enabled")
	}
}

func TestBuildESQLRequestLeavesDropNullColumnsUnsetByDefault(t *testing.T) {
	req := buildESQLRequest([]byte(`{"query":"FROM logs-*"}`), QueryOptions{
		Query: "FROM logs-*",
	})

	if req.DropNullColumns != nil {
		t.Fatal("expected DropNullColumns to be unset")
	}
}
