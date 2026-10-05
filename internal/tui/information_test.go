package tui

import (
	"charm.land/lipgloss/v2"
	"fmt"
	"github.com/77romin/minstock-tui/internal/app"
	"github.com/77romin/minstock-tui/internal/domain"
	"strings"
	"testing"
	"time"
)

func TestInformationRejectsStaleAndLateCachedResponses(t *testing.T) {
	symbol := domain.Symbol{Code: "005930", Name: "삼성전자", Currency: domain.KRW}
	m := Model{screen: detailScreen, selected: symbol, informationRequest: 3, informationLoading: true}
	live := app.InformationReport{Items: []domain.InformationItem{{Title: "live"}}}
	updated, _ := m.Update(informationMsg{symbol: symbol, request: 2, report: live})
	if len(updated.(Model).information.Items) != 0 {
		t.Fatal("old generation accepted")
	}
	other := symbol
	other.Code = "000660"
	updated, _ = m.Update(informationMsg{symbol: other, request: 3, report: live})
	if len(updated.(Model).information.Items) != 0 {
		t.Fatal("other stock accepted")
	}
	updated, _ = m.Update(informationMsg{symbol: symbol, request: 3, report: live})
	m = updated.(Model)
	updated, _ = m.Update(informationMsg{symbol: symbol, request: 3, cached: true, report: app.InformationReport{}})
	m = updated.(Model)
	if len(m.information.Items) != 1 || m.informationLoading {
		t.Fatal("late cache overwrote live response")
	}
}
func TestInformationViewFitsAndScrolls(t *testing.T) {
	for _, width := range []int{80, 120} {
		m := Model{screen: detailScreen, width: width, height: 24, informationTab: true, informationCursor: 19, selected: domain.Symbol{Name: "삼성전자", Code: "005930"}}
		m.information.Sources = []app.InformationSourceStatus{{Source: "NAVER", AsOf: time.Now(), Freshness: domain.FreshCached, Warning: "조회 실패"}, {Source: "DART", Warning: "키 미설정"}}
		for i := 0; i < 20; i++ {
			m.information.Items = append(m.information.Items, domain.InformationItem{Kind: domain.InformationNews, Title: fmt.Sprintf("기사 %d", i), Source: "publisher", URL: "https://example.com/article", PublishedAt: time.Now()})
		}
		view := m.informationView()
		if lipgloss.Width(view) > width || lipgloss.Height(view) > m.height-2 {
			t.Fatalf("view does not fit: %dx%d\n%s", lipgloss.Width(view), lipgloss.Height(view), view)
		}
		if !strings.Contains(view, "기사 19") || !strings.Contains(view, "조회 실패") {
			t.Fatal("selection or source warning missing")
		}
		next, _, handled := m.handleInformationKey("k")
		if !handled || next.(Model).informationCursor != 18 {
			t.Fatal("article navigation failed")
		}
	}
}

func TestInformationTabLoadsLazilyAndReturnsToChart(t *testing.T) {
	m := Model{screen: detailScreen, width: 80, height: 24}
	if m.informationLoading {
		t.Fatal("news loaded before tab selection")
	}
	next, cmd := m.handleKey("tab")
	m = next.(Model)
	if cmd == nil || !m.informationTab || !m.informationLoading {
		t.Fatal("tab did not initiate news loading")
	}
	next, cmd = m.handleKey("tab")
	if next.(Model).informationTab || cmd != nil {
		t.Fatal("tab did not return to chart")
	}
	m.informationTab = true
	m.previous = portfolioScreen
	next, _ = m.handleKey("esc")
	if next.(Model).screen != portfolioScreen {
		t.Fatal("escape did not return to previous screen")
	}
}
