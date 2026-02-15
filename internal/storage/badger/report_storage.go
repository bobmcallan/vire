package badger

import (
	"context"
	"fmt"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/timshannon/badgerhold/v4"
)

type reportStorage struct {
	store  *Store
	logger *common.Logger
}

// NewReportStorage creates a new ReportStorage backed by BadgerHold.
func NewReportStorage(store *Store, logger *common.Logger) *reportStorage {
	return &reportStorage{store: store, logger: logger}
}

func (s *reportStorage) GetReport(_ context.Context, portfolio string) (*models.PortfolioReport, error) {
	var report models.PortfolioReport
	err := s.store.db.Get(portfolio, &report)
	if err != nil {
		if err == badgerhold.ErrNotFound {
			return nil, fmt.Errorf("report for '%s' not found", portfolio)
		}
		return nil, fmt.Errorf("failed to get report for '%s': %w", portfolio, err)
	}
	return &report, nil
}

func (s *reportStorage) SaveReport(_ context.Context, report *models.PortfolioReport) error {
	if err := s.store.db.Upsert(report.Portfolio, report); err != nil {
		return fmt.Errorf("failed to save report: %w", err)
	}
	s.logger.Debug().Str("portfolio", report.Portfolio).Msg("Report saved")
	return nil
}

func (s *reportStorage) ListReports(_ context.Context) ([]string, error) {
	var reports []models.PortfolioReport
	if err := s.store.db.Find(&reports, nil); err != nil {
		return nil, fmt.Errorf("failed to list reports: %w", err)
	}
	names := make([]string, len(reports))
	for i, r := range reports {
		names[i] = r.Portfolio
	}
	return names, nil
}

func (s *reportStorage) DeleteReport(_ context.Context, portfolio string) error {
	err := s.store.db.Delete(portfolio, models.PortfolioReport{})
	if err != nil && err != badgerhold.ErrNotFound {
		return fmt.Errorf("failed to delete report for '%s': %w", portfolio, err)
	}
	s.logger.Debug().Str("portfolio", portfolio).Msg("Report deleted")
	return nil
}
