package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/77romin/minstock-tui/internal/app"
	"github.com/77romin/minstock-tui/internal/domain"
)

func TestNewsQualitySharedViewsAndLateSettings(t *testing.T) {
	symbol := domain.Symbol{Code: "QQQ", Ticker: "QQQ"}
	m := Model{screen: detailScreen, informationTab: true, selected: symbol}
	m.information.Items = []domain.InformationItem{
		{Kind: domain.InformationNews, Title: "RSQQQ price", Source: "noise"},
		{Kind: domain.InformationNews, Title: "QQQ news", Source: "good"},
	}
	next, cmd := m.handleKey("z")
	m = next.(Model)
	if cmd != nil || len(m.filteredInformation()) != 1 || m.filteredInformation()[0].Title != "QQQ news" {
		t.Fatal("strict filter failed")
	}
	next, _ = m.Update(newsPreferencesMsg{})
	m = next.(Model)
	if !m.newsPreferences.DirectMention {
		t.Fatal("late initial preferences replaced user setting")
	}
	next, cmd = m.handleKey("h")
	m = next.(Model)
	if cmd != nil || len(m.filteredInformation()) != 0 || len(m.information.Items) != 2 {
		t.Fatal("hide mutated source data")
	}
	next, _ = m.handleKey("H")
	m = next.(Model)
	if len(m.filteredInformation()) != 1 {
		t.Fatal("restore failed")
	}
	m.screen = newsFeedScreen
	m.feed.Entries = []app.NewsFeedEntry{{Item: m.information.Items[0], Symbols: []domain.Symbol{symbol}}, {Item: m.information.Items[1], Symbols: []domain.Symbol{symbol}}}
	if len(m.filteredFeed()) != 1 {
		t.Fatal("feed did not reuse preferences")
	}
	next, _ = m.handleKey("/")
	m = next.(Model)
	for _, key := range []string{"배", "당", "backspace", "esc"} {
		next, _ = m.handleKey(key)
		m = next.(Model)
	}
	if m.newsPreferences.Keyword != "" || m.newsKeywordEditing {
		t.Fatal("cancel persisted input")
	}
}

func TestQualityAndPlannerViewsFit(t *testing.T) {
	for _, width := range []int{80, 120} {
		m := Model{screen: detailScreen, width: width, height: 24, informationTab: true, selected: domain.Symbol{Code: "QQQ"}, newsPreferences: domain.NewsPreferences{DirectMention: true, Keyword: "QQQ"}}
		for i := 0; i < 20; i++ {
			m.information.Items = append(m.information.Items, domain.InformationItem{Kind: domain.InformationNews, Title: fmt.Sprintf("QQQ long title %d %s", i, strings.Repeat("한글 뉴스 ", 20)), Source: "Yahoo", URL: "https://example.com", PublishedAt: time.Now()})
		}
		m.informationCursor = 19
		if view := m.informationView(); lipgloss.Width(view) > width || lipgloss.Height(view) > m.height-2 {
			t.Fatalf("news view overflow %dx%d", lipgloss.Width(view), lipgloss.Height(view))
		}
		m.screen = allocationScreen
		m.rebalanceView = true
		if view := m.rebalancePlanView(); lipgloss.Width(view) > width || lipgloss.Height(view) > m.height-2 {
			t.Fatal("planner overflow")
		}
	}
}

func TestQualityStatsAndPresetsPreserveDisclosuresAndSourceData(t *testing.T) {
	symbol := domain.Symbol{Code: "QQQ", Ticker: "QQQ"}
	m := Model{screen: detailScreen, informationTab: true, width: 100, height: 30, selected: symbol,
		newsPreferences: domain.NewsPreferences{DirectMention: true, MinRelevance: 0.6, HideTemplates: true, HiddenSources: []string{"hidden"}}}
	m.information.Items = []domain.InformationItem{
		{Kind: domain.InformationNews, Title: "QQQ to KRW price", Relevance: "1"},
		{Kind: domain.InformationNews, Title: "QQQ weak connection", Relevance: "0.1"},
		{Kind: domain.InformationNews, Title: "Other company earnings", Relevance: "1"},
		{Kind: domain.InformationNews, Title: "QQQ earnings", Source: "hidden", Relevance: "1"},
		{Kind: domain.InformationNews, Title: "QQQ earnings from publisher"},
		{Kind: domain.InformationDisclosure, Title: "공시", Source: "hidden"},
	}
	stats := m.newsFilterStats()
	if stats.total != 6 || stats.shown != 2 || stats.unknownRelevance != 1 {
		t.Fatalf("bad stats %+v", stats)
	}
	sum := 0
	for _, count := range stats.reasons {
		sum += count
	}
	if sum != 4 || !strings.Contains(m.newsFilterSummary(), "2/6") {
		t.Fatal("reasons not additive")
	}
	next, cmd := m.handleKey("F")
	m = next.(Model)
	if cmd != nil || len(m.filteredInformation()) != 6 || len(m.information.Items) != 6 {
		t.Fatal("reset deleted source data or fetched network")
	}
	for _, want := range []string{"기본", "엄격", "전체"} {
		next, cmd = m.handleKey("p")
		m = next.(Model)
		if cmd != nil || m.newsPreferences.QualityMode() != want {
			t.Fatal("preset cycle failed")
		}
	}
}

func TestQualityStatsRespectFeedScopeAndClampLateFilteredReplies(t *testing.T) {
	qqq, voo := domain.Symbol{Code: "QQQ"}, domain.Symbol{Code: "VOO"}
	m := Model{screen: newsFeedScreen, feedSymbol: 1, feedUnread: true, newsPreferences: domain.NewsPreferences{DirectMention: true}}
	m.feed = app.NewsFeedReport{Symbols: []domain.Symbol{qqq, voo}, Entries: []app.NewsFeedEntry{
		{Item: domain.InformationItem{Title: "QQQ earnings"}, Symbols: []domain.Symbol{qqq}},
		{Item: domain.InformationItem{Title: "VOO earnings"}, Symbols: []domain.Symbol{qqq, voo}},
		{Item: domain.InformationItem{Title: "QQQ read article"}, Symbols: []domain.Symbol{qqq}, ReadAt: time.Now()},
	}}
	if stats := m.newsFilterStats(); stats.total != 2 || stats.shown != 1 {
		t.Fatalf("scope counts %+v", stats)
	}
	m = Model{screen: detailScreen, informationTab: true, selected: qqq, informationCursor: 20, newsPreferences: domain.NewsPreferences{DirectMention: true}}
	report := app.InformationReport{Items: []domain.InformationItem{{Title: "irrelevant"}, {Title: "QQQ earnings"}}}
	next, _ := m.Update(informationMsg{symbol: qqq, report: report})
	m = next.(Model)
	if m.informationCursor != 0 || len(m.filteredInformation()) != 1 {
		t.Fatal("late filtered reply left invalid selection")
	}
	m.newsPreferences = domain.NewsPreferences{}
	m.informationCursor = 1
	next, _ = m.Update(newsPreferencesMsg{preferences: domain.NewsPreferences{DirectMention: true}})
	if next.(Model).informationCursor != 0 {
		t.Fatal("loaded settings did not clamp selection")
	}
}

func TestExpandedQualityViewsWithWarningsFit(t *testing.T) {
	for _, width := range []int{80, 120} {
		for _, target := range []screen{detailScreen, newsFeedScreen} {
			m := Model{screen: target, width: width, height: 24, selected: domain.Symbol{Code: "QQQ"}, informationTab: true, informationInfo: true, feedInfo: true,
				newsPreferences: domain.NewsPreferences{DirectMention: true, MinRelevance: 0.6, HideTemplates: true}, notice: "뉴스 필터 적용"}
			item := domain.InformationItem{Kind: domain.InformationNews, Title: "QQQ " + strings.Repeat("긴 제목 ", 20), Source: "publisher", Relevance: "1", PublishedAt: time.Now(), URL: "https://example.com/article"}
			m.information = app.InformationReport{Items: []domain.InformationItem{item}, Sources: []app.InformationSourceStatus{{Source: "NAVER", Warning: "조회 안내"}, {Source: "DART", Warning: "공시 안내"}}}
			m.feed = app.NewsFeedReport{Total: 1, Limited: true, Warnings: []string{"안내1", "안내2", "안내3"}, Entries: []app.NewsFeedEntry{{Item: item, Symbols: []domain.Symbol{m.selected}}}}
			view := m.View().Content
			if lipgloss.Width(view) > width || lipgloss.Height(view) > 24 {
				t.Fatalf("screen=%v overflow %dx%d\n%s", target, lipgloss.Width(view), lipgloss.Height(view), view)
			}
			if !strings.Contains(view, "선택 기사") || !strings.Contains(view, "품질 엄격") {
				t.Fatal("selected article or quality mode hidden")
			}
		}
	}
}
