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
	"github.com/charmbracelet/x/ansi"
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
	m.informationInfo = false
	m.informationRequest++
	m.err = nil
	m.chartErr, m.candles = nil, nil
	return m, tea.Batch(m.candlesCmd(symbol), m.quoteCmd(symbol))
}

func (m Model) loadInformation(us ...bool) (tea.Model, tea.Cmd) {
	if m.informationLoading {
		return m, nil
	}
	m.informationLoading, m.informationLive = true, false
	m.informationRequest++
	return m, tea.Batch(m.informationCmd(true), m.informationCmd(false, us...))
}

func (m Model) informationCmd(cached bool, us ...bool) tea.Cmd {
	usOnly := len(us) > 0 && us[0]
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
		} else if usOnly {
			report = m.service.InformationUS(ctx, symbol)
		} else {
			report = m.service.Information(ctx, symbol)
		}
		return informationMsg{symbol: symbol, request: request, report: report, cached: cached}
	}
}

func (m Model) handleInformationKey(key string) (tea.Model, tea.Cmd, bool) {
	items := m.filteredInformation()
	count := len(items)
	switch key {
	case "i":
		m.informationInfo = !m.informationInfo
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
			return m, informationOpenCmd(items[m.informationCursor].URL), true
		}
	case "r":
		next, cmd := m.loadInformation()
		return next, cmd, true
	case "a":
		if m.selected.Currency != domain.USD && m.selected.Market != domain.MarketUS {
			return m, nil, true
		}
		next, cmd := m.loadInformation(true)
		return next, cmd, true
	default:
		return m, nil, false
	}
	m.informationPendingG = false
	return m, nil, true
}

func informationOpenCmd(raw string) tea.Cmd {
	return informationOpenWithCallback(raw, func(err error) tea.Msg { return informationOpenedMsg{err: err} })
}

func informationOpenWithCallback(raw string, callback func(error) tea.Msg) tea.Cmd {
	target := domain.InformationURL(raw)
	if target == "" {
		return func() tea.Msg {
			return callback(fmt.Errorf("유효한 HTTP/HTTPS 원문 링크가 없습니다"))
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
	return tea.ExecProcess(command, callback)
}

func (m Model) informationView() string {
	width := max(40, m.width-4)
	contentWidth := width - 4
	name := domain.InformationText(m.selected.Name)
	lines := []string{trimDisplay(name+" ("+m.selected.Code+")", contentWidth), "정보·차트  [뉴스·공시] (Tab)"}
	loc := time.FixedZone("KST", 9*60*60)
	var statusLines []string
	warnings := len(m.information.Warnings)
	for _, source := range m.information.Sources {
		state := "미조회"
		if !source.AsOf.IsZero() {
			state = freshnessName(source.Freshness) + " · " + source.AsOf.In(loc).Format("01-02 15:04")
		}
		line := source.Source + " · " + state
		if source.Warning != "" {
			line += " · " + source.Warning
			warnings++
		}
		statusLines = append(statusLines, trimDisplay(domain.InformationText(line), contentWidth))
	}
	for _, warning := range m.information.Warnings {
		statusLines = append(statusLines, trimDisplay(domain.InformationText(warning), contentWidth))
	}
	if m.informationInfo {
		lines = append(lines, statusLines[:min(len(statusLines), max(1, min(6, m.height-12)))]...)
	} else if warnings > 0 {
		lines = append(lines, fmt.Sprintf("조회 안내 %d건 · i 상태 보기 · 미국 조회 a", warnings))
	}
	if m.informationLoading {
		lines = append(lines, "뉴스·공시를 조회하고 있습니다…")
	}
	if hint := m.newsFilterSummary(); hint != "" {
		lines = append(lines, trimDisplay(hint, contentWidth))
	}
	if m.informationInfo {
		for _, line := range m.newsFilterDetails() {
			lines = append(lines, trimDisplay(line, contentWidth))
		}
	}
	items := m.filteredInformation()
	var detail []string
	if m.informationCursor >= 0 && m.informationCursor < len(items) {
		detail = informationSelection(items[m.informationCursor], contentWidth, max(1, m.height-len(lines)-12), m.informationInfo)
	}
	lines = append(lines, "")
	visible := max(1, m.height-len(lines)-len(detail)-7)
	start := max(0, m.informationCursor-visible+1)
	end := min(len(items), start+visible)
	for i := start; i < end; i++ {
		lines = append(lines, selectLine(informationRow(items[i], cursor(i, m.informationCursor), contentWidth), i == m.informationCursor))
	}
	if len(items) == 0 && !m.informationLoading {
		message := "최근 뉴스·공시가 없습니다."
		if len(m.information.Items) > 0 {
			message = "품질 필터로 모두 제외되었습니다 · F 초기화"
		}
		failed := len(m.information.Warnings) > 0
		for _, source := range m.information.Sources {
			failed = failed || source.Warning != ""
		}
		if failed && len(m.information.Items) == 0 {
			message = "설정 또는 조회 상태를 확인하세요."
		}
		lines = append(lines, message)
	}
	lines = append(lines, detail...)
	if len(items) > 0 {
		lines = append(lines, fmt.Sprintf("%d/%d · Enter/o 원문 열기", m.informationCursor+1, len(items)))
	}
	return panel.Width(width).Render(strings.Join(lines, "\n"))
}

func informationRow(item domain.InformationItem, prefix string, width int) string {
	kind := "뉴스"
	if item.Kind == domain.InformationDisclosure {
		kind = "공시"
	}
	date := item.PublishedAt.In(time.FixedZone("KST", 9*60*60)).Format("01-02")
	source := fitCell(trimDisplay(domain.InformationText(item.Source), 10), 10, false)
	return trimDisplay(fmt.Sprintf("%s %s %s %s %s", prefix, date, kind, source, domain.InformationText(item.Title)), width)
}

func informationSelection(item domain.InformationItem, width, titleLimit int, info bool) []string {
	title := strings.Split(ansi.Wrap(domain.InformationText(item.Title), width, ""), "\n")
	// An unexpectedly large API title must not push the list off-screen.
	limit := max(1, min(6, titleLimit))
	if len(title) > limit {
		title = title[:limit]
		title[limit-1] = trimDisplay(title[limit-1]+" …", width)
	}
	lines := append([]string{"", "선택 기사"}, title...)
	stamp := item.PublishedAt.In(time.FixedZone("KST", 9*60*60)).Format("01-02 15:04 KST")
	if item.DateOnly {
		stamp = item.PublishedAt.In(time.FixedZone("KST", 9*60*60)).Format("01-02") + " 날짜만 제공"
	}
	lines = append(lines, trimDisplay(domain.InformationText(item.Source)+" · "+stamp, width), trimDisplay(domain.InformationText(item.URL), width))
	if info && item.Relevance != "" {
		lines = append(lines, trimDisplay("관련도 "+domain.InformationText(item.Relevance), width))
	}
	return lines
}
