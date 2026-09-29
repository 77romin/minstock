package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/shopspring/decimal"
)

var (
	defaultDailyChangeThreshold = decimal.NewFromInt(5)
	defaultHoldingLossThreshold = decimal.NewFromInt(-10)
	defaultAssetWeightThreshold = decimal.NewFromInt(30)
)

type AlertReport struct {
	Rules          []domain.PriceAlertRule
	Events         []domain.AlertEvent
	Unacknowledged int
}

func (s *Service) AlertReport(ctx context.Context) (AlertReport, error) {
	rules, rulesErr := s.repo.ListPriceAlertRules(ctx)
	events, eventsErr := s.repo.ListAlertEvents(ctx, 100)
	report := AlertReport{Rules: rules, Events: events}
	for _, event := range events {
		if event.AcknowledgedAt.IsZero() {
			report.Unacknowledged++
		}
	}
	return report, errors.Join(rulesErr, eventsErr)
}

func (s *Service) SavePriceAlert(ctx context.Context, symbol domain.Symbol, targetPrice, currentPrice decimal.Decimal) error {
	if !targetPrice.IsPositive() {
		return errors.New("목표가는 0보다 커야 합니다")
	}
	direction := "ABOVE"
	if currentPrice.IsPositive() && targetPrice.LessThan(currentPrice) {
		direction = "BELOW"
	}
	return s.repo.SavePriceAlertRule(ctx, domain.PriceAlertRule{
		Symbol: symbol, TargetPrice: targetPrice, Direction: direction, Enabled: true, CreatedAt: time.Now(),
	})
}

func (s *Service) DeletePriceAlert(ctx context.Context, id int64) error {
	return s.repo.DeletePriceAlertRule(ctx, id)
}

func (s *Service) AcknowledgeAlert(ctx context.Context, id int64) error {
	return s.repo.AcknowledgeAlertEvent(ctx, id)
}

func (s *Service) AcknowledgeAllAlerts(ctx context.Context) error {
	return s.repo.AcknowledgeAllAlertEvents(ctx)
}

func (s *Service) EvaluateAlerts(ctx context.Context, snapshot Snapshot) error {
	now := time.Now()
	rules, err := s.repo.ListPriceAlertRules(ctx)
	if err != nil {
		return err
	}
	quotes := snapshot.Quotes
	positions := make(map[string][]domain.Position)
	symbols := make(map[string]domain.Symbol)
	values := make(map[string]decimal.Decimal)
	totalAssets := decimal.Zero
	for _, balance := range snapshot.Balances {
		value := balance.ValueTotalKRW
		cash := balance.CashKRW
		if value.IsZero() && balance.Currency == domain.KRW {
			value = balance.ValueTotal
		}
		if cash.IsZero() && balance.Currency == domain.KRW {
			cash = balance.Cash
		}
		if balance.ExchangeRate.IsPositive() {
			if value.IsZero() {
				value = balance.ValueTotal.Mul(balance.ExchangeRate)
			}
			if cash.IsZero() {
				cash = balance.Cash.Mul(balance.ExchangeRate)
			}
		}
		totalAssets = totalAssets.Add(value).Add(cash)
	}
	for _, position := range snapshot.Positions {
		key := position.Symbol.Key()
		positions[key] = append(positions[key], position)
		symbols[key] = position.Symbol
		value := position.MarketValueKRW
		if value.IsZero() && position.Symbol.Currency == domain.KRW {
			value = position.MarketValue
		}
		if value.IsZero() && position.ExchangeRate.IsPositive() {
			value = position.MarketValue.Mul(position.ExchangeRate)
		}
		values[key] = values[key].Add(value)
	}

	var events []domain.AlertEvent
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		price := quotePrice(quotes[rule.Symbol.Key()])
		if price.IsZero() {
			for _, position := range positions[rule.Symbol.Key()] {
				if position.CurrentPrice.IsPositive() {
					price = position.CurrentPrice
					break
				}
			}
		}
		reached := rule.Direction == "BELOW" && price.IsPositive() && price.LessThanOrEqual(rule.TargetPrice)
		if rule.Direction != "BELOW" {
			reached = price.GreaterThanOrEqual(rule.TargetPrice)
		}
		if reached {
			direction := "이상"
			if rule.Direction == "BELOW" {
				direction = "이하"
			}
			events = append(events, newAlertEvent(now, domain.AlertTargetPrice, "주의", symbolLabel(rule.Symbol), fmt.Sprintf("현재가 %s · 목표가 %s %s 도달", price.StringFixed(2), rule.TargetPrice.StringFixed(2), direction), price, rule.TargetPrice, fmt.Sprintf("rule:%d:%s", rule.ID, rule.TargetPrice.String())))
		}
	}

	for key, symbol := range symbols {
		label := symbolLabel(symbol)
		if quote, ok := quotes[key]; ok && quote.ChangeRate.Abs().GreaterThanOrEqual(defaultDailyChangeThreshold) {
			events = append(events, newAlertEvent(now, domain.AlertDailyChange, "주의", label, fmt.Sprintf("일간 변동률 %s%%", signedDecimal(quote.ChangeRate)), quote.ChangeRate, defaultDailyChangeThreshold, key))
		}
		worstLoss := decimal.Zero
		for _, position := range positions[key] {
			if position.ProfitRate.LessThan(worstLoss) {
				worstLoss = position.ProfitRate
			}
		}
		if worstLoss.LessThanOrEqual(defaultHoldingLossThreshold) {
			events = append(events, newAlertEvent(now, domain.AlertHoldingLoss, "위험", label, fmt.Sprintf("보유 손실률 %s%%", worstLoss.StringFixed(2)), worstLoss, defaultHoldingLossThreshold, key))
		}
		if totalAssets.IsPositive() {
			weight := values[key].Div(totalAssets).Mul(decimal.NewFromInt(100))
			if weight.GreaterThanOrEqual(defaultAssetWeightThreshold) {
				events = append(events, newAlertEvent(now, domain.AlertAssetWeight, "주의", label, fmt.Sprintf("자산 비중 %s%%", weight.StringFixed(2)), weight, defaultAssetWeightThreshold, key))
			}
		}
	}

	for _, status := range snapshot.Statuses {
		count := s.connectionFailureCount(status)
		if count >= 3 {
			message := strings.TrimSpace(status.Message)
			if message == "" {
				message = "API 연결 실패가 3회 연속 발생했습니다"
			}
			events = append(events, newAlertEvent(now, domain.AlertConnectionFailed, "위험", string(status.Broker), message, decimal.NewFromInt(int64(count)), decimal.NewFromInt(3), string(status.Broker)))
		}
	}
	for _, event := range events {
		if err := s.repo.SaveAlertEvent(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) connectionFailureCount(status domain.BrokerStatus) int {
	s.alertMu.Lock()
	defer s.alertMu.Unlock()
	if s.alertFails == nil {
		s.alertFails = make(map[domain.BrokerID]int)
	}
	if status.Connected {
		s.alertFails[status.Broker] = 0
	} else {
		s.alertFails[status.Broker]++
	}
	return s.alertFails[status.Broker]
}

func newAlertEvent(now time.Time, kind domain.AlertKind, severity, subject, message string, value, threshold decimal.Decimal, subjectKey string) domain.AlertEvent {
	return domain.AlertEvent{
		Kind: kind, Severity: severity, Subject: subject, Message: message, Value: value, Threshold: threshold,
		DedupeKey: fmt.Sprintf("%s:%s:%s", kind, subjectKey, now.Format("2006-01-02")), OccurredAt: now,
	}
}

func quotePrice(quote domain.Quote) decimal.Decimal {
	if quote.Price.IsPositive() {
		return quote.Price
	}
	return decimal.Zero
}

func symbolLabel(symbol domain.Symbol) string {
	if ticker := strings.TrimSpace(symbol.Ticker); ticker != "" {
		return strings.ToUpper(ticker)
	}
	return strings.ToUpper(strings.TrimSpace(symbol.Code))
}

func signedDecimal(value decimal.Decimal) string {
	if value.IsPositive() {
		return "+" + value.StringFixed(2)
	}
	return value.StringFixed(2)
}
