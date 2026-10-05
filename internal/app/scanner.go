package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/ports"
	"github.com/shopspring/decimal"
)

type ScannerOptions struct {
	Policy          domain.SurgePolicy
	Query           domain.ScannerQuery
	RefreshInterval time.Duration
}

func DefaultScannerOptions() ScannerOptions {
	return ScannerOptions{Policy: domain.DefaultSurgePolicy(), Query: domain.ScannerQuery{ExcludeETF: true, Limit: 20}, RefreshInterval: time.Minute}
}

type ScannerReport struct {
	Reports                      []domain.SurgeReport
	Provider                     domain.BrokerID
	AsOf                         time.Time
	Freshness                    domain.Freshness
	Candidates, Checked, Missing int
	Limited                      bool
	Warnings                     []string
	Error                        string
	Options                      ScannerOptions
}

func (s *Service) SetScannerOptions(options ScannerOptions) { s.scannerOptions = options }

func (s *Service) ScanSurges(ctx context.Context) ScannerReport {
	s.scannerMu.Lock()
	defer s.scannerMu.Unlock()
	options := s.scannerOptions
	var source ports.MarketScanner
	for _, p := range s.providers {
		if scanner, ok := p.(ports.MarketScanner); ok {
			source = scanner
			break
		}
	}
	if source == nil {
		return ScannerReport{Options: options, Error: "키움 연결이 필요합니다 · NH 단독 연결은 시장 스캐너 미지원"}
	}
	keyBytes, _ := json.Marshal(options)
	cacheKey := "scanner:v1:" + string(source.ID()) + ":" + string(keyBytes)
	if s.scannerLast.AsOf.IsZero() {
		if payload, _, err := s.repo.LoadCache(ctx, cacheKey); err == nil {
			_ = json.Unmarshal(payload, &s.scannerLast)
			s.scannerLast.Freshness = domain.FreshCached
		}
	}
	if s.scannerNow().Before(s.scannerRetry) {
		return s.scannerLast
	}
	report := ScannerReport{Options: options, Provider: source.ID(), Freshness: domain.FreshLive}
	candidates, err := source.SurgeCandidates(ctx, options.Query)
	s.scannerRetry = s.scannerNow().Add(options.RefreshInterval)
	if err != nil {
		if !s.scannerLast.AsOf.IsZero() {
			report = s.scannerLast
			report.Freshness = domain.FreshCached
		}
		report.Error = "급등 조회 실패: " + err.Error()
		s.scannerLast = report
		return report
	}
	report.Candidates, report.Limited, report.Warnings = len(candidates.Quotes), candidates.Truncated, candidates.Warnings
	for _, candidate := range candidates.Quotes {
		if candidate.ChangeRate.LessThan(options.Policy.MinChangeRate) {
			continue
		}
		if options.Query.ExcludeETF && candidate.Symbol.Market == domain.MarketETF {
			continue
		}
		// Restore the canonical market from the local instrument index when present.
		matches, _ := s.repo.SearchInstruments(ctx, candidate.Symbol.Code, 10)
		for _, symbol := range matches {
			if symbol.Code == candidate.Symbol.Code && symbol.Currency == domain.KRW {
				candidate.Symbol = symbol
				break
			}
		}
		if options.Query.ExcludeETF && candidate.Symbol.Market == domain.MarketETF {
			continue
		}
		if options.Query.Market != "" && candidate.Symbol.Market != options.Query.Market {
			continue
		}
		if report.Checked >= options.Query.Limit {
			report.Limited = true
			break
		}
		report.Checked++
		quoteReader := source.Quote
		if reader, ok := source.(ports.ScannerQuoteReader); ok {
			quoteReader = reader.SurgeQuote
		}
		quote, quoteErr := quoteReader(ctx, candidate.Symbol)
		if quoteErr != nil {
			report.Missing++
			report.Warnings = append(report.Warnings, candidate.Symbol.Code+": "+quoteErr.Error())
			continue
		}
		if !candidate.Turnover.IsPositive() && !quote.Turnover.IsPositive() {
			report.Missing++
			continue
		}
		if !quote.Turnover.IsPositive() && candidate.Turnover.IsPositive() {
			quote.Turnover = candidate.Turnover
		}
		if !quote.TradePower.IsPositive() && candidate.TradePower.IsPositive() {
			quote.TradePower = candidate.TradePower
		}
		if quote.Turnover.LessThan(options.Policy.MinTurnover) || quote.ChangeRate.LessThan(options.Policy.MinChangeRate) {
			continue
		}
		// Deliberately bypass the chart cache: a cached minute series cannot detect
		// current momentum. Keep quote/candles on the same provider and KRX session.
		candles, candleErr := source.Candles(ctx, domain.CandleQuery{Symbol: candidate.Symbol, Interval: domain.Interval1Min, To: s.scannerNow(), Limit: 30, Adjusted: true})
		if candleErr != nil {
			report.Missing++
			report.Warnings = append(report.Warnings, candidate.Symbol.Code+": "+candleErr.Error())
			continue
		}
		metricNow := s.scannerNow()
		if source.ID() == domain.BrokerMock && len(candles) > 0 {
			metricNow = candles[len(candles)-1].CloseTime
		}
		momentum, ratio, asOf, metricErr := domain.SurgeMomentum(candles, metricNow)
		if metricErr != nil {
			report.Missing++
			continue
		}
		if !quote.High.IsPositive() || quote.Price.GreaterThan(quote.High) {
			report.Missing++
			continue
		}
		quote.MarketTime = asOf
		surge, ok := domain.ScoreSurge(quote, momentum, ratio, options.Policy)
		if ok {
			surge.Reasons = append(surge.Reasons, fmt.Sprintf("거래대금 %s억원", quote.Turnover.Div(decimal.NewFromInt(100_000_000)).StringFixed(1)))
			if !quote.TradePower.IsPositive() {
				surge.Warnings = append(surge.Warnings, "체결강도 미제공 · 해당 점수 0점")
			}
			report.Reports = append(report.Reports, surge)
		}
	}
	// A timed-out scan is incomplete. Preserve the last successful observation.
	if ctx.Err() != nil {
		if !s.scannerLast.AsOf.IsZero() {
			report = s.scannerLast
			report.Freshness = domain.FreshCached
		}
		report.Error = "급등 조회 시간 초과 · 다음 주기에 재시도"
	} else {
		sort.Slice(report.Reports, func(i, j int) bool {
			if report.Reports[i].Score != report.Reports[j].Score {
				return report.Reports[i].Score > report.Reports[j].Score
			}
			return report.Reports[i].Symbol.Code < report.Reports[j].Symbol.Code
		})
		report.AsOf = s.scannerNow()
		payload, _ := json.Marshal(report)
		if err := s.repo.SaveCache(ctx, cacheKey, payload); err != nil {
			report.Warnings = append(report.Warnings, "급등 캐시 저장 실패")
		}
	}
	s.scannerRetry = s.scannerNow().Add(options.RefreshInterval)
	s.scannerLast = report
	return report
}
