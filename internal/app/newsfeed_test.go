package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	db "github.com/77romin/minstock-tui/internal/adapters/sqlite"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/ports"
	"github.com/shopspring/decimal"
)

type feedStub struct {
	calls  atomic.Int32
	cancel context.CancelFunc
}

func TestNewsFeedDomesticDefaultAndManualUSBatches(t *testing.T) {
	domestic := &feedStub{}
	s, _ := newsFeedTestService(t, domestic)
	us := &informationStub{source: "Alpha Vantage", items: []domain.InformationItem{{Kind: domain.InformationNews, Title: "US", URL: "https://example.com/us", PublishedAt: time.Now()}}}
	s.SetInformationProviders([]ports.InformationProvider{domestic, us})
	symbols := []domain.Symbol{{Code: "005930", Currency: domain.KRW}}
	for i := 0; i < 12; i++ {
		symbols = append(symbols, domain.Symbol{Code: fmt.Sprintf("US%d", i), Currency: domain.USD})
	}
	report := s.NewsFeed(t.Context(), symbols, 0, true)
	if us.calls.Load() != 0 || domestic.calls.Load() != 1 || report.Queried != 1 || report.RefreshTotal != 1 {
		t.Fatal("default feed consumed US quota or US targets displaced domestic batch")
	}
	// The permissive domestic stub supports every symbol, so on US symbols it is
	// refreshed only by the explicit US action, just like any future US provider.
	report = s.NewsFeedUS(t.Context(), symbols, 0)
	if us.calls.Load() != 10 || report.Queried != 10 || report.RefreshTotal != 12 || !report.Limited {
		t.Fatalf("manual batch not bounded: %#v", report)
	}
	report = s.NewsFeedUS(t.Context(), symbols, 10)
	if us.calls.Load() != 12 || report.Queried != 2 || domestic.calls.Load() != 13 {
		t.Fatal("manual next batch refreshed domestic symbol or skipped US targets")
	}
	s.NewsFeed(t.Context(), symbols, 0, true)
	if us.calls.Load() != 12 {
		t.Fatal("default feed called US provider after manual refresh")
	}
	if len(report.Entries) == 0 {
		t.Fatal("manual feed lost domestic cached entries")
	}
}

func (p *feedStub) InformationSource() string              { return "NAVER" }
func (p *feedStub) SupportsInformation(domain.Symbol) bool { return true }
func (p *feedStub) Information(_ context.Context, symbol domain.Symbol) ([]domain.InformationItem, error) {
	p.calls.Add(1)
	if p.cancel != nil {
		p.cancel()
		return nil, context.Canceled
	}
	return []domain.InformationItem{
		{Kind: domain.InformationNews, Title: "공통 기사", URL: "https://example.com/shared?utm_campaign=" + symbol.Code, PublishedAt: time.Date(2026, 10, 5, 3, 0, 0, 0, time.UTC)},
		{Kind: domain.InformationNews, Title: symbol.Code + " 개별 기사", URL: "https://example.com/" + symbol.Code, PublishedAt: time.Date(2026, 10, 5, 4, 0, 0, 0, time.UTC)},
	}, nil
}
func newsFeedTestService(t *testing.T, p *feedStub) (*Service, *db.Repository) {
	t.Helper()
	repo, err := db.Open(filepath.Join(t.TempDir(), "feed.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	if err := repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	s := New(repo, nil, nil, nil, nil)
	s.SetInformationProviders([]ports.InformationProvider{p})
	return s, repo
}
func TestNewsFeedSymbolsDeduplicatesHoldingsAndWatchlist(t *testing.T) {
	a := domain.Symbol{Code: "005930", Name: "삼성전자", Currency: domain.KRW, Market: domain.MarketKOSPI}
	b := a
	b.Market = domain.MarketKRX
	us := domain.Symbol{Code: "AAPL", Ticker: "AAPL", Currency: domain.USD, Market: domain.MarketUS}
	snapshot := Snapshot{Positions: []domain.Position{{Symbol: a, Quantity: decimal.NewFromInt(1)}, {Symbol: b, Quantity: decimal.NewFromInt(2)}, {Symbol: us, Quantity: decimal.Zero}}, Watchlist: []domain.WatchlistItem{{Symbol: a}, {Symbol: b}, {Symbol: us}}}
	if symbols := NewsFeedSymbols(snapshot, false); len(symbols) != 1 {
		t.Fatalf("holdings targets: %#v", symbols)
	}
	if symbols := NewsFeedSymbols(snapshot, true); len(symbols) != 2 {
		t.Fatalf("watchlist targets: %#v", symbols)
	}
}
func TestNewsFeedCacheDedupReadStateAndBatchLimit(t *testing.T) {
	p := &feedStub{}
	s, repo := newsFeedTestService(t, p)
	var symbols []domain.Symbol
	for i := 0; i < 12; i++ {
		symbols = append(symbols, domain.Symbol{Code: fmt.Sprintf("%06d", i), Name: fmt.Sprintf("종목%d", i), Currency: domain.KRW})
	}
	before := s.NewsFeed(t.Context(), symbols, 0, false)
	if p.calls.Load() != 0 || len(before.Entries) != 0 {
		t.Fatal("cache-only feed called API")
	}
	first := s.NewsFeed(t.Context(), symbols, 0, true)
	if p.calls.Load() != 10 || first.Queried != 10 || !first.Limited || len(first.Entries) != 11 {
		t.Fatalf("bounded batch: calls=%d report=%#v", p.calls.Load(), first)
	}
	if first.Entries[0].Item.Title == "공통 기사" {
		t.Fatal("timeline is not newest first")
	}
	var shared NewsFeedEntry
	for _, entry := range first.Entries {
		if entry.Item.Title == "공통 기사" {
			shared = entry
		}
	}
	if len(shared.Symbols) != 10 {
		t.Fatalf("lost related symbols: %#v", shared)
	}
	if _, err := s.SetNewsFeedRead(t.Context(), shared.ID, true); err != nil {
		t.Fatal(err)
	}
	restarted := New(repo, nil, nil, nil, nil)
	restarted.SetInformationProviders([]ports.InformationProvider{p})
	cached := restarted.NewsFeed(t.Context(), symbols, 0, false)
	if p.calls.Load() != 10 || len(cached.Entries) != 11 {
		t.Fatal("persistent cache not reused")
	}
	for _, entry := range cached.Entries {
		if entry.ID == shared.ID && entry.ReadAt.IsZero() {
			t.Fatal("read state lost on restart")
		}
	}
	last := s.NewsFeed(t.Context(), symbols, 10, true)
	if p.calls.Load() != 12 || last.Queried != 2 || len(last.Entries) != 13 {
		t.Fatalf("next batch: calls=%d items=%d", p.calls.Load(), len(last.Entries))
	}
	if InformationArticleID("https://example.com/shared?utm_source=x") != shared.ID || InformationArticleID("javascript:bad") != "" {
		t.Fatal("invalid canonical article identity")
	}
}
func TestNewsFeedCancellationRetainsCachesOutsideBatch(t *testing.T) {
	p := &feedStub{}
	s, repo := newsFeedTestService(t, p)
	symbols := []domain.Symbol{{Code: "000001", Name: "하나", Currency: domain.KRW}, {Code: "000002", Name: "둘", Currency: domain.KRW}}
	s.Information(t.Context(), symbols[1])
	readID := InformationArticleID("https://example.com/000002")
	repo.SetInformationRead(t.Context(), readID, true)
	ctx, cancel := context.WithCancel(t.Context())
	p.cancel = cancel
	result := s.NewsFeed(ctx, symbols, 0, true)
	if len(result.Entries) != 2 {
		t.Fatalf("timeout erased unqueried cache: %#v", result)
	}
	found := false
	for _, entry := range result.Entries {
		found = found || (entry.ID == readID && !entry.ReadAt.IsZero())
	}
	if !found || !strings.Contains(strings.Join(result.Warnings, " "), "시간 초과") {
		t.Fatal("read state or cancellation warning lost")
	}
	if p.calls.Load() != 2 {
		t.Fatal("continued API calls after cancellation")
	}
}
