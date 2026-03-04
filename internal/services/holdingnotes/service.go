// Package holdingnotes provides portfolio holding notes management services
package holdingnotes

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// Compile-time interface check
var _ interfaces.HoldingNoteService = (*Service)(nil)

// Service implements HoldingNoteService
type Service struct {
	storage interfaces.StorageManager
	logger  *common.Logger
}

// NewService creates a new holding notes service
func NewService(storage interfaces.StorageManager, logger *common.Logger) *Service {
	return &Service{
		storage: storage,
		logger:  logger,
	}
}

// GetNotes retrieves all holding notes for a portfolio
func (s *Service) GetNotes(ctx context.Context, portfolioName string) (*models.PortfolioHoldingNotes, error) {
	userID := common.ResolveUserID(ctx)
	rec, err := s.storage.UserDataStore().Get(ctx, userID, "holding_notes", portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get holding notes: %w", err)
	}
	var notes models.PortfolioHoldingNotes
	if err := json.Unmarshal([]byte(rec.Value), &notes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal holding notes: %w", err)
	}
	return &notes, nil
}

// SaveNotes saves notes with version increment
func (s *Service) SaveNotes(ctx context.Context, notes *models.PortfolioHoldingNotes) error {
	if err := s.saveNotesRecord(ctx, notes); err != nil {
		return fmt.Errorf("failed to save holding notes: %w", err)
	}
	s.logger.Info().Str("portfolio", notes.PortfolioName).Msg("Holding notes saved")
	return nil
}

func (s *Service) saveNotesRecord(ctx context.Context, notes *models.PortfolioHoldingNotes) error {
	userID := common.ResolveUserID(ctx)
	notes.Version++
	notes.UpdatedAt = time.Now()
	if notes.CreatedAt.IsZero() {
		notes.CreatedAt = time.Now()
	}
	data, err := json.Marshal(notes)
	if err != nil {
		return fmt.Errorf("failed to marshal holding notes: %w", err)
	}
	return s.storage.UserDataStore().Put(ctx, &models.UserRecord{
		UserID:  userID,
		Subject: "holding_notes",
		Key:     notes.PortfolioName,
		Value:   string(data),
	})
}

// AddOrUpdateNote adds a new note or updates an existing one (upsert keyed on ticker)
func (s *Service) AddOrUpdateNote(ctx context.Context, portfolioName string, note *models.HoldingNote) (*models.PortfolioHoldingNotes, error) {
	notes, err := s.GetNotes(ctx, portfolioName)
	if err != nil {
		// No existing notes — create new collection
		notes = &models.PortfolioHoldingNotes{
			PortfolioName: portfolioName,
			Items:         []models.HoldingNote{},
		}
	}

	now := time.Now()

	existing, idx := notes.FindByTicker(note.Ticker)
	if idx >= 0 {
		// Update existing: preserve CreatedAt, update ReviewedAt
		note.CreatedAt = existing.CreatedAt
		note.ReviewedAt = now
		note.UpdatedAt = now
		notes.Items[idx] = *note
	} else {
		// New note: set timestamps
		note.CreatedAt = now
		note.ReviewedAt = now
		note.UpdatedAt = now
		notes.Items = append(notes.Items, *note)
	}

	if err := s.saveNotesRecord(ctx, notes); err != nil {
		return nil, fmt.Errorf("failed to save notes after upsert: %w", err)
	}

	return notes, nil
}

// UpdateNote updates an existing note by ticker (merge semantics — only overwrite non-zero fields)
func (s *Service) UpdateNote(ctx context.Context, portfolioName, ticker string, update *models.HoldingNote) (*models.PortfolioHoldingNotes, error) {
	notes, err := s.GetNotes(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get holding notes: %w", err)
	}

	existing, idx := notes.FindByTicker(ticker)
	if idx < 0 {
		return notes, nil // Not found — return unchanged
	}

	now := time.Now()

	// Merge: only update non-zero/non-empty fields
	if update.Name != "" {
		existing.Name = update.Name
	}
	if update.AssetType != "" {
		existing.AssetType = update.AssetType
	}
	if update.LiquidityProfile != "" {
		existing.LiquidityProfile = update.LiquidityProfile
	}
	if update.Thesis != "" {
		existing.Thesis = update.Thesis
	}
	if update.KnownBehaviours != "" {
		existing.KnownBehaviours = update.KnownBehaviours
	}
	if update.SignalOverrides != "" {
		existing.SignalOverrides = update.SignalOverrides
	}
	if update.Notes != "" {
		existing.Notes = update.Notes
	}
	if update.StaleDays != 0 {
		existing.StaleDays = update.StaleDays
	}

	existing.ReviewedAt = now
	existing.UpdatedAt = now
	notes.Items[idx] = *existing

	if err := s.saveNotesRecord(ctx, notes); err != nil {
		return nil, fmt.Errorf("failed to save notes after update: %w", err)
	}

	return notes, nil
}

// RemoveNote removes a note by ticker
func (s *Service) RemoveNote(ctx context.Context, portfolioName, ticker string) (*models.PortfolioHoldingNotes, error) {
	notes, err := s.GetNotes(ctx, portfolioName)
	if err != nil {
		return nil, fmt.Errorf("failed to get holding notes: %w", err)
	}

	_, idx := notes.FindByTicker(ticker)
	if idx < 0 {
		return notes, nil // Not found — return unchanged
	}

	// Remove by splicing
	notes.Items = append(notes.Items[:idx], notes.Items[idx+1:]...)

	if err := s.saveNotesRecord(ctx, notes); err != nil {
		return nil, fmt.Errorf("failed to save notes after removal: %w", err)
	}

	return notes, nil
}
