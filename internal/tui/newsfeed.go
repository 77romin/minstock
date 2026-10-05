package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/77romin/minstock-tui/internal/app"
	"github.com/77romin/minstock-tui/internal/domain"
)

type newsFeedMsg struct {
	request uint64
	cached  bool
	report  app.NewsFeedReport
}
type newsFeedReadMsg struct {
	id  string
	at  time.Time
	err error
}
type newsFeedOpenedMsg struct {
	id  string
	err error
}

func (m Model) loadNewsFeed(us ...bool) (tea.Model, tea.Cmd) {
	usOnly := len(us) > 0 && us[0]
	m.feedUS = usOnly
	if m.feedCancel != nil {
		m.feedCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	m.feedCancel = cancel
	m.notice = ""
	m.err = nil
	m.feedLoading, m.feedLive = true, false
	m.feedRequest++
	symbols := app.NewsFeedSymbols(m.snapshot, m.feedWatchlist)
	request, offset, service := m.feedRequest, m.feedOffset, m.service
	if usOnly {
		count := 0
		for _, symbol := range symbols {
			if symbol.Currency == domain.USD || symbol.Market == domain.MarketUS {
				count++
			}
		}
		if m.feedUSOffset >= count {
			m.feedUSOffset = 0
		}
		offset = m.feedUSOffset
		m.feedUSOffset += app.NewsFeedBatchSize
	}
	return m, tea.Batch(func() tea.Msg {
		cacheCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		return newsFeedMsg{request: request, cached: true, report: service.NewsFeed(cacheCtx, symbols, offset, false)}
	}, func() tea.Msg {
		defer cancel()
		if usOnly {
			return newsFeedMsg{request: request, report: service.NewsFeedUS(ctx, symbols, offset)}
		}
		return newsFeedMsg{request: request, report: service.NewsFeed(ctx, symbols, offset, true)}
	})
}

func (m Model) scopedFeed() []app.NewsFeedEntry {
	var result []app.NewsFeedEntry
	symbolKey := ""
	if m.feedSymbol > 0 && m.feedSymbol <= len(m.feed.Symbols) {
		symbolKey = app.NewsFeedSymbolKey(m.feed.Symbols[m.feedSymbol-1])
	}
	for _, entry := range m.feed.Entries {
		if m.feedUnread && !entry.ReadAt.IsZero() {
			continue
		}
		if m.feedKind == 1 && entry.Item.Kind != domain.InformationNews {
			continue
		}
		if m.feedKind == 2 && entry.Item.Kind != domain.InformationDisclosure {
			continue
		}
		match := symbolKey == ""
		for _, symbol := range entry.Symbols {
			match = match || app.NewsFeedSymbolKey(symbol) == symbolKey
		}
		if match {
			result = append(result, entry)
		}
	}
	return result
}

func (m Model) feedEntrySymbols(entry app.NewsFeedEntry) []domain.Symbol {
	if m.feedSymbol <= 0 || m.feedSymbol > len(m.feed.Symbols) {
		return entry.Symbols
	}
	key := app.NewsFeedSymbolKey(m.feed.Symbols[m.feedSymbol-1])
	var result []domain.Symbol
	for _, symbol := range entry.Symbols {
		if app.NewsFeedSymbolKey(symbol) == key {
			result = append(result, symbol)
		}
	}
	return result
}

func (m Model) filteredFeed() []app.NewsFeedEntry {
	var result []app.NewsFeedEntry
	for _, entry := range m.scopedFeed() {
		if m.newsPreferences.Allows(entry.Item, m.feedEntrySymbols(entry)) {
			result = append(result, entry)
		}
	}
	return result
}

func (m Model) compactFeedQualityInfo() bool {
	return m.feedInfo && m.height < 28 && m.newsFilterSummary() != ""
}

func (m Model) feedWarningBatchSize() int {
	if m.compactFeedQualityInfo() {
		return 1
	}
	return 2
}

func (m *Model) feedMarkRead(id string, read bool) tea.Cmd {
	if m.feedReadPending == nil {
		m.feedReadPending = map[string]bool{}
	}
	if m.feedReadPending[id] {
		return nil
	}
	m.feedReadPending[id] = true
	service := m.service
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		at, err := service.SetNewsFeedRead(ctx, id, read)
		return newsFeedReadMsg{id: id, at: at, err: err}
	}
}

func (m Model) handleFeedKey(key string) (tea.Model, tea.Cmd, bool) {
	items := m.filteredFeed()
	switch key {
	case "i":
		m.feedInfo = !m.feedInfo
	case "tab", "shift+tab":
		m.feedWatchlist = !m.feedWatchlist
		m.feedOffset, m.feedSymbol, m.cursor = 0, 0, 0
		m.feedUSOffset = 0
		m.feed = app.NewsFeedReport{}
		next, cmd := m.loadNewsFeed()
		return next, cmd, true
	case "f":
		m.feedKind = (m.feedKind + 1) % 3
		m.cursor = 0
	case "s":
		m.feedSymbol = (m.feedSymbol + 1) % (len(m.feed.Symbols) + 1)
		m.cursor = 0
	case "u":
		m.feedUnread = !m.feedUnread
		m.cursor = 0
	case "w":
		m.feedInfo = true
		batch := m.feedWarningBatchSize()
		m.feedWarningPage = (m.feedWarningPage + 1) % max(1, (len(m.feed.Warnings)+batch-1)/batch)
	case "r":
		next, cmd := m.loadNewsFeed()
		return next, cmd, true
	case "a":
		next, cmd := m.loadNewsFeed(true)
		return next, cmd, true
	case "n":
		m.feedOffset += app.NewsFeedBatchSize
		domestic := 0
		for _, symbol := range m.feed.Symbols {
			if symbol.Currency != domain.USD && symbol.Market != domain.MarketUS {
				domestic++
			}
		}
		if m.feedOffset >= domestic {
			m.feedOffset = 0
		}
		next, cmd := m.loadNewsFeed()
		return next, cmd, true
	case "enter", "o":
		if m.cursor < len(items) {
			item := items[m.cursor]
			return m, informationOpenWithCallback(item.Item.URL, func(err error) tea.Msg { return newsFeedOpenedMsg{id: item.ID, err: err} }), true
		}
	case "x":
		if m.cursor < len(items) {
			item := items[m.cursor]
			if m.feedReadPending == nil {
				m.feedReadPending = map[string]bool{}
			}
			cmd := m.feedMarkRead(item.ID, item.ReadAt.IsZero())
			return m, cmd, true
		}
	default:
		return m, nil, false
	}
	return m, nil, true
}

func (m Model) newsFeedView() string {
	width := max(40, m.width-4)
	contentWidth := width - 4
	scope := "보유종목"
	if m.feedWatchlist {
		scope = "관심종목"
	}
	kind := []string{"전체", "뉴스", "공시"}[m.feedKind]
	symbol := "전체 종목"
	if m.feedSymbol > 0 && m.feedSymbol <= len(m.feed.Symbols) {
		selected := m.feed.Symbols[m.feedSymbol-1]
		symbol = domain.InformationText(selected.Name)
		if symbol == "" {
			symbol = domain.InformationText(selected.Code)
		}
	}
	read := "전체"
	if m.feedUnread {
		read = "안 읽음"
	}
	lines := []string{trimDisplay("통합 뉴스·공시 · "+scope+" (Tab)", contentWidth), trimDisplay("종류 "+kind+" (f) · 종목 "+symbol+" (s) · "+read+" (u)", contentWidth)}
	state := "캐시 표시"
	if m.feedLive {
		state = "조회 완료 · 종목별 캐시 주기 적용"
	}
	if m.feedLoading {
		state = "캐시 우선 · 최신 조회 중…"
	}
	mode := "국내 NAVER·DART"
	if m.feedUS {
		mode = "미국 별도 조회"
	}
	state = mode + " · " + state
	lines = append(lines, trimDisplay(fmt.Sprintf("%s · 대상 %d · 이번 조회 %d", state, m.feed.Total, m.feed.Queried), contentWidth))
	if m.feed.Limited {
		lines = append(lines, trimDisplay(fmt.Sprintf("조회 구간 %d~%d · 나머지 캐시 · n 국내 / a 미국 다음", m.feed.Offset+1, min(m.feed.RefreshTotal, m.feed.Offset+app.NewsFeedBatchSize)), contentWidth))
	}
	warningBatch := m.feedWarningBatchSize()
	warningPages := max(1, (len(m.feed.Warnings)+warningBatch-1)/warningBatch)
	warningStart := (m.feedWarningPage % warningPages) * warningBatch
	if m.feedInfo {
		for i := warningStart; i < min(len(m.feed.Warnings), warningStart+warningBatch); i++ {
			line := domain.InformationText(m.feed.Warnings[i])
			if warningBatch == 1 {
				line = fmt.Sprintf("안내 %d/%d(w) · %s", warningStart+1, warningPages, line)
			}
			lines = append(lines, trimDisplay(line, contentWidth))
		}
	} else if len(m.feed.Warnings) > 0 {
		lines = append(lines, fmt.Sprintf("조회 안내 %d건 · i 상태 보기", len(m.feed.Warnings)))
	}
	if m.feedInfo && warningBatch > 1 && len(m.feed.Warnings) > warningBatch {
		lines = append(lines, fmt.Sprintf("안내 %d/%d · w 다음 안내", warningStart/warningBatch+1, warningPages))
	}
	items := m.filteredFeed()
	if hint := m.newsFilterSummary(); hint != "" {
		lines = append(lines, trimDisplay(hint, contentWidth))
	}
	if m.feedInfo {
		for _, line := range m.newsFilterDetails() {
			lines = append(lines, trimDisplay(line, contentWidth))
		}
	}
	var detail []string
	if m.cursor >= 0 && m.cursor < len(items) {
		entry := items[m.cursor]
		var related []string
		for _, symbol := range entry.Symbols {
			related = append(related, domain.InformationText(symbol.Name)+"("+domain.InformationText(symbol.Code)+")")
		}
		reserve := 15
		if m.feedInfo {
			reserve++
		}
		detail = informationSelection(entry.Item, contentWidth, max(1, m.height-len(lines)-reserve), m.feedInfo)
		detail = append(detail, trimDisplay("관련 종목 · "+strings.Join(related, ", "), contentWidth))
		if m.feedInfo && !m.compactFeedQualityInfo() {
			stamp := "미조회"
			if !entry.ObservedAt.IsZero() {
				stamp = freshnessName(entry.Freshness) + " · " + entry.ObservedAt.In(time.FixedZone("KST", 9*60*60)).Format("01-02 15:04")
			}
			detail = append(detail, trimDisplay("데이터 조회 "+stamp, contentWidth))
		}
	}
	lines = append(lines, "")
	visible := max(1, m.height-len(lines)-len(detail)-7)
	start := max(0, m.cursor-visible+1)
	end := min(len(items), start+visible)
	for i := start; i < end; i++ {
		entry := items[i]
		status := "●"
		if !entry.ReadAt.IsZero() {
			status = "○"
		}
		lines = append(lines, selectLine(informationRow(entry.Item, cursor(i, m.cursor)+" "+status, contentWidth), i == m.cursor))
	}
	if len(items) == 0 {
		message := "현재 필터에 표시할 뉴스·공시가 없습니다."
		if len(m.scopedFeed()) > 0 {
			message = "품질 필터로 모두 제외되었습니다 · F 초기화"
		}
		if m.feedLoading {
			message = "캐시와 최신 소식을 불러오고 있습니다…"
		} else if m.feed.Total == 0 {
			message = "대상 종목이 없습니다. 보유 계좌를 갱신하거나 관심종목을 추가하세요."
		}
		lines = append(lines, trimDisplay(message, contentWidth))
	}
	lines = append(lines, detail...)
	if m.notice != "" {
		lines = append(lines, trimDisplay(domain.InformationText(m.notice), contentWidth))
	}
	lines = append(lines, "", trimDisplay(fmt.Sprintf("%d건 · ● 안 읽음 / ○ 읽음 · Enter/o 원문 · x 읽음 전환", len(items)), contentWidth))
	return panel.Width(width).Render(strings.Join(lines, "\n"))
}
