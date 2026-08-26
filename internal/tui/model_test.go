package tui

import (
	"strings"
	"testing"
	"time"

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
	next, _ = m.handleKey("shift+tab")
	m = next.(Model)
	if m.portfolioTab != filterKR || m.itemCount() != 2 {
		t.Fatalf("KR tab=%v count=%d", m.portfolioTab, m.itemCount())
	}
}

func TestSearchRequiresSlashToEnterEditMode(t *testing.T) {
	m := Model{screen: dashboardScreen}
	next, cmd := m.handleKey("3")
	m = next.(Model)
	if m.screen != searchScreen || m.searchEditing || cmd == nil {
		t.Fatalf("3 must open search normal mode: screen=%v editing=%v cmd=%v", m.screen, m.searchEditing, cmd)
	}
	next, _ = m.handleKey("q")
	m = next.(Model)
	if m.query != "" {
		t.Fatalf("normal-mode text changed query: %q", m.query)
	}
	next, _ = m.handleKey("/")
	m = next.(Model)
	if !m.searchEditing {
		t.Fatal("/ must enter search edit mode")
	}
	next, _ = m.handleKey("q")
	if next.(Model).query != "q" {
		t.Fatalf("edit-mode text did not change query: %q", next.(Model).query)
	}
}

func TestPrimaryScreensSupportHorizontalVimNavigation(t *testing.T) {
	m := Model{screen: dashboardScreen, portfolioTab: filterKR}
	next, _ := m.handleKey("right")
	m = next.(Model)
	if m.screen != portfolioScreen {
		t.Fatalf("right screen=%v", m.screen)
	}
	next, _ = m.handleKey("l")
	m = next.(Model)
	if m.screen != searchScreen || m.searchEditing {
		t.Fatalf("l screen=%v editing=%v", m.screen, m.searchEditing)
	}
	next, _ = m.handleKey("h")
	m = next.(Model)
	if m.screen != portfolioScreen {
		t.Fatalf("h screen=%v", m.screen)
	}
	next, _ = m.handleKey("left")
	m = next.(Model)
	if m.screen != dashboardScreen {
		t.Fatalf("left screen=%v", m.screen)
	}
}

func TestPortfolioViewRendersRequestedColumnsAndWeights(t *testing.T) {
	profit := decimal.NewFromInt(25000)
	position := domain.Position{
		Broker:        domain.BrokerKiwoom,
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
	for _, header := range []string{"구분", "종목명", "평가손익", "수익률", "잔고수량", "평가금액", "매입가", "현재가", "매입금액", "보유비중"} {
		if !strings.Contains(view, header) {
			t.Fatalf("missing portfolio column %q in %q", header, view)
		}
	}
	if !strings.Contains(view, "100.00%") {
		t.Fatalf("portfolio weight missing in %q", view)
	}
	var topLine string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "통화") {
			topLine = line
			break
		}
	}
	if topLine == "" || !strings.Contains(topLine, "열 1-10/10") || !strings.Contains(topLine, "[/]: 열 이동") {
		t.Fatalf("column range must be on the currency line: %q", view)
	}
	if !strings.Contains(view, fitCell("키움", portfolioColumns[0].width, false)) {
		t.Fatalf("broker display name is missing in %q", view)
	}
	if !strings.Contains(view, positive.Render(fitCell("+25000원", portfolioColumns[2].width, true))) {
		t.Fatalf("positive profit is not red in %q", view)
	}
}

func TestPortfolioLossUsesBlue(t *testing.T) {
	loss := decimal.NewFromInt(-1000)
	p := domain.Position{Symbol: domain.Symbol{Name: "테스트", Market: domain.MarketKOSPI, Currency: domain.KRW}, ProfitLoss: loss}
	row := Model{}.renderPortfolioRow(p, decimal.Zero, []int{2, 3})
	if !strings.Contains(row, negative.Render(fitCell("-1000원", portfolioColumns[2].width, true))) {
		t.Fatalf("negative profit is not blue: %q", row)
	}
	if lipgloss.Width(row) == 0 {
		t.Fatal("rendered loss row must not be empty")
	}
}

func TestPortfolioTotalUsesAggregatePurchaseReturn(t *testing.T) {
	positions := []domain.Position{
		{Symbol: domain.Symbol{Name: "수익", Market: domain.MarketKOSPI, Currency: domain.KRW}, PurchaseValue: decimal.NewFromInt(1000), MarketValue: decimal.NewFromInt(1100), ProfitLoss: decimal.NewFromInt(100)},
		{Symbol: domain.Symbol{Name: "손실", Market: domain.MarketKOSPI, Currency: domain.KRW}, PurchaseValue: decimal.NewFromInt(3000), MarketValue: decimal.NewFromInt(2980), ProfitLoss: decimal.NewFromInt(-20)},
	}
	m := Model{screen: portfolioScreen, portfolioTab: filterKR, width: 180, snapshot: app.Snapshot{Positions: positions}}
	view := m.portfolioView()
	if !strings.Contains(view, "합계") || !strings.Contains(view, "+80원") || !strings.Contains(view, "+2.00%") {
		t.Fatalf("aggregate profit summary is incorrect: %q", view)
	}
	if !strings.Contains(view, positive.Bold(true).Render(fitCell("+80원", portfolioColumns[2].width, true))) {
		t.Fatalf("aggregate profit is not red: %q", view)
	}
}

func TestBrokerDisplayNames(t *testing.T) {
	cases := map[domain.BrokerID]string{
		domain.BrokerKiwoom: "키움",
		domain.BrokerNH:     "NH",
		domain.BrokerMock:   "모의",
	}
	for broker, want := range cases {
		if got := brokerDisplayName(broker); got != want {
			t.Fatalf("brokerDisplayName(%q)=%q, want %q", broker, got, want)
		}
	}
}

func TestWatchlistViewIsTableAndColorsChangeRate(t *testing.T) {
	up := domain.Symbol{Code: "005930", Name: "삼성전자", Market: domain.MarketKOSPI, Currency: domain.KRW}
	down := domain.Symbol{Code: "000660", Name: "SK하이닉스", Market: domain.MarketKOSPI, Currency: domain.KRW}
	m := Model{
		screen: watchlistScreen,
		width:  100,
		snapshot: app.Snapshot{
			Watchlist: []domain.WatchlistItem{
				{Provider: domain.BrokerKiwoom, GroupID: "1", Symbol: up},
				{Provider: domain.BrokerMock, GroupID: "default", Symbol: down},
			},
			Quotes: map[string]domain.Quote{
				up.Key():   {Symbol: up, Price: decimal.NewFromInt(80000), ChangeRate: decimal.NewFromFloat(2.5)},
				down.Key(): {Symbol: down, Price: decimal.NewFromInt(190000), ChangeRate: decimal.NewFromFloat(-1.25)},
			},
		},
	}
	view := m.watchlistView()
	for _, header := range []string{"구분", "종목명", "종목코드", "현재가", "등락률"} {
		if !strings.Contains(view, header) {
			t.Fatalf("missing watchlist column %q: %q", header, view)
		}
	}
	if !strings.Contains(view, "키움") || !strings.Contains(view, "로컬") {
		t.Fatalf("watchlist providers missing: %q", view)
	}
	if !strings.Contains(view, positive.Render(fitCell("+2.50%", watchlistColumns[4].width, true))) {
		t.Fatalf("positive change rate is not red: %q", view)
	}
	if !strings.Contains(view, negative.Render(fitCell("-1.25%", watchlistColumns[4].width, true))) {
		t.Fatalf("negative change rate is not blue: %q", view)
	}
}

func TestSearchHasEditAndNormalModes(t *testing.T) {
	m := Model{
		screen:        searchScreen,
		searchEditing: true,
		results: []domain.Symbol{
			{Code: "005930", Name: "삼성전자", Market: domain.MarketKOSPI, Currency: domain.KRW},
			{Code: "000660", Name: "SK하이닉스", Market: domain.MarketKOSPI, Currency: domain.KRW},
		},
	}
	next, _ := m.handleKey("down")
	m = next.(Model)
	if m.cursor != 1 || !m.searchEditing {
		t.Fatalf("down in search edit mode: cursor=%d editing=%v", m.cursor, m.searchEditing)
	}
	next, _ = m.handleKey("up")
	m = next.(Model)
	if m.cursor != 0 {
		t.Fatalf("up in search edit mode: cursor=%d", m.cursor)
	}
	next, _ = m.handleKey("q")
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

func TestSearchViewRendersAlignedTable(t *testing.T) {
	kr := domain.Symbol{Code: "005930", Name: "삼성전자", Market: domain.MarketKOSPI, Currency: domain.KRW}
	us := domain.Symbol{Code: "AAPL", Name: "Apple", Market: domain.MarketUS, Currency: domain.USD, Exchange: "ND"}
	m := Model{screen: searchScreen, width: 100, results: []domain.Symbol{kr, us}}
	view := m.searchView()
	for _, header := range []string{"종목명", "종목코드", "시장", "거래소", "통화"} {
		if !strings.Contains(view, header) {
			t.Fatalf("missing search column %q: %q", header, view)
		}
	}
	for _, value := range []string{"삼성전자", "005930", "Apple", "AAPL", "ND", "USD"} {
		if !strings.Contains(view, value) {
			t.Fatalf("missing search value %q: %q", value, view)
		}
	}
	if lipgloss.Width(renderSearchHeader()) != lipgloss.Width("  "+renderSearchRow(kr)) {
		t.Fatalf("search header and row widths differ: header=%d row=%d", lipgloss.Width(renderSearchHeader()), lipgloss.Width("  "+renderSearchRow(kr)))
	}
}

func TestProgressiveDashboardMessagesPreferLiveData(t *testing.T) {
	cached := app.Snapshot{Cached: true, Positions: []domain.Position{{Symbol: domain.Symbol{Code: "CACHE"}}}}
	live := app.Snapshot{Positions: []domain.Position{{Symbol: domain.Symbol{Code: "LIVE"}}}, LoadedAt: time.Now()}
	enriched := live
	enriched.Quotes = map[string]domain.Quote{"LIVE": {}}

	m := Model{refreshing: true, loading: true, refreshEvery: 5 * time.Second}
	next, _ := m.Update(cachedDashboardMsg{snapshot: cached})
	m = next.(Model)
	if m.snapshot.Positions[0].Symbol.Code != "CACHE" || !m.loading {
		t.Fatalf("cached snapshot was not shown first: %#v", m)
	}
	next, cmd := m.Update(dashboardMsg{snapshot: live})
	m = next.(Model)
	if cmd == nil || !m.liveLoaded || !m.enriching || m.loading || m.snapshot.Positions[0].Symbol.Code != "LIVE" {
		t.Fatalf("live core did not replace cache: %#v", m)
	}
	next, _ = m.Update(cachedDashboardMsg{snapshot: cached})
	m = next.(Model)
	if m.snapshot.Positions[0].Symbol.Code != "LIVE" {
		t.Fatal("late cache message overwrote live data")
	}
	next, _ = m.Update(enrichmentMsg{snapshot: enriched})
	m = next.(Model)
	if m.enriching || len(m.snapshot.Quotes) != 1 {
		t.Fatalf("enrichment was not applied: %#v", m)
	}
}

func TestConnectionIndicatorUsesThreeStates(t *testing.T) {
	connected := Model{snapshot: app.Snapshot{Statuses: []domain.BrokerStatus{{Connected: true}}}}
	if !strings.Contains(connected.footer(), "Connected") || !strings.Contains(connected.footer(), "●") {
		t.Fatalf("connected footer: %q", connected.footer())
	}
	delayed := connected
	delayed.enriching = true
	if !strings.Contains(delayed.footer(), "Delayed") {
		t.Fatalf("delayed footer: %q", delayed.footer())
	}
	notConnected := Model{snapshot: app.Snapshot{Statuses: []domain.BrokerStatus{{Connected: false}}}}
	if !strings.Contains(notConnected.footer(), "Not Connected") {
		t.Fatalf("not-connected footer: %q", notConnected.footer())
	}
}
