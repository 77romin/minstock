package app

import (
	"testing"

	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/shopspring/decimal"
)

func TestRebalanceBudgetConservationAndOverweight(t *testing.T) {
	r := AllocationReport{TotalKRW: decimal.NewFromInt(1000), Rows: []AllocationRow{
		{Target: domain.AllocationTarget{TargetPercent: decimal.NewFromInt(50)}, ValueKRW: decimal.NewFromInt(800)},
		{Target: domain.AllocationTarget{TargetPercent: decimal.NewFromInt(30)}, ValueKRW: decimal.NewFromInt(100)},
		{Target: domain.AllocationTarget{Cash: true, TargetPercent: decimal.NewFromInt(20)}, ValueKRW: decimal.NewFromInt(100)},
	}}
	for _, budget := range []int64{0, 1, 101, 1000} {
		p, err := BuildRebalancePlan(r, decimal.NewFromInt(budget))
		if err != nil {
			t.Fatal(err)
		}
		sum, adjustments := decimal.Zero, decimal.Zero
		for _, row := range p.Rows {
			sum = sum.Add(row.ContributionKRW)
			adjustments = adjustments.Add(row.AdjustmentKRW)
			if row.ContributionKRW.IsNegative() {
				t.Fatal("negative allocation")
			}
		}
		if !sum.Equal(decimal.NewFromInt(budget)) || !adjustments.Equal(decimal.NewFromInt(budget)) {
			t.Fatalf("budget lost %+v", p)
		}
		if budget == 101 && !p.Rows[0].ContributionKRW.IsZero() {
			t.Fatal("overweight asset got contribution")
		}
	}
	if _, err := BuildRebalancePlan(r, decimal.NewFromInt(-1)); err == nil {
		t.Fatal("negative budget accepted")
	}
	r.Rows[0].Target.TargetPercent = decimal.NewFromInt(49)
	if _, err := BuildRebalancePlan(r, decimal.Zero); err == nil {
		t.Fatal("incomplete targets accepted")
	}
	r.Rows[0].Target.TargetPercent = decimal.NewFromInt(50)
	r.Rows[0].ValueKRW = decimal.NewFromInt(900)
	if _, err := BuildRebalancePlan(r, decimal.Zero); err == nil {
		t.Fatal("inconsistent valuation accepted")
	}
}
