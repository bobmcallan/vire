package portfolio

import (
	"bytes"
	"fmt"
	"time"

	"github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"

	"github.com/bobmcallan/vire/internal/models"
)

// RenderGrowthChart renders a PNG line chart of portfolio market value over time.
// Single series: Portfolio Value (blue solid). Returns raw PNG bytes.
func RenderGrowthChart(points []models.GrowthDataPoint) ([]byte, error) {
	if len(points) < 2 {
		return nil, fmt.Errorf("need at least 2 data points, got %d", len(points))
	}

	xValues := make([]time.Time, len(points))
	valueY := make([]float64, len(points))

	for i, p := range points {
		xValues[i] = p.Date
		valueY[i] = p.EquityValue
	}

	// Adaptive x-axis format based on time span
	span := xValues[len(xValues)-1].Sub(xValues[0])
	xFormat := "Jan 06" // default: month + 2-digit year
	if span < 60*24*time.Hour {
		xFormat = "02 Jan" // < 60 days: day + month
	} else if span > 18*30*24*time.Hour {
		xFormat = "Jan 2006" // > ~18 months: month + 4-digit year
	}

	valueSeries := chart.TimeSeries{
		Name: "Portfolio Value",
		Style: chart.Style{
			StrokeColor: drawing.ColorFromHex("2563eb"), // blue-600
			StrokeWidth: 2.5,
		},
		XValues: xValues,
		YValues: valueY,
	}

	graph := chart.Chart{
		Title:  "Portfolio Value",
		Width:  900,
		Height: 400,
		Background: chart.Style{
			Padding: chart.Box{Top: 40, Left: 10, Right: 20, Bottom: 10},
		},
		XAxis: chart.XAxis{
			TickPosition: chart.TickPositionBetweenTicks,
			ValueFormatter: func(v interface{}) string {
				if t, ok := v.(float64); ok {
					return chart.TimeFromFloat64(t).Format(xFormat)
				}
				return ""
			},
		},
		YAxis: chart.YAxis{
			ValueFormatter: func(v interface{}) string {
				if f, ok := v.(float64); ok {
					return fmt.Sprintf("$%.0fk", f/1000)
				}
				return ""
			},
		},
		Series: []chart.Series{
			valueSeries,
		},
	}

	var buf bytes.Buffer
	if err := graph.Render(chart.PNG, &buf); err != nil {
		return nil, fmt.Errorf("chart render failed: %w", err)
	}

	return buf.Bytes(), nil
}
