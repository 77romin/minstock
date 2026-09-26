package mock

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/shopspring/decimal"
)

type Provider struct{}

func New() *Provider                    { return &Provider{} }
func (p *Provider) ID() domain.BrokerID { return domain.BrokerMock }
func (p *Provider) Status(context.Context) domain.BrokerStatus {
	return domain.BrokerStatus{Broker: p.ID(), Connected: true, Mode: "demo", Message: "sample data", CheckedAt: time.Now()}
}

var symbols = []domain.Symbol{
	{Code: "005930", Name: "삼성전자", Market: domain.MarketKOSPI, Currency: domain.KRW},
	{Code: "000660", Name: "SK하이닉스", Market: domain.MarketKOSPI, Currency: domain.KRW},
	{Code: "035420", Name: "NAVER", Market: domain.MarketKOSPI, Currency: domain.KRW},
	{Code: "035720", Name: "카카오", Market: domain.MarketKOSPI, Currency: domain.KRW},
	{Code: "068270", Name: "셀트리온", Market: domain.MarketKOSPI, Currency: domain.KRW},
	{Code: "247540", Name: "에코프로비엠", Market: domain.MarketKOSDAQ, Currency: domain.KRW},
	{Code: "012340", Name: "한빛테크 (예시)", Market: domain.MarketKOSDAQ, Currency: domain.KRW},
	{Code: "AAPL", Name: "Apple", Market: domain.MarketUS, Currency: domain.USD, Exchange: "ND"},
	{Code: "NVDA", Name: "NVIDIA", Market: domain.MarketUS, Currency: domain.USD, Exchange: "ND"},
}

func (p *Provider) Instruments(context.Context) ([]domain.Symbol, error) {
	return append([]domain.Symbol(nil), symbols...), nil
}
func (p *Provider) Accounts(context.Context) ([]domain.Account, error) {
	return []domain.Account{{ID: "mock-nh", Name: "NH 샘플", Broker: p.ID(), Currency: domain.KRW}, {ID: "mock-kiwoom", Name: "키움 국내 샘플", Broker: p.ID(), Currency: domain.KRW}, {ID: "mock-us", Name: "키움 미국 샘플", Broker: p.ID(), Currency: domain.USD}}, nil
}
func (p *Provider) Balance(_ context.Context, id string) (domain.Balance, error) {
	if id == "mock-us" {
		purchase := decimal.NewFromFloat(4200.50)
		value := decimal.NewFromFloat(4588.20)
		profit := value.Sub(purchase)
		return domain.Balance{AccountID: id, Broker: p.ID(), Currency: domain.USD, Cash: decimal.NewFromFloat(812.45), PurchaseTotal: purchase, ValueTotal: value, ProfitLoss: profit, ProfitRate: profit.Div(purchase).Mul(decimal.NewFromInt(100)), AsOf: time.Now()}, nil
	}
	if id != "mock-nh" && id != "mock-kiwoom" {
		return domain.Balance{}, fmt.Errorf("unknown mock account %s", id)
	}
	purchase := decimal.NewFromInt(30_000_000)
	value := decimal.NewFromInt(31_240_000)
	profit := value.Sub(purchase)
	return domain.Balance{AccountID: id, Broker: p.ID(), Currency: domain.KRW, Cash: decimal.NewFromInt(4_820_000), PurchaseTotal: purchase, ValueTotal: value, ProfitLoss: profit, ProfitRate: profit.Div(purchase).Mul(decimal.NewFromInt(100)), AsOf: time.Now()}, nil
}
func (p *Provider) Positions(_ context.Context, id string) ([]domain.Position, error) {
	all := []domain.Position{{AccountID: "mock-nh", Broker: p.ID(), Symbol: symbols[0], Quantity: decimal.NewFromInt(150), Tradable: decimal.NewFromInt(150), AveragePrice: decimal.NewFromInt(76710), CurrentPrice: decimal.NewFromInt(82400), PurchaseValue: decimal.NewFromInt(11506500), MarketValue: decimal.NewFromInt(12360000), ProfitLoss: decimal.NewFromInt(853500), ProfitRate: decimal.NewFromFloat(7.42), AsOf: time.Now()}, {AccountID: "mock-kiwoom", Broker: p.ID(), Symbol: symbols[1], Quantity: decimal.NewFromInt(42), Tradable: decimal.NewFromInt(42), AveragePrice: decimal.NewFromInt(214760), CurrentPrice: decimal.NewFromInt(212250), PurchaseValue: decimal.NewFromInt(9019920), MarketValue: decimal.NewFromInt(8914500), ProfitLoss: decimal.NewFromInt(-105420), ProfitRate: decimal.NewFromFloat(-1.18), AsOf: time.Now()}}
	all = append(all, domain.Position{AccountID: "mock-us", Broker: p.ID(), Symbol: symbols[7], Quantity: decimal.NewFromInt(20), Tradable: decimal.NewFromInt(20), AveragePrice: decimal.NewFromFloat(210.025), CurrentPrice: decimal.NewFromFloat(229.41), PurchaseValue: decimal.NewFromFloat(4200.50), MarketValue: decimal.NewFromFloat(4588.20), ProfitLoss: decimal.NewFromFloat(387.70), ProfitRate: decimal.NewFromFloat(9.23), AsOf: time.Now()})
	var out []domain.Position
	for _, position := range all {
		if position.AccountID == id {
			out = append(out, position)
		}
	}
	return out, nil
}

func findSymbol(code string) domain.Symbol {
	for _, s := range symbols {
		if s.Code == code {
			return s
		}
	}
	return domain.Symbol{Code: code, Name: code, Market: domain.MarketKRX, Currency: domain.KRW}
}
func basePrice(code string) float64 {
	switch code {
	case "005930":
		return 82400
	case "000660":
		return 212250
	case "035420":
		return 224000
	case "035720":
		return 42800
	case "068270":
		return 187200
	case "247540":
		return 153600
	case "AAPL":
		return 229.41
	case "NVDA":
		return 201.47
	default:
		return 32650
	}
}
func (p *Provider) Quote(_ context.Context, s domain.Symbol) (domain.Quote, error) {
	if s.Name == "" {
		s = findSymbol(s.Code)
	}
	price := decimal.NewFromFloat(basePrice(s.Code))
	changeRate := decimal.NewFromFloat(map[bool]float64{true: 18.2, false: 7.8}[s.Code == "012340"])
	previous := price.Div(decimal.NewFromInt(100).Add(changeRate)).Mul(decimal.NewFromInt(100))
	now := time.Now()
	return domain.Quote{Symbol: s, Price: price, Previous: previous, Change: price.Sub(previous), ChangeRate: changeRate, Open: price.Mul(decimal.NewFromFloat(.94)), High: price.Mul(decimal.NewFromFloat(1.004)), Low: price.Mul(decimal.NewFromFloat(.93)), Volume: 24_821_302, Turnover: decimal.NewFromInt(1_980_000_000_000), TradePower: decimal.NewFromInt(148), MarketTime: now, ReceivedAt: now, Provider: p.ID(), Freshness: domain.FreshLive, SourceLabel: "sample"}, nil
}

func (p *Provider) Candles(_ context.Context, q domain.CandleQuery) ([]domain.Candle, error) {
	count := q.Limit
	if count <= 0 {
		count = 140
	}
	if count < 130 {
		count = 130
	}
	end := q.To
	if end.IsZero() {
		end = time.Now()
	}
	duration := intervalDuration(q.Interval)
	base := basePrice(q.Symbol.Code) * .76
	out := make([]domain.Candle, 0, count)
	for i := 0; i < count; i++ {
		x := float64(i)
		close := base + x*base*.0019 + math.Sin(x*.38)*base*.018
		open := close + math.Sin(x*1.17)*base*.004
		high := math.Max(open, close) + base*(.004+math.Abs(math.Sin(x*.71))*.004)
		low := math.Min(open, close) - base*(.004+math.Abs(math.Cos(x*.63))*.004)
		t := end.Add(-time.Duration(count-i) * duration)
		out = append(out, domain.Candle{Symbol: q.Symbol, Interval: q.Interval, OpenTime: t, CloseTime: t.Add(duration), Open: decimal.NewFromFloat(open).Round(0), High: decimal.NewFromFloat(high).Round(0), Low: decimal.NewFromFloat(low).Round(0), Close: decimal.NewFromFloat(close).Round(0), Volume: int64(800000 + math.Abs(math.Sin(x*.41))*4000000), Turnover: decimal.NewFromFloat(close * 2000000), Adjusted: true, Complete: i < count-1, Provider: p.ID()})
	}
	return out, nil
}
func intervalDuration(i domain.CandleInterval) time.Duration {
	switch i {
	case domain.IntervalTick:
		return time.Second
	case domain.Interval1Min:
		return time.Minute
	case domain.Interval5Min:
		return 5 * time.Minute
	case domain.Interval15Min:
		return 15 * time.Minute
	case domain.Interval60Min:
		return time.Hour
	case domain.IntervalWeek:
		return 7 * 24 * time.Hour
	case domain.IntervalMonth:
		return 30 * 24 * time.Hour
	case domain.IntervalYear:
		return 365 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

func (p *Provider) WatchlistGroups(context.Context) ([]domain.WatchlistGroup, error) {
	return []domain.WatchlistGroup{{ID: "sample", ExternalID: "sample", Name: "샘플 관심종목", Provider: p.ID()}}, nil
}
func (p *Provider) WatchlistItems(context.Context, string) ([]domain.WatchlistItem, error) {
	return []domain.WatchlistItem{{GroupID: "sample", Provider: p.ID(), Symbol: symbols[0]}, {GroupID: "sample", Provider: p.ID(), Symbol: symbols[1]}, {GroupID: "sample", Provider: p.ID(), Symbol: symbols[6]}}, nil
}
func (p *Provider) USDKRW(context.Context) (domain.FXRate, error) {
	return domain.FXRate{Base: domain.USD, Quote: domain.KRW, Rate: decimal.NewFromFloat(1338.40), Change: decimal.NewFromFloat(4.20), ChangeRate: decimal.NewFromFloat(.31), AsOf: time.Now(), Provider: p.ID(), Freshness: domain.FreshLive}, nil
}
