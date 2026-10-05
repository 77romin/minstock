package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/shopspring/decimal"
)

func TestPerformanceShowsAccountAssetsWithoutBaselineControls(t *testing.T) {
	m := Model{screen: performanceScreen, width: 120, height: 30, history: []domain.PortfolioSnapshot{
		{Date: time.Date(2026, 10, 4, 0, 0, 0, 0, time.Local), ValueTotalKRW: decimal.NewFromInt(1000)},
		{Date: time.Date(2026, 10, 5, 0, 0, 0, 0, time.Local), ValueTotalKRW: decimal.NewFromInt(1400)},
	}}
	before := m.performanceView()
	next, cmd := m.handleKey("b")
	m = next.(Model)
	if cmd != nil || m.err != nil || m.performanceView() != before {
		t.Fatal("b must not set a baseline, display an error or alter performance")
	}
	for _, unexpected := range []string{"기준 자산", "보정 손익", "순입금", "입출금 기록"} {
		if strings.Contains(m.performanceView(), unexpected) || strings.Contains(m.footer(), unexpected) {
			t.Fatalf("removed feature remains: %s", unexpected)
		}
	}
	if !strings.Contains(before, "1,400원") || !strings.Contains(before, "+400원") || !strings.Contains(before, "+40.00%") {
		t.Fatalf("account asset change lost\n%s", before)
	}
	next, _ = m.handleKey("tab")
	m = next.(Model)
	if m.performance != performanceChart || !strings.Contains(m.performanceView(), "기간 증감") {
		t.Fatal("asset chart lost")
	}
}
