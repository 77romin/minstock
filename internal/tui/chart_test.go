package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/shopspring/decimal"
)

func TestRenderChart(t *testing.T) {
	candles := make([]domain.Candle, 130)
	for i := range candles {
		price := decimal.NewFromInt(int64(100 + i))
		candles[i] = domain.Candle{OpenTime: time.Now().Add(time.Duration(i) * time.Minute), Open: price, High: price.Add(decimal.NewFromInt(3)), Low: price.Sub(decimal.NewFromInt(2)), Close: price.Add(decimal.NewFromInt(1))}
	}
	got := renderChart(candles, 80, 20)
	if strings.TrimSpace(got) == "" || !strings.Contains(got, "235") {
		t.Fatalf("chart was not rendered: %q", got)
	}
}
