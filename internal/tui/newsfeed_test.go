package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/77romin/minstock-tui/internal/adapters/mock"
	db "github.com/77romin/minstock-tui/internal/adapters/sqlite"
	"github.com/77romin/minstock-tui/internal/app"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/ports"
)

func feedTestModel() Model {
	a := domain.Symbol{Code: "005930", Name: "삼성전자", Currency: domain.KRW}
	b := domain.Symbol{Code: "000660", Name: "SK하이닉스", Currency: domain.KRW}
	return Model{screen: newsFeedScreen, width: 120, height: 24, feed: app.NewsFeedReport{Symbols: []domain.Symbol{a, b}, Total: 2, Entries: []app.NewsFeedEntry{
		{ID: app.InformationArticleID("https://example.com/a"), Item: domain.InformationItem{Kind: domain.InformationNews, Title: "기사", URL: "https://example.com/a", PublishedAt: time.Now()}, Symbols: []domain.Symbol{a, b}},
		{ID: app.InformationArticleID("https://example.com/b"), Item: domain.InformationItem{Kind: domain.InformationDisclosure, Title: "공시", URL: "https://example.com/b", PublishedAt: time.Now()}, Symbols: []domain.Symbol{b}, ReadAt: time.Now()},
	}}}
}
func TestNewsFeedFiltersDoNotRequestAPI(t *testing.T) {
	m := feedTestModel()
	for _, key := range []string{"f", "s", "u"} {
		next, cmd, handled := m.handleFeedKey(key)
		if !handled || cmd != nil {
			t.Fatal("filter created network request")
		}
		m = next.(Model)
	}
	if len(m.filteredFeed()) != 1 || m.filteredFeed()[0].Item.Title != "기사" {
		t.Fatal("combined filters failed")
	}
	m.feedKind = 2
	if len(m.filteredFeed()) != 0 {
		t.Fatal("unread/stock filters ignored")
	}
	m.feedUnread = false
	m.feedSymbol = 2
	if len(m.filteredFeed()) != 1 {
		t.Fatal("disclosure filter failed")
	}
}
func TestNewsFeedRejectsStaleAndLateCachedResponses(t *testing.T) {
	m := feedTestModel()
	m.feedRequest = 2
	next, _ := m.Update(newsFeedMsg{request: 1})
	if len(next.(Model).feed.Entries) != 2 {
		t.Fatal("old request accepted")
	}
	next, _ = m.Update(newsFeedMsg{request: 2, report: m.feed})
	m = next.(Model)
	next, _ = m.Update(newsFeedMsg{request: 2, cached: true})
	if len(next.(Model).feed.Entries) != 2 {
		t.Fatal("late cache overwrote live report")
	}
	m.screen = portfolioScreen
	m.cursor = 7
	next, _ = m.Update(newsFeedMsg{request: 2, report: app.NewsFeedReport{}})
	if next.(Model).cursor != 7 {
		t.Fatal("background response changed another screen's selection")
	}
}
func TestNewsFeedReadOverlaySurvivesInFlightResponse(t *testing.T) {
	m := feedTestModel()
	id := m.feed.Entries[0].ID
	m.feedRequest = 1
	next, _ := m.Update(newsFeedReadMsg{id: id, at: time.Now()})
	m = next.(Model)
	old := feedTestModel().feed
	next, _ = m.Update(newsFeedMsg{request: 1, report: old})
	m = next.(Model)
	if m.feed.Entries[0].ReadAt.IsZero() {
		t.Fatal("in-flight report erased read update")
	}
	next, _ = m.Update(newsFeedReadMsg{id: id})
	m = next.(Model)
	next, _ = m.Update(newsFeedMsg{request: 1, report: old})
	if !next.(Model).feed.Entries[0].ReadAt.IsZero() {
		t.Fatal("unread override lost")
	}
	next, cmd := m.Update(newsFeedOpenedMsg{id: id, err: fmt.Errorf("open failed")})
	if cmd != nil || !next.(Model).feed.Entries[0].ReadAt.IsZero() {
		t.Fatal("failed open marked read")
	}
}
func TestNewsFeedViewFitsAndScrolls(t *testing.T) {
	for _, width := range []int{80, 120} {
		m := feedTestModel()
		m.width = width
		m.feed.Limited = true
		m.feed.Total = 30
		m.feed.Warnings = []string{"NAVER 호출 제한", "DART 실패", "추가 안내"}
		m.feed.Entries = nil
		m.notice = "읽음 상태 저장 실패"
		for i := 0; i < 20; i++ {
			m.feed.Entries = append(m.feed.Entries, app.NewsFeedEntry{Item: domain.InformationItem{Kind: domain.InformationNews, Title: fmt.Sprintf("기사 %d", i), URL: "https://example.com/a", PublishedAt: time.Now()}})
		}
		m.cursor = 19
		view := m.newsFeedView()
		if lipgloss.Width(view) > width || lipgloss.Height(view) > m.height-2 {
			t.Fatalf("does not fit %dx24: %dx%d\n%s", width, lipgloss.Width(view), lipgloss.Height(view), view)
		}
		if !strings.Contains(view, "기사 19") || !strings.Contains(view, "캐시") {
			t.Fatal("selection or cache status missing")
		}
		full := m.View().Content
		if lipgloss.Width(full) > width || lipgloss.Height(full) > 24 {
			t.Fatalf("full feed does not fit %dx24: %dx%d", width, lipgloss.Width(full), lipgloss.Height(full))
		}
	}
}

func TestNewsFeedModelLoadsAndPersistsReadsEndToEnd(t *testing.T) {
	repo, err := db.Open(filepath.Join(t.TempDir(), "feed.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	p := mock.New()
	service := app.New(repo, []ports.Provider{p}, nil, nil, []ports.FXProvider{p})
	service.SetInformationProviders([]ports.InformationProvider{p})
	m := New(service, "demo", time.Minute)
	m.snapshot = service.Dashboard(t.Context())
	m.loading = false
	next, cmd := m.Update(tea.KeyPressMsg{Code: '0'})
	m = next.(Model)
	if cmd == nil || !m.feedLoading {
		t.Fatal("news tab did not load")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatal("missing cache/live commands")
	}
	// Deliver cache after live to exercise the real command/generation guards.
	next, _ = m.Update(batch[1]())
	m = next.(Model)
	next, _ = m.Update(batch[0]())
	m = next.(Model)
	if m.feedLoading || len(m.feed.Entries) != 4 {
		t.Fatalf("demo feed load: %#v", m.feed)
	}
	next, cmd = m.handleKey("a")
	m = next.(Model)
	batch = cmd().(tea.BatchMsg)
	next, _ = m.Update(batch[1]())
	m = next.(Model)
	if m.feedLoading || len(m.feed.Entries) != 5 || m.feed.Queried != 1 {
		t.Fatalf("manual US feed load: %#v", m.feed)
	}
	id := m.filteredFeed()[0].ID
	next, cmd = m.handleKey("x")
	m = next.(Model)
	if cmd == nil {
		t.Fatal("mark-read did not schedule persistence")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.feed.Entries[0].ReadAt.IsZero() {
		t.Fatal("read response not applied")
	}
	states, err := repo.InformationReadStates(t.Context(), []string{id})
	if err != nil || states[id].IsZero() {
		t.Fatal("read marker not persisted")
	}
	next, _ = m.handleKey("u")
	m = next.(Model)
	if len(m.filteredFeed()) != 4 {
		t.Fatal("unread filter did not remove read entry")
	}
}

func TestInformationDetailManualUSKey(t *testing.T) {
	repo, err := db.Open(filepath.Join(t.TempDir(), "information.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	service := app.New(repo, nil, nil, nil, nil)
	service.SetInformationProviders([]ports.InformationProvider{mock.New()})
	m := Model{service: service, screen: detailScreen, informationTab: true, selected: domain.Symbol{Code: "AAPL", Currency: domain.USD}}
	for _, key := range []string{"r", "a", "r"} {
		next, cmd := m.handleKey(key)
		m = next.(Model)
		if cmd == nil {
			t.Fatalf("%s did not schedule lookup", key)
		}
		batch := cmd().(tea.BatchMsg)
		next, _ = m.Update(batch[1]())
		m = next.(Model)
		want := 1
		if key == "r" && !m.informationLive {
			t.Fatal("lookup did not complete")
		}
		if key == "r" && m.informationRequest == 1 {
			want = 0
		}
		if len(m.information.Items) != want {
			t.Fatalf("%s: got %d items, want %d", key, len(m.information.Items), want)
		}
	}
}

func TestNewsFeedKeepsStockFilterWhenTargetOrderChanges(t *testing.T) {
	m := feedTestModel()
	m.feedSymbol = 1
	m.feedRequest = 1
	report := m.feed
	report.Symbols = []domain.Symbol{m.feed.Symbols[1], m.feed.Symbols[0]}
	next, _ := m.Update(newsFeedMsg{request: 1, report: report})
	if next.(Model).feedSymbol != 2 {
		t.Fatal("stock filter switched to a different stock")
	}
}
func TestNewsFeedNavigationCancelsObsoleteRequest(t *testing.T) {
	m := feedTestModel()
	ctx, cancel := context.WithCancel(context.Background())
	m.feedCancel = cancel
	m.feedLoading = true
	m.feedRequest = 3
	next, _ := m.Update(tea.KeyPressMsg{Code: '1'})
	if ctx.Err() == nil || next.(Model).feedLoading || next.(Model).feedRequest != 4 {
		t.Fatal("leaving feed did not cancel/invalidate request")
	}
	m.screen = alertScreen
	m.nextPrimaryScreen(1)
	if m.screen != newsFeedScreen {
		t.Fatal("news tab missing from primary navigation")
	}
	m.nextPrimaryScreen(1)
	if m.screen != dashboardScreen {
		t.Fatal("news tab does not wrap to dashboard")
	}
}
