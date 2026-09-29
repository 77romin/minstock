package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/shopspring/decimal"
	_ "modernc.org/sqlite"
)

type Repository struct{ db *sql.DB }

func Open(path string) (*Repository, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure sqlite: %w", err)
	}
	return &Repository{db: db}, nil
}

func (r *Repository) Close() error { return r.db.Close() }

func (r *Repository) Migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS instruments (
  provider TEXT NOT NULL,
  market TEXT NOT NULL,
  symbol TEXT NOT NULL,
  ticker TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'KRW',
  exchange_code TEXT NOT NULL DEFAULT '',
  instrument_type TEXT NOT NULL DEFAULT 'stock',
  active INTEGER NOT NULL DEFAULT 1,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(provider, market, symbol)
);
CREATE INDEX IF NOT EXISTS idx_instruments_name ON instruments(display_name);
CREATE INDEX IF NOT EXISTS idx_instruments_symbol ON instruments(symbol);
CREATE TABLE IF NOT EXISTS watchlists (
  provider TEXT NOT NULL,
  group_id TEXT NOT NULL,
  external_id TEXT,
  name TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(provider, group_id)
);
CREATE TABLE IF NOT EXISTS watchlist_items (
  provider TEXT NOT NULL,
  group_id TEXT NOT NULL,
  market TEXT NOT NULL,
  symbol TEXT NOT NULL,
  ticker TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  currency TEXT NOT NULL DEFAULT 'KRW',
  exchange_code TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL,
  PRIMARY KEY(provider, group_id, market, symbol),
  FOREIGN KEY(provider, group_id) REFERENCES watchlists(provider, group_id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS candles (
  provider TEXT NOT NULL,
  market TEXT NOT NULL,
  symbol TEXT NOT NULL,
  interval TEXT NOT NULL,
  open_time TEXT NOT NULL,
  close_time TEXT NOT NULL,
  open TEXT NOT NULL,
  high TEXT NOT NULL,
  low TEXT NOT NULL,
  close TEXT NOT NULL,
  volume INTEGER NOT NULL,
  turnover TEXT NOT NULL DEFAULT '0',
  adjusted INTEGER NOT NULL DEFAULT 0,
  complete INTEGER NOT NULL DEFAULT 1,
  PRIMARY KEY(provider, market, symbol, interval, open_time)
);
CREATE INDEX IF NOT EXISTS idx_candles_lookup ON candles(market, symbol, interval, open_time);
CREATE TABLE IF NOT EXISTS broker_sync_state (
  provider TEXT NOT NULL,
  resource TEXT NOT NULL,
  synced_at TEXT NOT NULL,
  status TEXT NOT NULL,
  message TEXT NOT NULL DEFAULT '',
  PRIMARY KEY(provider, resource)
);
CREATE TABLE IF NOT EXISTS app_cache (
  cache_key TEXT PRIMARY KEY,
  payload BLOB NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS portfolio_snapshots (
  snapshot_date TEXT NOT NULL,
  captured_at TEXT NOT NULL,
  provider TEXT NOT NULL,
  account_ref TEXT NOT NULL,
  currency TEXT NOT NULL,
  cash TEXT NOT NULL,
  purchase_total TEXT NOT NULL,
  value_total TEXT NOT NULL,
  profit_loss TEXT NOT NULL,
  cash_krw TEXT NOT NULL,
  purchase_total_krw TEXT NOT NULL,
  value_total_krw TEXT NOT NULL,
  profit_loss_krw TEXT NOT NULL,
  exchange_rate TEXT NOT NULL,
  PRIMARY KEY(snapshot_date, provider, account_ref, currency)
);
CREATE TABLE IF NOT EXISTS allocation_targets (
  scope TEXT NOT NULL,
  asset_key TEXT NOT NULL,
  market TEXT NOT NULL,
  symbol TEXT NOT NULL,
  ticker TEXT NOT NULL,
  display_name TEXT NOT NULL,
  currency TEXT NOT NULL,
  is_cash INTEGER NOT NULL DEFAULT 0,
  target_percent TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY(scope, asset_key)
);`
	if _, err := r.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	if err := r.ensureColumn(ctx, "instruments", "exchange_code", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn(ctx, "instruments", "ticker", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn(ctx, "watchlist_items", "exchange_code", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := r.ensureColumn(ctx, "watchlist_items", "ticker", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	// Older demo runs persisted MOCK/sample watchlists. Keep the user's local
	// MOCK/default group, but remove provider-owned sample groups when upgrading.
	if _, err := r.db.ExecContext(ctx, `DELETE FROM watchlists WHERE provider=? AND group_id<>'default'`, domain.BrokerMock); err != nil {
		return fmt.Errorf("remove legacy demo watchlists: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(1, ?),(2, ?),(3, ?),(4, ?),(5, ?),(6, ?),(7, ?)`, now, now, now, now, now, now, now)
	return err
}

func (r *Repository) ListAllocationTargets(ctx context.Context, scope string) ([]domain.AllocationTarget, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT market,symbol,ticker,display_name,currency,is_cash,target_percent,updated_at FROM allocation_targets WHERE scope=? ORDER BY is_cash,symbol`, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.AllocationTarget
	for rows.Next() {
		var item domain.AllocationTarget
		var market, currency, percent, updated string
		var cash int
		item.Scope = scope
		if err := rows.Scan(&market, &item.Symbol.Code, &item.Symbol.Ticker, &item.Symbol.Name, &currency, &cash, &percent, &updated); err != nil {
			return nil, err
		}
		item.Symbol.Market, item.Symbol.Currency, item.Cash = domain.Market(market), domain.Currency(currency), cash == 1
		item.TargetPercent, err = decimal.NewFromString(percent)
		if err != nil {
			return nil, err
		}
		item.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) ReplaceAllocationTargets(ctx context.Context, scope string, targets []domain.AllocationTarget) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM allocation_targets WHERE scope=?`, scope); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, item := range targets {
		key := item.Symbol.Key()
		if item.Cash {
			key = "CASH"
		} else if item.Symbol.Code == "OTHER" {
			key = "OTHER"
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO allocation_targets(scope,asset_key,market,symbol,ticker,display_name,currency,is_cash,target_percent,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, scope, key, item.Symbol.Market, item.Symbol.Code, item.Symbol.Ticker, item.Symbol.Name, item.Symbol.Currency, item.Cash, item.TargetPercent.String(), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) SaveCache(ctx context.Context, key string, payload []byte) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO app_cache(cache_key,payload,updated_at) VALUES(?,?,?)
ON CONFLICT(cache_key) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at`,
		key, payload, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (r *Repository) LoadCache(ctx context.Context, key string) ([]byte, time.Time, error) {
	var payload []byte
	var updatedAt string
	if err := r.db.QueryRowContext(ctx, `SELECT payload,updated_at FROM app_cache WHERE cache_key=?`, key).Scan(&payload, &updatedAt); err != nil {
		return nil, time.Time{}, err
	}
	stamp, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return nil, time.Time{}, err
	}
	return payload, stamp, nil
}

func accountRef(provider domain.BrokerID, accountID string) string {
	sum := sha256.Sum256([]byte(string(provider) + "\x00" + accountID))
	return fmt.Sprintf("%x", sum[:8])
}

func (r *Repository) SavePortfolioSnapshots(ctx context.Context, snapshots []domain.PortfolioSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO portfolio_snapshots(
snapshot_date,captured_at,provider,account_ref,currency,cash,purchase_total,value_total,profit_loss,
cash_krw,purchase_total_krw,value_total_krw,profit_loss_krw,exchange_rate)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(snapshot_date,provider,account_ref,currency) DO UPDATE SET
captured_at=excluded.captured_at,cash=excluded.cash,purchase_total=excluded.purchase_total,
value_total=excluded.value_total,profit_loss=excluded.profit_loss,cash_krw=excluded.cash_krw,
purchase_total_krw=excluded.purchase_total_krw,value_total_krw=excluded.value_total_krw,
profit_loss_krw=excluded.profit_loss_krw,exchange_rate=excluded.exchange_rate`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, snapshot := range snapshots {
		capturedAt := snapshot.CapturedAt
		if capturedAt.IsZero() {
			capturedAt = time.Now()
		}
		date := snapshot.Date
		if date.IsZero() {
			date = capturedAt
		}
		_, err = stmt.ExecContext(ctx,
			date.In(time.Local).Format("2006-01-02"), capturedAt.UTC().Format(time.RFC3339Nano), snapshot.Broker,
			accountRef(snapshot.Broker, snapshot.AccountID), snapshot.Currency,
			snapshot.Cash.String(), snapshot.PurchaseTotal.String(), snapshot.ValueTotal.String(), snapshot.ProfitLoss.String(),
			snapshot.CashKRW.String(), snapshot.PurchaseTotalKRW.String(), snapshot.ValueTotalKRW.String(), snapshot.ProfitLossKRW.String(), snapshot.ExchangeRate.String(),
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) ListPortfolioSnapshots(ctx context.Context, from, to time.Time) ([]domain.PortfolioSnapshot, error) {
	if from.IsZero() {
		from = time.Unix(0, 0)
	}
	if to.IsZero() {
		to = time.Now().AddDate(100, 0, 0)
	}
	rows, err := r.db.QueryContext(ctx, `SELECT snapshot_date,captured_at,provider,currency,cash,purchase_total,
value_total,profit_loss,cash_krw,purchase_total_krw,value_total_krw,profit_loss_krw,exchange_rate
FROM portfolio_snapshots WHERE snapshot_date>=? AND snapshot_date<=?
ORDER BY snapshot_date,provider,account_ref,currency`, from.In(time.Local).Format("2006-01-02"), to.In(time.Local).Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.PortfolioSnapshot
	for rows.Next() {
		var snapshot domain.PortfolioSnapshot
		var date, capturedAt string
		var cash, purchase, value, profit, cashKRW, purchaseKRW, valueKRW, profitKRW, rate string
		if err := rows.Scan(&date, &capturedAt, &snapshot.Broker, &snapshot.Currency, &cash, &purchase, &value, &profit, &cashKRW, &purchaseKRW, &valueKRW, &profitKRW, &rate); err != nil {
			return nil, err
		}
		snapshot.Date, err = time.ParseInLocation("2006-01-02", date, time.Local)
		if err != nil {
			return nil, err
		}
		snapshot.CapturedAt, err = time.Parse(time.RFC3339Nano, capturedAt)
		if err != nil {
			return nil, err
		}
		snapshot.Cash = mustDecimal(cash)
		snapshot.PurchaseTotal = mustDecimal(purchase)
		snapshot.ValueTotal = mustDecimal(value)
		snapshot.ProfitLoss = mustDecimal(profit)
		snapshot.CashKRW = mustDecimal(cashKRW)
		snapshot.PurchaseTotalKRW = mustDecimal(purchaseKRW)
		snapshot.ValueTotalKRW = mustDecimal(valueKRW)
		snapshot.ProfitLossKRW = mustDecimal(profitKRW)
		snapshot.ExchangeRate = mustDecimal(rate)
		result = append(result, snapshot)
	}
	return result, rows.Err()
}

func (r *Repository) ensureColumn(ctx context.Context, table, column, definition string) error {
	rows, err := r.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid int
		var name, kind string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == column {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = r.db.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN "+column+" "+definition)
	return err
}

func (r *Repository) UpsertInstruments(ctx context.Context, symbols []domain.Symbol, provider domain.BrokerID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO instruments(provider,market,symbol,ticker,display_name,currency,exchange_code,updated_at)
VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(provider,market,symbol) DO UPDATE SET ticker=excluded.ticker,display_name=excluded.display_name,currency=excluded.currency,exchange_code=excluded.exchange_code,active=1,updated_at=excluded.updated_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, s := range symbols {
		ticker := s.Ticker
		if ticker == "" {
			ticker = s.Code
		}
		if _, err := stmt.ExecContext(ctx, provider, s.Market, s.Code, ticker, s.Name, s.Currency, s.Exchange, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) SearchInstruments(ctx context.Context, query string, limit int) ([]domain.Symbol, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	query = strings.TrimSpace(query)
	pattern := "%" + strings.ReplaceAll(query, "%", "\\%") + "%"
	rows, err := r.db.QueryContext(ctx, `SELECT market,symbol,ticker,display_name,currency,exchange_code FROM instruments
WHERE active=1 AND (symbol LIKE ? ESCAPE '\' OR ticker LIKE ? ESCAPE '\' OR display_name LIKE ? ESCAPE '\')
ORDER BY CASE WHEN symbol=? THEN 0 WHEN ticker=? THEN 0 WHEN display_name=? THEN 1 ELSE 2 END, display_name LIMIT ?`, pattern, pattern, pattern, query, query, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Symbol
	for rows.Next() {
		var s domain.Symbol
		if err := rows.Scan(&s.Market, &s.Code, &s.Ticker, &s.Name, &s.Currency, &s.Exchange); err != nil {
			return nil, err
		}
		if s.Ticker == "" {
			s.Ticker = s.Code
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repository) SaveWatchlistGroup(ctx context.Context, group domain.WatchlistGroup) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO watchlists(provider,group_id,external_id,name,updated_at) VALUES(?,?,?,?,?)
ON CONFLICT(provider,group_id) DO UPDATE SET external_id=excluded.external_id,name=excluded.name,updated_at=excluded.updated_at`,
		group.Provider, group.ID, group.ExternalID, group.Name, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (r *Repository) ReplaceWatchlistItems(ctx context.Context, group domain.WatchlistGroup, items []domain.WatchlistItem) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO watchlists(provider,group_id,external_id,name,updated_at) VALUES(?,?,?,?,?)
ON CONFLICT(provider,group_id) DO UPDATE SET external_id=excluded.external_id,name=excluded.name,updated_at=excluded.updated_at`, group.Provider, group.ID, group.ExternalID, group.Name, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM watchlist_items WHERE provider=? AND group_id=?`, group.Provider, group.ID); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO watchlist_items(provider,group_id,market,symbol,ticker,display_name,currency,exchange_code,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, item := range items {
		ticker := item.Symbol.Ticker
		if ticker == "" {
			ticker = item.Symbol.Code
		}
		if _, err := stmt.ExecContext(ctx, group.Provider, group.ID, item.Symbol.Market, item.Symbol.Code, ticker, item.Symbol.Name, item.Symbol.Currency, item.Symbol.Exchange, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) ensureLocalGroup(ctx context.Context) error {
	return r.SaveWatchlistGroup(ctx, domain.WatchlistGroup{ID: "default", ExternalID: "default", Name: "기본", Provider: domain.BrokerMock})
}

func (r *Repository) AddLocalWatchlistItem(ctx context.Context, symbol domain.Symbol) error {
	if err := r.ensureLocalGroup(ctx); err != nil {
		return err
	}
	ticker := symbol.Ticker
	if ticker == "" {
		ticker = symbol.Code
	}
	_, err := r.db.ExecContext(ctx, `INSERT OR REPLACE INTO watchlist_items(provider,group_id,market,symbol,ticker,display_name,currency,exchange_code,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, domain.BrokerMock, "default", symbol.Market, symbol.Code, ticker, symbol.Name, symbol.Currency, symbol.Exchange, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (r *Repository) RemoveLocalWatchlistItem(ctx context.Context, symbol domain.Symbol) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM watchlist_items WHERE provider=? AND group_id='default' AND market=? AND symbol=?`, domain.BrokerMock, symbol.Market, symbol.Code)
	return err
}

func (r *Repository) ListWatchlist(ctx context.Context) ([]domain.WatchlistItem, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT MIN(w.provider),GROUP_CONCAT(DISTINCT w.group_id),i.market,i.symbol,
COALESCE(NULLIF(MAX(i.ticker),''),NULLIF(MAX(x.ticker),''),i.symbol),
COALESCE(NULLIF(MAX(i.display_name),''),NULLIF(MAX(x.display_name),''),i.symbol),MAX(i.currency),
COALESCE(NULLIF(MAX(i.exchange_code),''),MAX(x.exchange_code),'')
FROM watchlist_items i
JOIN watchlists w ON w.provider=i.provider AND w.group_id=i.group_id
LEFT JOIN instruments x ON x.symbol=i.symbol AND x.active=1
GROUP BY i.market,i.symbol ORDER BY 5,i.symbol`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.WatchlistItem
	for rows.Next() {
		var item domain.WatchlistItem
		if err := rows.Scan(&item.Provider, &item.GroupID, &item.Symbol.Market, &item.Symbol.Code, &item.Symbol.Ticker, &item.Symbol.Name, &item.Symbol.Currency, &item.Symbol.Exchange); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) SaveCandles(ctx context.Context, candles []domain.Candle) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO candles(provider,market,symbol,interval,open_time,close_time,open,high,low,close,volume,turnover,adjusted,complete)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(provider,market,symbol,interval,open_time) DO UPDATE SET close_time=excluded.close_time,open=excluded.open,high=excluded.high,low=excluded.low,close=excluded.close,volume=excluded.volume,turnover=excluded.turnover,adjusted=excluded.adjusted,complete=excluded.complete`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, c := range candles {
		if _, err := stmt.ExecContext(ctx, c.Provider, c.Symbol.Market, c.Symbol.Code, c.Interval, c.OpenTime.UTC().Format(time.RFC3339Nano), c.CloseTime.UTC().Format(time.RFC3339Nano), c.Open.String(), c.High.String(), c.Low.String(), c.Close.String(), c.Volume, c.Turnover.String(), boolInt(c.Adjusted), boolInt(c.Complete)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) LoadCandles(ctx context.Context, q domain.CandleQuery) ([]domain.Candle, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 300
	}
	rows, err := r.db.QueryContext(ctx, `SELECT provider,open_time,close_time,open,high,low,close,volume,turnover,adjusted,complete
FROM candles WHERE market=? AND symbol=? AND interval=? AND open_time>=? AND open_time<=? ORDER BY open_time DESC LIMIT ?`, q.Symbol.Market, q.Symbol.Code, q.Interval, q.From.UTC().Format(time.RFC3339Nano), q.To.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reversed []domain.Candle
	for rows.Next() {
		var c domain.Candle
		var openTime, closeTime, open, high, low, closePrice, turnover string
		var adjusted, complete int
		c.Symbol, c.Interval = q.Symbol, q.Interval
		if err := rows.Scan(&c.Provider, &openTime, &closeTime, &open, &high, &low, &closePrice, &c.Volume, &turnover, &adjusted, &complete); err != nil {
			return nil, err
		}
		c.OpenTime, _ = time.Parse(time.RFC3339Nano, openTime)
		c.CloseTime, _ = time.Parse(time.RFC3339Nano, closeTime)
		c.Open = mustDecimal(open)
		c.High = mustDecimal(high)
		c.Low = mustDecimal(low)
		c.Close = mustDecimal(closePrice)
		c.Turnover = mustDecimal(turnover)
		c.Adjusted, c.Complete = adjusted != 0, complete != 0
		reversed = append(reversed, c)
	}
	out := make([]domain.Candle, len(reversed))
	for i := range reversed {
		out[len(reversed)-1-i] = reversed[i]
	}
	return out, rows.Err()
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
