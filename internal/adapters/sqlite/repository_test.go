package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/mink/stock-min-tui/internal/domain"
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
