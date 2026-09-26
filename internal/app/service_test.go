package app

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/77romin/minstock-tui/internal/adapters/mock"
	db "github.com/77romin/minstock-tui/internal/adapters/sqlite"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/ports"
)

func TestDemoServiceEndToEnd(t *testing.T) {
	ctx := context.Background()
	repo, err := db.Open(filepath.Join(t.TempDir(), "minstock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p := mock.New()
	service := New(repo, []ports.Provider{p}, []ports.InstrumentProvider{p}, []ports.WatchlistReader{p}, []ports.FXProvider{p})
	if errs := service.Sync(ctx); len(errs) > 0 {
		t.Fatal(errs)
	}

	results, err := service.Search(ctx, "삼성")
	if err != nil || len(results) == 0 {
		t.Fatalf("search: %#v %v", results, err)
	}
	core := service.DashboardCore(ctx)
	if len(core.Positions) == 0 || len(core.Watchlist) == 0 || core.FX.Rate.IsZero() || len(core.Quotes) != 0 {
		t.Fatalf("incomplete core snapshot: %#v", core)
	}
	snapshot := service.EnrichDashboard(ctx, core)
	if len(snapshot.Positions) == 0 || len(snapshot.Watchlist) == 0 || snapshot.FX.Rate.IsZero() {
		t.Fatalf("incomplete snapshot: %#v", snapshot)
	}
	cached, err := service.CachedDashboard(ctx)
	if err != nil || !cached.Cached || len(cached.Positions) != len(snapshot.Positions) || len(cached.Quotes) != len(snapshot.Quotes) {
		t.Fatalf("cached snapshot: %#v %v", cached, err)
	}
	candles, err := service.Candles(ctx, domain.CandleQuery{Symbol: results[0], Interval: domain.IntervalDay, From: time.Now().AddDate(-3, 0, 0), To: time.Now(), Limit: 180})
	if err != nil || len(candles) < 120 {
		t.Fatalf("candles: %d %v", len(candles), err)
	}
}
