package tui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/77romin/minstock-tui/internal/app"
	"github.com/77romin/minstock-tui/internal/domain"
)

type informationMsg struct {
	symbol  domain.Symbol
	request uint64
	report  app.InformationReport
	cached  bool
}
type informationOpenedMsg struct{ err error }

func (m Model) openDetail(symbol domain.Symbol, previous screen) (tea.Model, tea.Cmd) {
	m.previous, m.screen, m.selected, m.loading, m.detailQuote = previous, detailScreen, symbol, true, m.snapshot.Quotes[symbol.Key()]
	m.informationTab, m.informationCursor, m.informationLoading, m.informationLive, m.informationPendingG = false, 0, false, false, false
	m.information = app.InformationReport{}
	m.informationRequest++
	m.err = nil
	return m, tea.Batch(m.candlesCmd(symbol), m.quoteCmd(symbol))
}

func (m Model) loadInformation() (tea.Model, tea.Cmd) {
	if m.informationLoading {
		return m, nil
	}
	m.informationLoading, m.informationLive = true, false
	m.informationRequest++
	return m, tea.Batch(m.informationCmd(true), m.informationCmd(false))
}

func (m Model) informationCmd(cached bool) tea.Cmd {
	symbol, request := m.selected, m.informationRequest
	return func() tea.Msg {
		timeout := 30 * time.Second
		if cached {
			timeout = time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		var report app.InformationReport
		if cached {
			report = m.service.CachedInformation(ctx, symbol)
		} else {
			report = m.service.Information(ctx, symbol)
		}
		return informationMsg{symbol: symbol, request: request, report: report, cached: cached}
	}
}

func (m Model) handleInformationKey(key string) (tea.Model, tea.Cmd, bool) {
	count := len(m.information.Items)
	switch key {
	case "j", "down":
		m.informationCursor = min(max(0, count-1), m.informationCursor+1)
	case "k", "up":
		m.informationCursor = max(0, m.informationCursor-1)
	case "ctrl+d":
		m.informationCursor = min(max(0, count-1), m.informationCursor+max(1, m.height/4))
	case "ctrl+u":
		m.informationCursor = max(0, m.informationCursor-max(1, m.height/4))
	case "G":
		m.informationCursor = max(0, count-1)
	case "g":
		if m.informationPendingG {
			m.informationCursor = 0
			m.informationPendingG = false
		} else {
			m.informationPendingG = true
		}
		return m, nil, true
	case "enter", "o":
		if m.informationCursor < count {
			return m, informationOpenCmd(m.information.Items[m.informationCursor].URL), true
		}
	case "r":
		next, cmd := m.loadInformation()
		return next, cmd, true
	default:
		return m, nil, false
	}
	m.informationPendingG = false
	return m, nil, true
}

func informationOpenCmd(raw string) tea.Cmd {
	target := domain.InformationURL(raw)
	if target == "" {
		return func() tea.Msg {
			return informationOpenedMsg{err: fmt.Errorf("유효한 HTTP/HTTPS 원문 링크가 없습니다")}
		}
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", target)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	return tea.ExecProcess(command, func(err error) tea.Msg { return informationOpenedMsg{err: err} })
}

func (m Model) informationView() string {
	width := max(40, m.width-4)
	contentWidth := width - 4
	name := domain.InformationText(m.selected.Name)
	lines := []string{trim(name+" ("+m.selected.Code+")", contentWidth), "정보·차트  [뉴스·공시] (Tab)", ""}
	loc := time.FixedZone("KST", 9*60*60)
	for _, source := range m.information.Sources {
		state := "미조회"
		if !source.AsOf.IsZero() {
			state = freshnessName(source.Freshness) + " · " + source.AsOf.In(loc).Format("01-02 15:04")
		}
		line := source.Source + " · " + state
		if source.Warning != "" {
			line += " · " + source.Warning
		}
		lines = append(lines, trim(line, contentWidth))
	}
	for _, warning := range m.information.Warnings {
		lines = append(lines, trim(warning, contentWidth))
	}
	if m.informationLoading {
		lines = append(lines, "뉴스·공시를 조회하고 있습니다…")
	}
	items := m.information.Items
	visible := max(1, (m.height-len(lines)-8)/2)
	start := max(0, m.informationCursor-visible+1)
	end := min(len(items), start+visible)
	for i := start; i < end; i++ {
		item := items[i]
		kind := "뉴스"
		date := item.PublishedAt.In(loc).Format("01-02 15:04")
		if item.Kind == domain.InformationDisclosure {
			kind = "공시"
		}
		if item.DateOnly {
			date = item.PublishedAt.In(loc).Format("01-02") + " 날짜"
		}
		title := fmt.Sprintf("%s %s [%s] %s", cursor(i, m.informationCursor), date, kind, item.Title)
		lines = append(lines, selectLine(trim(title, contentWidth), i == m.informationCursor))
		source := "   " + item.Source
		if item.Relevance != "" {
			source += " · 관련도 " + item.Relevance
		}
		lines = append(lines, trim(source, contentWidth))
	}
	if len(items) == 0 && !m.informationLoading {
		message := "최근 뉴스·공시가 없습니다."
		failed := len(m.information.Warnings) > 0
		for _, source := range m.information.Sources {
			failed = failed || source.Warning != ""
		}
		if failed {
			message = "설정 또는 조회 상태를 확인하세요."
		}
		lines = append(lines, message)
	}
	if m.informationCursor < len(items) {
		lines = append(lines, "", trim(items[m.informationCursor].URL, contentWidth))
	}
	lines = append(lines, "", trim("제목·출처·원문 링크 · 표시 시각 KST · r 조회는 캐시 주기를 따릅니다", contentWidth))
	if len(items) > 0 {
		lines = append(lines, fmt.Sprintf("%d/%d · Enter/o 원문 열기", m.informationCursor+1, len(items)))
	}
	return panel.Width(width).Render(strings.Join(lines, "\n"))
}
