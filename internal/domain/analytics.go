package domain

import (
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// MovingAverage returns nil until enough complete closing prices exist.
func MovingAverage(candles []Candle, period int) []*decimal.Decimal {
	out := make([]*decimal.Decimal, len(candles))
	if period <= 0 {
		return out
	}
	window := decimal.Zero
	for i, candle := range candles {
		window = window.Add(candle.Close)
		if i >= period {
			window = window.Sub(candles[i-period].Close)
		}
		if i >= period-1 {
			v := window.Div(decimal.NewFromInt(int64(period)))
			out[i] = &v
		}
	}
	return out
}

// AggregateCandles converts smaller candles into a larger, fixed duration.
func AggregateCandles(input []Candle, target CandleInterval, duration time.Duration) []Candle {
	if len(input) == 0 || duration <= 0 {
		return nil
	}
	sorted := append([]Candle(nil), input...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].OpenTime.Before(sorted[j].OpenTime) })

	var out []Candle
	for _, source := range sorted {
		bucket := source.OpenTime.Truncate(duration)
		if len(out) == 0 || !out[len(out)-1].OpenTime.Equal(bucket) {
			c := source
			c.Interval = target
			c.OpenTime = bucket
			c.CloseTime = bucket.Add(duration)
			out = append(out, c)
			continue
		}
		current := &out[len(out)-1]
		if source.High.GreaterThan(current.High) {
			current.High = source.High
		}
		if source.Low.LessThan(current.Low) {
			current.Low = source.Low
		}
		current.Close = source.Close
		current.Volume += source.Volume
		current.Turnover = current.Turnover.Add(source.Turnover)
		current.Complete = current.Complete && source.Complete
	}
	return out
}

type SurgePolicy struct {
	MinChangeRate     decimal.Decimal
	MinFiveMinuteRate decimal.Decimal
	MinVolumeRatio    decimal.Decimal
	MinTurnover       decimal.Decimal
}

func DefaultSurgePolicy() SurgePolicy {
	return SurgePolicy{
		MinChangeRate:     decimal.NewFromInt(5),
		MinFiveMinuteRate: decimal.NewFromInt(2),
		MinVolumeRatio:    decimal.NewFromInt(2),
		MinTurnover:       decimal.NewFromInt(3_000_000_000),
	}
}

func ScoreSurge(q Quote, fiveMinuteRate, volumeRatio decimal.Decimal, policy SurgePolicy) (SurgeReport, bool) {
	highDistance := decimal.Zero
	if q.High.IsPositive() {
		highDistance = q.High.Sub(q.Price).Div(q.High).Mul(decimal.NewFromInt(100))
	}
	if q.ChangeRate.LessThan(policy.MinChangeRate) ||
		fiveMinuteRate.LessThan(policy.MinFiveMinuteRate) ||
		volumeRatio.LessThan(policy.MinVolumeRatio) || q.Turnover.LessThan(policy.MinTurnover) {
		return SurgeReport{}, false
	}

	clamp := func(v decimal.Decimal, ceiling float64) float64 {
		f, _ := v.Float64()
		if f < 0 {
			return 0
		}
		if f > ceiling {
			return ceiling
		}
		return f
	}
	score := int(clamp(fiveMinuteRate.Div(decimal.NewFromInt(5)), 1)*30 +
		clamp(volumeRatio.Div(decimal.NewFromInt(5)), 1)*25 +
		clamp(q.Turnover.Div(decimal.NewFromInt(10_000_000_000)), 1)*20 +
		clamp(decimal.NewFromInt(1).Sub(highDistance.Div(decimal.NewFromInt(5))), 1)*15 +
		clamp(q.TradePower.Div(decimal.NewFromInt(150)), 1)*10)

	report := SurgeReport{
		Symbol: q.Symbol, Score: score, ChangeRate: q.ChangeRate,
		FiveMinuteRate: fiveMinuteRate, VolumeRatio: volumeRatio,
		Turnover: q.Turnover, HighDistance: highDistance,
		TradePower: q.TradePower, AsOf: q.MarketTime, Provider: q.Provider,
		Reasons: []string{
			fmt.Sprintf("5분 상승률 %s%%", fiveMinuteRate.StringFixed(2)),
			fmt.Sprintf("거래량 %s배", volumeRatio.StringFixed(1)),
		},
	}
	if highDistance.LessThanOrEqual(decimal.NewFromFloat(0.5)) {
		report.Reasons = append(report.Reasons, "당일 고점 돌파 구간")
	}
	if q.ChangeRate.GreaterThan(decimal.NewFromInt(15)) {
		report.Warnings = append(report.Warnings, "단기 변동성이 매우 높음")
	}
	return report, true
}
