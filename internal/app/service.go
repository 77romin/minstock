package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/ports"
	"github.com/shopspring/decimal"
)

type Service struct {
	repo        ports.Repository
	providers   []ports.Provider
	instruments []ports.InstrumentProvider
	watchlists  []ports.WatchlistReader
	fx          []ports.FXProvider
}

type Snapshot struct {
	Balances  []domain.Balance
	Positions []domain.Position
	Watchlist []domain.WatchlistItem
	Quotes    map[string]domain.Quote
	Statuses  []domain.BrokerStatus
	FX        domain.FXRate
	Surges    []domain.SurgeReport
	Warnings  []string
	LoadedAt  time.Time
	Cached    bool `json:"-"`
}

const dashboardCacheKey = "dashboard:v1"

func New(repo ports.Repository, providers []ports.Provider, instruments []ports.InstrumentProvider, watchlists []ports.WatchlistReader, fx []ports.FXProvider) *Service {
	return &Service{repo: repo, providers: providers, instruments: instruments, watchlists: watchlists, fx: fx}
}

func (s *Service) Sync(ctx context.Context) []error {
	var errs []error
	for i, source := range s.instruments {
		symbols, err := source.Instruments(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("instruments: %w", err))
			continue
		}
		provider := domain.BrokerMock
		if i < len(s.providers) {
			provider = s.providers[i].ID()
		}
		if err := s.repo.UpsertInstruments(ctx, symbols, provider); err != nil {
			errs = append(errs, err)
		}
	}
	for i, source := range s.watchlists {
		groups, err := source.WatchlistGroups(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("watchlists: %w", err))
			continue
		}
		for _, group := range groups {
			items, err := source.WatchlistItems(ctx, group.ExternalID)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if i < len(s.providers) && group.Provider == "" {
				group.Provider = s.providers[i].ID()
			}
			for j := range items {
				if items[j].Provider == "" {
					items[j].Provider = group.Provider
				}
				// Watchlist APIs often return only a code. Reuse the freshly synced
				// instrument index to attach its canonical name and market.
				if items[j].Symbol.Name == "" {
					matches, searchErr := s.repo.SearchInstruments(ctx, items[j].Symbol.Code, 10)
					if searchErr == nil {
						for _, match := range matches {
							if match.Code == items[j].Symbol.Code {
								items[j].Symbol = match
								break
							}
						}
					}
				}
			}
			if err := s.repo.ReplaceWatchlistItems(ctx, group, items); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errs
}

func (s *Service) CachedDashboard(ctx context.Context) (Snapshot, error) {
	payload, _, err := s.repo.LoadCache(ctx, dashboardCacheKey)
	if err != nil {
		return Snapshot{}, err
	}
	var snapshot Snapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("decode dashboard cache: %w", err)
	}
	if snapshot.Quotes == nil {
		snapshot.Quotes = map[string]domain.Quote{}
	}
	snapshot.Cached = true
	return snapshot, nil
}

func (s *Service) saveDashboard(ctx context.Context, snapshot Snapshot) error {
	snapshot.Cached = false
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode dashboard cache: %w", err)
	}
	return s.repo.SaveCache(ctx, dashboardCacheKey, payload)
}

// DashboardCore loads only the data required for the portfolio screens. Quotes
// and surge analysis are deliberately deferred so the first useful frame does
// not wait for every held and watched symbol.
func (s *Service) DashboardCore(ctx context.Context) Snapshot {
	snap := Snapshot{Quotes: map[string]domain.Quote{}, LoadedAt: time.Now()}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, provider := range s.providers {
		provider := provider
		wg.Add(1)
		go func() {
			defer wg.Done()
			status := provider.Status(ctx)
			mu.Lock()
			snap.Statuses = append(snap.Statuses, status)
			mu.Unlock()
			if !status.Connected {
				return
			}
			accounts, err := provider.Accounts(ctx)
			if err != nil {
				mu.Lock()
				snap.Warnings = append(snap.Warnings, string(provider.ID())+": "+err.Error())
				mu.Unlock()
				return
			}
			for _, account := range accounts {
				var balances []domain.Balance
				if source, ok := provider.(ports.BalanceListReader); ok {
					balances, err = source.Balances(ctx, account.ID)
				} else {
					var balance domain.Balance
					balance, err = provider.Balance(ctx, account.ID)
					if err == nil {
						balances = []domain.Balance{balance}
					}
				}
				if err == nil {
					mu.Lock()
					snap.Balances = append(snap.Balances, balances...)
					mu.Unlock()
				} else {
					mu.Lock()
					snap.Warnings = append(snap.Warnings, err.Error())
					mu.Unlock()
				}
				positions, err := provider.Positions(ctx, account.ID)
				if err == nil {
					mu.Lock()
					snap.Positions = append(snap.Positions, positions...)
					mu.Unlock()
				} else {
					mu.Lock()
					snap.Warnings = append(snap.Warnings, err.Error())
					mu.Unlock()
				}
			}
		}()
	}
	if len(s.fx) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, source := range s.fx {
				rate, err := source.USDKRW(ctx)
				if err == nil {
					mu.Lock()
					snap.FX = rate
					mu.Unlock()
					return
				}
			}
		}()
	}
	wg.Wait()
	if !snap.FX.Rate.IsPositive() {
		for _, balance := range snap.Balances {
			if balance.Currency == domain.USD && balance.ExchangeRate.IsPositive() {
				snap.FX = domain.FXRate{Base: domain.USD, Quote: domain.KRW, Rate: balance.ExchangeRate, AsOf: balance.AsOf, Provider: balance.Broker, Freshness: domain.FreshLive}
				break
			}
		}
	}
	if !snap.FX.Rate.IsPositive() {
		for _, position := range snap.Positions {
			if position.Symbol.Currency == domain.USD && position.ExchangeRate.IsPositive() {
				snap.FX = domain.FXRate{Base: domain.USD, Quote: domain.KRW, Rate: position.ExchangeRate, AsOf: position.AsOf, Provider: position.Broker, Freshness: domain.FreshLive}
				break
			}
		}
	}
	if snapshots := portfolioSnapshots(snap.Balances, snap.FX, snap.LoadedAt); len(snapshots) > 0 {
		if err := s.repo.SavePortfolioSnapshots(ctx, snapshots); err != nil {
			snap.Warnings = append(snap.Warnings, err.Error())
		}
	}
	items, err := s.repo.ListWatchlist(ctx)
	if err == nil {
		snap.Watchlist = items
	} else {
		snap.Warnings = append(snap.Warnings, err.Error())
	}
	if err := s.saveDashboard(ctx, snap); err != nil {
		snap.Warnings = append(snap.Warnings, err.Error())
	}
	return snap
}

func portfolioSnapshots(balances []domain.Balance, fx domain.FXRate, capturedAt time.Time) []domain.PortfolioSnapshot {
	result := make([]domain.PortfolioSnapshot, 0, len(balances))
	for _, balance := range balances {
		rate := decimal.Zero
		if balance.Currency == domain.USD {
			rate = balance.ExchangeRate
			if !rate.IsPositive() && fx.Provider == balance.Broker {
				rate = fx.Rate
			}
		}
		cashKRW, purchaseKRW := balance.CashKRW, balance.PurchaseTotalKRW
		valueKRW, profitKRW := balance.ValueTotalKRW, balance.ProfitLossKRW
		if balance.Currency == domain.KRW {
			cashKRW, purchaseKRW = balance.Cash, balance.PurchaseTotal
			valueKRW, profitKRW = balance.ValueTotal, balance.ProfitLoss
		} else if rate.IsPositive() {
			if cashKRW.IsZero() {
				cashKRW = balance.Cash.Mul(rate)
			}
			if purchaseKRW.IsZero() {
				purchaseKRW = balance.PurchaseTotal.Mul(rate)
			}
			if valueKRW.IsZero() {
				valueKRW = balance.ValueTotal.Mul(rate)
			}
			if profitKRW.IsZero() {
				profitKRW = balance.ProfitLoss.Mul(rate)
			}
		}
		result = append(result, domain.PortfolioSnapshot{
			Date: capturedAt, CapturedAt: capturedAt, AccountID: balance.AccountID, Broker: balance.Broker, Currency: balance.Currency,
			Cash: balance.Cash, PurchaseTotal: balance.PurchaseTotal, ValueTotal: balance.ValueTotal, ProfitLoss: balance.ProfitLoss,
			CashKRW: cashKRW, PurchaseTotalKRW: purchaseKRW, ValueTotalKRW: valueKRW, ProfitLossKRW: profitKRW, ExchangeRate: rate,
		})
	}
	return result
}

// EnrichDashboard fills market quotes and derived reports after the core
// portfolio has already been rendered.
func (s *Service) EnrichDashboard(ctx context.Context, snap Snapshot) Snapshot {
	snap.Cached = false
	if snap.Quotes == nil {
		snap.Quotes = map[string]domain.Quote{}
	}
	// Make held symbols searchable without downloading an entire overseas
	// instrument master. This is especially useful for US positions.
	byBroker := map[domain.BrokerID][]domain.Symbol{}
	for _, position := range snap.Positions {
		byBroker[position.Broker] = append(byBroker[position.Broker], position.Symbol)
	}
	for broker, symbols := range byBroker {
		if err := s.repo.UpsertInstruments(ctx, symbols, broker); err != nil {
			snap.Warnings = append(snap.Warnings, err.Error())
		}
	}
	seen := map[string]domain.Symbol{}
	for _, position := range snap.Positions {
		seen[position.Symbol.Key()] = position.Symbol
	}
	for _, item := range snap.Watchlist {
		seen[item.Symbol.Key()] = item.Symbol
	}
	for _, symbol := range seen {
		q, err := s.Quote(ctx, symbol)
		if err != nil {
			continue
		}
		snap.Quotes[symbol.Key()] = q
		report, ok := domain.ScoreSurge(q, decimal.NewFromFloat(2.8), decimal.NewFromFloat(3.2), domain.DefaultSurgePolicy())
		if ok && symbol.Currency == domain.KRW {
			snap.Surges = append(snap.Surges, report)
		}
	}
	sort.Slice(snap.Surges, func(i, j int) bool { return snap.Surges[i].Score > snap.Surges[j].Score })
	snap.LoadedAt = time.Now()
	if err := s.saveDashboard(ctx, snap); err != nil {
		snap.Warnings = append(snap.Warnings, err.Error())
	}
	return snap
}

func (s *Service) Dashboard(ctx context.Context) Snapshot {
	return s.EnrichDashboard(ctx, s.DashboardCore(ctx))
}

func (s *Service) PortfolioHistory(ctx context.Context, from, to time.Time) ([]domain.PortfolioSnapshot, error) {
	return s.repo.ListPortfolioSnapshots(ctx, from, to)
}

func (s *Service) Quote(ctx context.Context, symbol domain.Symbol) (domain.Quote, error) {
	var errs []error
	for _, provider := range s.providers {
		q, err := provider.Quote(ctx, symbol)
		if err == nil {
			return q, nil
		}
		errs = append(errs, err)
	}
	return domain.Quote{}, errors.Join(errs...)
}
func (s *Service) Candles(ctx context.Context, q domain.CandleQuery) ([]domain.Candle, error) {
	cached, _ := s.repo.LoadCandles(ctx, q)
	if len(cached) >= q.Limit && q.Limit > 0 {
		return cached, nil
	}
	var errs []error
	for _, provider := range s.providers {
		candles, err := provider.Candles(ctx, q)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		_ = s.repo.SaveCandles(ctx, candles)
		return candles, nil
	}
	if len(cached) > 0 {
		return cached, nil
	}
	return nil, errors.Join(errs...)
}
func (s *Service) Search(ctx context.Context, query string) ([]domain.Symbol, error) {
	return s.repo.SearchInstruments(ctx, query, 40)
}
func (s *Service) AddWatchlist(ctx context.Context, symbol domain.Symbol) error {
	return s.repo.AddLocalWatchlistItem(ctx, symbol)
}
func (s *Service) RemoveWatchlist(ctx context.Context, symbol domain.Symbol) error {
	return s.repo.RemoveLocalWatchlistItem(ctx, symbol)
}
