package tui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/77romin/minstock-tui/internal/domain"
)

type newsPreferencesMsg struct {
	preferences domain.NewsPreferences
	err         error
}
type newsPreferencesSavedMsg struct{ err error }

func (m Model) newsPreferencesCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		p, err := m.service.NewsPreferences(ctx)
		return newsPreferencesMsg{p, err}
	}
}

func (m Model) filteredInformation() []domain.InformationItem {
	var items []domain.InformationItem
	for _, item := range m.information.Items {
		if m.newsPreferences.Allows(item, []domain.Symbol{m.selected}) {
			items = append(items, item)
		}
	}
	return items
}

func (m Model) newsFilterSummary() string {
	if m.newsKeywordEditing {
		return "제목 키워드: " + m.newsKeywordInput + "_ · Enter 적용 / Esc 취소"
	}
	p := m.newsPreferences
	if !p.DirectMention && p.MinRelevance == 0 && p.Keyword == "" && len(p.HiddenSources) == 0 && !p.HideTemplates {
		return ""
	}
	stats := m.newsFilterStats()
	return fmt.Sprintf("품질 %s(p) · 표시 %d/%d건 · 제외 %d · i 근거 / F 초기화", p.QualityMode(), stats.shown, stats.total, stats.total-stats.shown)
}

type newsFilterStats struct {
	total, shown, unknownRelevance int
	reasons                        map[domain.NewsExclusion]int
}

func (m Model) newsFilterStats() newsFilterStats {
	stats := newsFilterStats{reasons: map[domain.NewsExclusion]int{}}
	add := func(item domain.InformationItem, symbols []domain.Symbol) {
		stats.total++
		if reason := m.newsPreferences.ExclusionReason(item, symbols); reason != "" {
			stats.reasons[reason]++
		} else {
			stats.shown++
		}
		if item.Kind == domain.InformationNews {
			if _, known := domain.NewsRelevanceScore(item); !known {
				stats.unknownRelevance++
			}
		}
	}
	if m.screen == newsFeedScreen {
		for _, entry := range m.scopedFeed() {
			add(entry.Item, m.feedEntrySymbols(entry))
		}
	} else {
		for _, item := range m.information.Items {
			add(item, []domain.Symbol{m.selected})
		}
	}
	return stats
}

func (m Model) newsFilterDetails() []string {
	if m.newsFilterSummary() == "" {
		return nil
	}
	p, stats := m.newsPreferences, m.newsFilterStats()
	mention, templates := "끔", "표시"
	if p.DirectMention {
		mention = "켬"
	}
	if p.HideTemplates {
		templates = "숨김"
	}
	keyword := p.Keyword
	if keyword == "" {
		keyword = "없음"
	}
	settings := fmt.Sprintf("직접 언급 %s(z) · 관련도 %.1f(m) · 반복형 %s(v) · 검색 %s(/)", mention, p.MinRelevance, templates, keyword)
	var reasons []string
	for _, reason := range []domain.NewsExclusion{domain.NewsExcludeKeyword, domain.NewsExcludeSource, domain.NewsExcludeTemplate, domain.NewsExcludeRelevance, domain.NewsExcludeMention} {
		if stats.reasons[reason] > 0 {
			reasons = append(reasons, fmt.Sprintf("%s %d", reason, stats.reasons[reason]))
		}
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "없음")
	}
	detail := "제외 근거 · " + strings.Join(reasons, " / ")
	if p.MinRelevance > 0 && stats.unknownRelevance > 0 {
		detail += fmt.Sprintf(" · 점수 미제공/무효 %d건 미적용", stats.unknownRelevance)
	}
	if len(p.HiddenSources) > 0 {
		detail += " · 숨김 " + strings.Join(p.HiddenSources[:min(3, len(p.HiddenSources))], ", ") + "(H 복원)"
	}
	return []string{settings, detail}
}

func (m Model) handleNewsQualityKey(key string) (tea.Model, tea.Cmd, bool) {
	if m.newsPreferencesLoading && len(key) == 1 && strings.Contains("zmhH/pvF", key) {
		m.notice = "뉴스 필터 설정을 불러오는 중입니다"
		return m, nil, true
	}
	if m.newsPreferencesPending {
		if strings.Contains("zmhH/pvF", key) && len(key) == 1 {
			return m, nil, true
		}
		return m, nil, false
	}
	if m.newsKeywordEditing {
		switch key {
		case "esc":
			m.newsKeywordEditing = false
			return m, nil, true
		case "enter":
			m.newsPreferences.Keyword = m.newsKeywordInput
			m.newsKeywordEditing = false
		case "space":
			if utf8.RuneCountInString(m.newsKeywordInput) < 80 {
				m.newsKeywordInput += " "
			}
			return m, nil, true
		case "backspace":
			_, size := utf8.DecodeLastRuneInString(m.newsKeywordInput)
			if size > 0 {
				m.newsKeywordInput = m.newsKeywordInput[:len(m.newsKeywordInput)-size]
			}
			return m, nil, true
		default:
			if utf8.RuneCountInString(key) == 1 && utf8.RuneCountInString(m.newsKeywordInput) < 80 {
				m.newsKeywordInput += key
			}
			return m, nil, true
		}
	} else {
		switch key {
		case "p":
			m.newsPreferences = m.newsPreferences.NextQualityMode()
		case "v":
			m.newsPreferences.HideTemplates = !m.newsPreferences.HideTemplates
		case "F":
			m.newsPreferences = domain.NewsPreferences{}
		case "z":
			m.newsPreferences.DirectMention = !m.newsPreferences.DirectMention
		case "m":
			switch m.newsPreferences.MinRelevance {
			case 0:
				m.newsPreferences.MinRelevance = 0.3
			case 0.3:
				m.newsPreferences.MinRelevance = 0.6
			default:
				m.newsPreferences.MinRelevance = 0
			}
		case "/":
			m.newsKeywordEditing, m.newsKeywordInput = true, m.newsPreferences.Keyword
			return m, nil, true
		case "H":
			m.newsPreferences.HiddenSources = nil
		case "h":
			var item domain.InformationItem
			if m.screen == newsFeedScreen {
				items := m.filteredFeed()
				if m.cursor < len(items) && m.cursor >= 0 {
					item = items[m.cursor].Item
				}
			} else {
				items := m.filteredInformation()
				if m.informationCursor < len(items) && m.informationCursor >= 0 {
					item = items[m.informationCursor]
				}
			}
			if item.Kind != domain.InformationNews || item.Source == "" {
				m.notice = "선택한 뉴스 매체만 숨길 수 있습니다"
				return m, nil, true
			}
			m.newsPreferences.HiddenSources = append(append([]string(nil), m.newsPreferences.HiddenSources...), item.Source)
		default:
			return m, nil, false
		}
	}
	m.newsPreferences = m.newsPreferences.Normalized()
	m.newsPreferencesEdited = true
	m.cursor, m.informationCursor = 0, 0
	m.notice = "뉴스 필터 적용 · 추가 API 호출 없음"
	if m.service == nil {
		return m, nil, true
	}
	m.newsPreferencesPending = true
	p, service := m.newsPreferences, m.service
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return newsPreferencesSavedMsg{service.SaveNewsPreferences(ctx, p)}
	}, true
}
