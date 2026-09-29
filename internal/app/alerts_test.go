package app

import (
	"path/filepath"
	"testing"

	db "github.com/77romin/minstock-tui/internal/adapters/sqlite"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/shopspring/decimal"
)

func TestEvaluateAlertsPersistsAllConditionsWithoutDuplicates(t *testing.T) {
	repo, err := db.Open(filepath.Join(t.TempDir(), "minstock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	service := New(repo, nil, nil, nil, nil)
	symbol := domain.Symbol{Code: "VOO", Ticker: "VOO", Name: "Vanguard S&P 500 ETF", Market: domain.MarketUS, Currency: domain.USD}
	if err := service.SavePriceAlert(t.Context(), symbol, decimal.NewFromInt(100), decimal.NewFromInt(90)); err != nil {
		t.Fatal(err)
	}
	snapshot := Snapshot{
		Balances: []domain.Balance{{Broker: domain.BrokerNH, Currency: domain.KRW, ValueTotalKRW: decimal.NewFromInt(1000)}},
		Positions: []domain.Position{{
			Broker: domain.BrokerNH, Symbol: symbol, CurrentPrice: decimal.NewFromInt(101), MarketValueKRW: decimal.NewFromInt(400), ProfitRate: decimal.NewFromInt(-12),
		}},
		Quotes:   map[string]domain.Quote{symbol.Key(): {Symbol: symbol, Price: decimal.NewFromInt(101), ChangeRate: decimal.NewFromInt(6)}},
		Statuses: []domain.BrokerStatus{{Broker: domain.BrokerNH, Connected: false, Message: "연결 실패"}},
	}
	for range 4 {
		if err := service.EvaluateAlerts(t.Context(), snapshot); err != nil {
			t.Fatal(err)
		}
	}
	report, err := service.AlertReport(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Events) != 5 || report.Unacknowledged != 5 {
		t.Fatalf("events=%d unread=%d report=%#v", len(report.Events), report.Unacknowledged, report)
	}
	kinds := map[domain.AlertKind]bool{}
	for _, event := range report.Events {
		kinds[event.Kind] = true
	}
	for _, kind := range []domain.AlertKind{domain.AlertTargetPrice, domain.AlertDailyChange, domain.AlertHoldingLoss, domain.AlertAssetWeight, domain.AlertConnectionFailed} {
		if !kinds[kind] {
			t.Fatalf("missing alert kind %s: %#v", kind, report.Events)
		}
	}
	if err := service.AcknowledgeAlert(t.Context(), report.Events[0].ID); err != nil {
		t.Fatal(err)
	}
	report, _ = service.AlertReport(t.Context())
	if report.Unacknowledged != 4 {
		t.Fatalf("single acknowledge unread=%d", report.Unacknowledged)
	}
	if err := service.AcknowledgeAllAlerts(t.Context()); err != nil {
		t.Fatal(err)
	}
	report, _ = service.AlertReport(t.Context())
	if report.Unacknowledged != 0 {
		t.Fatalf("acknowledge all unread=%d", report.Unacknowledged)
	}
}

func TestSavePriceAlertChoosesBelowDirectionAndUpdatesRule(t *testing.T) {
	repo, err := db.Open(filepath.Join(t.TempDir(), "minstock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	service := New(repo, nil, nil, nil, nil)
	symbol := domain.Symbol{Code: "AAPL", Ticker: "AAPL", Market: domain.MarketUS, Currency: domain.USD}
	if err := service.SavePriceAlert(t.Context(), symbol, decimal.NewFromInt(180), decimal.NewFromInt(200)); err != nil {
		t.Fatal(err)
	}
	if err := service.SavePriceAlert(t.Context(), symbol, decimal.NewFromInt(220), decimal.NewFromInt(200)); err != nil {
		t.Fatal(err)
	}
	report, err := service.AlertReport(t.Context())
	if err != nil || len(report.Rules) != 1 || report.Rules[0].Direction != "ABOVE" || !report.Rules[0].TargetPrice.Equal(decimal.NewFromInt(220)) {
		t.Fatalf("upserted rule=%#v err=%v", report.Rules, err)
	}
}
