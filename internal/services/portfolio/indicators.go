package portfolio

import (
	"context"
	"fmt"
	"time"

	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/bobmcallan/vire/internal/signals"
)

// growthPointsToTimeSeries converts growth data points to time series points.
// Adds externalBalanceTotal to each point's value to represent true portfolio value.
func growthPointsToTimeSeries(points []models.GrowthDataPoint, externalBalanceTotal float64) []models.TimeSeriesPoint {
	ts := make([]models.TimeSeriesPoint, len(points))
	for i, p := range points {
		ts[i] = models.TimeSeriesPoint{
			Date:         p.Date,
			Value:        p.TotalValue + externalBalanceTotal,
			Cost:         p.TotalCost,
			NetReturn:    p.NetReturn,
			NetReturnPct: p.NetReturnPct,
			HoldingCount: p.HoldingCount,
		}
	}
	return ts
}

// growthToBars converts growth data points to EOD bars for indicator computation.
// Adds externalBalanceTotal to each point's value to represent true portfolio value.
// Returns bars in newest-first order (matching EODBar convention).
func growthToBars(points []models.GrowthDataPoint, externalBalanceTotal float64) []models.EODBar {
	bars := make([]models.EODBar, len(points))
	for i, p := range points {
		value := p.TotalValue + externalBalanceTotal
		bars[len(points)-1-i] = models.EODBar{
			Date:     p.Date,
			Open:     value,
			High:     value,
			Low:      value,
			Close:    value,
			AdjClose: value,
		}
	}
	return bars
}

// detectEMACrossover compares EMA50 vs EMA200 for current and previous bars.
func detectEMACrossover(bars []models.EODBar) string {
	if len(bars) < 201 {
		return "none"
	}
	currentEMA50 := signals.EMA(bars, 50)
	currentEMA200 := signals.EMA(bars, 200)
	prevEMA50 := signals.EMA(bars[1:], 50)
	prevEMA200 := signals.EMA(bars[1:], 200)

	if currentEMA50 > currentEMA200 && prevEMA50 <= prevEMA200 {
		return "golden_cross"
	}
	if currentEMA50 < currentEMA200 && prevEMA50 >= prevEMA200 {
		return "death_cross"
	}
	return "none"
}

// GetPortfolioIndicators computes technical indicators on the daily portfolio value time series.
func (s *Service) GetPortfolioIndicators(ctx context.Context, name string) (*models.PortfolioIndicators, error) {
	portfolio, err := s.getPortfolioRecord(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("portfolio '%s' not found: %w", name, err)
	}

	growth, err := s.GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to compute daily growth: %w", err)
	}

	if len(growth) == 0 {
		// No historical growth data yet (e.g. freshly synced portfolio).
		// Return a minimal response with DataPoints=1 representing the current state.
		dp := 0
		if portfolio.TotalValue > 0 {
			dp = 1
		}
		return &models.PortfolioIndicators{
			PortfolioName:    name,
			ComputeDate:      time.Now(),
			CurrentValue:     portfolio.TotalValue,
			DataPoints:       dp,
			EMA50CrossEMA200: "none",
			Trend:            models.TrendNeutral,
			TrendDescription: "Portfolio value trend is neutral — insufficient historical data for indicator computation",
			RSISignal:        "neutral",
		}, nil
	}

	bars := growthToBars(growth, portfolio.ExternalBalanceTotal)
	timeSeries := growthPointsToTimeSeries(growth, portfolio.ExternalBalanceTotal)

	ind := &models.PortfolioIndicators{
		PortfolioName: name,
		ComputeDate:   time.Now(),
		CurrentValue:  portfolio.TotalValue,
		DataPoints:    len(bars),
	}

	if len(bars) >= 20 {
		ind.EMA20 = signals.EMA(bars, 20)
		ind.AboveEMA20 = portfolio.TotalValue > ind.EMA20
	}
	if len(bars) >= 50 {
		ind.EMA50 = signals.EMA(bars, 50)
		ind.AboveEMA50 = portfolio.TotalValue > ind.EMA50
	}
	if len(bars) >= 200 {
		ind.EMA200 = signals.EMA(bars, 200)
		ind.AboveEMA200 = portfolio.TotalValue > ind.EMA200
	}
	if len(bars) >= 15 { // RSI(14) needs period+1 bars to compute real changes
		ind.RSI = signals.RSI(bars, 14)
		ind.RSISignal = signals.ClassifyRSI(ind.RSI)
	}

	if len(bars) >= 201 {
		ind.EMA50CrossEMA200 = detectEMACrossover(bars)
	} else {
		ind.EMA50CrossEMA200 = "none"
	}

	sma20 := signals.SMA(bars, 20)
	sma50 := signals.SMA(bars, 50)
	sma200 := signals.SMA(bars, 200)
	ind.Trend = signals.DetermineTrend(portfolio.TotalValue, sma20, sma50, sma200)

	switch ind.Trend {
	case models.TrendBullish:
		ind.TrendDescription = "Portfolio value is in an uptrend — above key moving averages"
	case models.TrendBearish:
		ind.TrendDescription = "Portfolio value is in a downtrend — below key moving averages"
	default:
		ind.TrendDescription = "Portfolio value trend is neutral — mixed signals from moving averages"
	}

	ind.TimeSeries = timeSeries

	return ind, nil
}
