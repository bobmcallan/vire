package models

import (
	"testing"
	"time"
)

func TestValidAssetCategory(t *testing.T) {
	tests := []struct {
		category AssetCategory
		valid    bool
	}{
		{AssetCategoryProperty, true},
		{AssetCategoryFixedIncome, true},
		{AssetCategoryCrypto, true},
		{AssetCategoryCollectible, true},
		{AssetCategoryOther, true},
		{"equity", false},      // equity uses Holdings[], not AssetSets
		{"unknown", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := ValidAssetCategory(tt.category); got != tt.valid {
			t.Errorf("ValidAssetCategory(%q) = %v, want %v", tt.category, got, tt.valid)
		}
	}
}

func TestAssetSetTotalValue(t *testing.T) {
	set := AssetSet{
		Items: []AssetItem{
			{Name: "Property A", Value: 500000},
			{Name: "Property B", Value: 750000},
		},
	}
	if got := set.TotalValue(); got != 1250000 {
		t.Errorf("TotalValue() = %v, want 1250000", got)
	}
}

func TestAssetSetTotalValue_Empty(t *testing.T) {
	set := AssetSet{Items: []AssetItem{}}
	if got := set.TotalValue(); got != 0 {
		t.Errorf("TotalValue() = %v, want 0", got)
	}
}

func TestAssetSetTotalCostBasis(t *testing.T) {
	set := AssetSet{
		Items: []AssetItem{
			{Name: "Property A", CostBasis: 400000},
			{Name: "Property B", CostBasis: 600000},
		},
	}
	if got := set.TotalCostBasis(); got != 1000000 {
		t.Errorf("TotalCostBasis() = %v, want 1000000", got)
	}
}

func TestAssetSetFindItemByID(t *testing.T) {
	set := AssetSet{
		Items: []AssetItem{
			{ID: "ai_001", Name: "Item 1"},
			{ID: "ai_002", Name: "Item 2"},
		},
	}

	item, idx := set.FindItemByID("ai_002")
	if idx != 1 || item.Name != "Item 2" {
		t.Errorf("FindItemByID('ai_002') = (%v, %d), want (Item 2, 1)", item, idx)
	}

	item, idx = set.FindItemByID("ai_999")
	if idx != -1 || item != nil {
		t.Errorf("FindItemByID('ai_999') = (%v, %d), want (nil, -1)", item, idx)
	}
}

func TestPortfolioAssetSetsTotalValue(t *testing.T) {
	pas := PortfolioAssetSets{
		Sets: []AssetSet{
			{Items: []AssetItem{{Value: 500000}, {Value: 300000}}},
			{Items: []AssetItem{{Value: 200000}}},
		},
	}
	if got := pas.TotalValue(); got != 1000000 {
		t.Errorf("TotalValue() = %v, want 1000000", got)
	}
}

func TestPortfolioAssetSetsTotalCostBasis(t *testing.T) {
	pas := PortfolioAssetSets{
		Sets: []AssetSet{
			{Items: []AssetItem{{CostBasis: 400000}, {CostBasis: 200000}}},
			{Items: []AssetItem{{CostBasis: 100000}}},
		},
	}
	if got := pas.TotalCostBasis(); got != 700000 {
		t.Errorf("TotalCostBasis() = %v, want 700000", got)
	}
}

func TestPortfolioAssetSetsFindSetByID(t *testing.T) {
	pas := PortfolioAssetSets{
		Sets: []AssetSet{
			{ID: "as_001", Name: "Properties"},
			{ID: "as_002", Name: "Crypto"},
		},
	}

	set, idx := pas.FindSetByID("as_002")
	if idx != 1 || set.Name != "Crypto" {
		t.Errorf("FindSetByID('as_002') = (%v, %d), want (Crypto, 1)", set, idx)
	}

	set, idx = pas.FindSetByID("as_999")
	if idx != -1 || set != nil {
		t.Errorf("FindSetByID('as_999') = (%v, %d), want (nil, -1)", set, idx)
	}
}

func TestPortfolioAssetSets_Empty(t *testing.T) {
	pas := PortfolioAssetSets{Sets: []AssetSet{}}
	if got := pas.TotalValue(); got != 0 {
		t.Errorf("TotalValue() on empty = %v, want 0", got)
	}
	if got := pas.TotalCostBasis(); got != 0 {
		t.Errorf("TotalCostBasis() on empty = %v, want 0", got)
	}
}

func TestMultipleSetsOfSameCategory(t *testing.T) {
	pas := PortfolioAssetSets{
		Sets: []AssetSet{
			{
				ID:       "as_001",
				Name:     "Sydney Property",
				Category: AssetCategoryProperty,
				Items: []AssetItem{
					{ID: "ai_001", Name: "Unit 1", Value: 600000, CostBasis: 500000, AcquiredAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)},
				},
			},
			{
				ID:       "as_002",
				Name:     "Melbourne Property",
				Category: AssetCategoryProperty,
				Items: []AssetItem{
					{ID: "ai_002", Name: "House", Value: 900000, CostBasis: 700000, AcquiredAt: time.Date(2019, 6, 1, 0, 0, 0, 0, time.UTC)},
				},
			},
		},
	}

	if got := pas.TotalValue(); got != 1500000 {
		t.Errorf("TotalValue() = %v, want 1500000", got)
	}
	if got := pas.TotalCostBasis(); got != 1200000 {
		t.Errorf("TotalCostBasis() = %v, want 1200000", got)
	}

	// Both sets should have the same category
	for _, s := range pas.Sets {
		if s.Category != AssetCategoryProperty {
			t.Errorf("Expected category property, got %v", s.Category)
		}
	}
}
