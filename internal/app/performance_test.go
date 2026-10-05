package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/77romin/minstock-tui/internal/adapters/mock"
	db "github.com/77romin/minstock-tui/internal/adapters/sqlite"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/ports"
	"github.com/shopspring/decimal"
)

type performanceBroker struct {
	*mock.Provider
	broker                                domain.BrokerID
	connected                             bool
	accountsErr, balanceErr, positionsErr error
	emptyAccounts, secondAccountFails     bool
	zeroAssets                            bool
	freshness                             domain.Freshness
	currency                              domain.Currency
}

func newPerformanceBroker(id domain.BrokerID) *performanceBroker {
	return &performanceBroker{Provider: mock.New(), broker: id, connected: true, freshness: domain.FreshLive, currency: domain.KRW}
}
func (p *performanceBroker) ID() domain.BrokerID { return p.broker }
func (p *performanceBroker) Status(context.Context) domain.BrokerStatus {
	return domain.BrokerStatus{Broker: p.broker, Connected: p.connected}
}
func (p *performanceBroker) Accounts(context.Context) ([]domain.Account, error) {
	if p.accountsErr != nil {
		return nil, p.accountsErr
	}
	if p.emptyAccounts {
		return nil, nil
	}
	accounts := []domain.Account{{ID: "account", Broker: p.broker}}
	if p.secondAccountFails {
		accounts = append(accounts, domain.Account{ID: "second", Broker: p.broker})
	}
	return accounts, nil
}
func (p *performanceBroker) Balance(_ context.Context, account string) (domain.Balance, error) {
	if p.balanceErr != nil {
		return domain.Balance{}, p.balanceErr
	}
	if account == "second" {
		return domain.Balance{}, errors.New("second account unavailable")
	}
	value := decimal.NewFromInt(100)
	if p.zeroAssets {
		value = decimal.Zero
	}
	return domain.Balance{Broker: p.broker, AccountID: account, Currency: p.currency, ValueTotal: value, Freshness: p.freshness}, nil
}
func (p *performanceBroker) Positions(context.Context, string) ([]domain.Position, error) {
	return nil, p.positionsErr
}

func performanceRepository(t *testing.T) *db.Repository {
	t.Helper()
	repo, err := db.Open(filepath.Join(t.TempDir(), "performance.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	if err := repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestPerformanceRecordsOnlyCompleteBrokerReads(t *testing.T) {
	cases := []struct {
		name   string
		change func(*performanceBroker)
		want   int
	}{
		{"both healthy", func(*performanceBroker) {}, 2},
		{"healthy zero balance", func(p *performanceBroker) { p.zeroAssets = true }, 2},
		{"disconnected", func(p *performanceBroker) { p.connected = false }, 0},
		{"accounts failure", func(p *performanceBroker) { p.accountsErr = errors.New("accounts unavailable") }, 0},
		{"empty accounts", func(p *performanceBroker) { p.emptyAccounts = true }, 0},
		{"balance failure", func(p *performanceBroker) { p.balanceErr = errors.New("balance unavailable") }, 0},
		{"positions failure", func(p *performanceBroker) { p.positionsErr = errors.New("positions unavailable") }, 0},
		{"cached balance", func(p *performanceBroker) { p.freshness = domain.FreshCached }, 0},
		{"mixed balance", func(p *performanceBroker) { p.freshness = domain.FreshMixed }, 0},
		{"partial account", func(p *performanceBroker) { p.secondAccountFails = true }, 0},
		{"USD missing FX", func(p *performanceBroker) { p.currency = domain.USD }, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := performanceRepository(t)
			nh, kiwoom := newPerformanceBroker(domain.BrokerNH), newPerformanceBroker(domain.BrokerKiwoom)
			tc.change(kiwoom)
			s := New(repo, []ports.Provider{nh, kiwoom}, nil, nil, nil)
			snap := s.DashboardCore(t.Context())
			history, err := repo.ListPortfolioSnapshots(t.Context(), time.Time{}, time.Time{})
			if err != nil || len(history) != tc.want {
				t.Fatalf("records=%d want=%d err=%v", len(history), tc.want, err)
			}
			if len(snap.Balances) == 0 {
				t.Fatal("partial dashboard data was erased")
			}
			if tc.want == 0 && len(snap.Warnings) == 0 {
				t.Fatal("missing recording pause notice")
			}
		})
	}
}

func TestPerformancePartialReadCannotOverwriteCompleteDay(t *testing.T) {
	repo := performanceRepository(t)
	nh, kiwoom := newPerformanceBroker(domain.BrokerNH), newPerformanceBroker(domain.BrokerKiwoom)
	s := New(repo, []ports.Provider{nh, kiwoom}, nil, nil, nil)
	s.DashboardCore(t.Context())
	before, _ := s.PortfolioHistory(t.Context(), time.Time{}, time.Time{})
	kiwoom.connected = false
	s.DashboardCore(t.Context())
	after, _ := s.PortfolioHistory(t.Context(), time.Time{}, time.Time{})
	if len(before) != 2 || len(after) != 2 {
		t.Fatal("complete day lost after disconnect")
	}
	for i := range before {
		if !before[i].CapturedAt.Equal(after[i].CapturedAt) || !before[i].ValueTotalKRW.Equal(after[i].ValueTotalKRW) {
			t.Fatal("partial read overwrote complete record")
		}
	}
	single := New(performanceRepository(t), []ports.Provider{nh}, nil, nil, nil)
	single.DashboardCore(t.Context())
	rows, _ := single.PortfolioHistory(t.Context(), time.Time{}, time.Time{})
	if len(rows) != 0 {
		t.Fatal("single configured broker created performance record")
	}
}

func TestPerformanceHistoryExcludesLegacyPartialAndMixedDaysWithoutDeleting(t *testing.T) {
	repo := performanceRepository(t)
	day := time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local)
	var history []domain.PortfolioSnapshot
	for i := 0; i < 3; i++ {
		capture := day.AddDate(0, 0, i)
		history = append(history, domain.PortfolioSnapshot{Date: capture, CapturedAt: capture, Broker: domain.BrokerNH, Currency: domain.KRW})
		if i > 0 {
			kiwoomCapture := capture
			if i == 2 {
				kiwoomCapture = capture.Add(time.Hour)
			}
			history = append(history, domain.PortfolioSnapshot{Date: capture, CapturedAt: kiwoomCapture, Broker: domain.BrokerKiwoom, Currency: domain.KRW})
		}
	}
	if err := repo.SavePortfolioSnapshots(t.Context(), history); err != nil {
		t.Fatal(err)
	}
	s := New(repo, nil, nil, nil, nil)
	visible, err := s.PortfolioHistory(t.Context(), time.Time{}, time.Time{})
	if err != nil || len(visible) != 2 || visible[0].Date.Format("2006-01-02") != "2026-10-02" {
		t.Fatalf("visible=%#v err=%v", visible, err)
	}
	raw, _ := repo.ListPortfolioSnapshots(t.Context(), time.Time{}, time.Time{})
	if len(raw) != 5 {
		t.Fatal("legacy records were deleted")
	}
}
