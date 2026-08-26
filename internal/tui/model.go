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
}

type dashboardMsg struct{ snapshot app.Snapshot }
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
	return Model{service: service, mode: mode, loading: true, width: 100, height: 30, intervalIndex: 5, refreshEvery: refreshEvery}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.syncCmd(), m.dashboardCmd(), m.tickCmd())
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
		return dashboardMsg{snapshot: m.service.Dashboard(ctx)}
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
	case dashboardMsg:
		m.snapshot, m.loading, m.err = msg.snapshot, false, nil
	case syncMsg:
		if len(msg.errs) > 0 {
			m.err = msg.errs[0]
		}
		return m, m.dashboardCmd()
	case watchlistMsg:
		m.err = msg.err
		return m, m.dashboardCmd()
	case searchMsg:
		if msg.query == m.query {
			m.results, m.err, m.cursor = msg.results, msg.err, 0
		}
	case candlesMsg:
		if msg.symbol.Key() == m.selected.Key() && msg.interval == intervals[m.intervalIndex] {
			m.candles, m.err, m.loading = msg.candles, msg.err, false
		}
	case refreshMsg:
		if m.screen != detailScreen {
			m.loading = true
			return m, tea.Batch(m.dashboardCmd(), m.tickCmd())
		}
		return m, m.tickCmd()
	case tea.KeyPressMsg:
		return m.handleKey(msg.String())
	}
	return m, nil
}

func (m Model) handleKey(key string) (tea.Model, tea.Cmd) {
	if key == "ctrl+c" || key == "q" {
		return m, tea.Quit
	}
	if key == "?" {
		if m.screen == helpScreen {
			m.screen = m.previous
		} else {
			m.previous, m.screen = m.screen, helpScreen
		}
		return m, nil
	}
	if key == "esc" {
		if m.screen == detailScreen || m.screen == helpScreen {
			m.screen = m.previous
		} else {
			m.screen = dashboardScreen
		}
		m.cursor = 0
		return m, nil
	}
	if m.screen == searchScreen {
		return m.handleSearchKey(key)
	}
	if m.screen == detailScreen {
		if key == "a" {
			symbol := m.selected
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				return watchlistMsg{err: m.service.AddWatchlist(ctx, symbol)}
			}
		}
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
	case "3", "/":
		m.screen, m.cursor = searchScreen, 0
		return m, m.searchCmd(m.query)
	case "4":
		m.screen, m.cursor = watchlistScreen, 0
	case "5":
		m.screen, m.cursor = moversScreen, 0
	case "r":
		m.loading = true
		return m, m.dashboardCmd()
	case "d":
		if m.screen == watchlistScreen {
			if symbol, ok := m.currentSymbol(); ok {
				return m, func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					return watchlistMsg{err: m.service.RemoveWatchlist(ctx, symbol)}
				}
			}
		}
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor+1 < m.itemCount() {
			m.cursor++
		}
	case "enter":
		if symbol, ok := m.currentSymbol(); ok {
			m.previous, m.screen, m.selected, m.loading = m.screen, detailScreen, symbol, true
			return m, m.candlesCmd(symbol)
		}
	}
	return m, nil
}

func (m Model) handleSearchKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down":
		if m.cursor+1 < len(m.results) {
			m.cursor++
		}
	case "enter":
		if symbol, ok := m.currentSymbol(); ok {
			m.previous, m.screen, m.selected, m.loading = searchScreen, detailScreen, symbol, true
			return m, m.candlesCmd(symbol)
		}
	case "backspace":
		if len(m.query) > 0 {
			_, size := utf8.DecodeLastRuneInString(m.query)
			m.query = m.query[:len(m.query)-size]
			return m, m.searchCmd(m.query)
		}
	default:
		if utf8.RuneCountInString(key) == 1 && key != "q" && key != "?" {
			m.query += key
			return m, m.searchCmd(m.query)
		}
	}
	return m, nil
}

func (m Model) itemCount() int {
	switch m.screen {
	case portfolioScreen:
		return len(m.snapshot.Positions)
	case watchlistScreen:
		return len(m.snapshot.Watchlist)
	case moversScreen:
		return len(m.snapshot.Surges)
	case searchScreen:
		return len(m.results)
	}
	return 0
}

func (m Model) currentSymbol() (domain.Symbol, bool) {
	if m.cursor < 0 {
		return domain.Symbol{}, false
	}
	switch m.screen {
	case portfolioScreen:
		if m.cursor < len(m.snapshot.Positions) {
			return m.snapshot.Positions[m.cursor].Symbol, true
		}
	case watchlistScreen:
		if m.cursor < len(m.snapshot.Watchlist) {
			return m.snapshot.Watchlist[m.cursor].Symbol, true
		}
	case moversScreen:
		if m.cursor < len(m.snapshot.Surges) {
			return m.snapshot.Surges[m.cursor].Symbol, true
		}
	case searchScreen:
		if m.cursor < len(m.results) {
			return m.results[m.cursor], true
		}
	}
	return domain.Symbol{}, false
}

func (m Model) View() tea.View {
	content := m.header() + "\n" + m.body() + "\n" + m.footer()
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

var (
	brand    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	muted    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	selected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))
	positive = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	negative = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	panel    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
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
	}
	if m.err != nil {
		body += "\n" + negative.Render("오류: "+m.err.Error())
	}
	return body
}

func (m Model) dashboardView() string {
	totalValue, totalProfit := decimal.Zero, decimal.Zero
	for _, b := range m.snapshot.Balances {
		totalValue = totalValue.Add(b.ValueTotal).Add(b.Cash)
		totalProfit = totalProfit.Add(b.ProfitLoss)
	}
	fx := "-"
	if !m.snapshot.FX.Rate.IsZero() {
		fx = m.snapshot.FX.Rate.StringFixed(2) + "원"
	}
	left := panel.Width(max(28, m.width/2-4)).Render(fmt.Sprintf("통합 자산\n\n  평가자산  %s원\n  평가손익  %s원\n  USD/KRW   %s", money(totalValue), signedMoney(totalProfit), fx))
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
	lines := []string{"   종목                 증권사       수량        현재가        손익(수익률)"}
	for i, p := range m.snapshot.Positions {
		line := fmt.Sprintf("%-2s %-18s %-10s %8s %12s %14s (%s%%)", cursor(i, m.cursor), trim(p.Symbol.Name, 16), p.Broker, p.Quantity.String(), money(p.CurrentPrice), signedMoney(p.ProfitLoss), p.ProfitRate.StringFixed(2))
		lines = append(lines, selectLine(line, i == m.cursor))
	}
	if len(m.snapshot.Positions) == 0 {
		lines = append(lines, "  보유 종목이 없습니다.")
	}
	return panel.Width(max(60, m.width-4)).Render(strings.Join(lines, "\n"))
}

func (m Model) searchView() string {
	lines := []string{"종목 검색 > " + m.query + "█", ""}
	for i, s := range m.results {
		line := fmt.Sprintf("%-2s %-10s %-22s %s", cursor(i, m.cursor), s.Code, trim(s.Name, 20), s.Market)
		lines = append(lines, selectLine(line, i == m.cursor))
	}
	if len(m.results) == 0 {
		lines = append(lines, "  이름 또는 종목코드를 입력하세요.")
	}
	return panel.Width(max(60, m.width-4)).Render(strings.Join(lines, "\n"))
}

func (m Model) watchlistView() string {
	lines := []string{"   종목                 출처        현재가      등락률"}
	for i, item := range m.snapshot.Watchlist {
		q := m.snapshot.Quotes[item.Symbol.Key()]
		line := fmt.Sprintf("%-2s %-18s %-10s %11s %8s%%", cursor(i, m.cursor), trim(item.Symbol.Name, 16), item.Provider, money(q.Price), q.ChangeRate.StringFixed(2))
		lines = append(lines, selectLine(line, i == m.cursor))
	}
	if len(m.snapshot.Watchlist) == 0 {
		lines = append(lines, "  관심종목이 없습니다. 검색 결과에서 추후 로컬 관심종목으로 추가할 수 있습니다.")
	}
	return panel.Width(max(60, m.width-4)).Render(strings.Join(lines, "\n"))
}

func (m Model) moversView() string {
	items := append([]domain.SurgeReport(nil), m.snapshot.Surges...)
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
	header := fmt.Sprintf("%s (%s)  %s  │  %s", m.selected.Name, m.selected.Code, interval.KoreanName(), legend)
	chartHeight := max(10, m.height-10)
	return header + "\n" + panel.Render(renderChart(m.candles, m.width-6, chartHeight))
}

func (m Model) helpView() string {
	return panel.Render("단축키\n\n1~5 화면 이동   ↑/↓ 또는 j/k 선택   Enter 상세보기\n/ 종목 검색      상세에서 ←/→ 또는 h/l 봉 단위 변경\na 관심종목 추가  관심 화면에서 d 로컬 항목 삭제\nr 새로고침       Esc 뒤로            q 종료\n\n상세 차트: 틱·1/5/15/60분·일·주·월·년 / MA5·20·60·120")
}
func (m Model) footer() string {
	status := ""
	if m.loading {
		status = "  동기화 중…"
	}
	return muted.Render(" ↑↓ 선택  Enter 상세  / 검색  r 새로고침  ? 도움말  q 종료" + status)
}
func money(v decimal.Decimal) string { return v.Round(0).StringFixed(0) }
func signedMoney(v decimal.Decimal) string {
	if v.IsPositive() {
		return "+" + money(v)
	}
	return money(v)
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
