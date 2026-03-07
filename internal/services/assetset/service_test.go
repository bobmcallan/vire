package assetset

import (
	"strings"
	"testing"
)

func TestGenerateSetID(t *testing.T) {
	id := generateSetID()
	if !strings.HasPrefix(id, "as_") {
		t.Errorf("ID should start with 'as_', got %q", id)
	}
	if len(id) != 11 { // "as_" + 8 hex chars
		t.Errorf("ID should be 11 chars (as_ + 8 hex), got %d: %q", len(id), id)
	}

	id2 := generateSetID()
	if id == id2 {
		t.Errorf("IDs should be unique, got %q twice", id)
	}
}

func TestGenerateItemID(t *testing.T) {
	id := generateItemID()
	if !strings.HasPrefix(id, "ai_") {
		t.Errorf("ID should start with 'ai_', got %q", id)
	}
	if len(id) != 11 { // "ai_" + 8 hex chars
		t.Errorf("ID should be 11 chars (ai_ + 8 hex), got %d: %q", len(id), id)
	}

	id2 := generateItemID()
	if id == id2 {
		t.Errorf("IDs should be unique, got %q twice", id)
	}
}

func TestGenerateSetID_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 10000; i++ {
		id := generateSetID()
		if ids[id] {
			t.Fatalf("duplicate set ID after %d iterations: %q", i, id)
		}
		ids[id] = true
	}
}

func TestGenerateItemID_Uniqueness(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 10000; i++ {
		id := generateItemID()
		if ids[id] {
			t.Fatalf("duplicate item ID after %d iterations: %q", i, id)
		}
		ids[id] = true
	}
}
