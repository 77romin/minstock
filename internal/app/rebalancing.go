package app

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/shopspring/decimal"
)

type AllocationRow struct {
	Target                               domain.AllocationTarget
	ValueKRW, CurrentPercent, Difference decimal.Decimal
	Status                               string
}

type AllocationReport struct {
	Scope                 string
	Rows                  []AllocationRow
	TotalKRW, TargetTotal decimal.Decimal
	KRMarketPercent       decimal.Decimal
	USMarketPercent       decimal.Decimal
	OtherMarketPercent    decimal.Decimal
	KRWPercent            decimal.Decimal
	USDPercent            decimal.Decimal
}

func (s *Service) AllocationReport(ctx context.Context, scope string, snapshot Snapshot) (AllocationReport, error) {
	targets, err := s.repo.ListAllocationTargets(ctx, scope)
	if err != nil {
		return AllocationReport{}, err
	}
	values := map[string]decimal.Decimal{}
	symbols := map[string]domain.Symbol{}
	totalAssets := decimal.Zero
	krMarket, usMarket, otherMarket, krwValue, usdValue := decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero
	match := func(b domain.BrokerID) bool { return scope == "ALL" || string(b) == scope }
	for _, p := range snapshot.Positions {
		if !match(p.Broker) {
			continue
		}
		key := p.Symbol.Key()
		value := p.MarketValueKRW
		if value.IsZero() && p.Symbol.Currency == domain.KRW {
			value = p.MarketValue
		}
		if value.IsZero() && p.ExchangeRate.IsPositive() {
			value = p.MarketValue.Mul(p.ExchangeRate)
		}
		values[key], symbols[key] = values[key].Add(value), p.Symbol
		if p.Symbol.Market == domain.MarketUS {
			usMarket = usMarket.Add(value)
		} else {
			krMarket = krMarket.Add(value)
		}
	}
	for _, b := range snapshot.Balances {
		if !match(b.Broker) {
			continue
		}
		cash := b.CashKRW
		value := b.ValueTotalKRW
		if cash.IsZero() && b.Currency == domain.KRW {
			cash = b.Cash
		}
		if value.IsZero() && b.Currency == domain.KRW {
			value = b.ValueTotal
		}
		if cash.IsZero() && b.ExchangeRate.IsPositive() {
			cash = b.Cash.Mul(b.ExchangeRate)
		}
		if value.IsZero() && b.ExchangeRate.IsPositive() {
			value = b.ValueTotal.Mul(b.ExchangeRate)
		}
		values["CASH"] = values["CASH"].Add(cash)
		otherMarket = otherMarket.Add(cash)
		totalAssets = totalAssets.Add(value).Add(cash)
		if b.Currency == domain.USD {
			usdValue = usdValue.Add(value).Add(cash)
		} else {
			krwValue = krwValue.Add(value).Add(cash)
		}
	}
	positionTotal := decimal.Zero
	for key, value := range values {
		if key != "CASH" {
			positionTotal = positionTotal.Add(value)
		}
	}
	if residual := totalAssets.Sub(values["CASH"]).Sub(positionTotal); residual.IsPositive() {
		values["OTHER"] = residual
		symbols["OTHER"] = domain.Symbol{Code: "OTHER", Ticker: "OTHER", Name: "기타 자산", Currency: domain.KRW}
		otherMarket = otherMarket.Add(residual)
	}
	targetMap := map[string]domain.AllocationTarget{}
	for _, t := range targets {
		key := allocationTargetKey(t)
		targetMap[key] = t
	}
	for key, symbol := range symbols {
		if _, ok := targetMap[key]; !ok {
			targetMap[key] = domain.AllocationTarget{Scope: scope, Symbol: symbol}
		}
	}
	if _, ok := targetMap["CASH"]; !ok {
		targetMap["CASH"] = domain.AllocationTarget{Scope: scope, Cash: true, Symbol: domain.Symbol{Name: "현금", Currency: domain.KRW}}
	}
	report := AllocationReport{Scope: scope, TotalKRW: totalAssets}
	if totalAssets.IsPositive() {
		hundred := decimal.NewFromInt(100)
		report.KRMarketPercent = krMarket.Div(totalAssets).Mul(hundred)
		report.USMarketPercent = usMarket.Div(totalAssets).Mul(hundred)
		report.OtherMarketPercent = otherMarket.Div(totalAssets).Mul(hundred)
		report.KRWPercent = krwValue.Div(totalAssets).Mul(hundred)
		report.USDPercent = usdValue.Div(totalAssets).Mul(hundred)
	}
	for key, t := range targetMap {
		current := decimal.Zero
		if report.TotalKRW.IsPositive() {
			current = values[key].Div(report.TotalKRW).Mul(decimal.NewFromInt(100))
		}
		diff := current.Sub(t.TargetPercent)
		status := "적정"
		if diff.GreaterThan(decimal.NewFromInt(5)) {
			status = "집중"
		} else if diff.GreaterThan(decimal.NewFromInt(2)) {
			status = "초과"
		} else if diff.LessThan(decimal.NewFromInt(-2)) {
			status = "부족"
		}
		report.Rows = append(report.Rows, AllocationRow{Target: t, ValueKRW: values[key], CurrentPercent: current, Difference: diff, Status: status})
		report.TargetTotal = report.TargetTotal.Add(t.TargetPercent)
	}
	sort.Slice(report.Rows, func(i, j int) bool {
		if report.Rows[i].Target.Cash {
			return false
		}
		if report.Rows[j].Target.Cash {
			return true
		}
		return strings.ToUpper(report.Rows[i].Target.Symbol.Ticker) < strings.ToUpper(report.Rows[j].Target.Symbol.Ticker)
	})
	return report, nil
}

func allocationTargetKey(target domain.AllocationTarget) string {
	if target.Cash {
		return "CASH"
	}
	if target.Symbol.Code == "OTHER" {
		return "OTHER"
	}
	return target.Symbol.Key()
}

func (s *Service) SaveAllocationTargets(ctx context.Context, scope string, targets []domain.AllocationTarget) error {
	total := decimal.Zero
	for _, t := range targets {
		if t.TargetPercent.IsNegative() || t.TargetPercent.GreaterThan(decimal.NewFromInt(100)) {
			return errors.New("목표 비중은 0~100%여야 합니다")
		}
		total = total.Add(t.TargetPercent)
	}
	if !total.Equal(decimal.NewFromInt(100)) {
		return errors.New("목표 비중 합계가 100%여야 합니다")
	}
	return s.repo.ReplaceAllocationTargets(ctx, scope, targets)
}
