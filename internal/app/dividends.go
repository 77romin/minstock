package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/ports"
	"github.com/shopspring/decimal"
)

const (
	dividendCacheTTL      = 24 * time.Hour
	dividendFailureTTL    = 15 * time.Minute
	dividendFailurePrefix = "dividend-failure:v1:"
)

var usDividendTaxRate = decimal.RequireFromString("0.15")

type DividendHolding struct {
	Symbol         string
	Name           string
	Quantity       decimal.Decimal
	RecentAmount   decimal.Decimal
	RecentDate     time.Time
	AnnualPerShare decimal.Decimal
	GrossAnnual    decimal.Decimal
	NetAnnual      decimal.Decimal
	NetAnnualKRW   decimal.Decimal
}

type DividendMonth struct {
	Month  time.Time
	Gross  decimal.Decimal
	Net    decimal.Decimal
	NetKRW decimal.Decimal
}

type DividendReport struct {
	Holdings     []DividendHolding
	Months       []DividendMonth
	GrossAnnual  decimal.Decimal
	NetAnnual    decimal.Decimal
	NetAnnualKRW decimal.Decimal
	Source       string
	Freshness    domain.Freshness
	AsOf         time.Time
	Warnings     []string
}

type dividendCall struct {
	done      chan struct{}
	events    []domain.DividendEvent
	freshness domain.Freshness
	asOf      time.Time
	err       error
}

type dividendFailure struct {
	Message string    `json:"message"`
	RetryAt time.Time `json:"retry_at"`
}

func (s *Service) SetDividendProvider(provider ports.DividendProvider) {
	s.dividends = provider
}

func (s *Service) DividendPortfolio(ctx context.Context, positions []domain.Position, fx domain.FXRate) DividendReport {
	type heldPosition struct {
		name      string
		positions []domain.Position
	}
	bySymbol := make(map[string]*heldPosition)
	for _, position := range positions {
		if position.Symbol.Currency != domain.USD || !position.Quantity.IsPositive() {
			continue
		}
		symbol := strings.ToUpper(strings.TrimSpace(position.Symbol.Ticker))
		if symbol == "" {
			symbol = strings.ToUpper(strings.TrimSpace(position.Symbol.Code))
		}
		if symbol == "" {
			continue
		}
		held := bySymbol[symbol]
		if held == nil {
			held = &heldPosition{name: position.Symbol.Name}
			bySymbol[symbol] = held
		}
		held.positions = append(held.positions, position)
	}

	now := time.Now()
	report := DividendReport{}
	if s.dividends != nil {
		report.Source = s.dividends.DividendSource()
	} else {
		report.Source = "미설정"
	}
	monthTotals := make(map[string]DividendMonth)
	symbols := make([]string, 0, len(bySymbol))
	for symbol := range bySymbol {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	for _, symbol := range symbols {
		held := bySymbol[symbol]
		events, freshness, asOf, err := s.loadDividendEvents(ctx, symbol)
		if err != nil {
			report.Warnings = append(report.Warnings, symbol+": "+err.Error())
			continue
		}
		if report.Source == "미설정" && len(events) > 0 && strings.TrimSpace(events[0].Provider) != "" {
			report.Source = events[0].Provider + " (현재 키 미설정)"
		}
		report.Freshness = mergeDividendFreshness(report.Freshness, freshness)
		if report.AsOf.IsZero() || (!asOf.IsZero() && asOf.Before(report.AsOf)) {
			report.AsOf = asOf
		}
		trailingStart := now.AddDate(-1, 0, 0)
		annualPerShare, recentAmount := decimal.Zero, decimal.Zero
		var recentDate time.Time
		for _, event := range events {
			date := dividendEventDate(event)
			if date.After(now) || date.Before(trailingStart) {
				continue
			}
			annualPerShare = annualPerShare.Add(event.Amount)
			if recentDate.IsZero() || date.After(recentDate) {
				recentDate, recentAmount = date, event.Amount
			}
		}
		quantity, gross, net, netKRW := decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero
		for _, position := range held.positions {
			quantity = quantity.Add(position.Quantity)
			positionGross := annualPerShare.Mul(position.Quantity)
			positionNet := positionGross.Mul(decimal.NewFromInt(1).Sub(usDividendTaxRate))
			gross, net = gross.Add(positionGross), net.Add(positionNet)
			if rate := dividendPositionRate(position, fx); rate.IsPositive() {
				netKRW = netKRW.Add(positionNet.Mul(rate))
			}
		}
		report.Holdings = append(report.Holdings, DividendHolding{
			Symbol: symbol, Name: held.name, Quantity: quantity, RecentAmount: recentAmount,
			RecentDate: recentDate, AnnualPerShare: annualPerShare, GrossAnnual: gross,
			NetAnnual: net, NetAnnualKRW: netKRW,
		})
		report.GrossAnnual, report.NetAnnual = report.GrossAnnual.Add(gross), report.NetAnnual.Add(net)
		report.NetAnnualKRW = report.NetAnnualKRW.Add(netKRW)
		addDividendMonths(monthTotals, events, held.positions, fx, now)
	}
	for _, month := range monthTotals {
		report.Months = append(report.Months, month)
	}
	sort.Slice(report.Months, func(i, j int) bool { return report.Months[i].Month.Before(report.Months[j].Month) })
	return report
}

func (s *Service) loadDividendEvents(ctx context.Context, symbol string) ([]domain.DividendEvent, domain.Freshness, time.Time, error) {
	key := "dividends:v1:" + symbol
	payload, updatedAt, cacheErr := s.repo.LoadCache(ctx, key)
	var cached []domain.DividendEvent
	cacheValid := false
	if cacheErr == nil {
		cacheValid = json.Unmarshal(payload, &cached) == nil
		if cacheValid && time.Since(updatedAt) < dividendCacheTTL {
			return cached, domain.FreshCached, updatedAt, nil
		}
	}
	if s.dividends == nil {
		if cacheValid {
			return cached, domain.FreshCached, updatedAt, nil
		}
		return nil, domain.FreshCached, time.Time{}, fmt.Errorf("배당 API 키가 없습니다: minstock setup dividend")
	}
	if failure, retryAt, ok := s.recentDividendFailure(ctx, symbol); ok {
		if cacheValid {
			return cached, domain.FreshCached, updatedAt, nil
		}
		remaining := time.Until(retryAt)
		return nil, "", retryAt, fmt.Errorf("최근 조회 실패: %s (약 %d분 후 재시도)", failure, max(1, int(remaining.Minutes())+1))
	}
	return s.fetchDividendEvents(ctx, symbol, key, cached, updatedAt, cacheValid)
}

func (s *Service) fetchDividendEvents(ctx context.Context, symbol, cacheKey string, cached []domain.DividendEvent, updatedAt time.Time, cacheValid bool) ([]domain.DividendEvent, domain.Freshness, time.Time, error) {
	s.dividendMu.Lock()
	if s.dividendRun == nil {
		s.dividendRun = make(map[string]*dividendCall)
	}
	if running := s.dividendRun[symbol]; running != nil {
		s.dividendMu.Unlock()
		select {
		case <-running.done:
			return running.events, running.freshness, running.asOf, running.err
		case <-ctx.Done():
			return nil, "", time.Time{}, ctx.Err()
		}
	}
	call := &dividendCall{done: make(chan struct{})}
	s.dividendRun[symbol] = call
	s.dividendMu.Unlock()

	defer func() {
		s.dividendMu.Lock()
		delete(s.dividendRun, symbol)
		close(call.done)
		s.dividendMu.Unlock()
	}()
	events, err := s.dividends.Dividends(ctx, symbol)
	if err != nil {
		call.err = err
		s.saveDividendFailure(ctx, symbol, err)
		if cacheValid {
			call.events, call.freshness, call.asOf = cached, domain.FreshCached, updatedAt
			call.err = nil
			return cached, domain.FreshCached, updatedAt, nil
		}
		return nil, "", time.Time{}, err
	}
	payload, err := json.Marshal(events)
	if err == nil {
		err = s.repo.SaveCache(ctx, cacheKey, payload)
	}
	if err != nil {
		call.err = err
		return nil, "", time.Time{}, err
	}
	call.events, call.freshness, call.asOf = events, domain.FreshLive, time.Now()
	return call.events, domain.FreshLive, call.asOf, nil
}

func (s *Service) recentDividendFailure(ctx context.Context, symbol string) (string, time.Time, bool) {
	payload, updatedAt, err := s.repo.LoadCache(ctx, dividendFailurePrefix+symbol)
	if err != nil {
		return "", time.Time{}, false
	}
	var failure dividendFailure
	if json.Unmarshal(payload, &failure) != nil || strings.TrimSpace(failure.Message) == "" {
		return "", time.Time{}, false
	}
	if failure.RetryAt.IsZero() {
		failure.RetryAt = updatedAt.Add(dividendFailureTTL)
		if strings.Contains(failure.Message, "일일 호출 한도") || strings.Contains(strings.ToLower(failure.Message), "daily rate limit") {
			failure.RetryAt = updatedAt.Add(24 * time.Hour)
		}
	}
	if !time.Now().Before(failure.RetryAt) {
		return "", time.Time{}, false
	}
	return failure.Message, failure.RetryAt, true
}

func (s *Service) saveDividendFailure(ctx context.Context, symbol string, loadErr error) {
	retryAt := time.Now().Add(dividendFailureTTL)
	message := loadErr.Error()
	lower := strings.ToLower(message)
	if strings.Contains(message, "일일 호출 한도") || strings.Contains(lower, "daily rate limit") || strings.Contains(lower, "25 requests") {
		retryAt = time.Now().Add(24 * time.Hour)
	}
	payload, err := json.Marshal(dividendFailure{Message: message, RetryAt: retryAt})
	if err == nil {
		_ = s.repo.SaveCache(ctx, dividendFailurePrefix+symbol, payload)
	}
}

func dividendEventDate(event domain.DividendEvent) time.Time {
	if !event.PaymentDate.IsZero() {
		return event.PaymentDate
	}
	return event.ExDate
}

func dividendPositionRate(position domain.Position, fx domain.FXRate) decimal.Decimal {
	if position.ExchangeRate.IsPositive() {
		return position.ExchangeRate
	}
	if fx.Provider == position.Broker {
		return fx.Rate
	}
	return decimal.Zero
}

func mergeDividendFreshness(current, next domain.Freshness) domain.Freshness {
	if current == "" {
		return next
	}
	if current == next {
		return current
	}
	return domain.FreshMixed
}

func addDividendMonths(months map[string]DividendMonth, events []domain.DividendEvent, positions []domain.Position, fx domain.FXRate, now time.Time) {
	end := now.AddDate(1, 0, 0)
	type candidate struct {
		date   time.Time
		amount decimal.Decimal
		future bool
	}
	byMonth := make(map[string]candidate)
	for _, event := range events {
		date := dividendEventDate(event)
		if date.IsZero() {
			continue
		}
		projected := date
		future := date.After(now)
		if !future {
			projected = date.AddDate(1, 0, 0)
		}
		if projected.Before(now) || projected.After(end) {
			continue
		}
		key := projected.Format("2006-01")
		current, exists := byMonth[key]
		switch {
		case !exists:
			byMonth[key] = candidate{date: projected, amount: event.Amount, future: future}
		case future && !current.future:
			byMonth[key] = candidate{date: projected, amount: event.Amount, future: true}
		case future == current.future:
			current.amount = current.amount.Add(event.Amount)
			byMonth[key] = current
		}
	}
	for key, item := range byMonth {
		month := months[key]
		month.Month = time.Date(item.date.Year(), item.date.Month(), 1, 0, 0, 0, 0, time.Local)
		for _, position := range positions {
			gross := item.amount.Mul(position.Quantity)
			net := gross.Mul(decimal.NewFromInt(1).Sub(usDividendTaxRate))
			month.Gross, month.Net = month.Gross.Add(gross), month.Net.Add(net)
			if rate := dividendPositionRate(position, fx); rate.IsPositive() {
				month.NetKRW = month.NetKRW.Add(net.Mul(rate))
			}
		}
		months[key] = month
	}
}
