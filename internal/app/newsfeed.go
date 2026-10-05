package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/ports"
)

const NewsFeedBatchSize = 10
const newsFeedItemLimit = 300

type NewsFeedEntry struct {
	ID         string
	Item       domain.InformationItem
	Symbols    []domain.Symbol
	ReadAt     time.Time
	ObservedAt time.Time
	Freshness  domain.Freshness
}

type NewsFeedReport struct {
	Entries                               []NewsFeedEntry
	Warnings                              []string
	Symbols                               []domain.Symbol
	Offset, Queried, Total, CachedSymbols int
	Limited                               bool
	RefreshTotal                          int
	AsOf                                  time.Time
}

func NewsFeedSymbols(snapshot Snapshot, watchlist bool) []domain.Symbol {
	var raw []domain.Symbol
	if watchlist {
		for _, item := range snapshot.Watchlist {
			raw = append(raw, item.Symbol)
		}
	} else {
		for _, p := range snapshot.Positions {
			if p.Quantity.IsPositive() {
				raw = append(raw, p.Symbol)
			}
		}
	}
	seen := map[string]domain.Symbol{}
	for _, symbol := range raw {
		if strings.TrimSpace(symbol.Code) == "" {
			continue
		}
		key := NewsFeedSymbolKey(symbol)
		if previous, ok := seen[key]; !ok || (previous.Name == "" && symbol.Name != "") {
			seen[key] = symbol
		}
	}
	result := make([]domain.Symbol, 0, len(seen))
	for _, symbol := range seen {
		result = append(result, symbol)
	}
	sort.Slice(result, func(i, j int) bool { return NewsFeedSymbolKey(result[i]) < NewsFeedSymbolKey(result[j]) })
	return result
}

func NewsFeedSymbolKey(symbol domain.Symbol) string {
	code := symbol.Code
	if symbol.Currency == domain.USD || symbol.Market == domain.MarketUS {
		if symbol.Ticker != "" {
			code = symbol.Ticker
		}
		return "US:" + strings.ToUpper(strings.TrimSpace(code))
	}
	return "KR:" + strings.TrimSpace(code)
}

func InformationArticleID(raw string) string {
	canonical := domain.InformationURL(raw)
	if canonical == "" {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(canonical)))
}

// A cache-only pass includes every target. The live pass refreshes a single
// bounded batch sequentially and retains caches outside that batch. Filters
// operate on the returned entries and never create additional API requests.
func (s *Service) NewsFeed(ctx context.Context, symbols []domain.Symbol, offset int, fetch bool) NewsFeedReport {
	return s.newsFeed(ctx, symbols, offset, fetch, false)
}

// NewsFeedUS refreshes only US symbols after an explicit user action.
func (s *Service) NewsFeedUS(ctx context.Context, symbols []domain.Symbol, offset int) NewsFeedReport {
	return s.newsFeed(ctx, symbols, offset, true, true)
}

func (s *Service) newsFeed(ctx context.Context, symbols []domain.Symbol, offset int, fetch, usOnly bool) NewsFeedReport {
	report := NewsFeedReport{Symbols: append([]domain.Symbol(nil), symbols...), Total: len(symbols), AsOf: time.Now()}
	if len(symbols) == 0 {
		return report
	}
	var targets []int
	for i, symbol := range symbols {
		us := symbol.Currency == domain.USD || symbol.Market == domain.MarketUS
		if us == usOnly {
			targets = append(targets, i)
		}
	}
	report.RefreshTotal = len(targets)
	offset = max(0, offset)
	if offset >= len(targets) {
		offset = 0
	}
	report.Offset, report.Limited = offset, len(targets) > NewsFeedBatchSize
	batch := map[int]bool{}
	for _, i := range targets[offset:min(len(targets), offset+NewsFeedBatchSize)] {
		batch[i] = true
	}
	byID := map[string]*NewsFeedEntry{}
	warnings := map[string]bool{}
	// Take the entire cache snapshot before any request can exhaust the context.
	// This keeps unqueried/stale rows visible after a timeout midway through a batch.
	informationBySymbol := make([]InformationReport, len(symbols))
	for i, symbol := range symbols {
		informationBySymbol[i] = s.CachedInformation(ctx, symbol)
	}
	for i, symbol := range symbols {
		information := informationBySymbol[i]
		if fetch && batch[i] && ctx.Err() == nil {
			// Space symbol requests without sleeping after the last one. Context
			// cancellation prevents an obsolete scope from consuming more quota.
			if report.Queried > 0 {
				timer := time.NewTimer(250 * time.Millisecond)
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
				}
			}
			if ctx.Err() == nil {
				var latest InformationReport
				if usOnly {
					latest = s.InformationUS(ctx, symbol)
				} else {
					latest = s.Information(ctx, symbol)
				}
				if ctx.Err() != nil {
					latest.Items = domain.NormalizeInformation(append(latest.Items, information.Items...))
					if len(latest.Sources) == 0 {
						latest.Sources = information.Sources
					}
				}
				information = latest
				report.Queried++
			}
		} else if len(information.Items) > 0 {
			report.CachedSymbols++
		}
		for _, warning := range information.Warnings {
			warnings[warning] = true
		}
		for _, source := range information.Sources {
			if source.Warning != "" {
				warnings[source.Source+" · "+symbol.Code+": "+source.Warning] = true
			}
		}
		for _, item := range information.Items {
			id := InformationArticleID(item.URL)
			if id == "" {
				continue
			}
			entry := byID[id]
			if entry == nil {
				entry = &NewsFeedEntry{ID: id, Item: item}
				byID[id] = entry
			} else if item.PublishedAt.After(entry.Item.PublishedAt) {
				entry.Item = item
			}
			for _, source := range information.Sources {
				matches := source.Source == "demo" || (item.Kind == domain.InformationDisclosure && source.Source == "DART") || (item.Kind == domain.InformationNews && (source.Source == "NAVER" || source.Source == "Alpha Vantage"))
				if matches && source.AsOf.After(entry.ObservedAt) {
					entry.ObservedAt, entry.Freshness = source.AsOf, source.Freshness
				}
			}
			already := false
			for _, related := range entry.Symbols {
				already = already || NewsFeedSymbolKey(related) == NewsFeedSymbolKey(symbol)
			}
			if !already {
				entry.Symbols = append(entry.Symbols, symbol)
			}
		}
	}
	if ctx.Err() != nil {
		warnings["뉴스 피드 조회 시간 초과/취소 · 조회되지 않은 종목은 캐시로 표시합니다"] = true
	}
	for _, entry := range byID {
		report.Entries = append(report.Entries, *entry)
	}
	sort.Slice(report.Entries, func(i, j int) bool {
		a, b := report.Entries[i], report.Entries[j]
		if !a.Item.PublishedAt.Equal(b.Item.PublishedAt) {
			return a.Item.PublishedAt.After(b.Item.PublishedAt)
		}
		return a.ID < b.ID
	})
	if len(report.Entries) > newsFeedItemLimit {
		report.Entries = report.Entries[:newsFeedItemLimit]
		warnings["최신 300건만 표시합니다"] = true
	}
	if repo, ok := s.repo.(ports.InformationReadRepository); ok {
		ids := make([]string, len(report.Entries))
		for i, entry := range report.Entries {
			ids[i] = entry.ID
		}
		stateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		states, err := repo.InformationReadStates(stateCtx, ids)
		cancel()
		if err != nil {
			warnings["읽음 상태 조회 실패"] = true
		} else {
			for i := range report.Entries {
				report.Entries[i].ReadAt = states[report.Entries[i].ID]
			}
		}
	}
	for warning := range warnings {
		report.Warnings = append(report.Warnings, warning)
	}
	sort.Strings(report.Warnings)
	return report
}

func (s *Service) SetNewsFeedRead(ctx context.Context, id string, read bool) (time.Time, error) {
	repo, ok := s.repo.(ports.InformationReadRepository)
	if !ok {
		return time.Time{}, fmt.Errorf("읽음 상태 저장소가 없습니다")
	}
	return repo.SetInformationRead(ctx, id, read)
}
