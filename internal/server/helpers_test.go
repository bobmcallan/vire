package server

import (
	"encoding/json"
	"testing"
)

type testBalance struct {
	Type  string  `json:"type"`
	Label string  `json:"label"`
	Value float64 `json:"value"`
	Rate  float64 `json:"rate,omitempty"`
}

func TestUnmarshalArrayParam_NativeObjects(t *testing.T) {
	raw := json.RawMessage(`[{"type":"cash","label":"ANZ","value":48000},{"type":"accumulate","label":"Stake","value":50000,"rate":0.05}]`)
	var result []testBalance
	if err := UnmarshalArrayParam(raw, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result))
	}
	if result[0].Label != "ANZ" || result[0].Value != 48000 {
		t.Errorf("item 0: got %+v", result[0])
	}
	if result[1].Label != "Stake" || result[1].Rate != 0.05 {
		t.Errorf("item 1: got %+v", result[1])
	}
}

func TestUnmarshalArrayParam_StringEncodedObjects(t *testing.T) {
	raw := json.RawMessage(`["{\"type\":\"cash\",\"label\":\"ANZ\",\"value\":48000}","{\"type\":\"accumulate\",\"label\":\"Stake\",\"value\":50000,\"rate\":0.05}"]`)
	var result []testBalance
	if err := UnmarshalArrayParam(raw, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result))
	}
	if result[0].Label != "ANZ" || result[0].Value != 48000 {
		t.Errorf("item 0: got %+v", result[0])
	}
	if result[1].Label != "Stake" || result[1].Rate != 0.05 {
		t.Errorf("item 1: got %+v", result[1])
	}
}

func TestUnmarshalArrayParam_EmptyArray(t *testing.T) {
	raw := json.RawMessage(`[]`)
	var result []testBalance
	if err := UnmarshalArrayParam(raw, &result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 items, got %d", len(result))
	}
}

func TestUnmarshalArrayParam_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`not json`)
	var result []testBalance
	if err := UnmarshalArrayParam(raw, &result); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestUnmarshalArrayParam_InvalidStringContent(t *testing.T) {
	raw := json.RawMessage(`["not a json object","also not"]`)
	var result []testBalance
	if err := UnmarshalArrayParam(raw, &result); err == nil {
		t.Fatal("expected error for non-object strings")
	}
}
