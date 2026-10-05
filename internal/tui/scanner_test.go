package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/77romin/minstock-tui/internal/app"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/shopspring/decimal"
	"strings"
	"testing"
	"time"
)

func TestScannerViewExplainsMetricsAndCachedFailure(t *testing.T) {
	report := domain.SurgeReport{Symbol: domain.Symbol{Code: "999001", Name: "시장 후보", Market: domain.MarketKOSPI, Currency: domain.KRW}, Score: 70, ChangeRate: decimal.NewFromInt(8), FiveMinuteRate: decimal.NewFromInt(3), VolumeRatio: decimal.NewFromInt(3), Turnover: decimal.NewFromInt(4_000_000_000), AsOf: time.Now(), Provider: domain.BrokerKiwoom}
	m := Model{screen: moversScreen, width: 120, height: 32, scanner: app.ScannerReport{Reports: []domain.SurgeReport{report}, AsOf: time.Now(), Options: app.DefaultScannerOptions(), Freshness: domain.FreshCached, Error: "급등 조회 실패", Missing: 2, Candidates: 10, Checked: 3, Limited: true}}
	view := m.moversView()
	for _, want := range []string{"캐시", "시장 후보", "3.00%", "3.0배", "40.0억원", "데이터 부족 2", "조회 실패", "조회 한도", "직전 5분"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q: %s", want, view)
		}
	}
	// Updating accounts and quotes must not replace independently loaded scans.
	updated, _ := m.Update(enrichmentMsg{snapshot: app.Snapshot{}})
	if len(updated.(Model).scanner.Reports) != 1 {
		t.Fatal("quote enrichment erased scanner")
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if strings.Contains(updated.(Model).moversView(), "NaN") {
		t.Fatal("invalid narrow view")
	}
}

func TestScannerViewDistinguishesUnsupportedAndEmpty(t *testing.T) {
	m := Model{width: 100, height: 30, scanner: app.ScannerReport{Error: "키움 연결이 필요합니다"}}
	if !strings.Contains(m.moversView(), "키움 연결이 필요합니다") {
		t.Fatal("unsupported source hidden")
	}
	m.scanner = app.ScannerReport{AsOf: time.Now(), Options: app.DefaultScannerOptions()}
	if !strings.Contains(m.moversView(), "현재 기준을 통과한 급등 후보가 없습니다") {
		t.Fatal("empty result missing")
	}
}

func TestScannerViewFitsTerminalAndScrollsSelection(t *testing.T) {
	for _, width := range []int{80, 120} {
		m := Model{width: width, height: 24, cursor: 19, scanner: app.ScannerReport{Options: app.DefaultScannerOptions(), Limited: true, Error: "조회 실패", Warnings: []string{"일부 순위 실패"}}}
		for i := 0; i < 20; i++ {
			m.scanner.Reports = append(m.scanner.Reports, domain.SurgeReport{Symbol: domain.Symbol{Name: "시장후보", Code: "999001", Currency: domain.KRW}, Warnings: []string{"체결강도 미제공"}})
		}
		view := m.moversView()
		if lipgloss.Width(view) > width || lipgloss.Height(view) > m.height-2 {
			t.Fatalf("view %dx%d does not fit %dx%d: %s", lipgloss.Width(view), lipgloss.Height(view), width, m.height, view)
		}
		if !strings.Contains(view, "999001") {
			t.Fatal("selected detail missing")
		}
	}
}
