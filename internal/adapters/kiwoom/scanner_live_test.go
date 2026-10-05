package kiwoom_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/77romin/minstock-tui/internal/adapters/kiwoom"
	db "github.com/77romin/minstock-tui/internal/adapters/sqlite"
	"github.com/77romin/minstock-tui/internal/app"
	"github.com/77romin/minstock-tui/internal/config"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/ports"
	"github.com/77romin/minstock-tui/internal/security"
	"github.com/shopspring/decimal"
)

// This explicit opt-in probe uses public market read APIs and an isolated DB.
// Credentials are loaded by the application, never printed or persisted here.
func TestScannerLiveSmoke(t *testing.T) {
	if os.Getenv("MINSTOCK_SCANNER_SMOKE") != "1" {
		t.Skip("set MINSTOCK_SCANNER_SMOKE=1 to probe the configured Kiwoom server")
	}
	cfg, err := config.Load(os.Getenv("MINSTOCK_SCANNER_CONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := security.Load("kiwoom")
	if err != nil {
		t.Fatal(err)
	}
	client, err := kiwoom.New(cfg.Kiwoom.BaseURL, credentials)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	status := client.Status(ctx)
	if !status.Connected {
		t.Fatalf("authentication failed: %s", status.Message)
	}
	t.Logf("Kiwoom authentication: connected; mode=%s", status.Mode)
	query := domain.ScannerQuery{ExcludeETF: cfg.Scanner.ExcludeETF, Limit: 5, Market: map[string]domain.Market{"kospi": domain.MarketKOSPI, "kosdaq": domain.MarketKOSDAQ}[cfg.Scanner.Market]}
	candidates, err := client.SurgeCandidates(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("market rankings: candidates=%d, bounded=%v, warnings=%d", len(candidates.Quotes), candidates.Truncated, len(candidates.Warnings))
	for _, warning := range candidates.Warnings {
		t.Logf("ranking warning: %s", warning)
	}
	if len(candidates.Warnings) > 0 {
		t.Error("one or more market ranking APIs failed")
	}
	if len(candidates.Quotes) > 0 {
		sample := candidates.Quotes[0]
		quote, err := client.SurgeQuote(ctx, sample.Symbol)
		if err != nil {
			t.Fatal(err)
		}
		if !quote.Price.IsPositive() || !quote.High.IsPositive() {
			t.Fatalf("quote lacks price or high: price=%s high=%s", quote.Price, quote.High)
		}
		candles, err := client.Candles(ctx, domain.CandleQuery{Symbol: sample.Symbol, Interval: domain.Interval1Min, To: time.Now(), Limit: 30, Adjusted: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(candles) == 0 {
			t.Fatal("minute candle response is empty")
		}
		rate, ratio, asOf, metricErr := domain.SurgeMomentum(candles, time.Now())
		t.Logf("sample: code=%s, price=%s, high=%s, turnover_KRW=%s, trade_power=%s, minutes=%d, last_minute=%s", sample.Symbol.Code, quote.Price, quote.High, quote.Turnover, quote.TradePower, len(candles), candles[len(candles)-1].CloseTime.Format(time.RFC3339))
		if metricErr != nil {
			t.Logf("momentum unavailable (closed market or incomplete data): %s", metricErr)
		} else {
			t.Logf("momentum: five_minute_rate=%s volume_ratio=%s as_of=%s", rate, ratio, asOf.Format(time.RFC3339))
		}
	}
	repo, err := db.Open(filepath.Join(t.TempDir(), "scanner-smoke.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	service := app.New(repo, []ports.Provider{client}, nil, nil, nil)
	options := app.DefaultScannerOptions()
	options.Query = query
	options.Policy = domain.SurgePolicy{MinChangeRate: decimal.RequireFromString(cfg.Scanner.MinChangeRate), MinFiveMinuteRate: decimal.RequireFromString(cfg.Scanner.MinFiveMinuteRate), MinVolumeRatio: decimal.RequireFromString(cfg.Scanner.MinVolumeRatio), MinTurnover: decimal.NewFromInt(cfg.Scanner.MinTurnoverKRW)}
	service.SetScannerOptions(options)
	report := service.ScanSurges(ctx)
	t.Logf("scanner: candidates=%d checked=%d insufficient=%d reports=%d bounded=%v freshness=%s", report.Candidates, report.Checked, report.Missing, len(report.Reports), report.Limited, report.Freshness)
	if report.Error != "" {
		t.Fatal(report.Error)
	}
	if report.AsOf.IsZero() {
		t.Fatal("scan completion timestamp missing")
	}
	second := service.ScanSurges(ctx)
	if !second.AsOf.Equal(report.AsOf) {
		t.Fatal("immediate second scan bypassed refresh cooldown")
	}
}
