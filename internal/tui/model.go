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
	"github.com/77romin/minstock-tui/internal/app"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/shopspring/decimal"
)

type screen int

const (
	dashboardScreen screen = iota
	portfolioScreen
	searchScreen
	watchlistScreen
	moversScreen
	performanceScreen
	dividendScreen
	allocationScreen
	alertScreen
	newsFeedScreen
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
	currencyNative currencyDisplay = iota
	currencyKRW
)

type performanceDisplay int

const (
	performanceTable performanceDisplay = iota
	performanceChart
)

type dividendDisplay int

const (
	dividendHoldings dividendDisplay = iota
	dividendCalendar
)

type alertDisplay int

const (
	alertHistory alertDisplay = iota
	alertRules
)

type performancePeriod int

const (
	performanceDaily performancePeriod = iota
	performanceWeekly
	performanceMonthly
	performanceYearly
)

var intervals = []domain.CandleInterval{
	domain.IntervalTick, domain.Interval1Min, domain.Interval5Min, domain.Interval15Min,
	domain.Interval60Min, domain.IntervalDay, domain.IntervalWeek, domain.IntervalMonth, domain.IntervalYear,
}

type Model struct {
	service                *app.Service
	mode                   string
	screen                 screen
	previous               screen
	width, height          int
	cursor                 int
	loading                bool
	err                    error
	snapshot               app.Snapshot
	history                []domain.PortfolioSnapshot
	dividends              app.DividendReport
	query                  string
	results                []domain.Symbol
	selected               domain.Symbol
	candles                []domain.Candle
	detailQuote            domain.Quote
	intervalIndex          int
	maVisible              [4]bool
	refreshEvery           time.Duration
	filter                 marketFilter
	portfolioTab           marketFilter
	portfolioCol           int
	portfolioSort          int
	currency               currencyDisplay
	performance            performanceDisplay
	perfPeriod             performancePeriod
	dividendDisplay        dividendDisplay
	commandMode            bool
	pendingG               bool
	searchEditing          bool
	notice                 string
	refreshing             bool
	enriching              bool
	syncing                bool
	liveLoaded             bool
	dividendLoading        bool
	dividendHoldingsKey    string
	allocation             app.AllocationReport
	allocationScope        int
	allocationEditing      bool
	allocationAdding       bool
	allocationPending      bool
	allocationDirty        bool
	allocationInput        string
	rebalanceView          bool
	rebalanceEditing       bool
	rebalanceInput         string
	rebalanceBudget        decimal.Decimal
	alerts                 app.AlertReport
	alertDisplay           alertDisplay
	alertAdding            bool
	alertInput             string
	scanner                app.ScannerReport
	scannerLoading         bool
	scannerAttempt         time.Time
	information            app.InformationReport
	newsPreferences        domain.NewsPreferences
	newsPreferencesEdited  bool
	newsPreferencesPending bool
	newsPreferencesLoading bool
	newsKeywordEditing     bool
	newsKeywordInput       string
	informationTab         bool
	informationInfo        bool
	chartErr               error
	informationCursor      int
	informationLoading     bool
	informationLive        bool
	informationRequest     uint64
	informationPendingG    bool
	feed                   app.NewsFeedReport
	feedWatchlist          bool
	feedLoading            bool
	feedUSOffset           int
	feedUS                 bool
	feedInfo               bool
	feedLive               bool
	feedUnread             bool
	feedKind               int
	feedSymbol             int
	feedOffset             int
	feedWarningPage        int
	feedRequest            uint64
	feedCancel             context.CancelFunc
	feedReadChanges        map[string]time.Time
	feedReadPending        map[string]bool
}

type cachedDashboardMsg struct {
	snapshot app.Snapshot
	err      error
}
type dashboardMsg struct{ snapshot app.Snapshot }
type enrichmentMsg struct{ snapshot app.Snapshot }
type performanceMsg struct {
	history []domain.PortfolioSnapshot
	err     error
}
type dividendMsg struct{ report app.DividendReport }
type cachedDividendMsg struct {
	report      app.DividendReport
	holdingsKey string
}
type allocationMsg struct {
	report app.AllocationReport
	err    error
}
type allocationSavedMsg struct{ err error }
type allocationSymbolMsg struct {
	symbol domain.Symbol
	err    error
}
type alertMsg struct {
	report app.AlertReport
	err    error
}
type alertActionMsg struct{ err error }
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
type quoteMsg struct {
	symbol domain.Symbol
	quote  domain.Quote
	err    error
}
type syncMsg struct{ errs []error }
type watchlistMsg struct{ err error }
type refreshMsg time.Time
type scannerMsg struct{ report app.ScannerReport }

func New(service *app.Service, mode string, refreshEvery time.Duration) Model {
	if refreshEvery < time.Second {
		refreshEvery = 5 * time.Second
	}
	return Model{service: service, mode: mode, loading: true, refreshing: true, newsPreferencesLoading: true, width: 100, height: 30, intervalIndex: 5, refreshEvery: refreshEvery, portfolioTab: filterKR, maVisible: [4]bool{true, true, true, true}}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.cachedDashboardCmd(), m.dashboardCmd(), m.performanceCmd(), m.alertCmd(), m.newsPreferencesCmd(), m.tickCmd())
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

func (m Model) performanceCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		to := time.Now()
		history, err := m.service.PortfolioHistory(ctx, time.Time{}, to)
		return performanceMsg{history: history, err: err}
	}
}

func (m Model) dividendCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return dividendMsg{report: m.service.DividendPortfolio(ctx, m.snapshot.Positions, m.snapshot.FX)}
	}
}

func (m Model) cachedDividendCmd(snapshot app.Snapshot) tea.Cmd {
	holdingsKey := dividendHoldingsKey(snapshot.Positions)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		return cachedDividendMsg{
			report:      m.service.CachedDividendPortfolio(ctx, snapshot.Positions, snapshot.FX),
			holdingsKey: holdingsKey,
		}
	}
}

func (m Model) allocationCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		report, err := m.service.AllocationReport(ctx, m.allocationScopeName(), m.snapshot)
		return allocationMsg{report, err}
	}
}
func (m Model) saveAllocationCmd() tea.Cmd {
	targets := make([]domain.AllocationTarget, len(m.allocation.Rows))
	for i, row := range m.allocation.Rows {
		targets[i] = row.Target
	}
	scope := m.allocationScopeName()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return allocationSavedMsg{m.service.SaveAllocationTargets(ctx, scope, targets)}
	}
}

func (m Model) alertCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		report, err := m.service.AlertReport(ctx)
		return alertMsg{report: report, err: err}
	}
}

func (m Model) evaluateAlertsCmd(snapshot app.Snapshot) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := m.service.EvaluateAlerts(ctx, snapshot); err != nil {
			return alertMsg{err: err}
		}
		report, err := m.service.AlertReport(ctx)
		return alertMsg{report: report, err: err}
	}
}

func (m Model) savePriceAlertCmd(input string) tea.Cmd {
	snapshot := m.snapshot
	return func() tea.Msg {
		fields := strings.Fields(input)
		if len(fields) != 2 {
			return alertActionMsg{err: fmt.Errorf("입력 형식: 티커 목표가 (예: VOO 700)")}
		}
		target, err := decimal.NewFromString(strings.ReplaceAll(fields[1], ",", ""))
		if err != nil || !target.IsPositive() {
			return alertActionMsg{err: fmt.Errorf("목표가는 0보다 큰 숫자여야 합니다")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		results, err := m.service.Search(ctx, fields[0])
		if err != nil {
			return alertActionMsg{err: err}
		}
		query := strings.ToUpper(fields[0])
		var symbol domain.Symbol
		for _, candidate := range results {
			if strings.ToUpper(candidate.Ticker) == query || strings.ToUpper(candidate.Code) == query {
				symbol = candidate
				break
			}
		}
		if symbol.Code == "" && len(results) > 0 {
			symbol = results[0]
		}
		if symbol.Code == "" {
			return alertActionMsg{err: fmt.Errorf("종목 %q을 찾지 못했습니다", fields[0])}
		}
		current := snapshot.Quotes[symbol.Key()].Price
		if current.IsZero() {
			for _, position := range snapshot.Positions {
				if position.Symbol.Key() == symbol.Key() {
					current = position.CurrentPrice
					break
				}
			}
		}
		return alertActionMsg{err: m.service.SavePriceAlert(ctx, symbol, target, current)}
	}
}

func (m Model) deletePriceAlertCmd(id int64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return alertActionMsg{err: m.service.DeletePriceAlert(ctx, id)}
	}
}

func (m Model) acknowledgeAlertCmd(id int64) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return alertActionMsg{err: m.service.AcknowledgeAlert(ctx, id)}
	}
}

func (m Model) acknowledgeAllAlertsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return alertActionMsg{err: m.service.AcknowledgeAllAlerts(ctx)}
	}
}

func (m Model) allocationSymbolCmd(query string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		results, err := m.service.Search(ctx, query)
		if err != nil {
			return allocationSymbolMsg{err: err}
		}
		query = strings.ToUpper(strings.TrimSpace(query))
		for _, symbol := range results {
			if strings.ToUpper(symbol.Ticker) == query || strings.ToUpper(symbol.Code) == query {
				return allocationSymbolMsg{symbol: symbol}
			}
		}
		if len(results) > 0 {
			return allocationSymbolMsg{symbol: results[0]}
		}
		return allocationSymbolMsg{err: fmt.Errorf("종목 %q을 찾지 못했습니다", query)}
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

func (m Model) scannerCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		return scannerMsg{report: m.service.ScanSurges(ctx)}
	}
}

func (m Model) startScanner(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	interval := m.scanner.Options.RefreshInterval
	if interval < time.Minute {
		interval = time.Minute
	}
	if m.service != nil && m.screen == moversScreen && !m.scannerLoading && time.Since(m.scannerAttempt) >= interval {
		m.scannerLoading, m.scannerAttempt = true, time.Now()
		return m, tea.Batch(cmd, m.scannerCmd())
	}
	return m, cmd
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

func (m Model) quoteCmd(symbol domain.Symbol) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		q, err := m.service.Quote(ctx, symbol)
		return quoteMsg{symbol: symbol, quote: q, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case newsFeedMsg:
		if msg.request == m.feedRequest && (!msg.cached || !m.feedLive) {
			selectedKey := ""
			if m.feedSymbol > 0 && m.feedSymbol <= len(m.feed.Symbols) {
				selectedKey = app.NewsFeedSymbolKey(m.feed.Symbols[m.feedSymbol-1])
			}
			m.feed = msg.report
			if selectedKey != "" {
				m.feedSymbol = 0
				for i, symbol := range m.feed.Symbols {
					if app.NewsFeedSymbolKey(symbol) == selectedKey {
						m.feedSymbol = i + 1
						break
					}
				}
			}
			for i := range m.feed.Entries {
				if at, ok := m.feedReadChanges[m.feed.Entries[i].ID]; ok {
					m.feed.Entries[i].ReadAt = at
				}
			}
			if !msg.cached {
				m.feedLoading, m.feedLive = false, true
			}
			if m.screen == newsFeedScreen {
				m.cursor = min(m.cursor, max(0, len(m.filteredFeed())-1))
			}
		}
	case newsFeedReadMsg:
		delete(m.feedReadPending, msg.id)
		if msg.err != nil {
			m.notice = "읽음 상태 저장 실패: " + msg.err.Error()
		} else {
			if m.feedReadChanges == nil {
				m.feedReadChanges = map[string]time.Time{}
			}
			m.feedReadChanges[msg.id] = msg.at
			for i := range m.feed.Entries {
				if m.feed.Entries[i].ID == msg.id {
					m.feed.Entries[i].ReadAt = msg.at
				}
			}
			if m.screen == newsFeedScreen {
				m.cursor = min(m.cursor, max(0, len(m.filteredFeed())-1))
			}
			m.notice = "읽음 상태를 저장했습니다"
		}
	case newsFeedOpenedMsg:
		if msg.err != nil {
			m.notice = "원문 열기 실패: " + msg.err.Error()
		} else {
			cmd := m.feedMarkRead(msg.id, true)
			return m, cmd
		}
	case informationMsg:
		if msg.symbol.Key() == m.selected.Key() && msg.request == m.informationRequest && (!msg.cached || !m.informationLive) {
			m.information = msg.report
			if !msg.cached {
				m.informationLoading, m.informationLive = false, true
			}
			m.informationCursor = min(m.informationCursor, max(0, len(m.filteredInformation())-1))
		}
	case informationOpenedMsg:
		if msg.err != nil {
			m.notice = "원문 열기 실패: " + msg.err.Error()
		} else {
			m.notice = "원문을 브라우저로 열었습니다"
		}
	case scannerMsg:
		m.scanner, m.scannerLoading = msg.report, false
		m.cursor = min(m.cursor, max(0, len(m.filteredSurges())-1))
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case cachedDashboardMsg:
		if msg.err == nil && !m.liveLoaded {
			m.snapshot = msg.snapshot
			m.notice = "캐시 표시 · 최신 데이터 확인 중"
			m.dividendHoldingsKey = dividendHoldingsKey(msg.snapshot.Positions)
			if m.dividendHoldingsKey != "" {
				return m, m.cachedDividendCmd(msg.snapshot)
			}
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
		commands := []tea.Cmd{m.enrichmentCmd(msg.snapshot), m.performanceCmd()}
		key := dividendHoldingsKey(msg.snapshot.Positions)
		if m.screen == dividendScreen && key != m.dividendHoldingsKey {
			m.dividendHoldingsKey = key
			m.dividendLoading = len(m.dividends.Holdings) == 0
			commands = append(commands, m.dividendCmd())
		} else if m.screen != dividendScreen {
			m.dividendHoldingsKey = key
		}
		if m.screen == allocationScreen && !m.allocationDirty && !m.allocationEditing && !m.allocationAdding && !m.allocationPending {
			commands = append(commands, m.allocationCmd())
		}
		return m, tea.Batch(commands...)
	case enrichmentMsg:
		m.snapshot, m.enriching = msg.snapshot, false
		m.notice = "최신 데이터"
		return m, m.evaluateAlertsCmd(msg.snapshot)
	case newsPreferencesMsg:
		m.newsPreferencesLoading = false
		if msg.err != nil {
			m.notice = "뉴스 필터 설정을 불러오지 못했습니다"
		} else if !m.newsPreferencesEdited {
			m.newsPreferences = msg.preferences
			m.informationCursor = min(m.informationCursor, max(0, len(m.filteredInformation())-1))
			if m.screen == newsFeedScreen {
				m.cursor = min(m.cursor, max(0, len(m.filteredFeed())-1))
			}
		}
	case newsPreferencesSavedMsg:
		m.newsPreferencesPending = false
		if msg.err != nil {
			m.notice = "뉴스 필터는 적용됐지만 설정 저장에 실패했습니다"
		}
	case performanceMsg:
		if msg.err == nil {
			m.history = msg.history
		} else {
			m.err = msg.err
		}
	case dividendMsg:
		m.dividends, m.dividendLoading = msg.report, false
	case cachedDividendMsg:
		if msg.holdingsKey == m.dividendHoldingsKey && len(m.dividends.Holdings) == 0 && len(msg.report.Holdings) > 0 {
			m.dividends = msg.report
		}
	case alertMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.alerts = msg.report
			if m.screen == alertScreen && m.cursor >= m.itemCount() {
				m.cursor = max(0, m.itemCount()-1)
			}
		}
	case alertActionMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.alertAdding, m.alertInput = false, ""
			m.notice = "알림 설정을 갱신했습니다"
			return m, m.alertCmd()
		}
	case allocationMsg:
		if msg.report.Scope != "" && msg.report.Scope != m.allocationScopeName() {
			break
		}
		if m.allocationDirty || m.allocationEditing || m.allocationAdding || m.allocationPending {
			break
		}
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.allocation = msg.report
		}
	case allocationSavedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.allocationDirty = false
			m.notice = "목표 비중을 저장했습니다"
			return m, m.allocationCmd()
		}
	case allocationSymbolMsg:
		m.allocationPending = false
		if msg.err != nil {
			m.err = msg.err
			break
		}
		for _, row := range m.allocation.Rows {
			if row.Target.Symbol.Key() == msg.symbol.Key() {
				m.notice = "이미 목표 목록에 있는 종목입니다"
				return m, nil
			}
		}
		m.allocation.Rows = append(m.allocation.Rows, app.AllocationRow{Target: domain.AllocationTarget{Scope: m.allocationScopeName(), Symbol: msg.symbol}})
		m.allocationDirty = true
		m.cursor = len(m.allocation.Rows) - 1
		m.notice = msg.symbol.Name + "을 목표 목록에 추가했습니다"
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
			m.candles, m.chartErr, m.loading = msg.candles, msg.err, false
		}
	case quoteMsg:
		if msg.symbol.Key() == m.selected.Key() && msg.err == nil {
			m.detailQuote = msg.quote
		}
	case refreshMsg:
		nextTick := m.tickCmd()
		if m.screen == moversScreen {
			return m.startScanner(nextTick)
		}
		if m.liveLoaded && time.Since(m.snapshot.LoadedAt) < m.refreshEvery {
			return m, nextTick
		}
		if m.screen != detailScreen && !m.refreshing && !m.enriching && !m.syncing {
			m.refreshing = true
			m.notice = "최신 데이터 확인 중"
			return m, tea.Batch(m.dashboardCmd(), nextTick)
		}
		return m, nextTick
	case tea.PasteMsg:
		text := domain.InformationText(msg.Content)
		switch {
		case (m.screen == newsFeedScreen || (m.screen == detailScreen && m.informationTab)) && m.newsKeywordEditing:
			m.newsKeywordInput = boundedInput(m.newsKeywordInput+text, 80)
		case m.screen == allocationScreen && m.rebalanceEditing:
			if len(text) <= 20 && strings.Trim(text, "0123456789,") == "" {
				m.rebalanceInput = boundedInput(m.rebalanceInput+text, 20)
			}
		}
		return m, nil
	case tea.KeyPressMsg:
		updated, cmd := m.handleKey(msg.String())
		if next, ok := updated.(Model); ok {
			if m.screen == newsFeedScreen && next.screen != newsFeedScreen && next.feedCancel != nil {
				next.feedCancel()
				next.feedLoading = false
				next.feedRequest++
			}
			if next.screen == newsFeedScreen && m.screen != newsFeedScreen && cmd == nil {
				loaded, loadCmd := next.loadNewsFeed()
				next, cmd = loaded.(Model), loadCmd
			}
			return next.startScanner(cmd)
		}
		return updated, cmd
	}
	return m, nil
}

func (m Model) handleKey(key string) (tea.Model, tea.Cmd) {
	if key == "ctrl+c" {
		if m.feedCancel != nil {
			m.feedCancel()
		}
		return m, tea.Quit
	}
	if !m.commandMode && (m.screen == newsFeedScreen || (m.screen == detailScreen && m.informationTab)) {
		if next, cmd, handled := m.handleNewsQualityKey(key); handled {
			return next, cmd
		}
	}
	if !m.commandMode && m.screen == allocationScreen {
		if next, cmd, handled := m.handleRebalanceKey(key); handled {
			return next, cmd
		}
	}
	if m.screen == allocationScreen && (m.allocationEditing || m.allocationAdding) {
		return m.handleAllocationInput(key)
	}
	if m.screen == alertScreen && m.alertAdding {
		return m.handleAlertInput(key)
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
		case "q", "ㅂ":
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
	if m.screen == newsFeedScreen {
		if next, cmd, handled := m.handleFeedKey(key); handled {
			if model, ok := next.(Model); ok {
				model.pendingG = false
				next = model
			}
			return next, cmd
		}
	}
	if m.screen == detailScreen && m.informationTab {
		if next, cmd, handled := m.handleInformationKey(key); handled {
			return next, cmd
		}
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
		if key == "tab" || key == "shift+tab" {
			m.informationTab = !m.informationTab
			if m.informationTab {
				return m.loadInformation()
			}
			return m, nil
		}
		if key >= "1" && key <= "4" {
			index := int(key[0] - '1')
			m.maVisible[index] = !m.maVisible[index]
			return m, nil
		}
		if key == "left" || key == "h" {
			m.intervalIndex = (m.intervalIndex - 1 + len(intervals)) % len(intervals)
			m.loading = true
			m.chartErr = nil
			return m, m.candlesCmd(m.selected)
		}
		if key == "right" || key == "l" {
			m.intervalIndex = (m.intervalIndex + 1) % len(intervals)
			m.loading = true
			m.chartErr = nil
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
	case "6":
		m.screen, m.cursor = performanceScreen, 0
	case "7":
		m.screen, m.cursor, m.dividendLoading = dividendScreen, 0, len(m.dividends.Holdings) == 0
		m.dividendHoldingsKey = dividendHoldingsKey(m.snapshot.Positions)
		return m, m.dividendCmd()
	case "8":
		m.screen, m.cursor = allocationScreen, 0
		if m.allocationDirty {
			return m, nil
		}
		return m, m.allocationCmd()
	case "9":
		m.screen, m.cursor = alertScreen, 0
		return m, m.alertCmd()
	case "0":
		m.screen, m.cursor = newsFeedScreen, 0
		return m.loadNewsFeed()
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
		} else if m.screen == performanceScreen {
			m.performance = (m.performance + 1) % 2
			m.notice = "성과 보기: " + m.performanceDisplayName()
		} else if m.screen == dividendScreen {
			m.dividendDisplay = (m.dividendDisplay + 1) % 2
			m.notice = "배당 보기: " + m.dividendDisplayName()
		} else if m.screen == allocationScreen {
			if m.allocationDirty {
				m.notice = "편집 내용이 있습니다. s로 저장하거나 u로 되돌린 뒤 범위를 바꾸세요"
				return m, nil
			}
			m.allocationScope = (m.allocationScope + 1) % 3
			m.cursor = 0
			return m, m.allocationCmd()
		} else if m.screen == alertScreen {
			m.alertDisplay = (m.alertDisplay + 1) % 2
			m.cursor = 0
		}
	case "shift+tab":
		if m.screen == portfolioScreen {
			m.switchPortfolioTab(-1)
		} else if m.screen == performanceScreen {
			m.performance = (m.performance + 1) % 2
			m.notice = "성과 보기: " + m.performanceDisplayName()
		} else if m.screen == dividendScreen {
			m.dividendDisplay = (m.dividendDisplay + 1) % 2
			m.notice = "배당 보기: " + m.dividendDisplayName()
		} else if m.screen == allocationScreen {
			if m.allocationDirty {
				m.notice = "편집 내용이 있습니다. s로 저장하거나 u로 되돌린 뒤 범위를 바꾸세요"
				return m, nil
			}
			m.allocationScope = (m.allocationScope + 2) % 3
			m.cursor = 0
			return m, m.allocationCmd()
		} else if m.screen == alertScreen {
			m.alertDisplay = (m.alertDisplay + 1) % 2
			m.cursor = 0
		}
	case "right", "l":
		if m.screen >= dashboardScreen && m.screen <= newsFeedScreen {
			m.nextPrimaryScreen(1)
			if m.screen == allocationScreen {
				if m.allocationDirty {
					return m, nil
				}
				return m, m.allocationCmd()
			}
		}
	case "left", "h":
		if m.screen >= dashboardScreen && m.screen <= newsFeedScreen {
			m.nextPrimaryScreen(-1)
			if m.screen == allocationScreen {
				if m.allocationDirty {
					return m, nil
				}
				return m, m.allocationCmd()
			}
		}
	case "]":
		if m.screen == portfolioScreen && m.portfolioCol < len(portfolioColumns)-1 {
			m.portfolioCol++
		}
	case "[":
		if m.screen == portfolioScreen && m.portfolioCol > 0 {
			m.portfolioCol--
		}
	case "t":
		if m.screen == portfolioScreen {
			m.portfolioSort = (m.portfolioSort + 1) % 3
			m.cursor = 0
		} else if m.screen == performanceScreen && m.performance == performanceChart {
			m.perfPeriod = (m.perfPeriod + 1) % 4
			m.notice = "성과 주기: " + m.performancePeriodName()
		}
	case "c":
		m.currency = (m.currency + 1) % 2
		m.notice = "통화 표시: " + m.currencyName()
	case "e":
		if m.screen == allocationScreen && m.cursor < len(m.allocation.Rows) {
			m.allocationEditing = true
			m.allocationInput = ""
			return m, nil
		}
	case "a":
		if m.screen == allocationScreen {
			m.allocationAdding, m.allocationInput = true, ""
			m.notice = "추가할 종목의 티커 또는 코드를 입력하세요"
			return m, nil
		} else if m.screen == alertScreen && m.alertDisplay == alertRules {
			m.alertAdding, m.alertInput = true, ""
			m.notice = "티커와 목표가를 입력하세요 (예: VOO 700)"
			return m, nil
		}
	case "r":
		if m.screen == allocationScreen {
			copyCurrentAllocationTargets(&m.allocation)
			m.allocationDirty = true
			m.notice = "현재 비중을 목표로 복사했습니다 · 합계 100.00% 보정 완료"
		}
	case "s":
		if m.screen == allocationScreen {
			return m, m.saveAllocationCmd()
		}
	case "d":
		if m.screen == allocationScreen && m.cursor < len(m.allocation.Rows) {
			m.allocation.Rows[m.cursor].Target.TargetPercent = decimal.Zero
			m.allocationDirty = true
			m.notice = "목표 비중을 0%로 변경했습니다"
		} else if m.screen == alertScreen && m.alertDisplay == alertRules && m.cursor < len(m.alerts.Rules) {
			return m, m.deletePriceAlertCmd(m.alerts.Rules[m.cursor].ID)
		}
	case "x":
		if m.screen == alertScreen && m.alertDisplay == alertHistory && m.cursor < len(m.alerts.Events) {
			return m, m.acknowledgeAlertCmd(m.alerts.Events[m.cursor].ID)
		}
	case "X":
		if m.screen == alertScreen && m.alertDisplay == alertHistory {
			return m, m.acknowledgeAllAlertsCmd()
		}
	case "u":
		if m.screen == allocationScreen {
			m.allocationDirty = false
			m.allocationEditing, m.allocationAdding, m.allocationPending = false, false, false
			m.notice = "저장하지 않은 편집을 되돌렸습니다"
			return m, m.allocationCmd()
		}
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
		if m.screen == allocationScreen && m.cursor < len(m.allocation.Rows) {
			m.allocationEditing = true
			m.allocationInput = ""
			return m, nil
		}
		if symbol, ok := m.currentSymbol(); ok {
			return m.openDetail(symbol, m.screen)
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
		case "6":
			m.screen, m.cursor = performanceScreen, 0
		case "7":
			m.screen, m.cursor, m.dividendLoading = dividendScreen, 0, len(m.dividends.Holdings) == 0
			m.dividendHoldingsKey = dividendHoldingsKey(m.snapshot.Positions)
			return m, m.dividendCmd()
		case "8":
			m.screen, m.cursor = allocationScreen, 0
			if m.allocationDirty {
				return m, nil
			}
			return m, m.allocationCmd()
		case "9":
			m.screen, m.cursor = alertScreen, 0
			return m, m.alertCmd()
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
				return m.openDetail(symbol, searchScreen)
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
	screens := []screen{dashboardScreen, portfolioScreen, searchScreen, watchlistScreen, moversScreen, performanceScreen, dividendScreen, allocationScreen, alertScreen, newsFeedScreen}
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
	sort.SliceStable(result, func(i, j int) bool {
		switch m.portfolioSort {
		case 0:
			return result[i].ProfitLoss.GreaterThan(result[j].ProfitLoss)
		case 1:
			return result[i].ProfitRate.GreaterThan(result[j].ProfitRate)
		case 2:
			// Within one market tab, weight has the same ordering as value.
			return result[i].MarketValue.GreaterThan(result[j].MarketValue)
		}
		return false
	})
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
	result := make([]domain.SurgeReport, 0, len(m.scanner.Reports))
	for _, report := range m.scanner.Reports {
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
		return "$ / ₩"
	case currencyKRW:
		return "₩"
	default:
		return "$ / ₩"
	}
}

func (m Model) performanceDisplayName() string {
	if m.performance == performanceChart {
		return "그래프"
	}
	return "표"
}

func (m Model) dividendDisplayName() string {
	if m.dividendDisplay == dividendCalendar {
		return "월별"
	}
	return "종목별"
}

func (m Model) performancePeriodName() string {
	switch m.perfPeriod {
	case performanceWeekly:
		return "주간"
	case performanceMonthly:
		return "월간"
	case performanceYearly:
		return "연간"
	default:
		return "일간"
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
	case performanceScreen:
		return len(m.performancePoints())
	case dividendScreen:
		return len(m.dividends.Holdings)
	case allocationScreen:
		return len(m.allocation.Rows)
	case alertScreen:
		if m.alertDisplay == alertRules {
			return len(m.alerts.Rules)
		}
		return len(m.alerts.Events)
	case newsFeedScreen:
		return len(m.filteredFeed())
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
	neutral     = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	statusOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	statusWait  = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	statusDown  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	activeTab   = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("86"))
	sortHeader  = lipgloss.NewStyle().Underline(true)
	panel       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
)

func (m Model) header() string {
	labels := []string{"1 현황", "2 내 주식", "3 검색", "4 관심", "5 급등", "6 성과", "7 배당", "8 비중", "9 알림", "0 뉴스"}
	gap := "  "
	if m.width < 110 {
		labels = []string{"현황", "주식", "검색", "관심", "급등", "성과", "배당", "비중", "알림", "뉴스"}
		gap = " "
	}
	active := m.screen
	if active == detailScreen || active == helpScreen || active == diagnosticsScreen {
		active = m.previous
	}
	if active < dashboardScreen || active > newsFeedScreen {
		active = dashboardScreen
	}
	tabs := make([]string, len(labels))
	for i, label := range labels {
		if screen(i) == active {
			tabs[i] = activeTab.Render(label)
		} else {
			tabs[i] = label
		}
	}
	return brand.Render(" MINSTOCK ") + "  " + strings.Join(tabs, gap) + "  " + muted.Render("["+m.mode+"]")
}

func (m Model) body() string {
	if m.loading && len(m.snapshot.Balances) == 0 && m.screen != detailScreen && m.screen != newsFeedScreen {
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
	case performanceScreen:
		body = m.performanceView()
	case dividendScreen:
		body = m.dividendView()
	case allocationScreen:
		body = m.allocationView()
	case alertScreen:
		body = m.alertView()
	case newsFeedScreen:
		body = m.newsFeedView()
	case detailScreen:
		body = m.detailView()
	case helpScreen:
		body = m.helpView()
	case diagnosticsScreen:
		body = m.diagnosticsView()
	}
	if m.err != nil && m.screen != newsFeedScreen {
		body += "\n" + negative.Render("오류: "+m.err.Error())
	}
	if m.chartErr != nil && m.screen == detailScreen && !m.informationTab {
		body += "\n" + negative.Render(trimDisplay("차트 조회 오류: "+domain.InformationText(m.chartErr.Error()), max(40, m.width-2)))
	}
	return body
}

func (m Model) dashboardView() string {
	summaries := brokerAssetSummaries(m.snapshot)
	totalValue, totalProfit := decimal.Zero, decimal.Zero
	usdValue, usdProfit := decimal.Zero, decimal.Zero
	for _, summary := range summaries {
		if !summary.HasData {
			continue
		}
		totalValue = totalValue.Add(summary.TotalAssetsKRW)
		totalProfit = totalProfit.Add(summary.ProfitLossKRW)
		usdValue = usdValue.Add(summary.USAssets)
		usdProfit = usdProfit.Add(summary.USProfitLoss)
	}
	fx := "-"
	if !m.snapshot.FX.Rate.IsZero() {
		fx = m.snapshot.FX.Rate.StringFixed(2) + "원"
	}
	dataFreshness := snapshotFreshness(m.snapshot, summaries)
	dataBadge := " [" + freshnessName(dataFreshness) + "]"
	fxBadge := ""
	if m.snapshot.FX.Rate.IsPositive() {
		fxFreshness := m.snapshot.FX.Freshness
		if m.snapshot.Cached {
			fxFreshness = domain.FreshCached
		}
		fxBadge = fmt.Sprintf(" [%s·%s]", brokerDisplayName(m.snapshot.FX.Provider), freshnessName(fxFreshness))
	}
	statusLegend := ""
	if m.width < 100 {
		dataBadge = " " + freshnessMark(dataFreshness)
		if m.snapshot.FX.Rate.IsPositive() {
			fxFreshness := m.snapshot.FX.Freshness
			if m.snapshot.Cached {
				fxFreshness = domain.FreshCached
			}
			fxBadge = fmt.Sprintf(" [%s·%s]", brokerDisplayName(m.snapshot.FX.Provider), freshnessMark(fxFreshness))
		}
		statusLegend = "\n  상태  ●실시간  ◐혼합  ○캐시"
	}
	totalProfitText := directionalValueStyle(totalProfit).Render(signedMoney(totalProfit) + "원")
	usdProfitText := directionalValueStyle(usdProfit).Render(signedUSD(usdProfit))
	left := panel.Width(max(28, m.width/2-4)).Render(fmt.Sprintf("통합 자산\n\n  원화환산  %s원%s\n  평가손익  %s%s\n  미국자산  $%s%s\n  미국손익  %s%s\n  USD/KRW   %s%s%s", money(totalValue), dataBadge, totalProfitText, dataBadge, usdValue.StringFixed(2), dataBadge, usdProfitText, dataBadge, fx, fxBadge, statusLegend))
	var status []string
	for _, s := range m.snapshot.Statuses {
		mark := "○"
		if s.Connected {
			mark = "●"
		}
		status = append(status, fmt.Sprintf("%s %-7s %-5s %s", mark, s.Broker, s.Mode, s.Message))
	}
	right := panel.Width(max(28, m.width/2-4)).Render("연결 상태\n\n  " + strings.Join(status, "\n  "))
	top := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
	return top + "\n" + m.brokerAssetBreakdownView(summaries, totalValue)
}

type brokerAssetSummary struct {
	Broker         domain.BrokerID
	HasData        bool
	TotalAssetsKRW decimal.Decimal
	PurchaseKRW    decimal.Decimal
	ProfitLossKRW  decimal.Decimal
	USAssets       decimal.Decimal
	USProfitLoss   decimal.Decimal
	ExchangeRate   decimal.Decimal
	Freshness      domain.Freshness
	AsOf           time.Time
}

func normalizeFreshness(freshness domain.Freshness) domain.Freshness {
	if freshness == "" {
		return domain.FreshLive
	}
	return freshness
}

func mergeFreshness(current, next domain.Freshness) domain.Freshness {
	next = normalizeFreshness(next)
	if current == "" || current == next {
		return next
	}
	return domain.FreshMixed
}

func freshnessName(freshness domain.Freshness) string {
	switch freshness {
	case domain.FreshCached:
		return "캐시"
	case domain.FreshDelayed:
		return "지연"
	case domain.FreshMixed:
		return "일부 캐시"
	default:
		return "실시간"
	}
}

func freshnessMark(freshness domain.Freshness) string {
	switch freshness {
	case domain.FreshCached:
		return "○"
	case domain.FreshDelayed, domain.FreshMixed:
		return "◐"
	default:
		return "●"
	}
}

func snapshotFreshness(snapshot app.Snapshot, summaries []brokerAssetSummary) domain.Freshness {
	if snapshot.Cached {
		return domain.FreshCached
	}
	result := domain.Freshness("")
	for _, summary := range summaries {
		if summary.HasData {
			result = mergeFreshness(result, summary.Freshness)
		}
	}
	return normalizeFreshness(result)
}

func brokerAssetSummaries(snapshot app.Snapshot) []brokerAssetSummary {
	byBroker := make(map[domain.BrokerID]*brokerAssetSummary)
	ensure := func(broker domain.BrokerID) *brokerAssetSummary {
		if summary := byBroker[broker]; summary != nil {
			return summary
		}
		summary := &brokerAssetSummary{Broker: broker}
		byBroker[broker] = summary
		return summary
	}
	for _, status := range snapshot.Statuses {
		ensure(status.Broker)
	}
	for _, balance := range snapshot.Balances {
		summary := ensure(balance.Broker)
		summary.HasData = true
		summary.Freshness = mergeFreshness(summary.Freshness, balance.Freshness)
		if summary.AsOf.IsZero() || (!balance.AsOf.IsZero() && balance.AsOf.Before(summary.AsOf)) {
			summary.AsOf = balance.AsOf
		}
		if balance.Currency != domain.USD {
			summary.TotalAssetsKRW = summary.TotalAssetsKRW.Add(balance.ValueTotal).Add(balance.Cash)
			summary.PurchaseKRW = summary.PurchaseKRW.Add(balance.PurchaseTotal)
			summary.ProfitLossKRW = summary.ProfitLossKRW.Add(balance.ProfitLoss)
			continue
		}
		summary.USAssets = summary.USAssets.Add(balance.ValueTotal).Add(balance.Cash)
		summary.USProfitLoss = summary.USProfitLoss.Add(balance.ProfitLoss)
		rate := balance.ExchangeRate
		if !rate.IsPositive() && snapshot.FX.Provider == balance.Broker {
			rate = snapshot.FX.Rate
		}
		if rate.IsPositive() {
			summary.ExchangeRate = rate
		}
		cashKRW, purchaseKRW := balance.CashKRW, balance.PurchaseTotalKRW
		valueKRW, profitKRW := balance.ValueTotalKRW, balance.ProfitLossKRW
		if rate.IsPositive() {
			if cashKRW.IsZero() {
				cashKRW = balance.Cash.Mul(rate)
			}
			if purchaseKRW.IsZero() {
				purchaseKRW = balance.PurchaseTotal.Mul(rate)
			}
			if valueKRW.IsZero() {
				valueKRW = balance.ValueTotal.Mul(rate)
			}
			if profitKRW.IsZero() {
				profitKRW = balance.ProfitLoss.Mul(rate)
			}
		}
		summary.TotalAssetsKRW = summary.TotalAssetsKRW.Add(valueKRW).Add(cashKRW)
		summary.PurchaseKRW = summary.PurchaseKRW.Add(purchaseKRW)
		summary.ProfitLossKRW = summary.ProfitLossKRW.Add(profitKRW)
	}
	result := make([]brokerAssetSummary, 0, len(byBroker))
	for _, summary := range byBroker {
		if snapshot.Cached && summary.HasData {
			summary.Freshness = domain.FreshCached
		}
		result = append(result, *summary)
	}
	order := func(broker domain.BrokerID) int {
		switch broker {
		case domain.BrokerKiwoom:
			return 0
		case domain.BrokerNH:
			return 1
		case domain.BrokerMock:
			return 2
		default:
			return 3
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := order(result[i].Broker), order(result[j].Broker)
		if left == right {
			return result[i].Broker < result[j].Broker
		}
		return left < right
	})
	return result
}

func (m Model) brokerAssetBreakdownView(summaries []brokerAssetSummary, totalAssets decimal.Decimal) string {
	wide := m.width >= 110
	lines := []string{"증권사별 자산 현황", ""}
	if wide {
		lines = append(lines, "  증권사           총자산          평가손익      수익률       미국자산      적용환율     자산비중          상태")
	} else {
		lines = append(lines, "  "+strings.Join([]string{fitCell("증권사·상태", 15, false), fitCell("총자산", 16, true), fitCell("평가손익", 16, true), fitCell("수익률", 9, true), fitCell("자산비중", 9, true)}, " "))
	}
	for _, summary := range summaries {
		brokerLabel, brokerWidth := brokerDisplayName(summary.Broker), 8
		if !wide {
			brokerWidth = 15
			if summary.HasData {
				brokerLabel += "·" + freshnessName(summary.Freshness)
			}
		}
		brokerCell := fitCell(brokerLabel, brokerWidth, false)
		if !summary.HasData {
			if wide {
				lines = append(lines, "  "+strings.Join([]string{brokerCell, fitCell("데이터 없음", 16, true), fitCell("-", 16, true), fitCell("-", 9, true), fitCell("-", 13, true), fitCell("-", 12, true), fitCell("-", 9, true), fitCell("연결 안 됨", 20, true)}, " "))
			} else {
				lines = append(lines, "  "+strings.Join([]string{brokerCell, fitCell("데이터 없음", 16, true), fitCell("-", 16, true), fitCell("-", 9, true), fitCell("-", 9, true)}, " "))
			}
			continue
		}
		profitRate := decimal.Zero
		if !summary.PurchaseKRW.IsZero() {
			profitRate = summary.ProfitLossKRW.Div(summary.PurchaseKRW).Mul(decimal.NewFromInt(100))
		}
		weight := decimal.Zero
		if !totalAssets.IsZero() {
			weight = summary.TotalAssetsKRW.Div(totalAssets).Mul(decimal.NewFromInt(100))
		}
		assetCell := fitCell(money(summary.TotalAssetsKRW)+"원", 16, true)
		profitStyle := directionalValueStyle(summary.ProfitLossKRW)
		profitCell := profitStyle.Render(fitCell(signedMoney(summary.ProfitLossKRW)+"원", 16, true))
		rateCell := profitStyle.Render(fitCell(signedPercent(profitRate), 9, true))
		weightCell := fitCell(weight.StringFixed(2)+"%", 9, true)
		if wide {
			usAsset := "$" + commaNumber(summary.USAssets.StringFixed(2))
			fx := "-"
			if summary.ExchangeRate.IsPositive() {
				fx = summary.ExchangeRate.StringFixed(2) + "원"
			}
			status := freshnessName(summary.Freshness)
			if !summary.AsOf.IsZero() {
				status += " " + summary.AsOf.In(time.Local).Format("15:04:05")
			}
			lines = append(lines, "  "+strings.Join([]string{brokerCell, assetCell, profitCell, rateCell, fitCell(usAsset, 13, true), fitCell(fx, 12, true), weightCell, fitCell(status, 20, true)}, " "))
		} else {
			lines = append(lines, "  "+strings.Join([]string{brokerCell, assetCell, profitCell, rateCell, weightCell}, " "))
		}
	}
	if len(summaries) == 0 {
		lines = append(lines, "  표시할 증권사 자산 데이터가 없습니다.")
	}
	return panel.Width(max(60, m.width-4)).Render(strings.Join(lines, "\n"))
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
	top := krTab + "  " + usTab + "   " + muted.Render("t: 정렬  c: ₩/$")
	lines := []string{top, ""}
	if len(visible) > 0 {
		lines = append(lines, m.renderPortfolioHeader(visible))
	}
	totalValue, totalPurchase, totalProfit := decimal.Zero, decimal.Zero, decimal.Zero
	totalProfitAvailable, totalPurchaseAvailable := true, true
	for _, p := range items {
		totalValue = totalValue.Add(p.MarketValue)
		purchase, profit := p.PurchaseValue, p.ProfitLoss
		if m.currency == currencyKRW && p.Symbol.Currency == domain.USD {
			var ok bool
			purchase, ok = m.positionKRWValue(p.PurchaseValue, p.PurchaseValueKRW, p)
			if !ok {
				totalPurchaseAvailable = false
			}
			profit, ok = m.positionKRWValue(p.ProfitLoss, p.ProfitLossKRW, p)
			if !ok {
				totalProfitAvailable = false
			}
		}
		totalPurchase = totalPurchase.Add(purchase)
		totalProfit = totalProfit.Add(profit)
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
		if totalProfitAvailable && totalPurchaseAvailable && !totalPurchase.IsZero() {
			totalRate = totalProfit.Div(totalPurchase).Mul(decimal.NewFromInt(100))
		}
		lines = append(lines, "  "+muted.Render(renderPortfolioSeparator(visible)))
		totalCurrency := items[0].Symbol.Currency
		if m.currency == currencyKRW && totalCurrency == domain.USD {
			totalCurrency = domain.KRW
		}
		lines = append(lines, "  "+m.renderPortfolioTotal(totalProfit, totalRate, totalCurrency, totalProfitAvailable, visible))
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
	{"보유비중", 9, true}, {"보유금액", 17, true}, {"보유주식수", 12, true}, {"잔고수량", 12, true},
	{"평가금액", 17, true}, {"매입가", 15, true}, {"현재가", 15, true}, {"매입금액", 17, true},
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

func (m Model) renderPortfolioHeader(columns []int) string {
	cells := make([]string, 0, len(columns))
	for _, index := range columns {
		column := portfolioColumns[index]
		cell := fitCell(column.title, column.width, column.right)
		if (m.portfolioSort == 0 && index == 2) || (m.portfolioSort == 1 && index == 3) || (m.portfolioSort == 2 && index == 4) {
			cell = sortHeader.Render(cell)
		}
		cells = append(cells, cell)
	}
	return "  " + muted.Render(strings.Join(cells, " │ "))
}

func (m Model) renderPortfolioRow(p domain.Position, weight decimal.Decimal, columns []int) string {
	values := []string{
		brokerDisplayName(p.Broker),
		p.Symbol.Name,
		m.formatPositionSignedAmount(p.ProfitLoss, p.ProfitLossKRW, p),
		signedPercent(p.ProfitRate),
		weight.StringFixed(2) + "%",
		m.formatPositionAmount(p.MarketValue, p.MarketValueKRW, p),
		commaNumber(p.Quantity.String()),
		commaNumber(p.Quantity.String()),
		m.formatPositionAmount(p.MarketValue, p.MarketValueKRW, p),
		m.formatPositionAmount(p.AveragePrice, decimal.Zero, p),
		m.formatPositionAmount(p.CurrentPrice, decimal.Zero, p),
		m.formatPositionAmount(p.PurchaseValue, p.PurchaseValueKRW, p),
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

func (m Model) renderPortfolioTotal(profit, rate decimal.Decimal, currency domain.Currency, amountAvailable bool, columns []int) string {
	formattedProfit := m.formatSignedAmount(profit, currency)
	if !amountAvailable {
		formattedProfit = "₩-"
	}
	values := []string{"", "합계", formattedProfit, signedPercent(rate), "", "", "", "", "", "", "", ""}
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

func directionalValueStyle(value decimal.Decimal) lipgloss.Style {
	if value.IsPositive() {
		return positive
	}
	if value.IsNegative() {
		return negative
	}
	return neutral
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
	options := m.scanner.Options
	if options.RefreshInterval == 0 {
		options = app.DefaultScannerOptions()
	}
	market := "KOSPI+KOSDAQ"
	if options.Query.Market != "" {
		market = string(options.Query.Market)
	}
	status := freshnessName(m.scanner.Freshness)
	if m.scanner.Provider == domain.BrokerMock {
		status = "데모 · 합성 데이터"
	}
	if m.scannerLoading {
		status += " · 조회 중"
	}
	asOf := "미조회"
	if !m.scanner.AsOf.IsZero() {
		asOf = m.scanner.AsOf.In(time.FixedZone("KST", 9*60*60)).Format("01-02 15:04:05")
	}
	lines := []string{fmt.Sprintf("시장 급등 · %s · KRX · %s · %s", market, brokerDisplayName(m.scanner.Provider), status), fmt.Sprintf("기준 %s · 수집 %d / 분석 %d / 데이터 부족 %d", asOf, m.scanner.Candidates, m.scanner.Checked, m.scanner.Missing), fmt.Sprintf("조건 일간 ≥%s%% · 5분 ≥%s%% · 거래량 ≥%s배 · ≥%s억원", options.Policy.MinChangeRate, options.Policy.MinFiveMinuteRate, options.Policy.MinVolumeRatio, options.Policy.MinTurnover.Div(decimal.NewFromInt(100_000_000))), "", "   종목                  점수     당일      5분    거래량"}
	overhead := 15
	if m.cursor < len(items) && len(items[m.cursor].Warnings) > 0 {
		overhead++
	}
	if m.scanner.Limited {
		overhead++
	}
	if m.scanner.Error != "" {
		overhead++
	}
	if len(m.scanner.Warnings) > 0 {
		overhead++
	}
	visible := max(1, m.height-overhead)
	start := max(0, m.cursor-visible+1)
	end := min(len(items), start+visible)
	for i := start; i < end; i++ {
		r := items[i]
		line := fmt.Sprintf("%s %s %3d점 %8s%% %7s%% %6s배", cursor(i, m.cursor), fitCell(r.Symbol.Name, 18, false), r.Score, r.ChangeRate.StringFixed(2), r.FiveMinuteRate.StringFixed(2), r.VolumeRatio.StringFixed(1))
		lines = append(lines, selectLine(line, i == m.cursor))
	}
	if len(items) == 0 {
		message := "현재 기준을 통과한 급등 후보가 없습니다."
		if m.scannerLoading {
			message = "시장 순위와 최신 분봉을 조회하고 있습니다…"
		} else if m.scanner.AsOf.IsZero() {
			message = "시장 스캐너 조회 대기"
		} else if m.scanner.Missing > 0 {
			message = "후보 없음 · 데이터 부족 종목은 평가에서 제외했습니다."
		}
		lines = append(lines, "  "+message)
	}
	if m.cursor < len(items) {
		r := items[m.cursor]
		lines = append(lines, "", fmt.Sprintf("%s · %s · %s", r.Symbol.Code, brokerDisplayName(r.Provider), r.AsOf.In(time.FixedZone("KST", 9*60*60)).Format("15:04:05")), fmt.Sprintf("거래대금 %s억원 · 고점 거리 %s%% · 체결강도 %s", r.Turnover.Div(decimal.NewFromInt(100_000_000)).StringFixed(1), r.HighDistance.StringFixed(2), r.TradePower.StringFixed(1)))
		if len(r.Warnings) > 0 {
			lines = append(lines, strings.Join(r.Warnings, " · "))
		}
	}
	if m.scanner.Limited {
		lines = append(lines, "상위 후보 일부만 분석 · 조회 한도 적용")
	}
	if m.scanner.Error != "" {
		lines = append(lines, negative.Render(trim(m.scanner.Error, max(40, m.width-10))))
	}
	if len(m.scanner.Warnings) > 0 {
		lines = append(lines, "일부 조회 실패: "+trim(m.scanner.Warnings[0], max(40, m.width-24)))
	}
	lines = append(lines, muted.Render("거래량: 최근 완료 5분 / 직전 5분 · 장 마감/지연 분봉 제외"))
	lines = append(lines, "", muted.Render("※ 규칙 기반 관찰 리포트이며 투자 권유가 아닙니다."))
	return panel.Width(max(60, m.width-4)).Render(strings.Join(lines, "\n"))
}

type performancePoint struct {
	Date        time.Time
	TotalAssets decimal.Decimal
	ProfitLoss  decimal.Decimal
}

func aggregatePerformance(history []domain.PortfolioSnapshot) []performancePoint {
	byDate := make(map[string]performancePoint)
	for _, snapshot := range history {
		key := snapshot.Date.In(time.Local).Format("2006-01-02")
		point := byDate[key]
		if point.Date.IsZero() {
			point.Date = snapshot.Date
		}
		point.TotalAssets = point.TotalAssets.Add(snapshot.CashKRW).Add(snapshot.ValueTotalKRW)
		point.ProfitLoss = point.ProfitLoss.Add(snapshot.ProfitLossKRW)
		byDate[key] = point
	}
	result := make([]performancePoint, 0, len(byDate))
	for _, point := range byDate {
		result = append(result, point)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Date.Before(result[j].Date) })
	return result
}

func (m Model) performancePoints() []performancePoint {
	return aggregatePerformance(m.history)
}

func (m Model) performanceView() string {
	if m.performance == performanceChart {
		return m.performanceChartView()
	}
	return m.performanceTableView()
}

func (m Model) performanceStatus() string {
	for _, warning := range m.snapshot.Warnings {
		if strings.HasPrefix(warning, "성과 기록 보류:") {
			return trim(warning, max(52, m.width-8))
		}
	}
	if m.mode == "demo" {
		return "데모 기록 · 실제 계좌와 분리된 예시 데이터"
	}
	return "NH·키움 정상 잔고 동시 조회분만 기록·비교합니다."
}

func (m Model) performanceTableView() string {
	points := m.performancePoints()
	if len(points) == 0 {
		return panel.Width(max(60, m.width-4)).Render("포트폴리오 성과\n\n  비교 가능한 일별 기록이 없습니다.\n  NH·키움 잔고가 모두 정상 조회되면 기록됩니다.\n  " + m.performanceStatus())
	}
	latest := points[len(points)-1]
	change, changeRate := decimal.Zero, decimal.Zero
	comparison := "비교 기록 필요"
	if len(points) > 1 {
		previous := points[len(points)-2]
		change = latest.TotalAssets.Sub(previous.TotalAssets)
		if !previous.TotalAssets.IsZero() {
			changeRate = change.Div(previous.TotalAssets).Mul(decimal.NewFromInt(100))
		}
		style := directionalValueStyle(change)
		comparison = fmt.Sprintf("%s (%s)", style.Render(signedMoney(change)+"원"), style.Render(signedPercent(changeRate)))
	}
	lines := []string{
		fmt.Sprintf("포트폴리오 성과 · 원화 기준 · %d일 기록", len(points)), "",
		selected.Render(" 표 ") + "  " + muted.Render(" 그래프 ") + "    " + muted.Render("Tab: 보기 전환"), muted.Render(m.performanceStatus()),
		fmt.Sprintf("  총자산       %s원", money(latest.TotalAssets)),
		fmt.Sprintf("  평가손익     %s", directionalValueStyle(latest.ProfitLoss).Render(signedMoney(latest.ProfitLoss)+"원")),
		fmt.Sprintf("  전 기록 대비 %s", comparison), "",
	}
	wide := m.width >= 110
	if wide {
		lines = append(lines, "  날짜             총자산          평가손익          자산 증감       증감률")
	} else {
		lines = append(lines, "  날짜             총자산          평가손익       증감률")
	}
	start := max(0, len(points)-max(3, m.height-13))
	for i := start; i < len(points); i++ {
		point := points[i]
		delta := decimal.Zero
		deltaText, rateText := "-", "-"
		if i > 0 {
			delta = point.TotalAssets.Sub(points[i-1].TotalAssets)
			rate := decimal.Zero
			if !points[i-1].TotalAssets.IsZero() {
				rate = delta.Div(points[i-1].TotalAssets).Mul(decimal.NewFromInt(100))
			}
			deltaText, rateText = signedMoney(delta)+"원", signedPercent(rate)
		}
		dateCell := fitCell(point.Date.In(time.Local).Format("2006-01-02"), 10, false)
		assetCell := fitCell(money(point.TotalAssets)+"원", 16, true)
		profitCell := directionalValueStyle(point.ProfitLoss).Render(fitCell(signedMoney(point.ProfitLoss)+"원", 16, true))
		deltaStyle := directionalValueStyle(delta)
		rateCell := deltaStyle.Render(fitCell(rateText, 10, true))
		var line string
		if wide {
			deltaCell := deltaStyle.Render(fitCell(deltaText, 16, true))
			line = "  " + strings.Join([]string{dateCell, assetCell, profitCell, deltaCell, rateCell}, " ")
		} else {
			line = "  " + strings.Join([]string{dateCell, assetCell, profitCell, rateCell}, " ")
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", muted.Render("※ 자산 증감은 입출금을 포함하며 투자 수익률과 다를 수 있습니다."))
	return panel.Width(max(76, m.width-4)).Render(strings.Join(lines, "\n"))
}

func aggregatePerformancePeriod(points []performancePoint, period performancePeriod) []performancePoint {
	if period == performanceDaily || len(points) < 2 {
		return append([]performancePoint(nil), points...)
	}
	result := make([]performancePoint, 0, len(points))
	lastKey := ""
	for _, point := range points {
		date := point.Date.In(time.Local)
		key := ""
		switch period {
		case performanceWeekly:
			year, week := date.ISOWeek()
			key = fmt.Sprintf("%04d-W%02d", year, week)
		case performanceMonthly:
			key = date.Format("2006-01")
		case performanceYearly:
			key = date.Format("2006")
		}
		if key == lastKey {
			result[len(result)-1] = point
			continue
		}
		result = append(result, point)
		lastKey = key
	}
	return result
}

func (m Model) performanceChartView() string {
	allPoints := m.performancePoints()
	if len(allPoints) == 0 {
		return panel.Width(max(60, m.width-4)).Render("포트폴리오 성과\n\n  비교 가능한 일별 기록이 없습니다.\n  NH·키움 잔고가 모두 정상 조회되면 기록됩니다.\n  " + m.performanceStatus())
	}
	points := aggregatePerformancePeriod(allPoints, m.perfPeriod)
	first, latest := points[0], points[len(points)-1]
	change := latest.TotalAssets.Sub(first.TotalAssets)
	changeRate := decimal.Zero
	if !first.TotalAssets.IsZero() {
		changeRate = change.Div(first.TotalAssets).Mul(decimal.NewFromInt(100))
	}
	changeText := "비교 기록 필요"
	if len(points) > 1 {
		style := directionalValueStyle(change)
		changeText = fmt.Sprintf("%s (%s)", style.Render(signedMoney(change)+"원"), style.Render(signedPercent(changeRate)))
	}
	periodTabs := make([]string, 0, 4)
	for index, name := range []string{"일간", "주간", "월간", "연간"} {
		if performancePeriod(index) == m.perfPeriod {
			periodTabs = append(periodTabs, selected.Render(" "+name+" "))
		} else {
			periodTabs = append(periodTabs, muted.Render(" "+name+" "))
		}
	}
	contentWidth := max(72, m.width-8)
	chartHeight := max(10, m.height-14)
	lines := []string{
		fmt.Sprintf("포트폴리오 성과 · 원화 기준 · %d일 기록", len(allPoints)),
		muted.Render(" 표 ") + "  " + selected.Render(" 그래프 ") + "    " + strings.Join(periodTabs, " "), muted.Render(m.performanceStatus()),
		fmt.Sprintf("  총자산       %s원", money(latest.TotalAssets)),
		fmt.Sprintf("  평가손익     %s", directionalValueStyle(latest.ProfitLoss).Render(signedMoney(latest.ProfitLoss)+"원")),
		fmt.Sprintf("  기간 증감    %s", changeText), "",
		renderPerformanceLineChart(points, contentWidth, chartHeight), "",
		muted.Render("Tab: 표/그래프 전환   t: 일간 → 주간 → 월간 → 연간"),
	}
	return panel.Width(max(76, m.width-4)).Render(strings.Join(lines, "\n"))
}

func (m Model) dividendView() string {
	if m.dividendLoading {
		return panel.Width(max(60, m.width-4)).Render("배당 현황과 예상 배당금\n\n  보유 미국주식의 배당 이력을 불러오는 중…")
	}
	report := m.dividends
	if len(report.Holdings) == 0 {
		message := "보유 미국주식의 배당 데이터가 없습니다."
		if len(report.Warnings) > 0 {
			message += "\n\n  " + strings.Join(report.Warnings, "\n  ")
		}
		if report.Source == "미설정" {
			message += "\n\n  설정: minstock setup dividend"
		}
		return panel.Width(max(60, m.width-4)).Render("배당 현황과 예상 배당금\n\n  " + message)
	}
	freshness := freshnessName(report.Freshness)
	if report.Freshness == "" {
		freshness = "-"
	}
	lines := []string{
		"배당 현황과 예상 배당금 · USD", "",
		fmt.Sprintf("  연간 예상 세전  %s", signedUSD(report.GrossAnnual)),
		fmt.Sprintf("  연간 예상 세후  %s", signedUSD(report.NetAnnual)),
		fmt.Sprintf("  세후 원화환산   %s원", money(report.NetAnnualKRW)),
		fmt.Sprintf("  데이터          %s · %s", report.Source, freshness), "",
	}
	if len(report.Warnings) > 0 {
		lines = append(lines, statusWait.Render(fmt.Sprintf("  조회 경고        %d종목 · 아래 오류를 확인하세요", len(report.Warnings))), "")
	}
	if m.dividendDisplay == dividendCalendar {
		lines = append(lines, muted.Render(" 종목별 ")+"  "+selected.Render(" 월별 ")+"    "+muted.Render("Tab: 보기 전환"), "", "월별 예상 배당 · 향후 12개월")
		if len(report.Months) == 0 {
			lines = append(lines, "  예상 가능한 지급월이 없습니다.")
		} else {
			cells := make([]string, 0, len(report.Months))
			for _, month := range report.Months {
				cells = append(cells, fitCell(fmt.Sprintf("%s  $%s / %s원", month.Month.Format("2006-01"), month.Net.StringFixed(2), money(month.NetKRW)), 32, false))
			}
			for index := 0; index < len(cells); index += 2 {
				line := "  " + cells[index]
				if index+1 < len(cells) {
					line += "  " + cells[index+1]
				}
				lines = append(lines, line)
			}
			lines = append(lines, muted.Render("  각 월: 세후 USD / 세후 원화"))
		}
	} else {
		lines = append(lines, selected.Render(" 종목별 ")+"  "+muted.Render(" 월별 ")+"    "+muted.Render("Tab: 보기 전환"), "")
		wide := m.width >= 115
		if wide {
			lines = append(lines, "  종목       보유수량       최근 주당배당     최근 지급일    연간 주당예상       세전 예상       세후 예상       세후 원화")
		} else {
			lines = append(lines, "  종목       보유수량       최근 배당       연간 세후       세후 원화")
		}
		for _, holding := range report.Holdings {
			recent := "-"
			if holding.RecentAmount.IsPositive() {
				recent = "$" + holding.RecentAmount.StringFixed(4)
			}
			if wide {
				recentDate := "-"
				if !holding.RecentDate.IsZero() {
					recentDate = holding.RecentDate.Format("2006-01-02")
				}
				lines = append(lines, "  "+strings.Join([]string{
					fitCell(holding.Symbol, 8, false), fitCell(holding.Quantity.StringFixed(4), 12, true),
					fitCell(recent, 16, true), fitCell(recentDate, 13, true),
					fitCell("$"+holding.AnnualPerShare.StringFixed(4), 16, true),
					fitCell("$"+holding.GrossAnnual.StringFixed(2), 14, true),
					fitCell("$"+holding.NetAnnual.StringFixed(2), 14, true),
					fitCell(money(holding.NetAnnualKRW)+"원", 15, true),
				}, " "))
			} else {
				lines = append(lines, "  "+strings.Join([]string{
					fitCell(holding.Symbol, 8, false), fitCell(holding.Quantity.StringFixed(4), 12, true),
					fitCell(recent, 14, true), fitCell("$"+holding.NetAnnual.StringFixed(2), 14, true),
					fitCell(money(holding.NetAnnualKRW)+"원", 15, true),
				}, " "))
			}
		}
	}
	lines = append(lines, "", muted.Render("※ 최근 12개월 배당을 연간 예상치로 사용합니다. 세후는 미국 원천징수 15% 가정이며 미래 금액·지급월은 추정치입니다."))
	if len(report.Warnings) > 0 {
		lines = append(lines, muted.Render("경고: "+strings.Join(report.Warnings, " · ")))
	}
	return panel.Width(max(76, m.width-4)).Render(strings.Join(lines, "\n"))
}

func dividendHoldingsKey(positions []domain.Position) string {
	parts := make([]string, 0, len(positions))
	for _, position := range positions {
		if position.Symbol.Currency != domain.USD || !position.Quantity.IsPositive() {
			continue
		}
		symbol := strings.ToUpper(strings.TrimSpace(position.Symbol.Ticker))
		if symbol == "" {
			symbol = strings.ToUpper(strings.TrimSpace(position.Symbol.Code))
		}
		parts = append(parts, fmt.Sprintf("%s:%s:%s:%s", position.Broker, symbol, position.Quantity, position.ExchangeRate))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func (m Model) allocationScopeName() string {
	return []string{"ALL", string(domain.BrokerNH), string(domain.BrokerKiwoom)}[m.allocationScope%3]
}
func (m Model) allocationScopeLabel() string {
	return []string{"통합", "NH", "키움"}[m.allocationScope%3]
}

func (m Model) handleAlertInput(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.alertAdding, m.alertInput = false, ""
		m.notice = "목표가 입력을 취소했습니다"
	case "enter":
		input := strings.TrimSpace(m.alertInput)
		if input == "" {
			m.notice = "티커와 목표가를 입력하세요 (예: VOO 700)"
			return m, nil
		}
		m.alertAdding = false
		return m, m.savePriceAlertCmd(input)
	case "backspace":
		if len(m.alertInput) > 0 {
			_, size := utf8.DecodeLastRuneInString(m.alertInput)
			m.alertInput = m.alertInput[:len(m.alertInput)-size]
		}
	case "space":
		m.alertInput += " "
	default:
		if utf8.RuneCountInString(key) == 1 {
			m.alertInput += key
		}
	}
	return m, nil
}

func (m Model) handleAllocationInput(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.allocationEditing, m.allocationAdding = false, false
		m.allocationInput = ""
	case "backspace":
		if len(m.allocationInput) > 0 {
			_, size := utf8.DecodeLastRuneInString(m.allocationInput)
			m.allocationInput = m.allocationInput[:len(m.allocationInput)-size]
		}
	case "enter":
		if m.allocationAdding {
			query := strings.TrimSpace(m.allocationInput)
			if query == "" {
				m.notice = "종목 티커 또는 코드를 입력하세요"
				return m, nil
			}
			m.allocationAdding, m.allocationPending, m.allocationInput = false, true, ""
			return m, m.allocationSymbolCmd(query)
		}
		value, err := decimal.NewFromString(m.allocationInput)
		if err != nil || value.IsNegative() || value.GreaterThan(decimal.NewFromInt(100)) {
			m.notice = "0~100 사이 숫자를 입력하세요"
			return m, nil
		}
		m.allocation.Rows[m.cursor].Target.TargetPercent = value
		m.allocationDirty = true
		m.allocationEditing = false
		m.allocationInput = ""
	default:
		if m.allocationAdding && utf8.RuneCountInString(key) == 1 {
			m.allocationInput += key
		} else if (key >= "0" && key <= "9") || key == "." {
			m.allocationInput += key
		}
	}
	return m, nil
}

func (m Model) allocationView() string {
	if m.rebalanceView || m.rebalanceEditing {
		return m.rebalancePlanView()
	}
	targetTotal := decimal.Zero
	for _, row := range m.allocation.Rows {
		targetTotal = targetTotal.Add(row.Target.TargetPercent)
	}
	title := "목표 비중과 리밸런싱 조회 · " + m.allocationScopeLabel()
	if m.allocationDirty {
		title += " · 편집 중"
	}
	lines := []string{title, muted.Render(" 통합 / NH / 키움   Tab: 범위 전환"), "", fmt.Sprintf("  총자산 %s원    목표 합계 %s%%", money(m.allocation.TotalKRW), m.allocation.TargetTotal.StringFixed(2)), fmt.Sprintf("  시장  한국 %.1f%% · 미국 %.1f%% · 현금/기타 %.1f%%", m.allocation.KRMarketPercent.InexactFloat64(), m.allocation.USMarketPercent.InexactFloat64(), m.allocation.OtherMarketPercent.InexactFloat64()), fmt.Sprintf("  통화  KRW %.1f%% · USD %.1f%%", m.allocation.KRWPercent.InexactFloat64(), m.allocation.USDPercent.InexactFloat64()), "", "  종목             목표       현재       편차       상태"}
	lines[3] = fmt.Sprintf("  총자산 %s원    목표 합계 %s%%", money(m.allocation.TotalKRW), targetTotal.StringFixed(2))
	if m.allocationAdding {
		lines = append(lines, selected.Render("  종목 추가: "+m.allocationInput+"_"), "")
	}
	for i, row := range m.allocation.Rows {
		name := row.Target.Symbol.Ticker
		if row.Target.Symbol.Code == "OTHER" {
			name = row.Target.Symbol.Name
		}
		if name == "" {
			name = row.Target.Symbol.Code
		}
		if row.Target.Cash {
			name = "현금"
		}
		target := row.Target.TargetPercent.StringFixed(2) + "%"
		if m.allocationEditing && i == m.cursor {
			target = "[" + m.allocationInput + "]%"
		}
		difference := row.CurrentPercent.Sub(row.Target.TargetPercent)
		line := fmt.Sprintf("  %s %s %s %s %s", fitCell(name, 12, false), fitCell(target, 10, true), fitCell(row.CurrentPercent.StringFixed(2)+"%", 10, true), fitCell(signedPercent(difference), 10, true), allocationStatus(difference))
		if i == m.cursor {
			line = selected.Render(line)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", muted.Render("Enter/e 목표 입력 · a 추가 · d 0% · r 현재비중 복사 · s 저장 · u 되돌리기"), muted.Render("※ 조회 전용이며 주문·매매 수량을 제안하거나 실행하지 않습니다."))
	return panel.Width(max(76, m.width-4)).Render(strings.Join(lines, "\n"))
}

func copyCurrentAllocationTargets(report *app.AllocationReport) {
	total := decimal.Zero
	adjustmentIndex := -1
	largest := decimal.NewFromInt(-1)
	for index := range report.Rows {
		current := report.Rows[index].CurrentPercent.Round(2)
		report.Rows[index].Target.TargetPercent = current
		total = total.Add(current)
		if current.GreaterThan(largest) {
			largest, adjustmentIndex = current, index
		}
	}
	if adjustmentIndex >= 0 {
		report.Rows[adjustmentIndex].Target.TargetPercent = report.Rows[adjustmentIndex].Target.TargetPercent.Add(decimal.NewFromInt(100).Sub(total))
		report.TargetTotal = decimal.NewFromInt(100)
	}
}

func allocationStatus(difference decimal.Decimal) string {
	if difference.GreaterThan(decimal.NewFromInt(5)) {
		return "집중"
	}
	if difference.GreaterThan(decimal.NewFromInt(2)) {
		return "초과"
	}
	if difference.LessThan(decimal.NewFromInt(-2)) {
		return "부족"
	}
	return "적정"
}

func (m Model) alertView() string {
	historyTab, rulesTab := " 이력 ", " 규칙 "
	if m.alertDisplay == alertRules {
		rulesTab = activeTab.Render(rulesTab)
	} else {
		historyTab = activeTab.Render(historyTab)
	}
	lines := []string{
		fmt.Sprintf("가격·수익률 알림 · 미확인 %d건", m.alerts.Unacknowledged),
		historyTab + "  " + rulesTab + "    Tab: 보기 전환", "",
		muted.Render("  기본 조건  일간 변동 ±5% · 보유 손실 -10% · 종목 비중 30% · 연결 실패 3회"), "",
	}
	if m.alertDisplay == alertRules {
		if m.alertAdding {
			lines = append(lines, selected.Render("  목표가 추가: "+m.alertInput+"_"), muted.Render("  형식: 티커 목표가 (예: VOO 700)"), "")
		}
		lines = append(lines, "  종목             목표가      방향")
		if len(m.alerts.Rules) == 0 {
			lines = append(lines, "", muted.Render("  등록된 목표가 알림이 없습니다. a를 눌러 추가하세요."))
		}
		for i, rule := range m.alerts.Rules {
			direction := "이상 도달"
			if rule.Direction == "BELOW" {
				direction = "이하 도달"
			}
			unit := "원"
			if rule.Symbol.Currency == domain.USD {
				unit = "$"
			}
			price := rule.TargetPrice.StringFixed(2) + unit
			if unit == "$" {
				price = "$" + rule.TargetPrice.StringFixed(2)
			}
			line := fmt.Sprintf("  %s %s %s", fitCell(alertSymbolLabel(rule.Symbol), 14, false), fitCell(price, 12, true), direction)
			if i == m.cursor {
				line = selected.Render(line)
			}
			lines = append(lines, line)
		}
		lines = append(lines, "", muted.Render("a 목표가 추가 · d 선택 규칙 삭제"))
	} else {
		lines = append(lines, "  상태  시각              유형       대상        내용")
		if len(m.alerts.Events) == 0 {
			lines = append(lines, "", muted.Render("  발생한 알림이 없습니다."))
		}
		for i, event := range m.alerts.Events {
			state := "●"
			if !event.AcknowledgedAt.IsZero() {
				state = "○"
			}
			line := fmt.Sprintf("  %s  %s  %s %s %s", state, event.OccurredAt.Local().Format("01-02 15:04"), fitCell(alertKindName(event.Kind), 10, false), fitCell(event.Subject, 10, false), trim(event.Message, max(20, m.width-55)))
			if i == m.cursor {
				line = selected.Render(line)
			} else if event.AcknowledgedAt.IsZero() && event.Severity == "위험" {
				line = negative.Render(line)
			}
			lines = append(lines, line)
		}
		lines = append(lines, "", muted.Render("x 선택 확인 · X 모두 확인 · 같은 조건은 하루 한 번 기록"))
	}
	return panel.Width(max(76, m.width-4)).Render(strings.Join(lines, "\n"))
}

func alertKindName(kind domain.AlertKind) string {
	switch kind {
	case domain.AlertTargetPrice:
		return "목표가"
	case domain.AlertDailyChange:
		return "일간변동"
	case domain.AlertHoldingLoss:
		return "보유손실"
	case domain.AlertAssetWeight:
		return "비중집중"
	case domain.AlertConnectionFailed:
		return "연결실패"
	default:
		return string(kind)
	}
}

func alertSymbolLabel(symbol domain.Symbol) string {
	if ticker := strings.TrimSpace(symbol.Ticker); ticker != "" {
		return strings.ToUpper(ticker)
	}
	return strings.ToUpper(strings.TrimSpace(symbol.Code))
}

func (m Model) detailView() string {
	if m.informationTab {
		return m.informationView()
	}
	interval := intervals[m.intervalIndex]
	if m.loading {
		return fmt.Sprintf("%s(%s) · %s\n\n차트 데이터를 불러오는 중…", m.selected.Name, m.selected.Code, interval.KoreanName())
	}
	legendItems := []string{"MA5", "MA20", "MA60", "MA120"}
	legendStyles := []lipgloss.Style{ma5Style, ma20Style, ma60Style, ma120Style}
	legendParts := make([]string, len(legendItems))
	for i, label := range legendItems {
		if m.maVisible[i] {
			legendParts[i] = legendStyles[i].Render(label)
		} else {
			legendParts[i] = muted.Render("-" + label)
		}
	}
	legend := strings.Join(legendParts, "  ") + "  (1-4 토글)"
	exchange := ""
	if m.selected.Exchange != "" {
		exchange = " · " + m.selected.Exchange
	}
	header := fmt.Sprintf("%s (%s) · %s%s · %s  │  %s", m.selected.Name, m.selected.Code, m.selected.Market, exchange, interval.KoreanName(), legend) + "\n[정보·차트]  뉴스·공시 (Tab)"
	chartHeight := max(8, m.height/2-4)
	cellWidth := max(28, m.width/2-4)
	chartWidth := max(60, m.width-4)
	q := m.detailQuote
	if q.Symbol.Code == "" {
		q = m.snapshot.Quotes[m.selected.Key()]
	}
	// The chart gets the entire upper row so price movements and MA curves
	// have twice as much horizontal resolution in the terminal.
	chart := panel.Width(chartWidth).Height(chartHeight).Render(renderChart(m.candles, chartWidth-4, chartHeight-2, m.maVisible[:]...))
	info := panel.Width(cellWidth).Height(chartHeight).Render(strings.Join([]string{
		"주식 정보", "", detailMetric("시총", detailDecimal(q.MarketCap)), detailMetric("EPS", detailDecimal(q.EPS)), detailMetric("PER", detailDecimal(q.PER)),
		detailMetric("시가", m.formatAmount(q.Open, m.selected.Currency)), detailMetric("고가", m.formatAmount(q.High, m.selected.Currency)), detailMetric("저가", m.formatAmount(q.Low, m.selected.Currency)),
		detailMetric("전일대비", signedPercent(q.ChangeRate)), detailMetric("거래량", commaNumber(fmt.Sprintf("%d", q.Volume))),
	}, "\n"))
	analysis := panel.Width(cellWidth).Height(chartHeight).Render("종목 분석\n\n  " + detailAnalysis(q))
	bottom := lipgloss.JoinHorizontal(lipgloss.Top, info, "  ", analysis)
	return header + "\n" + chart + "\n" + bottom
}

func detailMetric(label, value string) string { return fmt.Sprintf("%-8s %s", label, value) }

func detailDecimal(v decimal.Decimal) string {
	if v.IsZero() {
		return "-"
	}
	return commaNumber(v.StringFixed(2))
}

func detailAnalysis(q domain.Quote) string {
	if q.ChangeRate.GreaterThan(decimal.Zero) {
		return "상승 추세 · 전일 대비 +" + q.ChangeRate.StringFixed(2) + "%"
	}
	if q.ChangeRate.LessThan(decimal.Zero) {
		return "하락 추세 · 전일 대비 " + q.ChangeRate.StringFixed(2) + "%"
	}
	return "등락 정보 없음"
}

func (m Model) helpView() string {
	return panel.Render(`Vim 단축키

↑/↓, j/k 선택      Enter 상세보기      Esc 뒤로/취소
←/→, h/l 화면 이동  gg/G 처음/끝        Ctrl+u/d 반 페이지
gt/gT 다음/이전 화면  / 검색 입력         m 관심종목 토글
f 시장 필터/보유탭  c USD/KRW 표시 전환
1~9 주요 화면 이동  0 통합 뉴스  ? 도움말

내 주식
Tab/Shift+Tab 한국·미국 탭    [/ ] 표 열 이동
t 기본순서→수익률→보유비중 정렬

성과
Tab 표·그래프 전환    그래프에서 t 일·주·월·연 전환

배당
최근 12개월 기준 연간·월별 예상 배당과 세전·세후·원화 조회
Tab 종목별·월별 화면 전환

목표 비중
Tab 통합·NH·키움 전환    Enter/e 편집    a 종목 추가    r 현재비중 복사    s 저장    u 되돌리기
b 추가 투자금 입력    v 부족/초과 금액과 추가금 배분 계산(주문 없음)

알림
Tab 이력·목표가 규칙 전환    a 목표가 추가    d 규칙 삭제    x/X 확인/모두 확인

통합 뉴스 (0)
Tab 보유·관심 전환    f 뉴스·공시    s 종목    u 안 읽음
Enter/o 원문    x 읽음 전환    i 상태    r/n 국내 조회    a 미국 조회
p 전체→기본→엄격    v 반복형 제목 숨김    F 품질 필터 초기화    i 제외 근거
z 제목 직접 언급    m 관련도 0/0.3/0.6    / 제목 검색    h 매체 숨김    H 매체 복원

상세 차트
h/l 또는 ←/→ 봉 단위 변경
Tab 정보·차트 / 뉴스·공시 전환
뉴스·공시: j/k 이동, Enter/o 원문, i 상태·관련도, r 국내 조회, a 미국 조회
뉴스 필터는 통합 피드와 공유·저장 · 공시는 뉴스 전용 필터에서 제외

콜론 명령
:r 새로고침   :s 전체 동기화   :d 연결 진단   :q 종료

상세 차트: 틱·1/5/15/60분·일·주·월·년 / MA5·20·60·120
조회 전용: 주문 기능 및 주문 API 호출 없음`)
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
	if m.screen == newsFeedScreen {
		base = " Tab 범위 f/s/u 필터 p 품질 F 초기화 x 읽음 Enter 원문 i 상태 a 미국"
		if m.width < 110 {
			base = " Tab 범위 p 품질 F 초기화 x 읽음 Enter 원문 i 상태 a 미국"
		}
	} else if m.screen == detailScreen {
		base = " Tab 정보/뉴스  h/l 봉 단위  1~4 MA  Esc 뒤로"
		if m.informationTab {
			base = " Tab 차트 p 품질 F 초기화 Enter 원문 i 상태 a 미국 Esc 뒤로"
		}
	} else if m.screen == performanceScreen {
		base = " Tab 표/그래프  t 일/주/월/연  : 명령  ? 도움말"
	} else if m.screen == dividendScreen {
		base = " Tab 종목별/월별  : 명령  ? 도움말"
	} else if m.screen == allocationScreen {
		base = " Tab 범위 Enter 편집 a 추가 s 저장 b 투자금 v 계산 u 취소"
	} else if m.screen == alertScreen {
		if m.alertDisplay == alertRules {
			base = " Tab 이력/규칙  a 목표가 추가  d 삭제  ? 도움말"
		} else {
			base = " Tab 이력/규칙  x 확인  X 모두 확인  ? 도움말"
		}
	}
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
func money(v decimal.Decimal) string { return commaNumber(v.Round(0).StringFixed(0)) }

func commaNumber(value string) string {
	value = strings.TrimSpace(value)
	sign := ""
	if strings.HasPrefix(value, "-") || strings.HasPrefix(value, "+") {
		sign, value = value[:1], value[1:]
	}
	parts := strings.SplitN(value, ".", 2)
	integer := parts[0]
	for i := len(integer) - 3; i > 0; i -= 3 {
		integer = integer[:i] + "," + integer[i:]
	}
	if len(parts) == 2 {
		return sign + integer + "." + parts[1]
	}
	return sign + integer
}
func signedMoney(v decimal.Decimal) string {
	if v.IsPositive() {
		return "+" + money(v)
	}
	return money(v)
}
func signedUSD(v decimal.Decimal) string {
	if v.IsPositive() {
		return "+$" + commaNumber(v.StringFixed(2))
	}
	if v.IsNegative() {
		return "-$" + commaNumber(v.Abs().StringFixed(2))
	}
	return "$0.00"
}
func (m Model) formatAmount(v decimal.Decimal, currency domain.Currency) string {
	if currency != domain.USD {
		return "₩" + money(v)
	}
	usd := "$" + commaNumber(v.StringFixed(2))
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
		return usd
	}
}

func (m Model) positionExchangeRate(p domain.Position) decimal.Decimal {
	if p.ExchangeRate.IsPositive() {
		return p.ExchangeRate
	}
	if m.snapshot.FX.Provider == p.Broker {
		return m.snapshot.FX.Rate
	}
	return decimal.Zero
}

func (m Model) positionKRWValue(native, brokerKRW decimal.Decimal, p domain.Position) (decimal.Decimal, bool) {
	if !brokerKRW.IsZero() || native.IsZero() {
		return brokerKRW, true
	}
	if rate := m.positionExchangeRate(p); rate.IsPositive() {
		return native.Mul(rate), true
	}
	return decimal.Zero, false
}

func (m Model) formatPositionAmount(v, brokerKRW decimal.Decimal, p domain.Position) string {
	if p.Symbol.Currency != domain.USD || m.currency != currencyKRW {
		return m.formatAmount(v, p.Symbol.Currency)
	}
	if krw, ok := m.positionKRWValue(v, brokerKRW, p); ok {
		return "₩" + money(krw)
	}
	return "₩-"
}

func (m Model) formatPositionSignedAmount(v, brokerKRW decimal.Decimal, p domain.Position) string {
	if p.Symbol.Currency != domain.USD || m.currency != currencyKRW {
		return m.formatSignedAmount(v, p.Symbol.Currency)
	}
	if krw, ok := m.positionKRWValue(v, brokerKRW, p); ok {
		return signedMoney(krw) + "원"
	}
	return "₩-"
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
		return usd
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
