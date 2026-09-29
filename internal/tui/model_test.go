package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/77romin/minstock-tui/internal/app"
	"github.com/77romin/minstock-tui/internal/domain"
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
	next, _ = m.handleKey(":")
	_, cmd = next.(Model).handleKey("ㅂ")
	if cmd == nil {
		t.Fatal(":ㅂ must quit as an alias for :q")
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

func TestPerformanceScreenNavigation(t *testing.T) {
	m := Model{screen: dashboardScreen}
	next, _ := m.handleKey("6")
	m = next.(Model)
	if m.screen != performanceScreen {
		t.Fatalf("6 screen=%v", m.screen)
	}
	next, _ = m.handleKey("right")
	if next.(Model).screen != dividendScreen {
		t.Fatalf("performance right must open dividends: %v", next.(Model).screen)
	}
}

func TestDividendScreenNavigation(t *testing.T) {
	m := Model{screen: dashboardScreen}
	next, cmd := m.handleKey("7")
	got := next.(Model)
	if got.screen != dividendScreen || !got.dividendLoading || cmd == nil {
		t.Fatalf("dividend navigation screen=%v loading=%v cmd=%v", got.screen, got.dividendLoading, cmd)
	}
	next, _ = got.handleKey("right")
	if next.(Model).screen != allocationScreen {
		t.Fatalf("dividend right must move to allocation: %v", next.(Model).screen)
	}
}

func TestAlertScreenNavigationAndRuleInput(t *testing.T) {
	m := Model{screen: dashboardScreen}
	next, cmd := m.handleKey("9")
	m = next.(Model)
	if m.screen != alertScreen || cmd == nil {
		t.Fatalf("alert navigation screen=%v cmd=%v", m.screen, cmd)
	}
	next, _ = m.handleKey("tab")
	m = next.(Model)
	if m.alertDisplay != alertRules {
		t.Fatalf("alert tab display=%v", m.alertDisplay)
	}
	next, _ = m.handleKey("a")
	m = next.(Model)
	for _, key := range []string{"V", "O", "O", " ", "7", "0", "0"} {
		next, _ = m.handleKey(key)
		m = next.(Model)
	}
	next, cmd = m.handleKey("enter")
	m = next.(Model)
	if m.alertAdding || m.alertInput != "VOO 700" || cmd == nil {
		t.Fatalf("alert input adding=%v input=%q cmd=%v", m.alertAdding, m.alertInput, cmd)
	}
}

func TestAlertViewShowsHistoryAndRules(t *testing.T) {
	now := time.Now()
	m := Model{width: 120, screen: alertScreen, alerts: app.AlertReport{
		Unacknowledged: 1,
		Events:         []domain.AlertEvent{{Kind: domain.AlertHoldingLoss, Severity: "위험", Subject: "QLD", Message: "보유 손실률 -12.00%", OccurredAt: now}},
		Rules:          []domain.PriceAlertRule{{Symbol: domain.Symbol{Ticker: "VOO", Currency: domain.USD}, TargetPrice: decimal.NewFromInt(700), Direction: "ABOVE"}},
	}}
	if view := m.alertView(); !strings.Contains(view, "미확인 1건") || !strings.Contains(view, "보유손실") || !strings.Contains(view, "QLD") {
		t.Fatalf("alert history missing: %q", view)
	}
	m.alertDisplay = alertRules
	if view := m.alertView(); !strings.Contains(view, "VOO") || !strings.Contains(view, "$700.00") || !strings.Contains(view, "이상 도달") {
		t.Fatalf("alert rules missing: %q", view)
	}
}

func TestNarrowHeaderFitsWithAlertTab(t *testing.T) {
	m := Model{width: 80, mode: "connected", screen: alertScreen}
	header := m.header()
	if width := lipgloss.Width(header); width > 80 || !strings.Contains(header, "9알") {
		t.Fatalf("narrow header width=%d header=%q", width, header)
	}
}

func TestDividendScreenKeepsPreloadedCacheVisibleWhileRefreshing(t *testing.T) {
	m := Model{
		screen:    dashboardScreen,
		dividends: app.DividendReport{Holdings: []app.DividendHolding{{Symbol: "VOO"}}},
	}
	next, cmd := m.handleKey("7")
	got := next.(Model)
	if got.screen != dividendScreen || got.dividendLoading || cmd == nil {
		t.Fatalf("preloaded dividend navigation screen=%v loading=%v cmd=%v", got.screen, got.dividendLoading, cmd)
	}
}

func TestCachedDividendMessageIgnoresDifferentHoldings(t *testing.T) {
	m := Model{dividendHoldingsKey: "NH:VOO:1:1400"}
	next, _ := m.Update(cachedDividendMsg{
		holdingsKey: "NH:IVV:1:1400",
		report:      app.DividendReport{Holdings: []app.DividendHolding{{Symbol: "IVV"}}},
	})
	if len(next.(Model).dividends.Holdings) != 0 {
		t.Fatalf("stale startup dividend cache was applied: %#v", next.(Model).dividends)
	}
}

func TestPerformanceScreenSwitchesTableChartAndPeriod(t *testing.T) {
	m := Model{screen: performanceScreen}
	next, _ := m.handleKey("tab")
	m = next.(Model)
	if m.performance != performanceChart {
		t.Fatalf("tab performance display=%v", m.performance)
	}
	next, _ = m.handleKey("t")
	m = next.(Model)
	if m.perfPeriod != performanceWeekly {
		t.Fatalf("t performance period=%v", m.perfPeriod)
	}
	next, _ = m.handleKey("tab")
	m = next.(Model)
	if m.performance != performanceTable {
		t.Fatalf("second tab performance display=%v", m.performance)
	}
}

func TestDividendTabTogglesHoldingsAndCalendar(t *testing.T) {
	m := Model{screen: dividendScreen}
	next, _ := m.handleKey("tab")
	m = next.(Model)
	if m.dividendDisplay != dividendCalendar || !strings.Contains(m.notice, "월별") {
		t.Fatalf("tab dividend display=%v notice=%q", m.dividendDisplay, m.notice)
	}
	next, _ = m.handleKey("shift+tab")
	m = next.(Model)
	if m.dividendDisplay != dividendHoldings || !strings.Contains(m.notice, "종목별") {
		t.Fatalf("shift+tab dividend display=%v notice=%q", m.dividendDisplay, m.notice)
	}
}

func TestAllocationInputUpdatesTargetAndStatus(t *testing.T) {
	m := Model{screen: allocationScreen, allocation: app.AllocationReport{Rows: []app.AllocationRow{{
		Target:         domain.AllocationTarget{Cash: true, TargetPercent: decimal.NewFromInt(50)},
		CurrentPercent: decimal.NewFromInt(80),
	}}}}
	next, _ := m.handleKey("enter")
	m = next.(Model)
	if !m.allocationEditing {
		t.Fatal("allocation edit did not start")
	}
	m.allocationInput = "75"
	next, _ = m.handleKey("enter")
	m = next.(Model)
	if m.allocationEditing || !m.allocation.Rows[0].Target.TargetPercent.Equal(decimal.NewFromInt(75)) {
		t.Fatalf("target not updated: %#v", m.allocation)
	}
	if !m.allocationDirty {
		t.Fatal("edited allocation must remain dirty until saved")
	}
	if view := m.allocationView(); !strings.Contains(view, "+5.00%") || !strings.Contains(view, "초과") {
		t.Fatalf("updated deviation missing: %q", view)
	}
}

func TestAllocationCopyCurrentCorrectsRoundingToOneHundred(t *testing.T) {
	m := Model{screen: allocationScreen, allocation: app.AllocationReport{Rows: []app.AllocationRow{
		{CurrentPercent: decimal.RequireFromString("33.333333")},
		{CurrentPercent: decimal.RequireFromString("33.333333")},
		{Target: domain.AllocationTarget{Cash: true}, CurrentPercent: decimal.RequireFromString("33.333334")},
	}}}
	next, _ := m.handleKey("r")
	m = next.(Model)
	total := decimal.Zero
	for _, row := range m.allocation.Rows {
		total = total.Add(row.Target.TargetPercent)
	}
	if !total.Equal(decimal.NewFromInt(100)) || !m.allocationDirty {
		t.Fatalf("copied target total=%s dirty=%v rows=%#v", total, m.allocationDirty, m.allocation.Rows)
	}
}

func TestAddedAllocationSurvivesBackgroundReload(t *testing.T) {
	m := Model{screen: allocationScreen}
	ivv := domain.Symbol{Code: "IVV", Ticker: "IVV", Name: "iShares Core S&P 500", Market: domain.MarketUS, Currency: domain.USD}
	next, _ := m.Update(allocationSymbolMsg{symbol: ivv})
	m = next.(Model)
	if len(m.allocation.Rows) != 1 || !m.allocationDirty {
		t.Fatalf("IVV was not added as dirty edit: %#v", m)
	}
	next, _ = m.Update(allocationMsg{report: app.AllocationReport{Scope: "ALL"}})
	m = next.(Model)
	if len(m.allocation.Rows) != 1 || m.allocation.Rows[0].Target.Symbol.Ticker != "IVV" {
		t.Fatalf("background reload removed unsaved IVV: %#v", m.allocation.Rows)
	}
}

func TestAllocationAddModeAcceptsTicker(t *testing.T) {
	m := Model{screen: allocationScreen}
	next, _ := m.handleKey("a")
	m = next.(Model)
	if !m.allocationAdding {
		t.Fatal("allocation add did not start")
	}
	next, _ = m.handleKey("Q")
	m = next.(Model)
	next, _ = m.handleKey("Q")
	m = next.(Model)
	next, _ = m.handleKey("Q")
	m = next.(Model)
	if m.allocationInput != "QQQ" {
		t.Fatalf("ticker input=%q", m.allocationInput)
	}
}

func TestDividendDashboardRefreshDoesNotReloadUnchangedHoldings(t *testing.T) {
	positions := []domain.Position{{
		Broker: domain.BrokerNH, Symbol: domain.Symbol{Ticker: "QQQM", Currency: domain.USD},
		Quantity: decimal.NewFromInt(15), ExchangeRate: decimal.NewFromInt(1360),
	}}
	key := dividendHoldingsKey(positions)
	m := Model{screen: dividendScreen, dividendHoldingsKey: key}
	next, _ := m.Update(dashboardMsg{snapshot: app.Snapshot{Positions: positions}})
	if next.(Model).dividendLoading {
		t.Fatal("unchanged holdings triggered another dividend load")
	}
	changed := append([]domain.Position(nil), positions...)
	changed[0].Quantity = decimal.NewFromInt(16)
	next, _ = next.(Model).Update(dashboardMsg{snapshot: app.Snapshot{Positions: changed}})
	if !next.(Model).dividendLoading {
		t.Fatal("changed holdings did not trigger dividend reload")
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
	m := Model{screen: portfolioScreen, portfolioTab: filterKR, width: 260, snapshot: app.Snapshot{Positions: []domain.Position{position}}}
	view := m.portfolioView()
	for _, header := range []string{"구분", "종목명", "수익률", "잔고수량", "평가금액", "매입가", "현재가", "매입금액", "보유비중", "보유금액", "보유주식수"} {
		if !strings.Contains(view, header) {
			t.Fatalf("missing portfolio column %q in %q", header, view)
		}
	}
	if !strings.Contains(view, "100.00%") {
		t.Fatalf("portfolio weight missing in %q", view)
	}
	var topLine string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "t: 정렬") {
			topLine = line
			break
		}
	}
	if topLine == "" || !strings.Contains(topLine, "c: ₩/$") {
		t.Fatalf("sort/currency hints must be on the portfolio top line: %q", view)
	}
	if !strings.Contains(view, fitCell("키움", portfolioColumns[0].width, false)) {
		t.Fatalf("broker display name is missing in %q", view)
	}
	if !strings.Contains(view, positive.Render(fitCell("+25,000원", portfolioColumns[2].width, true))) {
		t.Fatalf("positive profit is not red in %q", view)
	}
}

func TestPortfolioLossUsesBlue(t *testing.T) {
	loss := decimal.NewFromInt(-1000)
	p := domain.Position{Symbol: domain.Symbol{Name: "테스트", Market: domain.MarketKOSPI, Currency: domain.KRW}, ProfitLoss: loss}
	row := Model{}.renderPortfolioRow(p, decimal.Zero, []int{2, 3})
	if !strings.Contains(row, negative.Render(fitCell("-1,000원", portfolioColumns[2].width, true))) {
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

func TestUSPortfolioUsesEachBrokersExchangeRateInKRWMode(t *testing.T) {
	positions := []domain.Position{
		{Broker: domain.BrokerNH, Symbol: domain.Symbol{Name: "애플", Market: domain.MarketUS, Currency: domain.USD}, MarketValue: decimal.NewFromInt(600), PurchaseValue: decimal.NewFromInt(500), ProfitLoss: decimal.NewFromInt(100), ExchangeRate: decimal.NewFromInt(1400), MarketValueKRW: decimal.NewFromInt(850000), PurchaseValueKRW: decimal.NewFromInt(705000), ProfitLossKRW: decimal.NewFromInt(145000)},
		{Broker: domain.BrokerKiwoom, Symbol: domain.Symbol{Name: "엔비디아", Market: domain.MarketUS, Currency: domain.USD}, MarketValue: decimal.NewFromInt(300), PurchaseValue: decimal.NewFromInt(250), ProfitLoss: decimal.NewFromInt(50), ExchangeRate: decimal.NewFromInt(1300)},
	}
	m := Model{screen: portfolioScreen, portfolioTab: filterUS, currency: currencyKRW, width: 260, snapshot: app.Snapshot{
		Positions: positions,
		FX:        domain.FXRate{Rate: decimal.NewFromInt(9999), Provider: domain.BrokerKiwoom},
	}}
	view := m.portfolioView()
	for _, want := range []string{"+145,000원", "₩850,000", "+65,000원", "₩390,000", "+210,000원"} {
		if !strings.Contains(view, want) {
			t.Fatalf("broker-specific KRW value %q missing: %q", want, view)
		}
	}
}

func TestDashboardIncludesNHUSBalanceWithoutKiwoom(t *testing.T) {
	m := Model{width: 100, snapshot: app.Snapshot{
		Balances: []domain.Balance{
			{Broker: domain.BrokerNH, Currency: domain.KRW, ValueTotal: decimal.NewFromInt(1_000_000), ProfitLoss: decimal.NewFromInt(50_000)},
			{Broker: domain.BrokerNH, Currency: domain.USD, ValueTotal: decimal.NewFromInt(600), Cash: decimal.NewFromInt(400), ProfitLoss: decimal.NewFromInt(100), ValueTotalKRW: decimal.NewFromInt(840_000), ProfitLossKRW: decimal.NewFromInt(140_000), ExchangeRate: decimal.NewFromInt(1400)},
		},
		FX: domain.FXRate{Provider: domain.BrokerNH, Rate: decimal.NewFromInt(1400)},
	}}
	view := m.dashboardView()
	for _, want := range []string{"2,400,000원", "+190,000원", "$1000.00", "+$100.00", "1400.00원"} {
		if !strings.Contains(view, want) {
			t.Fatalf("NH-only dashboard value %q missing: %q", want, view)
		}
	}
	if !strings.Contains(view, positive.Render("+190,000원")) {
		t.Fatalf("positive total profit is not red: %q", view)
	}
	if !strings.Contains(view, positive.Render("+$100.00")) {
		t.Fatalf("positive US profit is not red: %q", view)
	}
}

func TestDashboardColorsNegativeAndZeroProfit(t *testing.T) {
	negativeView := (Model{width: 100, snapshot: app.Snapshot{Balances: []domain.Balance{
		{Broker: domain.BrokerNH, Currency: domain.KRW, ProfitLoss: decimal.NewFromInt(-10)},
		{Broker: domain.BrokerNH, Currency: domain.USD, ProfitLoss: decimal.NewFromInt(-2), ProfitLossKRW: decimal.NewFromInt(-2800)},
	}}}).dashboardView()
	for _, want := range []string{negative.Render("-2,810원"), negative.Render("-$2.00")} {
		if !strings.Contains(negativeView, want) {
			t.Fatalf("negative dashboard profit %q missing: %q", want, negativeView)
		}
	}

	zeroView := (Model{width: 100}).dashboardView()
	for _, want := range []string{neutral.Render("0원"), neutral.Render("$0.00")} {
		if !strings.Contains(zeroView, want) {
			t.Fatalf("zero dashboard profit %q missing: %q", want, zeroView)
		}
	}
}

func TestBrokerAssetSummariesKeepBrokerExchangeRatesSeparate(t *testing.T) {
	snapshot := app.Snapshot{
		Statuses: []domain.BrokerStatus{
			{Broker: domain.BrokerKiwoom, Connected: false},
			{Broker: domain.BrokerNH, Connected: true},
		},
		Balances: []domain.Balance{
			{Broker: domain.BrokerNH, Currency: domain.KRW, Cash: decimal.NewFromInt(100_000), PurchaseTotal: decimal.NewFromInt(900_000), ValueTotal: decimal.NewFromInt(1_000_000), ProfitLoss: decimal.NewFromInt(100_000)},
			{Broker: domain.BrokerNH, Currency: domain.USD, Cash: decimal.NewFromInt(400), PurchaseTotal: decimal.NewFromInt(500), ValueTotal: decimal.NewFromInt(600), ProfitLoss: decimal.NewFromInt(100), ExchangeRate: decimal.NewFromInt(1400)},
		},
		FX: domain.FXRate{Provider: domain.BrokerKiwoom, Rate: decimal.NewFromInt(9999)},
	}
	summaries := brokerAssetSummaries(snapshot)
	if len(summaries) != 2 || summaries[0].Broker != domain.BrokerKiwoom || summaries[0].HasData {
		t.Fatalf("disconnected Kiwoom summary=%#v", summaries)
	}
	nh := summaries[1]
	if nh.Broker != domain.BrokerNH || !nh.TotalAssetsKRW.Equal(decimal.NewFromInt(2_500_000)) || !nh.PurchaseKRW.Equal(decimal.NewFromInt(1_600_000)) || !nh.ProfitLossKRW.Equal(decimal.NewFromInt(240_000)) {
		t.Fatalf("NH summary=%#v", nh)
	}
	if !nh.USAssets.Equal(decimal.NewFromInt(1000)) || !nh.ExchangeRate.Equal(decimal.NewFromInt(1400)) {
		t.Fatalf("NH USD summary=%#v", nh)
	}

	view := (Model{width: 140, snapshot: snapshot}).dashboardView()
	for _, want := range []string{"증권사별 자산 현황", "데이터 없음", "2,500,000원", "+240,000원", "+15.00%", "$1,000.00", "1400.00원", "100.00%"} {
		if !strings.Contains(view, want) {
			t.Fatalf("broker breakdown value %q missing: %q", want, view)
		}
	}
	if !strings.Contains(view, positive.Render(fitCell("+240,000원", 16, true))) {
		t.Fatalf("broker profit is not red: %q", view)
	}
}

func TestBrokerAssetSummariesDoNotUseAnotherBrokersFX(t *testing.T) {
	summaries := brokerAssetSummaries(app.Snapshot{
		Balances: []domain.Balance{{Broker: domain.BrokerNH, Currency: domain.USD, Cash: decimal.NewFromInt(400), ValueTotal: decimal.NewFromInt(600), ProfitLoss: decimal.NewFromInt(100)}},
		FX:       domain.FXRate{Provider: domain.BrokerKiwoom, Rate: decimal.NewFromInt(1400)},
	})
	if len(summaries) != 1 || !summaries[0].TotalAssetsKRW.IsZero() || !summaries[0].ProfitLossKRW.IsZero() || !summaries[0].ExchangeRate.IsZero() {
		t.Fatalf("NH summary must not use Kiwoom FX: %#v", summaries)
	}
}

func TestDashboardShowsDataFreshnessAndFXProvider(t *testing.T) {
	asOf := time.Date(2026, 9, 29, 12, 34, 56, 0, time.Local)
	snapshot := app.Snapshot{
		Balances: []domain.Balance{
			{Broker: domain.BrokerNH, Currency: domain.KRW, ValueTotal: decimal.NewFromInt(1_000_000), AsOf: asOf, Freshness: domain.FreshLive},
			{Broker: domain.BrokerNH, Currency: domain.USD, ValueTotal: decimal.NewFromInt(100), ValueTotalKRW: decimal.NewFromInt(140_000), ExchangeRate: decimal.NewFromInt(1400), AsOf: asOf.Add(-time.Minute), Freshness: domain.FreshCached},
		},
		FX: domain.FXRate{Provider: domain.BrokerNH, Rate: decimal.NewFromInt(1400), AsOf: asOf, Freshness: domain.FreshLive},
	}
	view := (Model{width: 140, snapshot: snapshot}).dashboardView()
	for _, want := range []string{"[일부 캐시]", "[NH·실시간]", "일부 캐시 12:33:56"} {
		if !strings.Contains(view, want) {
			t.Fatalf("provenance value %q missing: %q", want, view)
		}
	}

	snapshot.Cached = true
	cachedView := (Model{width: 100, snapshot: snapshot}).dashboardView()
	for _, want := range []string{"[캐시]", "[NH·캐시]", "NH·캐시"} {
		if !strings.Contains(cachedView, want) {
			t.Fatalf("cached provenance value %q missing: %q", want, cachedView)
		}
	}
}

func TestPerformanceViewAggregatesCurrenciesByDate(t *testing.T) {
	loc := time.FixedZone("KST", 9*60*60)
	day1 := time.Date(2026, 9, 28, 0, 0, 0, 0, loc)
	day2 := day1.AddDate(0, 0, 1)
	m := Model{width: 120, height: 30, history: []domain.PortfolioSnapshot{
		{Date: day1, Broker: domain.BrokerNH, Currency: domain.KRW, CashKRW: decimal.NewFromInt(100), ValueTotalKRW: decimal.NewFromInt(900), ProfitLossKRW: decimal.NewFromInt(50)},
		{Date: day1, Broker: domain.BrokerNH, Currency: domain.USD, CashKRW: decimal.NewFromInt(140), ValueTotalKRW: decimal.NewFromInt(1260), ProfitLossKRW: decimal.NewFromInt(70)},
		{Date: day2, Broker: domain.BrokerNH, Currency: domain.KRW, CashKRW: decimal.NewFromInt(100), ValueTotalKRW: decimal.NewFromInt(1000), ProfitLossKRW: decimal.NewFromInt(80)},
		{Date: day2, Broker: domain.BrokerNH, Currency: domain.USD, CashKRW: decimal.NewFromInt(140), ValueTotalKRW: decimal.NewFromInt(1360), ProfitLossKRW: decimal.NewFromInt(70)},
	}}
	points := m.performancePoints()
	if len(points) != 2 || !points[0].TotalAssets.Equal(decimal.NewFromInt(2400)) || !points[1].TotalAssets.Equal(decimal.NewFromInt(2600)) {
		t.Fatalf("performance points=%#v", points)
	}
	view := m.performanceView()
	for _, want := range []string{"2일 기록", "2,600원", "+150원", "+200원", "+8.33%", "2026-09-28", "2026-09-29"} {
		if !strings.Contains(view, want) {
			t.Fatalf("performance value %q missing: %q", want, view)
		}
	}
}

func TestPerformanceViewExplainsSingleDayHistory(t *testing.T) {
	m := Model{width: 100, height: 24, history: []domain.PortfolioSnapshot{{
		Date: time.Now(), Broker: domain.BrokerNH, Currency: domain.KRW,
		CashKRW: decimal.NewFromInt(100), ValueTotalKRW: decimal.NewFromInt(900),
	}}}
	view := m.performanceView()
	if !strings.Contains(view, "1일 기록") || !strings.Contains(view, "비교 기록 필요") {
		t.Fatalf("single-day guidance missing: %q", view)
	}
}

func TestPerformanceViewColorsValuesByDirection(t *testing.T) {
	loc := time.FixedZone("KST", 9*60*60)
	day1 := time.Date(2026, 9, 27, 0, 0, 0, 0, loc)
	m := Model{width: 120, height: 30, history: []domain.PortfolioSnapshot{
		{Date: day1, Broker: domain.BrokerNH, Currency: domain.KRW, ValueTotalKRW: decimal.NewFromInt(1000)},
		{Date: day1.AddDate(0, 0, 1), Broker: domain.BrokerNH, Currency: domain.KRW, ValueTotalKRW: decimal.NewFromInt(1100), ProfitLossKRW: decimal.NewFromInt(50)},
		{Date: day1.AddDate(0, 0, 2), Broker: domain.BrokerNH, Currency: domain.KRW, ValueTotalKRW: decimal.NewFromInt(990), ProfitLossKRW: decimal.NewFromInt(-20)},
	}}
	view := m.performanceView()
	for _, want := range []string{
		neutral.Render(fitCell("0원", 16, true)),
		neutral.Render(fitCell("-", 16, true)),
		positive.Render(fitCell("+50원", 16, true)),
		positive.Render(fitCell("+100원", 16, true)),
		positive.Render(fitCell("+10.00%", 10, true)),
		negative.Render(fitCell("-20원", 16, true)),
		negative.Render(fitCell("-110원", 16, true)),
		negative.Render(fitCell("-10.00%", 10, true)),
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("directional performance value %q missing: %q", want, view)
		}
	}
}

func TestAggregatePerformancePeriodUsesLastValueInEachPeriod(t *testing.T) {
	loc := time.FixedZone("KST", 9*60*60)
	point := func(year int, month time.Month, day, value int) performancePoint {
		return performancePoint{Date: time.Date(year, month, day, 0, 0, 0, 0, loc), TotalAssets: decimal.NewFromInt(int64(value))}
	}
	points := []performancePoint{
		point(2026, time.January, 5, 100),
		point(2026, time.January, 6, 110),
		point(2026, time.January, 12, 120),
		point(2026, time.February, 1, 130),
		point(2027, time.January, 1, 140),
	}
	weekly := aggregatePerformancePeriod(points, performanceWeekly)
	if len(weekly) != 4 || !weekly[0].TotalAssets.Equal(decimal.NewFromInt(110)) {
		t.Fatalf("weekly points=%#v", weekly)
	}
	monthly := aggregatePerformancePeriod(points, performanceMonthly)
	if len(monthly) != 3 || !monthly[0].TotalAssets.Equal(decimal.NewFromInt(120)) {
		t.Fatalf("monthly points=%#v", monthly)
	}
	yearly := aggregatePerformancePeriod(points, performanceYearly)
	if len(yearly) != 2 || !yearly[0].TotalAssets.Equal(decimal.NewFromInt(130)) {
		t.Fatalf("yearly points=%#v", yearly)
	}
}

func TestPerformanceChartViewShowsLineChartSummary(t *testing.T) {
	loc := time.FixedZone("KST", 9*60*60)
	m := Model{screen: performanceScreen, performance: performanceChart, width: 120, height: 30, history: []domain.PortfolioSnapshot{
		{Date: time.Date(2026, 9, 28, 0, 0, 0, 0, loc), Broker: domain.BrokerNH, Currency: domain.KRW, ValueTotalKRW: decimal.NewFromInt(1000)},
		{Date: time.Date(2026, 9, 29, 0, 0, 0, 0, loc), Broker: domain.BrokerNH, Currency: domain.KRW, ValueTotalKRW: decimal.NewFromInt(1200), ProfitLossKRW: decimal.NewFromInt(50)},
	}}
	view := m.performanceView()
	for _, want := range []string{"그래프", "일간", "기간 증감", "+200원", "+20.00%", "2026-09-28", "2026-09-29"} {
		if !strings.Contains(view, want) {
			t.Fatalf("performance chart value %q missing: %q", want, view)
		}
	}
}

func TestDividendViewShowsGrossNetAndMonthlyCalendar(t *testing.T) {
	m := Model{width: 130, height: 32, dividends: app.DividendReport{
		Source: "Alpha Vantage", Freshness: domain.FreshCached,
		GrossAnnual: decimal.NewFromInt(100), NetAnnual: decimal.NewFromInt(85), NetAnnualKRW: decimal.NewFromInt(119000),
		Holdings: []app.DividendHolding{{
			Symbol: "QQQ", Quantity: decimal.NewFromInt(10), RecentAmount: decimal.RequireFromString("0.5911"),
			RecentDate: time.Date(2026, 7, 31, 0, 0, 0, 0, time.Local), AnnualPerShare: decimal.NewFromInt(10),
			GrossAnnual: decimal.NewFromInt(100), NetAnnual: decimal.NewFromInt(85), NetAnnualKRW: decimal.NewFromInt(119000),
		}},
		Months: []app.DividendMonth{{Month: time.Date(2026, 12, 1, 0, 0, 0, 0, time.Local), Gross: decimal.NewFromInt(25), Net: decimal.RequireFromString("21.25"), NetKRW: decimal.NewFromInt(29750)}},
	}}
	view := m.dividendView()
	for _, want := range []string{"Alpha Vantage · 캐시", "+$100.00", "+$85.00", "119,000원", "QQQ", "$0.5911", "15%"} {
		if !strings.Contains(view, want) {
			t.Fatalf("dividend value %q missing: %q", want, view)
		}
	}
	m.dividendDisplay = dividendCalendar
	calendar := m.dividendView()
	for _, want := range []string{"2026-12", "$21.25", "29,750원", "세후 USD / 세후 원화"} {
		if !strings.Contains(calendar, want) {
			t.Fatalf("dividend calendar value %q missing: %q", want, calendar)
		}
	}
}

func TestNHPortfolioDoesNotFallBackToKiwoomExchangeRate(t *testing.T) {
	position := domain.Position{
		Broker: domain.BrokerNH, Symbol: domain.Symbol{Name: "애플", Market: domain.MarketUS, Currency: domain.USD},
		MarketValue: decimal.NewFromInt(600), PurchaseValue: decimal.NewFromInt(500), ProfitLoss: decimal.NewFromInt(100),
	}
	m := Model{screen: portfolioScreen, portfolioTab: filterUS, currency: currencyKRW, width: 260, snapshot: app.Snapshot{
		Positions: []domain.Position{position},
		FX:        domain.FXRate{Rate: decimal.NewFromInt(1400), Provider: domain.BrokerKiwoom},
	}}
	view := m.portfolioView()
	if strings.Contains(view, "+140,000원") || !strings.Contains(view, "₩-") {
		t.Fatalf("NH position must not use Kiwoom exchange rate: %q", view)
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
	for _, header := range []string{"티커", "종목명", "종목코드", "시장", "거래소", "통화"} {
		if !strings.Contains(view, header) {
			t.Fatalf("missing search column %q: %q", header, view)
		}
	}
	for _, value := range []string{"005930", "삼성전자", "Apple", "AAPL", "ND", "USD"} {
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

func TestDashboardRefreshKeepsLastQuoteUntilEnrichment(t *testing.T) {
	previous := domain.Quote{Price: decimal.NewFromInt(123), ChangeRate: decimal.NewFromInt(2)}
	m := Model{snapshot: app.Snapshot{Quotes: map[string]domain.Quote{"US:AAPL": previous}}, refreshEvery: 5 * time.Second}
	next, _ := m.Update(dashboardMsg{snapshot: app.Snapshot{LoadedAt: time.Now()}})
	got := next.(Model).snapshot.Quotes["US:AAPL"]
	if !got.Price.Equal(previous.Price) || !got.ChangeRate.Equal(previous.ChangeRate) {
		t.Fatalf("last quote was cleared during refresh: %#v", got)
	}
}

func TestConnectionIndicatorUsesThreeStates(t *testing.T) {
	connected := Model{snapshot: app.Snapshot{Statuses: []domain.BrokerStatus{{Connected: true}}, LoadedAt: time.Now()}, refreshEvery: 5 * time.Second}
	if !strings.Contains(connected.footer(), "Connected") || !strings.Contains(connected.footer(), "●") {
		t.Fatalf("connected footer: %q", connected.footer())
	}
	delayed := connected
	delayed.snapshot.LoadedAt = time.Now().Add(-6 * time.Second)
	if !strings.Contains(delayed.footer(), "Delayed") {
		t.Fatalf("delayed footer: %q", delayed.footer())
	}
	notConnected := Model{snapshot: app.Snapshot{Statuses: []domain.BrokerStatus{{Connected: false}}}}
	if !strings.Contains(notConnected.footer(), "Not Connected") {
		t.Fatalf("not-connected footer: %q", notConnected.footer())
	}
}
