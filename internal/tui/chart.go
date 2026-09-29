package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/NimbleMarkets/ntcharts/v2/canvas"
	"github.com/NimbleMarkets/ntcharts/v2/canvas/graph"
	"github.com/NimbleMarkets/ntcharts/v2/canvas/runes"
)

var (
	upStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	downStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	axisStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	ma5Style   = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	ma20Style  = lipgloss.NewStyle().Foreground(lipgloss.Color("201"))
	ma60Style  = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	ma120Style = lipgloss.NewStyle().Foreground(lipgloss.Color("51"))
)

func renderChart(candles []domain.Candle, width, height int, maVisibility ...bool) string {
	if len(candles) == 0 {
		return "차트 데이터가 없습니다."
	}
	width = max(30, width)
	height = max(10, height)
	plotWidth := width - 14
	if plotWidth < 12 {
		plotWidth = 12
	}
	visible := candles
	if len(visible) > plotWidth {
		visible = visible[len(visible)-plotWidth:]
	}

	minPrice, maxPrice := math.MaxFloat64, -math.MaxFloat64
	for _, c := range visible {
		lo, _ := c.Low.Float64()
		hi, _ := c.High.Float64()
		minPrice = math.Min(minPrice, lo)
		maxPrice = math.Max(maxPrice, hi)
	}
	if minPrice == maxPrice {
		minPrice--
		maxPrice++
	}
	padding := (maxPrice - minPrice) * .04
	minPrice -= padding
	maxPrice += padding

	chart := canvas.New(plotWidth, height)
	graph.DrawXYAxis(&chart, canvas.Point{X: 0, Y: height - 1}, axisStyle)
	scale := func(v float64) float64 { return (v - minPrice) / (maxPrice - minPrice) * float64(height-2) }
	for i, c := range visible {
		o, _ := c.Open.Float64()
		h, _ := c.High.Float64()
		l, _ := c.Low.Float64()
		cl, _ := c.Close.Float64()
		style := upStyle
		if cl < o {
			style = downStyle
		}
		graph.DrawCandlestickBottomToTop(&chart, canvas.Point{X: i + 1, Y: height - 2}, scale(l), scale(math.Min(o, cl)), scale(math.Max(o, cl)), scale(h), style)
	}

	periods := []struct {
		period int
		style  lipgloss.Style
	}{{5, ma5Style}, {20, ma20Style}, {60, ma60Style}, {120, ma120Style}}
	start := len(candles) - len(visible)
	for periodIndex, item := range periods {
		if len(maVisibility) > periodIndex && !maVisibility[periodIndex] {
			continue
		}
		ma := domain.MovingAverage(candles, item.period)
		points := make([]canvas.Point, 0, len(visible))
		for i := range visible {
			v := ma[start+i]
			if v == nil {
				continue
			}
			f, _ := v.Float64()
			y := height - 2 - int(math.Round(scale(f)))
			y = max(0, min(height-2, y))
			points = append(points, canvas.Point{X: i + 1, Y: y})
		}
		if len(points) > 1 {
			drawSmoothMALine(&chart, points, item.style, plotWidth, height)
		}
	}

	// Keep the price scale compact and aligned with the plot, similar to a
	// terminal market dashboard. Styling the scale separately prevents it from
	// competing with candle/MA colors.
	labels := make([]string, height)
	for i := range labels {
		labels[i] = ""
	}
	labels[0] = fmt.Sprintf(" %10s", commaNumber(fmt.Sprintf("%.0f", maxPrice)))
	labels[height/2] = fmt.Sprintf(" %10s", commaNumber(fmt.Sprintf("%.0f", (maxPrice+minPrice)/2)))
	labels[height-1] = fmt.Sprintf(" %10s", commaNumber(fmt.Sprintf("%.0f", minPrice)))
	return lipgloss.JoinHorizontal(lipgloss.Top, chart.View(), axisStyle.Render(strings.Join(labels, "\n")))
}

func renderPerformanceLineChart(points []performancePoint, width, height int) string {
	if len(points) == 0 {
		return "성과 그래프 데이터가 없습니다."
	}
	width = max(30, width)
	height = max(10, height)
	plotWidth := max(12, width-16)
	visible := points
	if maxPoints := plotWidth - 1; len(visible) > maxPoints {
		visible = visible[len(visible)-maxPoints:]
	}

	minValue, maxValue := math.MaxFloat64, -math.MaxFloat64
	for _, point := range visible {
		value, _ := point.TotalAssets.Float64()
		minValue = math.Min(minValue, value)
		maxValue = math.Max(maxValue, value)
	}
	if minValue == maxValue {
		padding := math.Max(1, math.Abs(minValue)*.01)
		minValue -= padding
		maxValue += padding
	} else {
		padding := (maxValue - minValue) * .08
		minValue -= padding
		maxValue += padding
	}

	chart := canvas.New(plotWidth, height)
	graph.DrawXYAxis(&chart, canvas.Point{X: 0, Y: height - 1}, axisStyle)
	scaleY := func(value float64) int {
		scaled := (value - minValue) / (maxValue - minValue) * float64(height-2)
		return max(0, min(height-2, height-2-int(math.Round(scaled))))
	}
	chartPoints := make([]canvas.Point, len(visible))
	for i, point := range visible {
		x := 1
		if len(visible) > 1 {
			x += int(math.Round(float64(i) * float64(plotWidth-2) / float64(len(visible)-1)))
		}
		value, _ := point.TotalAssets.Float64()
		chartPoints[i] = canvas.Point{X: x, Y: scaleY(value)}
	}
	if len(chartPoints) == 1 {
		chart.SetRuneWithStyle(chartPoints[0], '●', neutral)
	} else {
		for i := 1; i < len(chartPoints); i++ {
			delta := visible[i].TotalAssets.Sub(visible[i-1].TotalAssets)
			graph.DrawLinePoints(&chart, chartPoints[i-1:i+1], runes.ArcLineStyle, performanceValueStyle(delta))
		}
	}

	labels := make([]string, height)
	labels[0] = fmt.Sprintf(" %12s원", commaNumber(fmt.Sprintf("%.0f", maxValue)))
	labels[height/2] = fmt.Sprintf(" %12s원", commaNumber(fmt.Sprintf("%.0f", (maxValue+minValue)/2)))
	labels[height-1] = fmt.Sprintf(" %12s원", commaNumber(fmt.Sprintf("%.0f", minValue)))
	graphView := lipgloss.JoinHorizontal(lipgloss.Top, chart.View(), axisStyle.Render(strings.Join(labels, "\n")))
	dateRange := visible[0].Date.In(time.Local).Format("2006-01-02")
	if len(visible) > 1 {
		dateRange += "  →  " + visible[len(visible)-1].Date.In(time.Local).Format("2006-01-02")
	}
	return graphView + "\n" + muted.Render(dateRange)
}

// drawSmoothMALine renders an interpolated moving average using line glyphs.
// Catmull-Rom interpolation adds intermediate points and rounded line joins,
// reducing the stair-step effect while keeping the MA visibly line-based.
func drawSmoothMALine(chart *canvas.Model, points []canvas.Point, style lipgloss.Style, width, height int) {
	if len(points) < 2 {
		return
	}
	smooth := make([]canvas.Point, 0, (len(points)-1)*8+1)
	for i := 0; i < len(points)-1; i++ {
		p0, p1 := points[max(0, i-1)], points[i]
		p2, p3 := points[i+1], points[min(len(points)-1, i+2)]
		// More samples make the curve continuous even when the source series
		// has been compressed to the terminal width.
		const samples = 8
		for step := 0; step <= samples; step++ {
			t := float64(step) / samples
			x := float64(p1.X) + (float64(p2.X)-float64(p1.X))*t
			y := catmullRom(float64(p0.Y), float64(p1.Y), float64(p2.Y), float64(p3.Y), t)
			p := canvas.Point{X: int(math.Round(x)), Y: max(0, min(height-2, int(math.Round(y))))}
			if len(smooth) == 0 || smooth[len(smooth)-1] != p {
				smooth = append(smooth, p)
			}
		}
	}
	if len(smooth) > 1 {
		graph.DrawLinePoints(chart, smooth, runes.ArcLineStyle, style)
	}
}

func catmullRom(p0, p1, p2, p3, t float64) float64 {
	t2, t3 := t*t, t*t*t
	return 0.5 * ((2 * p1) + (-p0+p2)*t + (2*p0-5*p1+4*p2-p3)*t2 + (-p0+3*p1-3*p2+p3)*t3)
}
