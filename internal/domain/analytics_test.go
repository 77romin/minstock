package domain

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestMovingAverage(t *testing.T) {
	candles := make([]Candle, 6)
	for i := range candles {
		candles[i].Close = decimal.NewFromInt(int64(i + 1))
	}
	got := MovingAverage(candles, 5)
	if got[3] != nil {
		t.Fatal("MA5 must be nil before five candles")
	}
	if got[4] == nil || !got[4].Equal(decimal.NewFromInt(3)) {
		t.Fatalf("MA5 at index 4 = %v, want 3", got[4])
	}
	if got[5] == nil || !got[5].Equal(decimal.NewFromInt(4)) {
		t.Fatalf("MA5 at index 5 = %v, want 4", got[5])
	}
}

func TestAggregateCandles(t *testing.T) {
	base := time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
	input := []Candle{
		{OpenTime: base, Open: decimal.NewFromInt(10), High: decimal.NewFromInt(12), Low: decimal.NewFromInt(9), Close: decimal.NewFromInt(11), Volume: 10, Complete: true},
		{OpenTime: base.Add(time.Minute), Open: decimal.NewFromInt(11), High: decimal.NewFromInt(15), Low: decimal.NewFromInt(10), Close: decimal.NewFromInt(14), Volume: 20, Complete: true},
	}
	got := AggregateCandles(input, Interval5Min, 5*time.Minute)
	if len(got) != 1 || got[0].Volume != 30 || !got[0].Close.Equal(decimal.NewFromInt(14)) || !got[0].High.Equal(decimal.NewFromInt(15)) {
		t.Fatalf("unexpected aggregate: %#v", got)
	}
}

func TestScoreSurge(t *testing.T) {
	q := Quote{Symbol: Symbol{Code: "012340"}, Price: decimal.NewFromInt(118), High: decimal.NewFromInt(119), ChangeRate: decimal.NewFromInt(18), Turnover: decimal.NewFromInt(20_000_000_000), TradePower: decimal.NewFromInt(150), MarketTime: time.Now()}
	report, ok := ScoreSurge(q, decimal.NewFromInt(4), decimal.NewFromInt(4), DefaultSurgePolicy())
	if !ok || report.Score <= 0 || len(report.Reasons) == 0 {
		t.Fatalf("expected surge report, got ok=%v report=%#v", ok, report)
	}
}
