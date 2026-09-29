package domain

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type BrokerID string

const (
	BrokerMock   BrokerID = "MOCK"
	BrokerKiwoom BrokerID = "KIWOOM"
	BrokerNH     BrokerID = "NH"
)

type Market string

const (
	MarketKRX    Market = "KRX"
	MarketKOSPI  Market = "KOSPI"
	MarketKOSDAQ Market = "KOSDAQ"
	MarketNXT    Market = "NXT"
	MarketETF    Market = "ETF"
	MarketKONEX  Market = "KONEX"
	MarketKOTC   Market = "K-OTC"
	MarketUS     Market = "US"
)

type Currency string

const (
	KRW Currency = "KRW"
	USD Currency = "USD"
)

type Freshness string

const (
	FreshLive    Freshness = "LIVE"
	FreshDelayed Freshness = "DELAYED"
	FreshCached  Freshness = "CACHED"
	FreshMixed   Freshness = "MIXED"
)

type Symbol struct {
	Code string
	// Ticker is the exchange-facing short symbol (for example AAPL). For
	// domestic instruments it commonly falls back to Code.
	Ticker   string
	Name     string
	Market   Market
	Currency Currency
	// Exchange is the broker exchange code when a market requires routing
	// (for example ND/NASDAQ, NY/NYSE, NA/AMEX).
	Exchange string
}

func (s Symbol) Key() string { return string(s.Market) + ":" + s.Code }

type Account struct {
	ID       string
	Name     string
	Broker   BrokerID
	Currency Currency
	Type     string
}

type Balance struct {
	AccountID        string
	Broker           BrokerID
	Currency         Currency
	Cash             decimal.Decimal
	PurchaseTotal    decimal.Decimal
	ValueTotal       decimal.Decimal
	ProfitLoss       decimal.Decimal
	ProfitRate       decimal.Decimal
	CashKRW          decimal.Decimal
	PurchaseTotalKRW decimal.Decimal
	ValueTotalKRW    decimal.Decimal
	ProfitLossKRW    decimal.Decimal
	ExchangeRate     decimal.Decimal
	AsOf             time.Time
	Freshness        Freshness
}

// PortfolioSnapshot is one broker/account/currency ledger captured for a
// calendar day. Native values are retained beside KRW-normalized values so
// future performance views can separate market returns from FX effects.
type PortfolioSnapshot struct {
	Date             time.Time
	CapturedAt       time.Time
	AccountID        string
	Broker           BrokerID
	Currency         Currency
	Cash             decimal.Decimal
	PurchaseTotal    decimal.Decimal
	ValueTotal       decimal.Decimal
	ProfitLoss       decimal.Decimal
	CashKRW          decimal.Decimal
	PurchaseTotalKRW decimal.Decimal
	ValueTotalKRW    decimal.Decimal
	ProfitLossKRW    decimal.Decimal
	ExchangeRate     decimal.Decimal
}

// DividendEvent is one cash distribution per share. Future events are limited
// to distributions already declared by the issuer; projected calendar values
// are derived separately from trailing payments.
type DividendEvent struct {
	Symbol          string
	ExDate          time.Time
	DeclarationDate time.Time
	RecordDate      time.Time
	PaymentDate     time.Time
	Amount          decimal.Decimal
	Currency        Currency
	Provider        string
	Freshness       Freshness
}

type AllocationTarget struct {
	Scope         string
	Symbol        Symbol
	Cash          bool
	TargetPercent decimal.Decimal
	UpdatedAt     time.Time
}

type AlertKind string

const (
	AlertTargetPrice      AlertKind = "TARGET_PRICE"
	AlertDailyChange      AlertKind = "DAILY_CHANGE"
	AlertHoldingLoss      AlertKind = "HOLDING_LOSS"
	AlertAssetWeight      AlertKind = "ASSET_WEIGHT"
	AlertConnectionFailed AlertKind = "CONNECTION_FAILED"
)

type PriceAlertRule struct {
	ID          int64
	Symbol      Symbol
	TargetPrice decimal.Decimal
	Direction   string
	Enabled     bool
	CreatedAt   time.Time
}

type AlertEvent struct {
	ID             int64
	Kind           AlertKind
	Severity       string
	Subject        string
	Message        string
	Value          decimal.Decimal
	Threshold      decimal.Decimal
	DedupeKey      string
	OccurredAt     time.Time
	AcknowledgedAt time.Time
}

type Position struct {
	AccountID     string
	Broker        BrokerID
	Symbol        Symbol
	Quantity      decimal.Decimal
	Tradable      decimal.Decimal
	AveragePrice  decimal.Decimal
	CurrentPrice  decimal.Decimal
	PurchaseValue decimal.Decimal
	MarketValue   decimal.Decimal
	ProfitLoss    decimal.Decimal
	ProfitRate    decimal.Decimal
	// The KRW fields preserve broker-calculated values when an overseas
	// balance response supplies them. They take precedence over converting
	// the corresponding USD value in the UI.
	PurchaseValueKRW decimal.Decimal
	MarketValueKRW   decimal.Decimal
	ProfitLossKRW    decimal.Decimal
	// ExchangeRate is the broker-provided USD/KRW rate used for this
	// position's account valuation. It is zero for KRW positions or when the
	// broker did not include an applicable rate in its balance response.
	ExchangeRate decimal.Decimal
	AsOf         time.Time
	Freshness    Freshness
}

type Quote struct {
	Symbol      Symbol
	Price       decimal.Decimal
	Open        decimal.Decimal
	High        decimal.Decimal
	Low         decimal.Decimal
	Previous    decimal.Decimal
	Change      decimal.Decimal
	ChangeRate  decimal.Decimal
	Volume      int64
	Turnover    decimal.Decimal
	MarketTime  time.Time
	ReceivedAt  time.Time
	Provider    BrokerID
	Freshness   Freshness
	TradePower  decimal.Decimal
	MarketCap   decimal.Decimal
	EPS         decimal.Decimal
	PER         decimal.Decimal
	SourceLabel string
}

type CandleInterval string

const (
	IntervalTick  CandleInterval = "tick"
	Interval1Min  CandleInterval = "1m"
	Interval5Min  CandleInterval = "5m"
	Interval15Min CandleInterval = "15m"
	Interval60Min CandleInterval = "60m"
	IntervalDay   CandleInterval = "day"
	IntervalWeek  CandleInterval = "week"
	IntervalMonth CandleInterval = "month"
	IntervalYear  CandleInterval = "year"
)

func ParseInterval(v string) (CandleInterval, error) {
	switch CandleInterval(strings.ToLower(v)) {
	case IntervalTick, Interval1Min, Interval5Min, Interval15Min, Interval60Min,
		IntervalDay, IntervalWeek, IntervalMonth, IntervalYear:
		return CandleInterval(strings.ToLower(v)), nil
	default:
		return "", fmt.Errorf("unsupported candle interval %q", v)
	}
}

func (i CandleInterval) KoreanName() string {
	switch i {
	case IntervalTick:
		return "틱봉"
	case Interval1Min:
		return "1분봉"
	case Interval5Min:
		return "5분봉"
	case Interval15Min:
		return "15분봉"
	case Interval60Min:
		return "60분봉"
	case IntervalDay:
		return "일봉"
	case IntervalWeek:
		return "주봉"
	case IntervalMonth:
		return "월봉"
	case IntervalYear:
		return "년봉"
	default:
		return string(i)
	}
}

type Candle struct {
	Symbol    Symbol
	Interval  CandleInterval
	OpenTime  time.Time
	CloseTime time.Time
	Open      decimal.Decimal
	High      decimal.Decimal
	Low       decimal.Decimal
	Close     decimal.Decimal
	Volume    int64
	Turnover  decimal.Decimal
	Adjusted  bool
	Complete  bool
	Provider  BrokerID
}

type CandleQuery struct {
	Symbol   Symbol
	Interval CandleInterval
	From     time.Time
	To       time.Time
	Limit    int
	Adjusted bool
}

type WatchlistGroup struct {
	ID         string
	Name       string
	Provider   BrokerID
	ExternalID string
}

type WatchlistItem struct {
	GroupID  string
	Symbol   Symbol
	Provider BrokerID
}

type FXRate struct {
	Base       Currency
	Quote      Currency
	Rate       decimal.Decimal
	Change     decimal.Decimal
	ChangeRate decimal.Decimal
	AsOf       time.Time
	Provider   BrokerID
	Freshness  Freshness
}

type BrokerStatus struct {
	Broker    BrokerID
	Connected bool
	Mode      string
	Message   string
	CheckedAt time.Time
}

type SurgeReport struct {
	Symbol         Symbol
	Score          int
	ChangeRate     decimal.Decimal
	FiveMinuteRate decimal.Decimal
	VolumeRatio    decimal.Decimal
	Turnover       decimal.Decimal
	HighDistance   decimal.Decimal
	TradePower     decimal.Decimal
	Reasons        []string
	Warnings       []string
	AsOf           time.Time
	Provider       BrokerID
}
