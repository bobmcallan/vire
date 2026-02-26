// Package quote provides a real-time quote service with automatic fallback
package quote

import (
	"context"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// StalenessThreshold is the age beyond which an EODHD quote is considered
// stale enough to attempt the ASX fallback. Set to 2 hours to distinguish
// normal EODHD delay (~20 min for ASX) from genuinely broken data (>24h stale).
var StalenessThreshold = 2 * time.Hour

// sydneyLocation is the Australia/Sydney timezone which handles both
// AEST (UTC+10) and AEDT (UTC+11) automatically based on DST rules.
var sydneyLocation = mustLoadLocation("Australia/Sydney")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		// Fallback to AEST fixed zone if tzdata is unavailable (e.g., minimal container)
		return time.FixedZone("AEST", 10*60*60)
	}
	return loc
}

// Service implements QuoteService with EODHD-primary and ASX-fallback.
type Service struct {
	eodhd   interfaces.EODHDClient
	asx     interfaces.ASXClient
	storage interfaces.StorageManager
	logger  *common.Logger
	now     func() time.Time // injectable clock for testing
}

// NewService creates a new quote service.
// asx may be nil if the ASX client is not available — fallback will be skipped.
// storage may be nil — historical price fields will be omitted if unavailable.
func NewService(eodhd interfaces.EODHDClient, asx interfaces.ASXClient, storage interfaces.StorageManager, logger *common.Logger) *Service {
	return &Service{
		eodhd:   eodhd,
		asx:     asx,
		storage: storage,
		logger:  logger,
		now:     time.Now,
	}
}

// GetRealTimeQuote retrieves a live quote, falling back to ASX Markit Digital
// when the EODHD quote is stale for an ASX-listed ticker during market hours.
func (s *Service) GetRealTimeQuote(ctx context.Context, ticker string) (*models.RealTimeQuote, error) {
	quote, eodhdErr := s.eodhd.GetRealTimeQuote(ctx, ticker)
	if eodhdErr == nil && quote != nil {
		quote.Source = "eodhd"
	}

	// Only attempt ASX fallback for .AU tickers when we have an ASX client
	if !isASXTicker(ticker) || s.asx == nil {
		if eodhdErr != nil {
			return nil, eodhdErr
		}
		s.populateHistoricalFields(ctx, ticker, quote)
		return quote, nil
	}

	// If EODHD succeeded with fresh data, return it
	if eodhdErr == nil && quote != nil && !s.isStale(quote.Timestamp) {
		s.populateHistoricalFields(ctx, ticker, quote)
		return quote, nil
	}

	// Only fall back during ASX market hours
	if !isASXMarketHours(s.now()) {
		if eodhdErr != nil {
			return nil, eodhdErr
		}
		s.populateHistoricalFields(ctx, ticker, quote)
		return quote, nil
	}

	// Try ASX fallback
	s.logger.Info().
		Str("ticker", ticker).
		Bool("eodhd_failed", eodhdErr != nil).
		Msg("Attempting ASX Markit fallback for stale quote")

	asxQuote, asxErr := s.asx.GetRealTimeQuote(ctx, ticker)
	if asxErr != nil {
		s.logger.Warn().Err(asxErr).Str("ticker", ticker).Msg("ASX Markit fallback failed")
		// Return the stale EODHD quote if we have one, otherwise propagate the original error
		if eodhdErr != nil {
			return nil, eodhdErr
		}
		s.populateHistoricalFields(ctx, ticker, quote)
		return quote, nil
	}

	s.logger.Info().
		Str("ticker", ticker).
		Str("source", "asx").
		Float64("price", asxQuote.Close).
		Msg("ASX Markit fallback succeeded")

	s.populateHistoricalFields(ctx, ticker, asxQuote)
	return asxQuote, nil
}

// populateHistoricalFields adds yesterday and last week price fields from EOD data.
func (s *Service) populateHistoricalFields(ctx context.Context, ticker string, quote *models.RealTimeQuote) {
	if s.storage == nil || quote == nil {
		return
	}

	marketData, err := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
	if err != nil || marketData == nil || len(marketData.EOD) < 2 {
		return
	}

	currentPrice := quote.Close

	// Yesterday: EOD[1] is previous trading day close
	if len(marketData.EOD) > 1 {
		quote.YesterdayClose = marketData.EOD[1].Close
		if marketData.EOD[1].Close > 0 {
			quote.YesterdayPct = ((currentPrice - marketData.EOD[1].Close) / marketData.EOD[1].Close) * 100
		}
	}

	// Last week: ~5 trading days back (offset 5 from today = EOD[5])
	if len(marketData.EOD) > 5 {
		lastWeekClose := marketData.EOD[5].Close
		quote.LastWeekClose = lastWeekClose
		if lastWeekClose > 0 {
			quote.LastWeekPct = ((currentPrice - lastWeekClose) / lastWeekClose) * 100
		}
	}
}

// isStale returns true when the quote timestamp is older than StalenessThreshold.
func (s *Service) isStale(ts time.Time) bool {
	if ts.IsZero() {
		return true
	}
	return s.now().Sub(ts) > StalenessThreshold
}

// isASXTicker returns true if the ticker has an .AU exchange suffix.
func isASXTicker(ticker string) bool {
	return strings.HasSuffix(strings.ToUpper(ticker), ".AU")
}

// isASXMarketHours returns true if the given time falls within ASX trading
// hours: 10:00–16:30 local Sydney time (AEST/AEDT), Monday–Friday.
func isASXMarketHours(t time.Time) bool {
	aest := t.In(sydneyLocation)
	weekday := aest.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return false
	}
	hour, min, _ := aest.Clock()
	minuteOfDay := hour*60 + min
	// 10:00 = 600, 16:30 = 990
	return minuteOfDay >= 600 && minuteOfDay <= 990
}

// Ensure Service implements QuoteService
var _ interfaces.QuoteService = (*Service)(nil)
