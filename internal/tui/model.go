package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mink/stock-min-tui/internal/app"
	"github.com/mink/stock-min-tui/internal/domain"
	"github.com/shopspring/decimal"
)

type screen int

const (
	dashboardScreen screen = iota
	portfolioScreen
	searchScreen
	watchlistScreen
	moversScreen
	detailScreen
	helpScreen
	diagnosticsScreen
)

type marketFilter int

const (
	filterAll marketFilter = iota
	filterKR
	filterUS
)

type currencyDisplay int

const (
	currencyBoth currencyDisplay = iota
	currencyNative
	currencyKRW
)

var intervals = []domain.CandleInterval{
	domain.IntervalTick, domain.Interval1Min, domain.Interval5Min, domain.Interval15Min,
	domain.Interval60Min, domain.IntervalDay, domain.IntervalWeek, domain.IntervalMonth, domain.IntervalYear,
}

type Model struct {
	service       *app.Service
	mode          string
	screen        screen
	previous      screen
	width, height int
	cursor        int
	loading       bool
	err           error
	snapshot      app.Snapshot
	query         string
	results       []domain.Symbol
	selected      domain.Symbol
	candles       []domain.Candle
	intervalIndex int
	refreshEvery  time.Duration
	filter        marketFilter
	portfolioTab  marketFilter
	portfolioCol  int
	currency      currencyDisplay
	commandMode   bool
	pendingG      bool
	searchEditing bool
	notice        string
	refreshing    bool
	enriching     bool
	syncing       bool
	liveLoaded    bool
}

type cachedDashboardMsg struct {
	snapshot app.Snapshot
	err      error
}
type dashboardMsg struct{ snapshot app.Snapshot }
type enrichmentMsg struct{ snapshot app.Snapshot }
type searchMsg struct {
	query   string
	results []domain.Symbol
	err     error
}
type candlesMsg struct {
	symbol   domain.Symbol
	interval domain.CandleInterval
	candles  []domain.Candle
	err      error
}
type syncMsg struct{ errs []error }
type watchlistMsg struct{ err error }
type refreshMsg time.Time

func New(service *app.Service, mode string, refreshEvery time.Duration) Model {
	if refreshEvery < time.Second {
		refreshEvery = 5 * time.Second
	}
	return Model{service: service, mode: mode, loading: true, refreshing: true, width: 100, height: 30, intervalIndex: 5, refreshEvery: refreshEvery, portfolioTab: filterKR}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.cachedDashboardCmd(), m.dashboardCmd(), m.tickCmd())
}

func (m Model) cachedDashboardCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		snapshot, err := m.service.CachedDashboard(ctx)
		return cachedDashboardMsg{snapshot: snapshot, err: err}
	}
}

func (m Model) syncCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		return syncMsg{errs: m.service.Sync(ctx)}
	}
}

func (m Model) dashboardCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return dashboardMsg{snapshot: m.service.DashboardCore(ctx)}
	}
}

func (m Model) enrichmentCmd(snapshot app.Snapshot) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return enrichmentMsg{snapshot: m.service.EnrichDashboard(ctx, snapshot)}
	}
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(m.refreshEvery, func(t time.Time) tea.Msg { return refreshMsg(t) })
}

func (m Model) searchCmd(query string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		results, err := m.service.Search(ctx, query)
		return searchMsg{query: query, results: results, err: err}
	}
}

func (m Model) candlesCmd(symbol domain.Symbol) tea.Cmd {
	interval := intervals[m.intervalIndex]
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		to := time.Now()
		candles, err := m.service.Candles(ctx, domain.CandleQuery{Symbol: symbol, Interval: interval, From: to.AddDate(-3, 0, 0), To: to, Limit: 180, Adjusted: true})
		return candlesMsg{symbol: symbol, interval: interval, candles: candles, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case cachedDashboardMsg:
		if msg.err == nil && !m.liveLoaded {
			m.snapshot = msg.snapshot
			m.notice = "캐시 표시 · 최신 데이터 확인 중"
		} else if msg.err != nil && !m.syncing {
			// A first run has no cache or instrument index yet. Populate it once;
			// subsequent starts use the cache and skip this expensive operation.
			m.syncing = true
			return m, m.syncCmd()
		}
	case dashboardMsg:
		// DashboardCore intentionally returns before quote enrichment. Keep the
		// last successful quotes while the next request is in flight; a transient
		// API delay must not render watchlist prices as zero.
		if msg.snapshot.Quotes == nil {
			msg.snapshot.Quotes = map[string]domain.Quote{}
		}
		for key, quote := range m.snapshot.Quotes {
			if _, ok := msg.snapshot.Quotes[key]; !ok {
				msg.snapshot.Quotes[key] = quote
			}
		}
		m.snapshot, m.loading, m.err = msg.snapshot, false, nil
		m.refreshing, m.enriching, m.liveLoaded = false, true, true
		m.notice = "계좌 갱신 완료 · 시세 보강 중"
		return m, m.enrichmentCmd(msg.snapshot)
	case enrichmentMsg:
		m.snapshot, m.enriching = msg.snapshot, false
		m.notice = "최신 데이터"
	case syncMsg:
		m.syncing = false
		if len(msg.errs) > 0 {
			m.err = msg.errs[0]
		}
		if !m.refreshing && !m.enriching {
			m.refreshing = true
			return m, m.dashboardCmd()
		}
	case watchlistMsg:
		m.err = msg.err
		if msg.err == nil {
			m.notice = "관심종목을 갱신했습니다"
		}
		if !m.refreshing && !m.enriching {
			m.refreshing = true
			return m, m.dashboardCmd()
		}
	case searchMsg:
		if msg.query == m.query {
			m.results, m.err, m.cursor = msg.results, msg.err, 0
		}
	case candlesMsg:
		if msg.symbol.Key() == m.selected.Key() && msg.interval == intervals[m.intervalIndex] {
			m.candles, m.err, m.loading = msg.candles, msg.err, false
		}
	case refreshMsg:
		nextTick := m.tickCmd()
		if m.liveLoaded && time.Since(m.snapshot.LoadedAt) < m.refreshEvery {
			return m, nextTick
		}
		if m.screen != detailScreen && !m.refreshing && !m.enriching && !m.syncing {
			m.refreshing = true
			m.notice = "최신 데이터 확인 중"
			return m, tea.Batch(m.dashboardCmd(), nextTick)
		}
		return m, nextTick
	case tea.KeyPressMsg:
		return m.handleKey(msg.String())
	}
	return m, nil
}

func (m Model) handleKey(key string) (tea.Model, tea.Cmd) {
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	if !m.commandMode && !m.searchEditing && key == "?" {
		if m.screen == helpScreen {
			m.screen = m.previous
		} else {
			m.previous, m.screen = m.screen, helpScreen
		}
		return m, nil
	}
	if m.commandMode {
		m.commandMode = false
		switch key {
		case "q":
			return m, tea.Quit
		case "r":
			if m.refreshing || m.enriching || m.syncing {
				m.notice = "이미 데이터를 갱신하고 있습니다"
				return m, nil
			}
			m.refreshing, m.notice = true, "새로고침 중"
			return m, m.dashboardCmd()
		case "s":
			if m.syncing {
				m.notice = "이미 전체 동기화 중입니다"
				return m, nil
			}
			m.syncing, m.notice = true, "전체 동기화 중"
			return m, m.syncCmd()
		case "d":
			if m.screen == diagnosticsScreen {
				m.screen = m.previous
			} else {
				m.previous, m.screen = m.screen, diagnosticsScreen
			}
		case "esc":
		default:
			m.notice = "알 수 없는 명령 :" + key
		}
		return m, nil
	}
	if key == ":" {
		m.commandMode, m.pendingG, m.notice = true, false, ""
		return m, nil
	}
	if key == "esc" {
		if m.searchEditing {
			m.searchEditing = false
			return m, nil
		}
		if m.screen == detailScreen || m.screen == helpScreen || m.screen == diagnosticsScreen {
			m.screen = m.previous
		} else {
			m.screen = dashboardScreen
		}
		m.cursor, m.pendingG = 0, false
		return m, nil
	}
	if m.screen == searchScreen {
		return m.handleSearchKey(key)
	}
	if m.pendingG {
		m.pendingG = false
		switch key {
		case "g":
			m.cursor = 0
		case "t":
			m.nextPrimaryScreen(1)
		case "T":
			m.nextPrimaryScreen(-1)
		default:
			m.notice = "알 수 없는 g 명령"
		}
		return m, nil
	}
	if key == "g" {
		m.pendingG, m.notice = true, "g"
		return m, nil
	}
	if m.screen == detailScreen {
		if key == "left" || key == "h" {
			m.intervalIndex = (m.intervalIndex - 1 + len(intervals)) % len(intervals)
			m.loading = true
			return m, m.candlesCmd(m.selected)
		}
		if key == "right" || key == "l" {
			m.intervalIndex = (m.intervalIndex + 1) % len(intervals)
			m.loading = true
			return m, m.candlesCmd(m.selected)
		}
	}

	switch key {
	case "1":
		m.screen, m.cursor = dashboardScreen, 0
	case "2":
		m.screen, m.cursor = portfolioScreen, 0
	case "3":
		m.screen, m.cursor, m.searchEditing = searchScreen, 0, false
		return m, m.searchCmd(m.query)
	case "/":
		m.screen, m.cursor, m.searchEditing = searchScreen, 0, true
		return m, m.searchCmd(m.query)
	case "4":
		m.screen, m.cursor = watchlistScreen, 0
	case "5":
		m.screen, m.cursor = moversScreen, 0
	case "m":
		return m.toggleWatchlist()
	case "f":
		if m.screen == portfolioScreen {
			m.switchPortfolioTab(1)
		} else {
			m.filter = (m.filter + 1) % 3
			m.cursor, m.notice = 0, "시장 필터: "+m.filterName()
		}
	case "tab":
		if m.screen == portfolioScreen {
			m.switchPortfolioTab(1)
		}
	case "shift+tab":
		if m.screen == portfolioScreen {
			m.switchPortfolioTab(-1)
		}
	case "right", "l":
		if m.screen >= dashboardScreen && m.screen <= moversScreen {
			m.nextPrimaryScreen(1)
		}
	case "left", "h":
		if m.screen >= dashboardScreen && m.screen <= moversScreen {
			m.nextPrimaryScreen(-1)
		}
	case "]":
		if m.screen == portfolioScreen && m.portfolioCol < len(portfolioColumns)-1 {
			m.portfolioCol++
		}
	case "[":
		if m.screen == portfolioScreen && m.portfolioCol > 0 {
			m.portfolioCol--
		}
	case "c":
		m.currency = (m.currency + 1) % 3
		m.notice = "통화 표시: " + m.currencyName()
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor+1 < m.itemCount() {
			m.cursor++
		}
	case "G":
		if count := m.itemCount(); count > 0 {
			m.cursor = count - 1
		}
	case "ctrl+u":
		m.cursor = max(0, m.cursor-max(1, m.height/2))
	case "ctrl+d":
		if count := m.itemCount(); count > 0 {
			m.cursor = min(count-1, m.cursor+max(1, m.height/2))
		}
	case "enter":
		if symbol, ok := m.currentSymbol(); ok {
			m.previous, m.screen, m.selected, m.loading = m.screen, detailScreen, symbol, true
			return m, m.candlesCmd(symbol)
		}
	}
	return m, nil
}

func (m *Model) switchPortfolioTab(delta int) {
	if m.portfolioTab != filterKR && m.portfolioTab != filterUS {
		m.portfolioTab = filterKR
	}
	if delta > 0 {
		if m.portfolioTab == filterKR {
			m.portfolioTab = filterUS
		} else {
			m.portfolioTab = filterKR
		}
	} else if m.portfolioTab == filterKR {
		m.portfolioTab = filterUS
	} else {
		m.portfolioTab = filterKR
	}
	m.cursor, m.portfolioCol = 0, 0
	m.notice = "보유시장: " + portfolioTabName(m.portfolioTab)
}

func (m Model) handleSearchKey(key string) (tea.Model, tea.Cmd) {
	if m.pendingG {
		m.pendingG = false
		switch key {
		case "g":
			m.cursor = 0
		case "t":
			m.nextPrimaryScreen(1)
		case "T":
			m.nextPrimaryScreen(-1)
		default:
			m.notice = "알 수 없는 g 명령"
		}
		return m, nil
	}
	if !m.searchEditing {
		switch key {
		case "1":
			m.screen, m.cursor = dashboardScreen, 0
		case "2":
			m.screen, m.cursor = portfolioScreen, 0
		case "3":
			m.cursor = 0
		case "4":
			m.screen, m.cursor = watchlistScreen, 0
		case "5":
			m.screen, m.cursor = moversScreen, 0
		case "/":
			m.searchEditing = true
		case "left", "h":
			m.nextPrimaryScreen(-1)
		case "right", "l":
			m.nextPrimaryScreen(1)
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor+1 < m.itemCount() {
				m.cursor++
			}
		case "G":
			if count := m.itemCount(); count > 0 {
				m.cursor = count - 1
			}
		case "g":
			m.pendingG = true
		case "m":
			return m.toggleWatchlist()
		case "f":
			m.filter = (m.filter + 1) % 3
			m.cursor = 0
		case "enter":
			if symbol, ok := m.currentSymbol(); ok {
				m.previous, m.screen, m.selected, m.loading = searchScreen, detailScreen, symbol, true
				return m, m.candlesCmd(symbol)
			}
		}
		return m, nil
	}
	switch key {
	case "enter":
		m.searchEditing = false
	case "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down":
		if m.cursor+1 < m.itemCount() {
			m.cursor++
		}
	case "backspace":
		if len(m.query) > 0 {
			_, size := utf8.DecodeLastRuneInString(m.query)
			m.query = m.query[:len(m.query)-size]
			return m, m.searchCmd(m.query)
		}
	default:
		if utf8.RuneCountInString(key) == 1 {
			m.query += key
			return m, m.searchCmd(m.query)
		}
	}
	return m, nil
}

func (m *Model) nextPrimaryScreen(delta int) {
	screens := []screen{dashboardScreen, portfolioScreen, searchScreen, watchlistScreen, moversScreen}
	current := 0
	for i, candidate := range screens {
		if m.screen == candidate {
			current = i
			break
		}
	}
	current = (current + delta + len(screens)) % len(screens)
	m.screen, m.cursor, m.searchEditing = screens[current], 0, false
}

func (m Model) toggleWatchlist() (tea.Model, tea.Cmd) {
	symbol, ok := m.currentSymbol()
	if m.screen == detailScreen {
		symbol, ok = m.selected, m.selected.Code != ""
	}
	if !ok {
		m.notice = "선택된 종목이 없습니다"
		return m, nil
	}
	remove := false
	for _, item := range m.snapshot.Watchlist {
		if item.Symbol.Key() == symbol.Key() && strings.Contains(","+item.GroupID+",", ",default,") {
			remove = true
			break
		}
	}
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if remove {
			return watchlistMsg{err: m.service.RemoveWatchlist(ctx, symbol)}
		}
		return watchlistMsg{err: m.service.AddWatchlist(ctx, symbol)}
	}
}

func (m Model) matchesFilter(symbol domain.Symbol) bool {
	isUS := symbol.Market == domain.MarketUS || symbol.Currency == domain.USD
	switch m.filter {
	case filterKR:
		return !isUS
	case filterUS:
		return isUS
	default:
		return true
	}
}

func (m Model) filteredPositions() []domain.Position {
	result := make([]domain.Position, 0, len(m.snapshot.Positions))
	for _, p := range m.snapshot.Positions {
		isUS := p.Symbol.Market == domain.MarketUS || p.Symbol.Currency == domain.USD
		if (m.portfolioTab == filterUS && isUS) || (m.portfolioTab != filterUS && !isUS) {
			result = append(result, p)
		}
	}
	return result
}

func (m Model) filteredWatchlist() []domain.WatchlistItem {
	result := make([]domain.WatchlistItem, 0, len(m.snapshot.Watchlist))
	for _, item := range m.snapshot.Watchlist {
		if m.matchesFilter(item.Symbol) {
			result = append(result, item)
		}
	}
	return result
}

func (m Model) filteredResults() []domain.Symbol {
	result := make([]domain.Symbol, 0, len(m.results))
	for _, symbol := range m.results {
		if m.matchesFilter(symbol) {
			result = append(result, symbol)
		}
	}
	return result
}

func (m Model) filteredSurges() []domain.SurgeReport {
	result := make([]domain.SurgeReport, 0, len(m.snapshot.Surges))
	for _, report := range m.snapshot.Surges {
		if m.matchesFilter(report.Symbol) {
			result = append(result, report)
		}
	}
	return result
}

func (m Model) filterName() string {
	switch m.filter {
	case filterKR:
		return "한국"
	case filterUS:
		return "미국"
	default:
		return "전체"
	}
}

func (m Model) currencyName() string {
	switch m.currency {
	case currencyNative:
		return "원통화"
	case currencyKRW:
		return "원화"
	default:
		return "원통화+원화"
	}
}

func (m Model) itemCount() int {
	switch m.screen {
	case portfolioScreen:
		return len(m.filteredPositions())
	case watchlistScreen:
		return len(m.filteredWatchlist())
	case moversScreen:
		return len(m.filteredSurges())
	case searchScreen:
		return len(m.filteredResults())
	}
	return 0
}

func (m Model) currentSymbol() (domain.Symbol, bool) {
	if m.cursor < 0 {
		return domain.Symbol{}, false
	}
	switch m.screen {
	case portfolioScreen:
		items := m.filteredPositions()
		if m.cursor < len(items) {
			return items[m.cursor].Symbol, true
		}
	case watchlistScreen:
		items := m.filteredWatchlist()
		if m.cursor < len(items) {
			return items[m.cursor].Symbol, true
		}
	case moversScreen:
		items := m.filteredSurges()
		if m.cursor < len(items) {
			return items[m.cursor].Symbol, true
		}
	case searchScreen:
		items := m.filteredResults()
		if m.cursor < len(items) {
			return items[m.cursor], true
		}
	}
	return domain.Symbol{}, false
}

func (m Model) View() tea.View {
	content := m.header() + "\n" + m.body()
	if m.commandMode {
		content += "\n\n" + m.commandPopup()
	}
	content += "\n" + m.footer()
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m Model) commandPopup() string {
	commands := []string{":r  새로고침", ":s  종목·관심종목 동기화", ":d  연결 진단", ":q  종료", "Esc 팝업 닫기"}
	return panel.Width(34).Render(brand.Render("콜론 명령") + "\n\n" + strings.Join(commands, "\n"))
}

var (
	brand       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	muted       = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	selected    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))
	selectedRow = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	positive    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	negative    = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	statusOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	statusWait  = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	statusDown  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	panel       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
)

func (m Model) header() string {
	tabs := []string{"1 현황", "2 내 주식", "3 검색", "4 관심", "5 급등 분석"}
	return brand.Render(" MINSTOCK ") + "  " + strings.Join(tabs, "  ") + "  " + muted.Render("["+m.mode+"]")
}

func (m Model) body() string {
	if m.loading && len(m.snapshot.Balances) == 0 && m.screen != detailScreen {
		return "\n  데이터를 불러오는 중…"
	}
	var body string
	switch m.screen {
	case dashboardScreen:
		body = m.dashboardView()
	case portfolioScreen:
		body = m.portfolioView()
	case searchScreen:
		body = m.searchView()
	case watchlistScreen:
		body = m.watchlistView()
	case moversScreen:
		body = m.moversView()
	case detailScreen:
		body = m.detailView()
	case helpScreen:
		body = m.helpView()
	case diagnosticsScreen:
		body = m.diagnosticsView()
	}
	if m.err != nil {
		body += "\n" + negative.Render("오류: "+m.err.Error())
	}
	return body
}

func (m Model) dashboardView() string {
	totalValue, totalProfit := decimal.Zero, decimal.Zero
	usdValue, usdProfit := decimal.Zero, decimal.Zero
	for _, b := range m.snapshot.Balances {
		value := b.ValueTotal.Add(b.Cash)
		if b.Currency == domain.USD {
			usdValue, usdProfit = usdValue.Add(value), usdProfit.Add(b.ProfitLoss)
			if m.snapshot.FX.Rate.IsPositive() {
				totalValue = totalValue.Add(value.Mul(m.snapshot.FX.Rate))
				totalProfit = totalProfit.Add(b.ProfitLoss.Mul(m.snapshot.FX.Rate))
			}
		} else {
			totalValue, totalProfit = totalValue.Add(value), totalProfit.Add(b.ProfitLoss)
		}
	}
	fx := "-"
	if !m.snapshot.FX.Rate.IsZero() {
		fx = m.snapshot.FX.Rate.StringFixed(2) + "원"
	}
	left := panel.Width(max(28, m.width/2-4)).Render(fmt.Sprintf("통합 자산\n\n  원화환산  %s원\n  평가손익  %s원\n  미국자산  $%s\n  미국손익  %s\n  USD/KRW   %s", money(totalValue), signedMoney(totalProfit), usdValue.StringFixed(2), signedUSD(usdProfit), fx))
	var status []string
	for _, s := range m.snapshot.Statuses {
		mark := "○"
		if s.Connected {
			mark = "●"
		}
		status = append(status, fmt.Sprintf("%s %-7s %-5s %s", mark, s.Broker, s.Mode, s.Message))
	}
	right := panel.Width(max(28, m.width/2-4)).Render("연결 상태\n\n  " + strings.Join(status, "\n  "))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
}

func (m Model) portfolioView() string {
	items := m.filteredPositions()
	krCount, usCount := 0, 0
	for _, p := range m.snapshot.Positions {
		if p.Symbol.Market == domain.MarketUS || p.Symbol.Currency == domain.USD {
			usCount++
		} else {
			krCount++
		}
	}
	krTab, usTab := fmt.Sprintf(" 한국주식 %d ", krCount), fmt.Sprintf(" 미국주식 %d ", usCount)
	if m.portfolioTab == filterUS {
		usTab = selected.Render(usTab)
		krTab = muted.Render(krTab)
	} else {
		krTab = selected.Render(krTab)
		usTab = muted.Render(usTab)
	}
	visible := visiblePortfolioColumns(max(30, m.width-10), m.portfolioCol)
	topLeft := krTab + "  " + usTab + "   " + muted.Render("통화 "+m.currencyName())
	topRight := ""
	if len(visible) > 0 {
		last := visible[len(visible)-1] + 1
		topRight = muted.Render(fmt.Sprintf("열 %d-%d/%d  ([/]: 열 이동)", visible[0]+1, last, len(portfolioColumns)))
	}
	lines := []string{joinPortfolioTop(topLeft, topRight, max(56, m.width-8)), ""}
	if len(visible) > 0 {
		lines = append(lines, renderPortfolioHeader(visible))
	}
	totalValue, totalPurchase, totalProfit := decimal.Zero, decimal.Zero, decimal.Zero
	for _, p := range items {
		totalValue = totalValue.Add(p.MarketValue)
		totalPurchase = totalPurchase.Add(p.PurchaseValue)
		totalProfit = totalProfit.Add(p.ProfitLoss)
	}
	for i, p := range items {
		weight := decimal.Zero
		if !totalValue.IsZero() {
			weight = p.MarketValue.Div(totalValue).Mul(decimal.NewFromInt(100))
		}
		row := m.renderPortfolioRow(p, weight, visible)
		prefix := "  "
		if i == m.cursor {
			prefix = brand.Render("> ")
			row = selectedRow.Render(row)
		}
		lines = append(lines, prefix+row)
	}
	if len(items) > 0 {
		totalRate := decimal.Zero
		if !totalPurchase.IsZero() {
			totalRate = totalProfit.Div(totalPurchase).Mul(decimal.NewFromInt(100))
		}
		lines = append(lines, "  "+muted.Render(renderPortfolioSeparator(visible)))
		lines = append(lines, "  "+m.renderPortfolioTotal(totalProfit, totalRate, items[0].Symbol.Currency, visible))
	}
	if len(items) == 0 {
		lines = append(lines, "  보유 종목이 없습니다.")
	}
	return panel.Width(max(60, m.width-4)).Render(strings.Join(lines, "\n"))
}

func joinPortfolioTop(left, right string, width int) string {
	if right == "" {
		return left
	}
	padding := width - lipgloss.Width(left) - lipgloss.Width(right)
	if padding < 2 {
		padding = 2
	}
	return left + strings.Repeat(" ", padding) + right
}

type portfolioColumn struct {
	title string
	width int
	right bool
}

var portfolioColumns = []portfolioColumn{
	{"구분", 8, false}, {"종목명", 16, false}, {"평가손익", 17, true}, {"수익률", 9, true},
	{"잔고수량", 12, true}, {"평가금액", 17, true}, {"매입가", 15, true},
	{"현재가", 15, true}, {"매입금액", 17, true}, {"보유비중", 9, true},
}

func visiblePortfolioColumns(width, start int) []int {
	if start < 0 || start >= len(portfolioColumns) {
		start = 0
	}
	used := 2
	result := make([]int, 0, len(portfolioColumns)-start)
	for i := start; i < len(portfolioColumns); i++ {
		need := portfolioColumns[i].width
		if len(result) > 0 {
			need += 3
		}
		if len(result) > 0 && used+need > width {
			break
		}
		result, used = append(result, i), used+need
	}
	return result
}

func renderPortfolioHeader(columns []int) string {
	cells := make([]string, 0, len(columns))
	for _, index := range columns {
		column := portfolioColumns[index]
		cells = append(cells, fitCell(column.title, column.width, column.right))
	}
	return "  " + muted.Render(strings.Join(cells, " │ "))
}

func (m Model) renderPortfolioRow(p domain.Position, weight decimal.Decimal, columns []int) string {
	values := []string{
		brokerDisplayName(p.Broker),
		p.Symbol.Name,
		m.formatSignedAmount(p.ProfitLoss, p.Symbol.Currency),
		signedPercent(p.ProfitRate),
		p.Quantity.String(),
		m.formatAmount(p.MarketValue, p.Symbol.Currency),
		m.formatAmount(p.AveragePrice, p.Symbol.Currency),
		m.formatAmount(p.CurrentPrice, p.Symbol.Currency),
		m.formatAmount(p.PurchaseValue, p.Symbol.Currency),
		weight.StringFixed(2) + "%",
	}
	cells := make([]string, 0, len(columns))
	for _, index := range columns {
		column := portfolioColumns[index]
		cell := fitCell(values[index], column.width, column.right)
		if index == 2 || index == 3 {
			cell = profitStyle(p.ProfitLoss).Render(cell)
		}
		cells = append(cells, cell)
	}
	return strings.Join(cells, " │ ")
}

func (m Model) renderPortfolioTotal(profit, rate decimal.Decimal, currency domain.Currency, columns []int) string {
	values := []string{"", "합계", m.formatSignedAmount(profit, currency), signedPercent(rate), "", "", "", "", "", ""}
	cells := make([]string, 0, len(columns))
	for _, index := range columns {
		column := portfolioColumns[index]
		cell := fitCell(values[index], column.width, column.right)
		if index == 2 || index == 3 {
			cell = profitStyle(profit).Bold(true).Render(cell)
		} else {
			cell = brand.Render(cell)
		}
		cells = append(cells, cell)
	}
	return strings.Join(cells, " │ ")
}

func brokerDisplayName(broker domain.BrokerID) string {
	switch broker {
	case domain.BrokerKiwoom:
		return "키움"
	case domain.BrokerNH:
		return "NH"
	case domain.BrokerMock:
		return "모의"
	default:
		return string(broker)
	}
}

func renderPortfolioSeparator(columns []int) string {
	parts := make([]string, 0, len(columns))
	for _, index := range columns {
		parts = append(parts, strings.Repeat("─", portfolioColumns[index].width))
	}
	return strings.Join(parts, "─┼─")
}

func portfolioTabName(tab marketFilter) string {
	if tab == filterUS {
		return "미국주식"
	}
	return "한국주식"
}

func profitStyle(value decimal.Decimal) lipgloss.Style {
	if value.IsPositive() {
		return positive
	}
	if value.IsNegative() {
		return negative
	}
	return lipgloss.NewStyle()
}

func signedPercent(value decimal.Decimal) string {
	if value.IsPositive() {
		return "+" + value.StringFixed(2) + "%"
	}
	return value.StringFixed(2) + "%"
}

func fitCell(value string, width int, right bool) string {
	value = trimDisplay(value, width)
	padding := max(0, width-lipgloss.Width(value))
	if right {
		return strings.Repeat(" ", padding) + value
	}
	return value + strings.Repeat(" ", padding)
}

func trimDisplay(value string, width int) string {
	if lipgloss.Width(value) <= width {
		return value
	}
	runes := []rune(value)
	for len(runes) > 0 && lipgloss.Width(string(runes)+"…") > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

func (m Model) searchView() string {
	caret := ""
	if m.searchEditing {
		caret = "█"
	}
	lines := []string{"종목 검색 [" + m.filterName() + "] > " + m.query + caret, "", renderSearchHeader()}
	for i, s := range m.filteredResults() {
		line := renderSearchRow(s)
		prefix := "  "
		if i == m.cursor {
			prefix = brand.Render("> ")
			line = selectedRow.Render(line)
		}
		lines = append(lines, prefix+line)
	}
	if len(m.results) == 0 {
		lines = append(lines, "  이름 또는 종목코드를 입력하세요.")
	}
	return panel.Width(max(60, m.width-4)).Render(strings.Join(lines, "\n"))
}

var searchColumns = []portfolioColumn{
	{"티커", 14, false}, {"종목명", 18, false}, {"종목코드", 12, false},
	{"시장", 10, false}, {"거래소", 10, false}, {"통화", 7, false},
}

func renderSearchHeader() string {
	cells := make([]string, 0, len(searchColumns))
	for _, column := range searchColumns {
		cells = append(cells, fitCell(column.title, column.width, column.right))
	}
	return "  " + muted.Render(strings.Join(cells, " │ "))
}

func renderSearchRow(symbol domain.Symbol) string {
	exchange := symbol.Exchange
	if exchange == "" {
		exchange = "-"
	}
	ticker := symbol.Ticker
	if ticker == "" {
		ticker = symbol.Code
	}
	values := []string{ticker, symbol.Name, symbol.Code, string(symbol.Market), exchange, string(symbol.Currency)}
	cells := make([]string, 0, len(searchColumns))
	for index, column := range searchColumns {
		cells = append(cells, fitCell(values[index], column.width, column.right))
	}
	return strings.Join(cells, " │ ")
}

func (m Model) watchlistView() string {
	items := m.filteredWatchlist()
	lines := []string{"관심종목 [" + m.filterName() + "]", "", renderWatchlistHeader()}
	for i, item := range items {
		q := m.snapshot.Quotes[item.Symbol.Key()]
		line := m.renderWatchlistRow(item, q)
		prefix := "  "
		if i == m.cursor {
			prefix = brand.Render("> ")
			line = selectedRow.Render(line)
		}
		lines = append(lines, prefix+line)
	}
	if len(items) == 0 {
		lines = append(lines, "  관심종목이 없습니다. 검색 결과에서 추후 로컬 관심종목으로 추가할 수 있습니다.")
	}
	return panel.Width(max(60, m.width-4)).Render(strings.Join(lines, "\n"))
}

var watchlistColumns = []portfolioColumn{
	{"구분", 7, false}, {"종목명", 16, false}, {"종목코드", 10, false},
	{"현재가", 15, true}, {"등락률", 10, true},
}

func renderWatchlistHeader() string {
	cells := make([]string, 0, len(watchlistColumns))
	for _, column := range watchlistColumns {
		cells = append(cells, fitCell(column.title, column.width, column.right))
	}
	return "  " + muted.Render(strings.Join(cells, " │ "))
}

func (m Model) renderWatchlistRow(item domain.WatchlistItem, quote domain.Quote) string {
	values := []string{
		watchlistProviderName(item),
		item.Symbol.Name,
		item.Symbol.Code,
		m.formatAmount(quote.Price, item.Symbol.Currency),
		signedPercent(quote.ChangeRate),
	}
	cells := make([]string, 0, len(watchlistColumns))
	for index, column := range watchlistColumns {
		cell := fitCell(values[index], column.width, column.right)
		if index == 4 {
			cell = profitStyle(quote.ChangeRate).Render(cell)
		}
		cells = append(cells, cell)
	}
	return strings.Join(cells, " │ ")
}

func watchlistProviderName(item domain.WatchlistItem) string {
	if item.Provider == domain.BrokerMock && strings.Contains(","+item.GroupID+",", ",default,") {
		return "로컬"
	}
	return brokerDisplayName(item.Provider)
}

func (m Model) moversView() string {
	items := append([]domain.SurgeReport(nil), m.filteredSurges()...)
	sort.Slice(items, func(i, j int) bool { return items[i].Score > items[j].Score })
	lines := []string{"   종목                  점수      등락률       분석 근거"}
	for i, r := range items {
		line := fmt.Sprintf("%-2s %-18s %3d점 %9s%%  %s", cursor(i, m.cursor), trim(r.Symbol.Name, 16), r.Score, r.ChangeRate.StringFixed(2), strings.Join(r.Reasons, ", "))
		lines = append(lines, selectLine(line, i == m.cursor))
	}
	if len(items) == 0 {
		lines = append(lines, "  현재 기준을 통과한 급등 후보가 없습니다.")
	}
	lines = append(lines, "", muted.Render("※ 규칙 기반 관찰 리포트이며 투자 권유가 아닙니다."))
	return panel.Width(max(60, m.width-4)).Render(strings.Join(lines, "\n"))
}

func (m Model) detailView() string {
	interval := intervals[m.intervalIndex]
	if m.loading {
		return fmt.Sprintf("%s(%s) · %s\n\n차트 데이터를 불러오는 중…", m.selected.Name, m.selected.Code, interval.KoreanName())
	}
	legend := ma5Style.Render("MA5") + "  " + ma20Style.Render("MA20") + "  " + ma60Style.Render("MA60") + "  " + ma120Style.Render("MA120")
	exchange := ""
	if m.selected.Exchange != "" {
		exchange = " · " + m.selected.Exchange
	}
	header := fmt.Sprintf("%s (%s) · %s%s · %s  │  %s", m.selected.Name, m.selected.Code, m.selected.Market, exchange, interval.KoreanName(), legend)
	chartHeight := max(10, m.height-10)
	return header + "\n" + panel.Render(renderChart(m.candles, m.width-6, chartHeight))
}

func (m Model) helpView() string {
	return panel.Render("Vim 단축키\n\n↑/↓, j/k 선택      Enter 상세보기      Esc 뒤로/취소\n←/→, h/l 화면 이동  gg/G 처음/끝        Ctrl+u/d 반 페이지\ngt/gT 다음/이전 화면  / 검색 입력         m 관심종목 토글\nf 시장 필터/보유탭  c USD/KRW 표시 전환\n? 도움말\n\n내 주식\nTab/Shift+Tab 한국·미국 탭    [/ ] 표 열 이동\n상세 차트\nh/l 또는 ←/→ 봉 단위 변경\n\n콜론 명령\n:r 새로고침   :s 전체 동기화   :d 연결 진단   :q 종료\n\n상세 차트: 틱·1/5/15/60분·일·주·월·년 / MA5·20·60·120\n조회 전용: 주문 기능 및 주문 API 호출 없음")
}

func (m Model) diagnosticsView() string {
	lines := []string{"연결 및 데이터 진단", ""}
	for _, status := range m.snapshot.Statuses {
		mark := "OFFLINE"
		if status.Connected {
			mark = "CONNECTED"
		}
		lines = append(lines, fmt.Sprintf("%-8s %-10s %-5s %s", status.Broker, mark, status.Mode, status.Message))
	}
	if m.snapshot.FX.Rate.IsPositive() {
		lines = append(lines, "", fmt.Sprintf("USD/KRW  %s · %s · %s", m.snapshot.FX.Rate.StringFixed(2), m.snapshot.FX.Provider, m.snapshot.FX.AsOf.Format("2006-01-02 15:04:05")))
	}
	lines = append(lines, "", "마지막 화면 갱신  "+m.snapshot.LoadedAt.Format("2006-01-02 15:04:05"))
	if len(m.snapshot.Warnings) > 0 {
		lines = append(lines, "", "경고:")
		for _, warning := range m.snapshot.Warnings {
			lines = append(lines, "- "+warning)
		}
	}
	return panel.Width(max(60, m.width-4)).Render(strings.Join(lines, "\n"))
}

func (m Model) footer() string {
	if m.commandMode {
		return brand.Render(" COMMAND :")
	}
	base := " ↑↓/jk 이동  Enter 상세  / 검색  : 명령  ? 도움말"
	return muted.Render(base) + "  │  " + m.connectionIndicator()
}

func (m Model) connectionIndicator() string {
	if len(m.snapshot.Statuses) == 0 {
		return statusDown.Render("● Not Connected")
	}
	connected, disconnected := 0, 0
	for _, status := range m.snapshot.Statuses {
		if status.Connected {
			connected++
		} else {
			disconnected++
		}
	}
	if connected == 0 {
		return statusDown.Render("● Not Connected")
	}
	// An in-flight request is not itself a delay. Only mark stale data after
	// the refresh window has elapsed since the last successful snapshot.
	if !m.snapshot.LoadedAt.IsZero() && time.Since(m.snapshot.LoadedAt) > m.refreshEvery {
		return statusWait.Render("● Delayed")
	}
	return statusOK.Render("● Connected")
}
func money(v decimal.Decimal) string { return v.Round(0).StringFixed(0) }
func signedMoney(v decimal.Decimal) string {
	if v.IsPositive() {
		return "+" + money(v)
	}
	return money(v)
}
func signedUSD(v decimal.Decimal) string {
	if v.IsPositive() {
		return "+$" + v.StringFixed(2)
	}
	if v.IsNegative() {
		return "-$" + v.Abs().StringFixed(2)
	}
	return "$0.00"
}
func (m Model) formatAmount(v decimal.Decimal, currency domain.Currency) string {
	if currency != domain.USD {
		return "₩" + money(v)
	}
	usd := "$" + v.StringFixed(2)
	krw := "₩-"
	if m.snapshot.FX.Rate.IsPositive() {
		krw = "₩" + money(v.Mul(m.snapshot.FX.Rate))
	}
	switch m.currency {
	case currencyNative:
		return usd
	case currencyKRW:
		return krw
	default:
		return usd + " / " + krw
	}
}
func (m Model) formatSignedAmount(v decimal.Decimal, currency domain.Currency) string {
	if currency != domain.USD {
		return signedMoney(v) + "원"
	}
	usd := signedUSD(v)
	krw := "₩-"
	if m.snapshot.FX.Rate.IsPositive() {
		krw = signedMoney(v.Mul(m.snapshot.FX.Rate)) + "원"
	}
	switch m.currency {
	case currencyNative:
		return usd
	case currencyKRW:
		return krw
	default:
		return usd + " / " + krw
	}
}
func cursor(i, current int) string {
	if i == current {
		return ">"
	}
	return " "
}
func selectLine(s string, on bool) string {
	if on {
		return selected.Render(s)
	}
	return s
}
func trim(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
