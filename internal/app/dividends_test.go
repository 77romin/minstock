package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	db "github.com/77romin/minstock-tui/internal/adapters/sqlite"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/shopspring/decimal"
)

type dividendProviderStub struct{ calls atomic.Int32 }

func (p *dividendProviderStub) DividendSource() string { return "test dividends" }
func (p *dividendProviderStub) Dividends(_ context.Context, symbol string) ([]domain.DividendEvent, error) {
	p.calls.Add(1)
	now := time.Now()
	return []domain.DividendEvent{
		{Symbol: symbol, PaymentDate: now.AddDate(0, -9, 0), Amount: decimal.NewFromInt(1), Currency: domain.USD},
		{Symbol: symbol, PaymentDate: now.AddDate(0, -6, 0), Amount: decimal.NewFromInt(1), Currency: domain.USD},
		{Symbol: symbol, PaymentDate: now.AddDate(0, -3, 0), Amount: decimal.NewFromInt(1), Currency: domain.USD},
		{Symbol: symbol, PaymentDate: now.AddDate(0, 0, -1), Amount: decimal.NewFromInt(1), Currency: domain.USD},
	}, nil
}

type failingDividendProviderStub struct{ calls atomic.Int32 }

func (p *failingDividendProviderStub) DividendSource() string { return "test dividends" }
func (p *failingDividendProviderStub) Dividends(context.Context, string) ([]domain.DividendEvent, error) {
	p.calls.Add(1)
	return nil, errors.New("daily rate limit")
}

type slowDividendProviderStub struct{ calls atomic.Int32 }

func (p *slowDividendProviderStub) DividendSource() string { return "test dividends" }
func (p *slowDividendProviderStub) Dividends(_ context.Context, symbol string) ([]domain.DividendEvent, error) {
	p.calls.Add(1)
	time.Sleep(50 * time.Millisecond)
	return []domain.DividendEvent{{Symbol: symbol, PaymentDate: time.Now().AddDate(0, 0, -1), Amount: decimal.NewFromInt(1), Currency: domain.USD}}, nil
}

func TestDividendPortfolioCalculatesTaxKRWAndUsesDailyCache(t *testing.T) {
	repo, err := db.Open(filepath.Join(t.TempDir(), "minstock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	provider := &dividendProviderStub{}
	service := New(repo, nil, nil, nil, nil)
	service.SetDividendProvider(provider)
	positions := []domain.Position{{
		Broker: domain.BrokerNH, Symbol: domain.Symbol{Ticker: "QQQ", Name: "Invesco QQQ", Currency: domain.USD},
		Quantity: decimal.NewFromInt(10), ExchangeRate: decimal.NewFromInt(1400),
	}}
	report := service.DividendPortfolio(t.Context(), positions, domain.FXRate{})
	if len(report.Holdings) != 1 || !report.GrossAnnual.Equal(decimal.NewFromInt(40)) || !report.NetAnnual.Equal(decimal.NewFromInt(34)) || !report.NetAnnualKRW.Equal(decimal.NewFromInt(47600)) {
		t.Fatalf("unexpected dividend report: %#v", report)
	}
	if len(report.Months) != 4 || provider.calls.Load() != 1 || report.Freshness != domain.FreshLive {
		t.Fatalf("first dividend load: %#v calls=%d", report, provider.calls.Load())
	}
	cached := service.DividendPortfolio(t.Context(), positions, domain.FXRate{})
	if provider.calls.Load() != 1 || cached.Freshness != domain.FreshCached || !cached.NetAnnual.Equal(decimal.NewFromInt(34)) {
		t.Fatalf("cache not reused: %#v calls=%d", cached, provider.calls.Load())
	}
}

func TestDividendPortfolioDoesNotUseAnotherBrokersFX(t *testing.T) {
	repo, err := db.Open(filepath.Join(t.TempDir(), "minstock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	service := New(repo, nil, nil, nil, nil)
	service.SetDividendProvider(&dividendProviderStub{})
	report := service.DividendPortfolio(t.Context(), []domain.Position{{
		Broker: domain.BrokerNH, Symbol: domain.Symbol{Ticker: "QQQ", Currency: domain.USD}, Quantity: decimal.NewFromInt(10),
	}}, domain.FXRate{Provider: domain.BrokerKiwoom, Rate: decimal.NewFromInt(1400)})
	if !report.NetAnnualKRW.IsZero() {
		t.Fatalf("NH dividend used Kiwoom FX: %#v", report)
	}
}

func TestDividendFailureCooldownPreventsRefreshStorm(t *testing.T) {
	repo, err := db.Open(filepath.Join(t.TempDir(), "minstock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	provider := &failingDividendProviderStub{}
	service := New(repo, nil, nil, nil, nil)
	service.SetDividendProvider(provider)
	positions := []domain.Position{{Symbol: domain.Symbol{Ticker: "QLD", Currency: domain.USD}, Quantity: decimal.NewFromInt(1)}}
	first := service.DividendPortfolio(t.Context(), positions, domain.FXRate{})
	second := service.DividendPortfolio(t.Context(), positions, domain.FXRate{})
	if provider.calls.Load() != 1 || len(first.Warnings) != 1 || len(second.Warnings) != 1 || !strings.Contains(second.Warnings[0], "최근 조회 실패") {
		t.Fatalf("failure cooldown did not apply: calls=%d first=%#v second=%#v", provider.calls.Load(), first.Warnings, second.Warnings)
	}
}

func TestDividendConcurrentLoadsAreCoalesced(t *testing.T) {
	repo, err := db.Open(filepath.Join(t.TempDir(), "minstock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	provider := &slowDividendProviderStub{}
	service := New(repo, nil, nil, nil, nil)
	service.SetDividendProvider(provider)
	positions := []domain.Position{{Symbol: domain.Symbol{Ticker: "QQQM", Currency: domain.USD}, Quantity: decimal.NewFromInt(1)}}
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			service.DividendPortfolio(t.Context(), positions, domain.FXRate{})
		}()
	}
	group.Wait()
	if provider.calls.Load() != 1 {
		t.Fatalf("concurrent loads=%d, want 1", provider.calls.Load())
	}
}
