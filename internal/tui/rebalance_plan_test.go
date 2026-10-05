package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/77romin/minstock-tui/internal/app"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/shopspring/decimal"
)

func TestRebalanceInputAndFilledPlanFits(t *testing.T) {
	for _, width := range []int{80, 120} {
		m := Model{screen: allocationScreen, width: width, height: 24}
		m.allocation = app.AllocationReport{TotalKRW: decimal.NewFromInt(1000), Rows: []app.AllocationRow{
			{Target: domain.AllocationTarget{Symbol: domain.Symbol{Code: "VOO"}, TargetPercent: decimal.NewFromInt(50)}, ValueKRW: decimal.NewFromInt(800)},
			{Target: domain.AllocationTarget{Cash: true, TargetPercent: decimal.NewFromInt(50)}, ValueKRW: decimal.NewFromInt(200)},
		}}
		next, cmd := m.handleKey("b")
		m = next.(Model)
		if cmd != nil || !m.rebalanceEditing {
			t.Fatal("budget input triggered network")
		}
		next, _ = m.Update(tea.PasteMsg{Content: "1,000"})
		m = next.(Model)
		next, cmd = m.handleKey("enter")
		m = next.(Model)
		if cmd != nil || !m.rebalanceBudget.Equal(decimal.NewFromInt(1000)) {
			t.Fatal("budget not parsed locally")
		}
		view := m.rebalancePlanView()
		if !strings.Contains(view, "현금 유지") || !strings.Contains(view, "추가금 배분") || lipgloss.Width(view) > width || lipgloss.Height(view) > 22 {
			t.Fatalf("invalid view\n%s", view)
		}
		next, _ = m.handleKey("esc")
		if next.(Model).rebalanceView {
			t.Fatal("did not return to weights")
		}
	}
}
