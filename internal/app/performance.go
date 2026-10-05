package app

import (
	"time"

	"github.com/77romin/minstock-tui/internal/domain"
)

func portfolioRecordingReady(snap Snapshot, complete map[domain.BrokerID]bool) bool {
	if len(snap.Balances) == 0 || snap.Cached {
		return false
	}
	if len(complete) == 1 && complete[domain.BrokerMock] {
		return true // Demo uses an isolated DB and synthetic broker.
	}
	if !complete[domain.BrokerNH] || !complete[domain.BrokerKiwoom] {
		return false
	}
	seen := map[domain.BrokerID]bool{}
	for _, balance := range snap.Balances {
		seen[balance.Broker] = true
		if balance.Currency != domain.KRW && balance.Currency != domain.USD {
			return false
		}
		if balance.Currency == domain.USD {
			rate := balance.ExchangeRate
			if !rate.IsPositive() && snap.FX.Provider == balance.Broker {
				rate = snap.FX.Rate
			}
			if !rate.IsPositive() && ((!balance.Cash.IsZero() && balance.CashKRW.IsZero()) || (!balance.ValueTotal.IsZero() && balance.ValueTotalKRW.IsZero()) || (!balance.ProfitLoss.IsZero() && balance.ProfitLossKRW.IsZero())) {
				return false
			}
		}
	}
	return seen[domain.BrokerNH] && seen[domain.BrokerKiwoom] && !seen[domain.BrokerMock]
}

// Keep legacy records intact, but compare only days containing both real
// brokers from the same capture. This also excludes same-day partial updates
// mixed with older rows. Demo-only days remain available in demo's isolated DB.
func completePortfolioHistory(history []domain.PortfolioSnapshot) []domain.PortfolioSnapshot {
	type dayState struct {
		brokers map[domain.BrokerID]bool
		capture time.Time
		mixed   bool
	}
	days := map[string]*dayState{}
	for _, row := range history {
		day := row.Date.In(time.Local).Format("2006-01-02")
		state := days[day]
		if state == nil {
			state = &dayState{brokers: map[domain.BrokerID]bool{}, capture: row.CapturedAt}
			days[day] = state
		}
		state.brokers[row.Broker] = true
		state.mixed = state.mixed || row.CapturedAt.IsZero() || !state.capture.Equal(row.CapturedAt)
	}
	var result []domain.PortfolioSnapshot
	for _, row := range history {
		state := days[row.Date.In(time.Local).Format("2006-01-02")]
		demo := len(state.brokers) == 1 && state.brokers[domain.BrokerMock]
		if demo || (!state.mixed && len(state.brokers) == 2 && state.brokers[domain.BrokerNH] && state.brokers[domain.BrokerKiwoom]) {
			result = append(result, row)
		}
	}
	return result
}
