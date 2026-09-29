package app

import (
	"path/filepath"
	"testing"

	db "github.com/77romin/minstock-tui/internal/adapters/sqlite"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/shopspring/decimal"
)

func TestAllocationTargetsPersistAndReportDeviation(t *testing.T) {
	repo, err := db.Open(filepath.Join(t.TempDir(), "minstock.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	service := New(repo, nil, nil, nil, nil)
	targets := []domain.AllocationTarget{
		{Scope: "ALL", Symbol: domain.Symbol{Code: "VOO", Ticker: "VOO", Name: "VOO", Market: domain.MarketUS, Currency: domain.USD}, TargetPercent: decimal.NewFromInt(50)},
		{Scope: "ALL", Cash: true, Symbol: domain.Symbol{Name: "현금", Currency: domain.KRW}, TargetPercent: decimal.NewFromInt(50)},
	}
	if err := service.SaveAllocationTargets(t.Context(), "ALL", targets); err != nil {
		t.Fatal(err)
	}
	report, err := service.AllocationReport(t.Context(), "ALL", Snapshot{Positions: []domain.Position{{Broker: domain.BrokerNH, Symbol: targets[0].Symbol, MarketValueKRW: decimal.NewFromInt(800)}}, Balances: []domain.Balance{{Broker: domain.BrokerNH, Currency: domain.KRW, Cash: decimal.NewFromInt(200), ValueTotal: decimal.NewFromInt(800)}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) != 2 || !report.TargetTotal.Equal(decimal.NewFromInt(100)) || report.Rows[0].Status != "집중" || !report.Rows[0].Difference.Equal(decimal.NewFromInt(30)) {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestAllocationTargetsRequireOneHundredPercent(t *testing.T) {
	repo, _ := db.Open(filepath.Join(t.TempDir(), "minstock.db"))
	defer repo.Close()
	_ = repo.Migrate(t.Context())
	service := New(repo, nil, nil, nil, nil)
	if err := service.SaveAllocationTargets(t.Context(), "ALL", []domain.AllocationTarget{{Cash: true, TargetPercent: decimal.NewFromInt(90)}}); err == nil {
		t.Fatal("invalid total was saved")
	}
}

func TestAllocationReportKeepsDashboardTotalWithResidualAsset(t *testing.T) {
	repo, _ := db.Open(filepath.Join(t.TempDir(), "minstock.db"))
	defer repo.Close()
	_ = repo.Migrate(t.Context())
	service := New(repo, nil, nil, nil, nil)
	report, err := service.AllocationReport(t.Context(), "ALL", Snapshot{
		Positions: []domain.Position{{Broker: domain.BrokerNH, Symbol: domain.Symbol{Code: "VOO", Ticker: "VOO", Market: domain.MarketUS, Currency: domain.USD}, MarketValueKRW: decimal.NewFromInt(400)}},
		Balances:  []domain.Balance{{Broker: domain.BrokerNH, Currency: domain.USD, CashKRW: decimal.NewFromInt(200), ValueTotalKRW: decimal.NewFromInt(800)}},
	})
	if err != nil || !report.TotalKRW.Equal(decimal.NewFromInt(1000)) {
		t.Fatalf("total=%s err=%v", report.TotalKRW, err)
	}
	if !report.USMarketPercent.Equal(decimal.NewFromInt(40)) || !report.OtherMarketPercent.Equal(decimal.NewFromInt(60)) || !report.USDPercent.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("concentration mismatch: %#v", report)
	}
	found := false
	for _, row := range report.Rows {
		if row.Target.Symbol.Code == "OTHER" && row.ValueKRW.Equal(decimal.NewFromInt(400)) {
			found = true
		}
	}
	if !found {
		t.Fatalf("residual asset missing: %#v", report.Rows)
	}
}
