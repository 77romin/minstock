package domain

import (
	"github.com/shopspring/decimal"
	"testing"
	"time"
)

func scannerMinutes() ([]Candle, time.Time) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	var candles []Candle
	for i := 0; i < 11; i++ {
		start := now.Add(time.Duration(i-11) * time.Minute)
		price, volume := int64(100), int64(10)
		if i > 5 {
			price, volume = 103, 30
		}
		candles = append(candles, Candle{Interval: Interval1Min, OpenTime: start, CloseTime: start.Add(time.Minute), Close: decimal.NewFromInt(price), Volume: volume, Complete: true})
	}
	return candles, now
}

func TestSurgeMomentumUsesCompletedAdjacentWindows(t *testing.T) {
	candles, now := scannerMinutes()
	candles = append(candles, Candle{Interval: Interval1Min, OpenTime: now, CloseTime: now.Add(time.Minute), Close: decimal.NewFromInt(10000), Volume: 999999, Complete: true})
	rate, ratio, asOf, err := SurgeMomentum(candles, now)
	if err != nil || !rate.Equal(decimal.NewFromInt(3)) || !ratio.Equal(decimal.NewFromInt(3)) || !asOf.Equal(now) {
		t.Fatalf("rate=%s ratio=%s asOf=%s err=%v", rate, ratio, asOf, err)
	}
}

func TestSurgeMomentumRejectsMissingStaleAndSessionCrossingData(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]Candle) []Candle
		delay  time.Duration
	}{
		{"too few", func(c []Candle) []Candle { return c[1:] }, 0},
		{"gap", func(c []Candle) []Candle { c[3].OpenTime = c[3].OpenTime.Add(-time.Minute); return c }, 0},
		{"zero volume", func(c []Candle) []Candle {
			for i := 1; i <= 5; i++ {
				c[i].Volume = 0
			}
			return c
		}, 0},
		{"stale", func(c []Candle) []Candle { return c }, 4 * time.Minute},
		{"overnight", func(c []Candle) []Candle { c[0].OpenTime = c[0].OpenTime.AddDate(0, 0, -1); return c }, 0},
		{"incomplete", func(c []Candle) []Candle { c[10].Complete = false; return c }, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candles, now := scannerMinutes()
			if _, _, _, err := SurgeMomentum(test.mutate(candles), now.Add(test.delay)); err == nil {
				t.Fatal("invalid series accepted")
			}
		})
	}
}
