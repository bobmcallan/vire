package portfolio

import (
	"bytes"
	"fmt"
	"time"

	"github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"

	"github.com/bobmccarthy/vire/internal/models"
)

// RenderGrowthChart renders a PNG line chart from growth data points.
// Two series: Portfolio Value (blue solid) and Total Cost (gray dashed).
// Returns raw PNG bytes.
func RenderGrowthChart(points []models.GrowthDataPoint) ([]byte, error) {
	if len(points) < 2 {
		return nil, fmt.Errorf("need at least 2 data points, got %d", len(points))
	}

	xValues := make([]time.Time, len(points))
	valueY := make([]float64, len(points))
	costY := make([]float64, len(points))

	for i, p := range points {
		xValues[i] = p.Date
		valueY[i] = p.TotalValue
		costY[i] = p.TotalCost
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

	costSeries := chart.TimeSeries{
		Name: "Total Cost",
		Style: chart.Style{
			StrokeColor:     drawing.ColorFromHex("9ca3af"), // gray-400
			StrokeWidth:     1.5,
			StrokeDashArray: []float64{5.0, 3.0},
		},
		XValues: xValues,
		YValues: costY,
	}

	graph := chart.Chart{
		Title:  "Portfolio Growth",
		Width:  900,
		Height: 400,
		Background: chart.Style{
			Padding: chart.Box{Top: 40, Left: 10, Right: 20, Bottom: 10},
		},
		XAxis: chart.XAxis{
			TickPosition: chart.TickPositionBetweenTicks,
			ValueFormatter: func(v interface{}) string {
				if t, ok := v.(float64); ok {
					return chart.TimeFromFloat64(t).Format("Jan 06")
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
			costSeries,
		},
	}

	graph.Elements = []chart.Renderable{
		chart.LegendLeft(&graph),
	}

	var buf bytes.Buffer
	if err := graph.Render(chart.PNG, &buf); err != nil {
		return nil, fmt.Errorf("chart render failed: %w", err)
	}

	return buf.Bytes(), nil
}
