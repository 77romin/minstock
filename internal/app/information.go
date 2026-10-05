package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/ports"
)

const informationTTL = 30 * time.Minute

type InformationSourceStatus struct {
	Source    string
	AsOf      time.Time
	Freshness domain.Freshness
	Warning   string
}
type InformationReport struct {
	Items    []domain.InformationItem
	Sources  []InformationSourceStatus
	Warnings []string
}
type informationResult struct {
	items  []domain.InformationItem
	status InformationSourceStatus
}
type informationCall struct {
	done   chan struct{}
	result informationResult
}

func (s *Service) SetInformationProviders(providers []ports.InformationProvider) {
	s.information = providers
}
func (s *Service) Information(ctx context.Context, symbol domain.Symbol) InformationReport {
	return s.informationReport(ctx, symbol, true)
}
func (s *Service) CachedInformation(ctx context.Context, symbol domain.Symbol) InformationReport {
	return s.informationReport(ctx, symbol, false)
}

func (s *Service) informationReport(ctx context.Context, symbol domain.Symbol, fetch bool) InformationReport {
	report := InformationReport{}
	var selected []ports.InformationProvider
	for _, source := range s.information {
		if source.SupportsInformation(symbol) {
			selected = append(selected, source)
		}
	}
	if len(selected) == 0 {
		report.Warnings = append(report.Warnings, "뉴스 공급자가 미설정입니다")
	}
	results := make([]informationResult, len(selected))
	var wg sync.WaitGroup
	for i, source := range selected {
		wg.Add(1)
		go func(i int, source ports.InformationProvider) {
			defer wg.Done()
			results[i] = s.loadInformation(ctx, source, symbol, fetch)
		}(i, source)
	}
	wg.Wait()
	for _, result := range results {
		report.Items = append(report.Items, result.items...)
		report.Sources = append(report.Sources, result.status)
	}
	report.Items = domain.NormalizeInformation(report.Items)
	// Only expose metadata; cap a combined timeline to an inexpensive TUI list.
	if len(report.Items) > 100 {
		report.Items = report.Items[:100]
	}
	if symbol.Currency == domain.USD || symbol.Market == domain.MarketUS {
		if len(selected) == 0 {
			report.Warnings = []string{"미국 뉴스: minstock setup dividend (기존 Alpha Vantage 키 재사용)"}
		}
	} else {
		hasNews, hasDART := false, false
		for _, source := range report.Sources {
			if source.Source == "NAVER" {
				hasNews = true
			}
			if source.Source == "DART" {
				hasDART = true
			}
			if source.Source == "demo" {
				hasNews, hasDART = true, true
			}
		}
		if !hasNews {
			report.Warnings = append(report.Warnings, "국내 뉴스: minstock setup naver")
		}
		if !hasDART {
			report.Warnings = append(report.Warnings, "국내 공시: minstock setup dart")
		}
	}
	return report
}

func (s *Service) loadInformation(ctx context.Context, provider ports.InformationProvider, symbol domain.Symbol, fetch bool) informationResult {
	source := provider.InformationSource()
	key := "information:v1:" + source + ":" + symbol.Key() + ":" + symbol.Name
	result := informationResult{status: InformationSourceStatus{Source: source}}
	// Cache-only reads never join or wait for a network operation.
	if !fetch {
		return s.informationCache(ctx, key, result)
	}
	s.informationMu.Lock()
	if s.informationRuns == nil {
		s.informationRuns = map[string]*informationCall{}
	}
	if call := s.informationRuns[key]; call != nil {
		s.informationMu.Unlock()
		select {
		case <-call.done:
			return call.result
		case <-ctx.Done():
			result.status.Warning = "뉴스 조회 취소/시간 초과"
			return result
		}
	}
	call := &informationCall{done: make(chan struct{})}
	s.informationRuns[key] = call
	s.informationMu.Unlock()
	defer func() {
		s.informationMu.Lock()
		call.result = result
		delete(s.informationRuns, key)
		close(call.done)
		s.informationMu.Unlock()
	}()
	result = s.informationCache(ctx, key, result)
	if !result.status.AsOf.IsZero() && time.Since(result.status.AsOf) < informationTTL {
		return result
	}
	var failure struct {
		Message string
		RetryAt time.Time
	}
	if payload, _, err := s.repo.LoadCache(ctx, "information-quota:v1:"+source); err == nil && json.Unmarshal(payload, &failure) == nil && time.Now().Before(failure.RetryAt) {
		result.status.Warning = failure.Message + " · 공급자 호출 제한 재시도 대기"
		return result
	}
	if payload, _, err := s.repo.LoadCache(ctx, "failure:"+key); err == nil && json.Unmarshal(payload, &failure) == nil && time.Now().Before(failure.RetryAt) {
		result.status.Warning = failure.Message + " · 재시도 대기"
		return result
	}
	items, err := provider.Information(ctx, symbol)
	if err != nil {
		result.status.Warning = domain.InformationText(err.Error())
		if ctx.Err() != nil {
			return result
		}
		failure.Message, failure.RetryAt = result.status.Warning, time.Now().Add(15*time.Minute)
		payload, _ := json.Marshal(failure)
		_ = s.repo.SaveCache(ctx, "failure:"+key, payload)
		if strings.Contains(failure.Message, "일일 호출") || strings.Contains(failure.Message, "HTTP 429") || (source == "DART" && strings.Contains(failure.Message, "상태 020")) {
			failure.RetryAt = time.Now().Add(24 * time.Hour)
			payload, _ = json.Marshal(failure)
			_ = s.repo.SaveCache(ctx, "information-quota:v1:"+source, payload)
		}
		return result
	}
	result.items = domain.NormalizeInformation(items)
	result.status.AsOf, result.status.Freshness, result.status.Warning = time.Now(), domain.FreshLive, ""
	payload, _ := json.Marshal(result.items)
	if err := s.repo.SaveCache(ctx, key, payload); err != nil {
		result.status.Warning = fmt.Sprintf("%s 캐시 저장 실패", source)
	}
	return result
}

func (s *Service) informationCache(ctx context.Context, key string, result informationResult) informationResult {
	if payload, asOf, err := s.repo.LoadCache(ctx, key); err == nil && json.Unmarshal(payload, &result.items) == nil {
		result.status.AsOf, result.status.Freshness = asOf, domain.FreshCached
	}
	return result
}
