package app

import (
	"context"
	"path/filepath"
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
