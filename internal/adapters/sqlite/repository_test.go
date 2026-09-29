package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/shopspring/decimal"
)

func TestRepositorySearchWatchlistAndCandles(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(filepath.Join(t.TempDir(), "minstock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	symbol := domain.Symbol{Code: "005930", Name: "삼성전자", Market: domain.MarketKOSPI, Currency: domain.KRW}
	if err := repo.UpsertInstruments(ctx, []domain.Symbol{symbol}, domain.BrokerKiwoom); err != nil {
		t.Fatal(err)
	}
	found, err := repo.SearchInstruments(ctx, "삼성", 10)
	if err != nil || len(found) != 1 || found[0].Code != symbol.Code {
		t.Fatalf("search: %#v, %v", found, err)
	}

	if err := repo.AddLocalWatchlistItem(ctx, symbol); err != nil {
		t.Fatal(err)
	}
	items, err := repo.ListWatchlist(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("watchlist: %#v, %v", items, err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	candle := domain.Candle{Symbol: symbol, Interval: domain.IntervalDay, OpenTime: now.Add(-24 * time.Hour), CloseTime: now, Open: decimal.NewFromInt(80), High: decimal.NewFromInt(90), Low: decimal.NewFromInt(79), Close: decimal.NewFromInt(88), Volume: 1000, Provider: domain.BrokerKiwoom, Complete: true}
	if err := repo.SaveCandles(ctx, []domain.Candle{candle}); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.LoadCandles(ctx, domain.CandleQuery{Symbol: symbol, Interval: domain.IntervalDay, From: now.Add(-48 * time.Hour), To: now.Add(time.Hour), Limit: 10})
	if err != nil || len(loaded) != 1 || !loaded[0].Close.Equal(candle.Close) {
		t.Fatalf("candles: %#v, %v", loaded, err)
	}
}

func TestUSExchangeRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(filepath.Join(t.TempDir(), "minstock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	symbol := domain.Symbol{Code: "US-AAPL", Ticker: "AAPL", Name: "Apple", Market: domain.MarketUS, Currency: domain.USD, Exchange: "ND"}
	if err := repo.UpsertInstruments(ctx, []domain.Symbol{symbol}, domain.BrokerKiwoom); err != nil {
		t.Fatal(err)
	}
	results, err := repo.SearchInstruments(ctx, "AAPL", 10)
	if err != nil || len(results) != 1 || results[0].Ticker != "AAPL" || results[0].Exchange != "ND" {
		t.Fatalf("search exchange: %#v %v", results, err)
	}
	if err := repo.AddLocalWatchlistItem(ctx, symbol); err != nil {
		t.Fatal(err)
	}
	items, err := repo.ListWatchlist(ctx)
	if err != nil || len(items) != 1 || items[0].Symbol.Ticker != "AAPL" || items[0].Symbol.Exchange != "ND" {
		t.Fatalf("watchlist exchange: %#v %v", items, err)
	}
}

func TestMigrationRemovesDemoWatchlistButKeepsLocalItems(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(filepath.Join(t.TempDir(), "minstock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	local := domain.Symbol{Code: "005930", Name: "삼성전자", Market: domain.MarketKOSPI, Currency: domain.KRW}
	if err := repo.AddLocalWatchlistItem(ctx, local); err != nil {
		t.Fatal(err)
	}
	demoGroup := domain.WatchlistGroup{ID: "sample", ExternalID: "sample", Name: "샘플 관심종목", Provider: domain.BrokerMock}
	demo := domain.WatchlistItem{GroupID: "sample", Provider: domain.BrokerMock, Symbol: domain.Symbol{Code: "000660", Name: "SK하이닉스", Market: domain.MarketKOSPI, Currency: domain.KRW}}
	if err := repo.ReplaceWatchlistItems(ctx, demoGroup, []domain.WatchlistItem{demo}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	items, err := repo.ListWatchlist(ctx)
	if err != nil || len(items) != 1 || items[0].Symbol.Code != local.Code || items[0].GroupID != "default" {
		t.Fatalf("watchlist cleanup: %#v, %v", items, err)
	}
}

func TestDashboardCacheRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(filepath.Join(t.TempDir(), "minstock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	want := []byte(`{"version":1,"positions":2}`)
	if err := repo.SaveCache(ctx, "dashboard:v1", want); err != nil {
		t.Fatal(err)
	}
	got, updatedAt, err := repo.LoadCache(ctx, "dashboard:v1")
	if err != nil || string(got) != string(want) || updatedAt.IsZero() {
		t.Fatalf("cache round trip: payload=%q updated=%s err=%v", got, updatedAt, err)
	}
}

func TestPortfolioSnapshotsUpsertDailyValuesWithoutRawAccountID(t *testing.T) {
	ctx := context.Background()
	repo, err := Open(filepath.Join(t.TempDir(), "minstock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	loc := time.FixedZone("KST", 9*60*60)
	first := time.Date(2026, 9, 29, 10, 0, 0, 0, loc)
	snapshot := domain.PortfolioSnapshot{
		Date: first, CapturedAt: first, AccountID: "12345678901", Broker: domain.BrokerNH, Currency: domain.USD,
		Cash: decimal.NewFromInt(100), ValueTotal: decimal.NewFromInt(900), ProfitLoss: decimal.NewFromInt(50),
		CashKRW: decimal.NewFromInt(140000), ValueTotalKRW: decimal.NewFromInt(1260000), ProfitLossKRW: decimal.NewFromInt(70000), ExchangeRate: decimal.NewFromInt(1400),
	}
	if err := repo.SavePortfolioSnapshots(ctx, []domain.PortfolioSnapshot{snapshot}); err != nil {
		t.Fatal(err)
	}
	snapshot.CapturedAt = first.Add(2 * time.Hour)
	snapshot.ValueTotal = decimal.NewFromInt(950)
	snapshot.ValueTotalKRW = decimal.NewFromInt(1330000)
	if err := repo.SavePortfolioSnapshots(ctx, []domain.PortfolioSnapshot{snapshot}); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.ListPortfolioSnapshots(ctx, first, first)
	if err != nil || len(loaded) != 1 {
		t.Fatalf("snapshots: %#v %v", loaded, err)
	}
	if !loaded[0].ValueTotal.Equal(decimal.NewFromInt(950)) || !loaded[0].ValueTotalKRW.Equal(decimal.NewFromInt(1330000)) || !loaded[0].CapturedAt.Equal(snapshot.CapturedAt.UTC()) {
		t.Fatalf("daily snapshot was not updated: %#v", loaded[0])
	}
	var account string
	if err := repo.db.QueryRowContext(ctx, `SELECT account_ref FROM portfolio_snapshots`).Scan(&account); err != nil {
		t.Fatal(err)
	}
	if account == snapshot.AccountID || len(account) != 16 {
		t.Fatalf("account reference was not hashed: %q", account)
	}
}
