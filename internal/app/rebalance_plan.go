package app

import (
	"errors"

	"github.com/shopspring/decimal"
)

type RebalancePlanRow struct {
	Allocation      AllocationRow
	TargetValueKRW  decimal.Decimal
	AdjustmentKRW   decimal.Decimal // signed target minus current; not an order
	ContributionKRW decimal.Decimal // budget-only, no sales
}

type RebalancePlan struct {
	TotalAfterKRW, ContributionKRW decimal.Decimal
	Rows                           []RebalancePlanRow
}

// Additional funds are divided in proportion to positive target shortfalls.
// This cannot always reach the target weights without selling overweight assets.
// Cash's contribution is money to retain, not a buy order.
func BuildRebalancePlan(report AllocationReport, additional decimal.Decimal) (RebalancePlan, error) {
	plan := RebalancePlan{TotalAfterKRW: report.TotalKRW.Add(additional), ContributionKRW: additional}
	if additional.IsNegative() || !additional.Equal(additional.Truncate(0)) {
		return plan, errors.New("추가 투자금은 0 이상의 원 단위 정수여야 합니다")
	}
	if !report.TotalKRW.IsPositive() {
		return plan, errors.New("정상 조회된 양수 자산이 필요합니다")
	}
	total, current, deficits := decimal.Zero, decimal.Zero, decimal.Zero
	lastPositive := -1
	for _, row := range report.Rows {
		if row.Target.TargetPercent.IsNegative() || row.Target.TargetPercent.GreaterThan(decimal.NewFromInt(100)) || row.ValueKRW.IsNegative() {
			return plan, errors.New("목표 비중 또는 현재 자산을 확인하세요")
		}
		total = total.Add(row.Target.TargetPercent)
		current = current.Add(row.ValueKRW)
		target := plan.TotalAfterKRW.Mul(row.Target.TargetPercent).Div(decimal.NewFromInt(100))
		difference := target.Sub(row.ValueKRW)
		if difference.IsPositive() {
			deficits = deficits.Add(difference)
			lastPositive = len(plan.Rows)
		}
		plan.Rows = append(plan.Rows, RebalancePlanRow{Allocation: row, TargetValueKRW: target, AdjustmentKRW: difference})
	}
	if !total.Equal(decimal.NewFromInt(100)) {
		return plan, errors.New("목표 비중 합계를 100%로 설정하세요")
	}
	if current.Sub(report.TotalKRW).Abs().GreaterThan(decimal.NewFromInt(1)) {
		return plan, errors.New("종목별 자산과 총자산이 일치하지 않습니다. 최신 잔고를 확인하세요")
	}
	if additional.IsPositive() && lastPositive < 0 {
		return plan, errors.New("추가금과 자산 합계를 확인하세요. 배분 가능한 부족 금액이 없습니다")
	}
	remaining := additional
	for i := range plan.Rows {
		if !plan.Rows[i].AdjustmentKRW.IsPositive() || deficits.IsZero() {
			continue
		}
		amount := additional.Mul(plan.Rows[i].AdjustmentKRW).Div(deficits).Truncate(0)
		if i == lastPositive {
			amount = remaining
		}
		plan.Rows[i].ContributionKRW = amount
		remaining = remaining.Sub(amount)
	}
	return plan, nil
}
