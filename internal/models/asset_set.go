package models

import "time"

// AssetCategory classifies the type of asset set.
type AssetCategory string

const (
	AssetCategoryProperty    AssetCategory = "property"     // Real estate
	AssetCategoryFixedIncome AssetCategory = "fixed_income" // Bonds, term deposits
	AssetCategoryCrypto      AssetCategory = "crypto"       // Cryptocurrency
	AssetCategoryCollectible AssetCategory = "collectible"  // Art, wine, cars, etc.
	AssetCategoryOther       AssetCategory = "other"        // Catch-all
)

// validAssetCategories lists all accepted categories for manual asset sets.
// Equity holdings use the existing Holdings[] system, not AssetSets.
var validAssetCategories = map[AssetCategory]bool{
	AssetCategoryProperty:    true,
	AssetCategoryFixedIncome: true,
	AssetCategoryCrypto:      true,
	AssetCategoryCollectible: true,
	AssetCategoryOther:       true,
}

// ValidAssetCategory returns true if c is a valid category for manual asset sets.
func ValidAssetCategory(c AssetCategory) bool {
	return validAssetCategories[c]
}

// AssetItem represents a single asset within an asset set.
type AssetItem struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`                    // e.g., "123 Main St", "Bitcoin"
	Value       float64   `json:"value"`                   // Current estimated market value
	CostBasis   float64   `json:"cost_basis"`              // Original purchase price / cost
	AcquiredAt  time.Time `json:"acquired_at,omitempty"`   // When acquired
	Description string    `json:"description,omitempty"`
	Notes       string    `json:"notes,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AssetSet is a named collection of assets within a category.
// A portfolio can have multiple sets of the same category
// (e.g., "Sydney Property", "Melbourne Property").
type AssetSet struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`              // e.g., "Investment Properties"
	Category  AssetCategory `json:"category"`          // property, crypto, etc.
	Items     []AssetItem   `json:"items"`
	Currency  string        `json:"currency,omitempty"` // ISO 4217 (default AUD)
	Notes     string        `json:"notes,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// TotalValue returns the sum of all item values in this set.
func (s *AssetSet) TotalValue() float64 {
	var total float64
	for _, item := range s.Items {
		total += item.Value
	}
	return total
}

// TotalCostBasis returns the sum of all item cost bases in this set.
func (s *AssetSet) TotalCostBasis() float64 {
	var total float64
	for _, item := range s.Items {
		total += item.CostBasis
	}
	return total
}

// FindItemByID returns the item and index for a given ID, or nil/-1 if not found.
func (s *AssetSet) FindItemByID(id string) (*AssetItem, int) {
	for i := range s.Items {
		if s.Items[i].ID == id {
			return &s.Items[i], i
		}
	}
	return nil, -1
}

// PortfolioAssetSets is the versioned collection of asset sets per portfolio.
// Stored as a single document in UserDataStore (subject="asset_sets").
type PortfolioAssetSets struct {
	PortfolioName string     `json:"portfolio_name"`
	Version       int        `json:"version"`
	Sets          []AssetSet `json:"sets"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// TotalValue returns the aggregate value across all asset sets.
func (p *PortfolioAssetSets) TotalValue() float64 {
	var total float64
	for _, s := range p.Sets {
		total += s.TotalValue()
	}
	return total
}

// TotalCostBasis returns the aggregate cost basis across all asset sets.
func (p *PortfolioAssetSets) TotalCostBasis() float64 {
	var total float64
	for _, s := range p.Sets {
		total += s.TotalCostBasis()
	}
	return total
}

// FindSetByID returns the set and index for a given ID, or nil/-1 if not found.
func (p *PortfolioAssetSets) FindSetByID(id string) (*AssetSet, int) {
	for i := range p.Sets {
		if p.Sets[i].ID == id {
			return &p.Sets[i], i
		}
	}
	return nil, -1
}
