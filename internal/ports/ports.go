package ports

import (
	"context"
	"time"

	"github.com/77romin/minstock-tui/internal/domain"
)

type PortfolioReader interface {
	Accounts(context.Context) ([]domain.Account, error)
	Balance(context.Context, string) (domain.Balance, error)
	Positions(context.Context, string) ([]domain.Position, error)
}

// BalanceListReader is implemented by brokers that expose multiple currency
// ledgers for one account number. Service falls back to PortfolioReader.Balance
// for providers that do not implement it.
type BalanceListReader interface {
	Balances(context.Context, string) ([]domain.Balance, error)
}

type MarketDataProvider interface {
	Quote(context.Context, domain.Symbol) (domain.Quote, error)
	Candles(context.Context, domain.CandleQuery) ([]domain.Candle, error)
}

type InstrumentProvider interface {
	Instruments(context.Context) ([]domain.Symbol, error)
}

type WatchlistReader interface {
	WatchlistGroups(context.Context) ([]domain.WatchlistGroup, error)
	WatchlistItems(context.Context, string) ([]domain.WatchlistItem, error)
}

type FXProvider interface {
	USDKRW(context.Context) (domain.FXRate, error)
}

type Provider interface {
	PortfolioReader
	MarketDataProvider
	ID() domain.BrokerID
	Status(context.Context) domain.BrokerStatus
}

type DividendProvider interface {
	Dividends(context.Context, string) ([]domain.DividendEvent, error)
	DividendSource() string
}

type Repository interface {
	Migrate(context.Context) error
	UpsertInstruments(context.Context, []domain.Symbol, domain.BrokerID) error
	SearchInstruments(context.Context, string, int) ([]domain.Symbol, error)
	SaveWatchlistGroup(context.Context, domain.WatchlistGroup) error
	ReplaceWatchlistItems(context.Context, domain.WatchlistGroup, []domain.WatchlistItem) error
	ListWatchlist(context.Context) ([]domain.WatchlistItem, error)
	AddLocalWatchlistItem(context.Context, domain.Symbol) error
	RemoveLocalWatchlistItem(context.Context, domain.Symbol) error
	SaveCache(context.Context, string, []byte) error
	LoadCache(context.Context, string) ([]byte, time.Time, error)
	SavePortfolioSnapshots(context.Context, []domain.PortfolioSnapshot) error
	ListPortfolioSnapshots(context.Context, time.Time, time.Time) ([]domain.PortfolioSnapshot, error)
	ListAllocationTargets(context.Context, string) ([]domain.AllocationTarget, error)
	ReplaceAllocationTargets(context.Context, string, []domain.AllocationTarget) error
	SaveCandles(context.Context, []domain.Candle) error
	LoadCandles(context.Context, domain.CandleQuery) ([]domain.Candle, error)
	Close() error
}
