package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestCompactHeaderUsesFullKoreanNamesWithoutDigits(t *testing.T) {
	for _, width := range []int{80, 100, 109} {
		m := Model{width: width, screen: newsFeedScreen, mode: "connected"}
		header := m.header()
		plain := ansi.Strip(header)
		for _, label := range []string{"현황", "주식", "검색", "관심", "급등", "성과", "배당", "비중", "알림", "뉴스"} {
			if !strings.Contains(plain, label) {
				t.Fatalf("missing %s: %s", label, plain)
			}
		}
		if strings.ContainsAny(plain, "0123456789") || lipgloss.Width(header) > width {
			t.Fatalf("invalid compact header: %s", plain)
		}
		if !strings.Contains(header, activeTab.Render("뉴스")) {
			t.Fatal("active tab highlight lost")
		}
		next, _ := m.handleKey("6")
		if next.(Model).screen != performanceScreen {
			t.Fatal("numeric navigation changed")
		}
	}
	wide := Model{width: 110, screen: portfolioScreen}
	if !strings.Contains(ansi.Strip(wide.header()), "2 내 주식") {
		t.Fatal("wide header changed")
	}
}
