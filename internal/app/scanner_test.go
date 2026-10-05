package app

import (
	"context"
	"errors"
	"github.com/77romin/minstock-tui/internal/adapters/mock"
	db "github.com/77romin/minstock-tui/internal/adapters/sqlite"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/ports"
	"github.com/shopspring/decimal"
	"path/filepath"
	"testing"
	"time"
)

type scannerStub struct {
	*mock.Provider
	now     time.Time
	calls   int
	failure bool
	queried []string
	empty   bool
}

func (p *scannerStub) ID() domain.BrokerID { return domain.BrokerKiwoom }
func (p *scannerStub) SurgeCandidates(context.Context, domain.ScannerQuery) (domain.ScannerCandidates, error) {
	p.calls++
	if p.failure {
		return domain.ScannerCandidates{}, errors.New("offline")
	}
	if p.empty {
		return domain.ScannerCandidates{}, nil
	}
	return domain.ScannerCandidates{Quotes: []domain.Quote{{Symbol: domain.Symbol{Code: "999001", Name: "未保有候補", Market: domain.MarketKOSPI, Currency: domain.KRW}, ChangeRate: decimal.NewFromInt(8), Turnover: decimal.NewFromInt(4_000_000_000), TradePower: decimal.NewFromInt(140)}}}, nil
}
func (p *scannerStub) Quote(_ context.Context, symbol domain.Symbol) (domain.Quote, error) {
	p.queried = append(p.queried, symbol.Code)
	return domain.Quote{Symbol: symbol, Price: decimal.NewFromInt(103), High: decimal.NewFromInt(104), ChangeRate: decimal.NewFromInt(8), Provider: p.ID()}, nil
}
func (p *scannerStub) Candles(_ context.Context, q domain.CandleQuery) ([]domain.Candle, error) {
	if q.Limit != 30 || q.Interval != domain.Interval1Min {
		return nil, errors.New("wrong scan query")
	}
	var result []domain.Candle
	for i := 0; i < 11; i++ {
		price, vol := int64(100), int64(10)
		if i > 5 {
			price, vol = 103, 30
		}
		start := p.now.Add(time.Duration(i-11) * time.Minute)
		result = append(result, domain.Candle{Interval: domain.Interval1Min, OpenTime: start, CloseTime: start.Add(time.Minute), Close: decimal.NewFromInt(price), Volume: vol, Complete: true})
	}
	return result, nil
}

func TestScannerMarketCandidateCacheAndRecovery(t *testing.T) {
	repo, err := db.Open(filepath.Join(t.TempDir(), "scanner.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	p := &scannerStub{Provider: mock.New(), now: now}
	s := New(repo, []ports.Provider{p}, nil, nil, nil)
	s.scannerNow = func() time.Time { return now }
	report := s.ScanSurges(t.Context())
	if report.Error != "" || len(report.Reports) != 1 || len(p.queried) != 1 || p.queried[0] != "999001" {
		t.Fatalf("market scan: %#v", report)
	}
	if !report.Reports[0].FiveMinuteRate.Equal(decimal.NewFromInt(3)) || !report.Reports[0].VolumeRatio.Equal(decimal.NewFromInt(3)) {
		t.Fatalf("fabricated metrics: %#v", report.Reports[0])
	}
	s.ScanSurges(t.Context())
	if p.calls != 1 {
		t.Fatal("refresh limit ignored")
	}
	p.failure = true
	restarted := New(repo, []ports.Provider{p}, nil, nil, nil)
	restarted.scannerNow = func() time.Time { return now.Add(time.Minute) }
	fallback := restarted.ScanSurges(t.Context())
	if len(fallback.Reports) != 1 || fallback.Freshness != domain.FreshCached || fallback.Error == "" || !fallback.AsOf.Equal(report.AsOf) {
		t.Fatalf("cache fallback: %#v", fallback)
	}
	restarted.ScanSurges(t.Context())
	if p.calls != 2 {
		t.Fatal("failure backoff ignored")
	}
	p.failure = false
	p.empty = true
	now = now.Add(3 * time.Minute)
	restarted.scannerNow = func() time.Time { return now }
	cleared := restarted.ScanSurges(t.Context())
	if len(cleared.Reports) != 0 || cleared.Error != "" {
		t.Fatalf("successful empty scan must clear stale candidates: %#v", cleared)
	}
}

func TestScannerDemoUsesSyntheticMinuteData(t *testing.T) {
	repo, err := db.Open(filepath.Join(t.TempDir(), "demo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	service := New(repo, []ports.Provider{mock.New()}, nil, nil, nil)
	report := service.ScanSurges(t.Context())
	if report.Error != "" || report.Provider != domain.BrokerMock || len(report.Reports) == 0 {
		t.Fatalf("demo scanner unavailable: %#v", report)
	}
}
