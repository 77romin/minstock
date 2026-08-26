package tui

import (
	"fmt"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/NimbleMarkets/ntcharts/v2/canvas"
	"github.com/NimbleMarkets/ntcharts/v2/canvas/graph"
	"github.com/NimbleMarkets/ntcharts/v2/canvas/runes"
	"github.com/mink/stock-min-tui/internal/domain"
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

func renderChart(candles []domain.Candle, width, height int) string {
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
	for _, item := range periods {
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
			graph.DrawLinePoints(&chart, points, runes.ThinLineStyle, item.style)
		}
	}

	labels := fmt.Sprintf(" %10.0f\n%s\n %10.0f", maxPrice, strings.Repeat("\n", max(0, height-3)), minPrice)
	return lipgloss.JoinHorizontal(lipgloss.Top, chart.View(), labels)
}
