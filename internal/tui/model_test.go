package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/mink/stock-min-tui/internal/app"
	"github.com/mink/stock-min-tui/internal/domain"
	"github.com/shopspring/decimal"
)

func TestColonCommandsRequirePrefix(t *testing.T) {
	m := Model{screen: portfolioScreen}
	next, cmd := m.handleKey("q")
	if cmd != nil || next.(Model).commandMode {
		t.Fatal("plain q must not quit")
	}
	next, _ = next.(Model).handleKey(":")
	if !next.(Model).commandMode {
		t.Fatal(": must enter command mode")
	}
	_, cmd = next.(Model).handleKey("q")
	if cmd == nil {
		t.Fatal(":q must quit")
	}
}

func TestVimNavigationAndPortfolioTabs(t *testing.T) {
	kr := domain.Position{Symbol: domain.Symbol{Code: "005930", Market: domain.MarketKOSPI, Currency: domain.KRW}}
	us := domain.Position{Symbol: domain.Symbol{Code: "AAPL", Market: domain.MarketUS, Currency: domain.USD}}
	m := Model{screen: portfolioScreen, portfolioTab: filterKR, snapshot: app.Snapshot{Positions: []domain.Position{kr, kr, us}}, height: 24}
	next, _ := m.handleKey("j")
	m = next.(Model)
	if m.cursor != 1 {
		t.Fatalf("j cursor=%d", m.cursor)
	}
	next, _ = m.handleKey("g")
	next, _ = next.(Model).handleKey("g")
	m = next.(Model)
	if m.cursor != 0 {
		t.Fatalf("gg cursor=%d", m.cursor)
	}
	next, _ = m.handleKey("G")
	m = next.(Model)
	if m.cursor != 1 {
		t.Fatalf("G cursor=%d", m.cursor)
	}
	next, _ = m.handleKey("tab")
	m = next.(Model)
	if m.portfolioTab != filterUS || m.itemCount() != 1 || m.cursor != 0 {
		t.Fatalf("US tab=%v count=%d cursor=%d", m.portfolioTab, m.itemCount(), m.cursor)
	}
	next, _ = m.handleKey("h")
	m = next.(Model)
	if m.portfolioTab != filterKR || m.itemCount() != 2 {
		t.Fatalf("KR tab=%v count=%d", m.portfolioTab, m.itemCount())
	}
}

func TestPortfolioViewRendersRequestedColumnsAndWeights(t *testing.T) {
	profit := decimal.NewFromInt(25000)
	position := domain.Position{
		Symbol:        domain.Symbol{Code: "005930", Name: "삼성전자", Market: domain.MarketKOSPI, Currency: domain.KRW},
		Quantity:      decimal.NewFromInt(10),
		AveragePrice:  decimal.NewFromInt(70000),
		CurrentPrice:  decimal.NewFromInt(75000),
		PurchaseValue: decimal.NewFromInt(700000),
		MarketValue:   decimal.NewFromInt(750000),
		ProfitLoss:    profit,
		ProfitRate:    decimal.NewFromFloat(3.57),
	}
	m := Model{screen: portfolioScreen, portfolioTab: filterKR, width: 180, snapshot: app.Snapshot{Positions: []domain.Position{position}}}
	view := m.portfolioView()
	for _, header := range []string{"종목명", "평가손익", "수익률", "잔고수량", "평가금액", "매입가", "현재가", "매입금액", "보유비중"} {
		if !strings.Contains(view, header) {
			t.Fatalf("missing portfolio column %q in %q", header, view)
		}
	}
	if !strings.Contains(view, "100.00%") {
		t.Fatalf("portfolio weight missing in %q", view)
	}
	if !strings.Contains(view, positive.Render(fitCell("+25000원", portfolioColumns[1].width, true))) {
		t.Fatalf("positive profit is not red in %q", view)
	}
}

func TestPortfolioLossUsesBlue(t *testing.T) {
	loss := decimal.NewFromInt(-1000)
	p := domain.Position{Symbol: domain.Symbol{Name: "테스트", Market: domain.MarketKOSPI, Currency: domain.KRW}, ProfitLoss: loss}
	row := Model{}.renderPortfolioRow(p, decimal.Zero, []int{1, 2})
	if !strings.Contains(row, negative.Render(fitCell("-1000원", portfolioColumns[1].width, true))) {
		t.Fatalf("negative profit is not blue: %q", row)
	}
	if lipgloss.Width(row) == 0 {
		t.Fatal("rendered loss row must not be empty")
	}
}

func TestSearchHasEditAndNormalModes(t *testing.T) {
	m := Model{screen: searchScreen, searchEditing: true}
	next, _ := m.handleKey("q")
	m = next.(Model)
	if m.query != "q" {
		t.Fatalf("search query=%q", m.query)
	}
	next, _ = m.handleKey("enter")
	m = next.(Model)
	if m.searchEditing {
		t.Fatal("enter must leave search edit mode")
	}
}
