// Package assetset provides management of non-equity asset sets (property, crypto, etc.)
package assetset

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// Compile-time interface check
var _ interfaces.AssetSetService = (*Service)(nil)

// Service implements AssetSetService
type Service struct {
	storage      interfaces.StorageManager
	portfolioSvc interfaces.PortfolioService
	logger       *common.Logger
}

// NewService creates a new asset set service
func NewService(storage interfaces.StorageManager, logger *common.Logger) *Service {
	return &Service{
		storage: storage,
		logger:  logger,
	}
}

// SetPortfolioService injects portfolio service for timeline invalidation
func (s *Service) SetPortfolioService(svc interfaces.PortfolioService) {
	s.portfolioSvc = svc
}

// generateSetID returns a unique ID with "as_" prefix + 8 hex chars.
func generateSetID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "as_00000000"
	}
	return "as_" + hex.EncodeToString(b)
}

// generateItemID returns a unique ID with "ai_" prefix + 8 hex chars.
func generateItemID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "ai_00000000"
	}
	return "ai_" + hex.EncodeToString(b)
}

// GetAssetSets retrieves all asset sets for a portfolio
func (s *Service) GetAssetSets(ctx context.Context, portfolioName string) (*models.PortfolioAssetSets, error) {
	userID := common.ResolveUserID(ctx)
	rec, err := s.storage.UserDataStore().Get(ctx, userID, "asset_sets", portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get asset sets: %w", err)
	}
	var sets models.PortfolioAssetSets
	if err := json.Unmarshal([]byte(rec.Value), &sets); err != nil {
		return nil, fmt.Errorf("failed to unmarshal asset sets: %w", err)
	}
	return &sets, nil
}

// SaveAssetSets saves asset sets with version increment
func (s *Service) SaveAssetSets(ctx context.Context, sets *models.PortfolioAssetSets) error {
	if err := s.saveRecord(ctx, sets); err != nil {
		return fmt.Errorf("failed to save asset sets: %w", err)
	}
	s.logger.Info().Str("portfolio", sets.PortfolioName).Msg("Asset sets saved")
	return nil
}

func (s *Service) saveRecord(ctx context.Context, sets *models.PortfolioAssetSets) error {
	userID := common.ResolveUserID(ctx)
	sets.Version++
	sets.UpdatedAt = time.Now()
	if sets.CreatedAt.IsZero() {
		sets.CreatedAt = time.Now()
	}
	data, err := json.Marshal(sets)
	if err != nil {
		return fmt.Errorf("failed to marshal asset sets: %w", err)
	}
	return s.storage.UserDataStore().Put(ctx, &models.UserRecord{
		UserID:  userID,
		Subject: "asset_sets",
		Key:     sets.PortfolioName,
		Value:   string(data),
	})
}

// invalidateTimeline triggers a timeline rebuild after asset set changes.
func (s *Service) invalidateTimeline(ctx context.Context, portfolioName string) {
	if s.portfolioSvc != nil {
		s.portfolioSvc.InvalidateAndRebuildTimeline(ctx, portfolioName)
		s.logger.Info().Str("portfolio", portfolioName).Msg("Timeline invalidation triggered by asset set change")
	}
}

// AddAssetSet adds a new asset set to the portfolio
func (s *Service) AddAssetSet(ctx context.Context, portfolioName string, set *models.AssetSet) (*models.PortfolioAssetSets, error) {
	if strings.TrimSpace(set.Name) == "" {
		return nil, fmt.Errorf("asset set name is required")
	}
	if !models.ValidAssetCategory(set.Category) {
		return nil, fmt.Errorf("invalid asset category %q", set.Category)
	}

	sets, err := s.GetAssetSets(ctx, portfolioName)
	if err != nil {
		sets = &models.PortfolioAssetSets{
			PortfolioName: portfolioName,
			Sets:          []models.AssetSet{},
		}
	}

	now := time.Now()
	set.ID = generateSetID()
	set.Name = strings.TrimSpace(set.Name)
	set.CreatedAt = now
	set.UpdatedAt = now
	if set.Items == nil {
		set.Items = []models.AssetItem{}
	}
	// Assign IDs to any items provided at creation
	for i := range set.Items {
		if set.Items[i].ID == "" {
			set.Items[i].ID = generateItemID()
		}
		set.Items[i].UpdatedAt = now
	}

	sets.Sets = append(sets.Sets, *set)

	if err := s.saveRecord(ctx, sets); err != nil {
		return nil, fmt.Errorf("failed to save after adding asset set: %w", err)
	}

	s.invalidateTimeline(ctx, portfolioName)
	return sets, nil
}

// UpdateAssetSet updates an existing asset set by ID (merge semantics)
func (s *Service) UpdateAssetSet(ctx context.Context, portfolioName string, setID string, update *models.AssetSet) (*models.PortfolioAssetSets, error) {
	sets, err := s.GetAssetSets(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get asset sets: %w", err)
	}

	existing, idx := sets.FindSetByID(setID)
	if idx < 0 {
		return nil, fmt.Errorf("asset set %q not found", setID)
	}

	now := time.Now()
	if update.Name != "" {
		existing.Name = strings.TrimSpace(update.Name)
	}
	if update.Category != "" {
		if !models.ValidAssetCategory(update.Category) {
			return nil, fmt.Errorf("invalid asset category %q", update.Category)
		}
		existing.Category = update.Category
	}
	if update.Currency != "" {
		existing.Currency = update.Currency
	}
	if update.Notes != "" {
		existing.Notes = update.Notes
	}
	existing.UpdatedAt = now
	sets.Sets[idx] = *existing

	if err := s.saveRecord(ctx, sets); err != nil {
		return nil, fmt.Errorf("failed to save after updating asset set: %w", err)
	}

	s.invalidateTimeline(ctx, portfolioName)
	return sets, nil
}

// RemoveAssetSet removes an asset set by ID
func (s *Service) RemoveAssetSet(ctx context.Context, portfolioName string, setID string) (*models.PortfolioAssetSets, error) {
	sets, err := s.GetAssetSets(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get asset sets: %w", err)
	}

	_, idx := sets.FindSetByID(setID)
	if idx < 0 {
		return nil, fmt.Errorf("asset set %q not found", setID)
	}

	sets.Sets = append(sets.Sets[:idx], sets.Sets[idx+1:]...)

	if err := s.saveRecord(ctx, sets); err != nil {
		return nil, fmt.Errorf("failed to save after removing asset set: %w", err)
	}

	s.invalidateTimeline(ctx, portfolioName)
	return sets, nil
}

// AddItem adds an item to an asset set
func (s *Service) AddItem(ctx context.Context, portfolioName string, setID string, item *models.AssetItem) (*models.PortfolioAssetSets, error) {
	if strings.TrimSpace(item.Name) == "" {
		return nil, fmt.Errorf("item name is required")
	}

	sets, err := s.GetAssetSets(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get asset sets: %w", err)
	}

	existing, idx := sets.FindSetByID(setID)
	if idx < 0 {
		return nil, fmt.Errorf("asset set %q not found", setID)
	}

	now := time.Now()
	item.ID = generateItemID()
	item.Name = strings.TrimSpace(item.Name)
	item.UpdatedAt = now

	existing.Items = append(existing.Items, *item)
	existing.UpdatedAt = now
	sets.Sets[idx] = *existing

	if err := s.saveRecord(ctx, sets); err != nil {
		return nil, fmt.Errorf("failed to save after adding item: %w", err)
	}

	s.invalidateTimeline(ctx, portfolioName)
	return sets, nil
}

// UpdateItem updates an item within an asset set (merge semantics)
func (s *Service) UpdateItem(ctx context.Context, portfolioName string, setID string, itemID string, update *models.AssetItem) (*models.PortfolioAssetSets, error) {
	sets, err := s.GetAssetSets(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get asset sets: %w", err)
	}

	set, setIdx := sets.FindSetByID(setID)
	if setIdx < 0 {
		return nil, fmt.Errorf("asset set %q not found", setID)
	}

	item, itemIdx := set.FindItemByID(itemID)
	if itemIdx < 0 {
		return nil, fmt.Errorf("item %q not found in set %q", itemID, setID)
	}

	now := time.Now()
	if update.Name != "" {
		item.Name = strings.TrimSpace(update.Name)
	}
	if update.Value != 0 {
		item.Value = update.Value
	}
	if update.CostBasis != 0 {
		item.CostBasis = update.CostBasis
	}
	if !update.AcquiredAt.IsZero() {
		item.AcquiredAt = update.AcquiredAt
	}
	if update.Description != "" {
		item.Description = update.Description
	}
	if update.Notes != "" {
		item.Notes = update.Notes
	}
	item.UpdatedAt = now

	set.Items[itemIdx] = *item
	set.UpdatedAt = now
	sets.Sets[setIdx] = *set

	if err := s.saveRecord(ctx, sets); err != nil {
		return nil, fmt.Errorf("failed to save after updating item: %w", err)
	}

	s.invalidateTimeline(ctx, portfolioName)
	return sets, nil
}

// RemoveItem removes an item from an asset set
func (s *Service) RemoveItem(ctx context.Context, portfolioName string, setID string, itemID string) (*models.PortfolioAssetSets, error) {
	sets, err := s.GetAssetSets(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get asset sets: %w", err)
	}

	set, setIdx := sets.FindSetByID(setID)
	if setIdx < 0 {
		return nil, fmt.Errorf("asset set %q not found", setID)
	}

	_, itemIdx := set.FindItemByID(itemID)
	if itemIdx < 0 {
		return nil, fmt.Errorf("item %q not found in set %q", itemID, setID)
	}

	set.Items = append(set.Items[:itemIdx], set.Items[itemIdx+1:]...)
	set.UpdatedAt = time.Now()
	sets.Sets[setIdx] = *set

	if err := s.saveRecord(ctx, sets); err != nil {
		return nil, fmt.Errorf("failed to save after removing item: %w", err)
	}

	s.invalidateTimeline(ctx, portfolioName)
	return sets, nil
}
