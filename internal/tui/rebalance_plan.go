package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/77romin/minstock-tui/internal/app"
	"github.com/shopspring/decimal"
)

func (m Model) handleRebalanceKey(key string) (tea.Model, tea.Cmd, bool) {
	if m.allocationEditing || m.allocationAdding {
		return m, nil, false
	}
	if m.rebalanceEditing {
		switch key {
		case "esc":
			m.rebalanceEditing = false
		case "enter":
			value, err := decimal.NewFromString(strings.ReplaceAll(m.rebalanceInput, ",", ""))
			if err != nil || value.IsNegative() || !value.Equal(value.Truncate(0)) {
				m.err = fmt.Errorf("0 이상의 원 단위 정수를 입력하세요")
				return m, nil, true
			}
			m.rebalanceBudget, m.rebalanceEditing, m.rebalanceView = value, false, true
			m.err = nil
		case "backspace":
			_, size := utf8.DecodeLastRuneInString(m.rebalanceInput)
			if size > 0 {
				m.rebalanceInput = m.rebalanceInput[:len(m.rebalanceInput)-size]
			}
		default:
			if len(key) == 1 && strings.Contains("0123456789,", key) && len(m.rebalanceInput) < 20 {
				m.rebalanceInput += key
			}
		}
		return m, nil, true
	}
	switch key {
	case "b":
		m.rebalanceEditing, m.rebalanceInput = true, ""
	case "v":
		m.rebalanceView = !m.rebalanceView
	case "esc":
		if !m.rebalanceView {
			return m, nil, false
		}
		m.rebalanceView = false
	default:
		return m, nil, false
	}
	return m, nil, true
}

func (m Model) rebalancePlanView() string {
	lines := []string{"리밸런싱 계산 · " + m.allocationScopeLabel() + " · 주문 없음", "b 추가 투자금 / v 목표 비중 화면 / Tab 범위 전환"}
	if m.allocationDirty {
		lines = append(lines, "저장하지 않은 목표 비중으로 계산 중")
	}
	if m.rebalanceEditing {
		lines = append(lines, "", "추가 투자금 (원) · Enter 계산 / Esc 취소", m.rebalanceInput+"_")
		return panel.Width(max(60, m.width-4)).Render(strings.Join(lines, "\n"))
	}
	plan, err := app.BuildRebalancePlan(m.allocation, m.rebalanceBudget)
	if err != nil {
		lines = append(lines, "", err.Error())
	} else {
		lines = append(lines, fmt.Sprintf("현재 %s원 + 추가 %s원 = %s원", money(m.allocation.TotalKRW), money(m.rebalanceBudget), money(plan.TotalAfterKRW)), "", "  종목            목표금액       부족(+)/초과(-)     추가금 배분")
		visible := max(1, m.height-len(lines)-9)
		start := max(0, m.cursor-visible+1)
		for i := start; i < min(len(plan.Rows), start+visible); i++ {
			row := plan.Rows[i]
			name := row.Allocation.Target.Symbol.Ticker
			if name == "" {
				name = row.Allocation.Target.Symbol.Code
			}
			if row.Allocation.Target.Cash {
				name = "현금 유지"
			}
			line := fmt.Sprintf("%s %s %s %s %s", cursor(i, m.cursor), fitCell(name, 12, false), fitCell(money(row.TargetValueKRW), 14, true), fitCell(signedMoney(row.AdjustmentKRW), 16, true), fitCell(money(row.ContributionKRW), 14, true))
			lines = append(lines, selectLine(trimDisplay(line, max(20, m.width-10)), i == m.cursor))
		}
		lines = append(lines, "", "추가금은 부족 금액에 비례 배분 · 매도 없이 목표 달성은 보장되지 않습니다.", "원화 추정 · 수수료/세금/환전/주식 수량 미반영 · 캐시/부분 조회 주의")
	}
	for i := range lines {
		lines[i] = trimDisplay(lines[i], max(20, m.width-10))
	}
	return panel.Width(max(60, m.width-4)).Render(strings.Join(lines, "\n"))
}
