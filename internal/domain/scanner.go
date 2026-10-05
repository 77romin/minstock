package domain

import (
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// SurgeMomentum compares two adjacent complete five-minute windows. The
// eleventh close is the price baseline; all eleven minutes must be contiguous
// in the same KRX session. Incomplete, stale and zero-volume data are rejected.
func SurgeMomentum(input []Candle, now time.Time) (rate, volumeRatio decimal.Decimal, asOf time.Time, err error) {
	var candles []Candle
	for _, c := range input {
		if c.Interval == Interval1Min && c.Complete && !c.CloseTime.After(now) {
			candles = append(candles, c)
		}
	}
	sort.Slice(candles, func(i, j int) bool { return candles[i].OpenTime.Before(candles[j].OpenTime) })
	if len(candles) < 11 {
		return rate, volumeRatio, asOf, fmt.Errorf("완료된 1분봉 11개 필요")
	}
	candles = candles[len(candles)-11:]
	loc := time.FixedZone("KST", 9*60*60)
	sessionDate := candles[10].OpenTime.In(loc).Format("2006-01-02")
	for i, c := range candles {
		t := c.OpenTime.In(loc)
		if t.Format("2006-01-02") != sessionDate || t.Hour() < 9 || t.Hour() > 15 || (t.Hour() == 15 && t.Minute() >= 30) || !c.Close.IsPositive() || c.Volume < 0 || c.CloseTime.Sub(c.OpenTime) != time.Minute {
			return rate, volumeRatio, asOf, fmt.Errorf("정규장 분봉 데이터 부족")
		}
		if i > 0 && c.OpenTime.Sub(candles[i-1].OpenTime) != time.Minute {
			return rate, volumeRatio, asOf, fmt.Errorf("분봉 시간 간격 누락")
		}
	}
	asOf = candles[10].CloseTime
	if now.Sub(asOf) > 3*time.Minute {
		return rate, volumeRatio, asOf, fmt.Errorf("최근 분봉 지연 또는 장 마감")
	}
	previous, recent := decimal.Zero, decimal.Zero
	for i := 1; i <= 5; i++ {
		previous = previous.Add(decimal.NewFromInt(candles[i].Volume))
	}
	for i := 6; i <= 10; i++ {
		recent = recent.Add(decimal.NewFromInt(candles[i].Volume))
	}
	if !previous.IsPositive() {
		return rate, volumeRatio, asOf, fmt.Errorf("직전 5분 거래량 없음")
	}
	rate = candles[10].Close.Sub(candles[5].Close).Div(candles[5].Close).Mul(decimal.NewFromInt(100))
	return rate, recent.Div(previous), asOf, nil
}
