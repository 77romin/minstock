package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mink/stock-min-tui/internal/domain"
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
  display_name TEXT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'KRW',
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
  display_name TEXT NOT NULL DEFAULT '',
  currency TEXT NOT NULL DEFAULT 'KRW',
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
);`
	if _, err := r.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate sqlite: %w", err)
	}
	_, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(1, ?)`, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (r *Repository) UpsertInstruments(ctx context.Context, symbols []domain.Symbol, provider domain.BrokerID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO instruments(provider,market,symbol,display_name,currency,updated_at)
VALUES(?,?,?,?,?,?) ON CONFLICT(provider,market,symbol) DO UPDATE SET display_name=excluded.display_name,currency=excluded.currency,active=1,updated_at=excluded.updated_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, s := range symbols {
		if _, err := stmt.ExecContext(ctx, provider, s.Market, s.Code, s.Name, s.Currency, now); err != nil {
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
	rows, err := r.db.QueryContext(ctx, `SELECT market,symbol,display_name,currency FROM instruments
WHERE active=1 AND (symbol LIKE ? ESCAPE '\' OR display_name LIKE ? ESCAPE '\')
ORDER BY CASE WHEN symbol=? THEN 0 WHEN display_name=? THEN 1 ELSE 2 END, display_name LIMIT ?`, pattern, pattern, query, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Symbol
	for rows.Next() {
		var s domain.Symbol
		if err := rows.Scan(&s.Market, &s.Code, &s.Name, &s.Currency); err != nil {
			return nil, err
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
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO watchlist_items(provider,group_id,market,symbol,display_name,currency,updated_at) VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, item := range items {
		if _, err := stmt.ExecContext(ctx, group.Provider, group.ID, item.Symbol.Market, item.Symbol.Code, item.Symbol.Name, item.Symbol.Currency, now); err != nil {
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
	_, err := r.db.ExecContext(ctx, `INSERT OR REPLACE INTO watchlist_items(provider,group_id,market,symbol,display_name,currency,updated_at) VALUES(?,?,?,?,?,?,?)`, domain.BrokerMock, "default", symbol.Market, symbol.Code, symbol.Name, symbol.Currency, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (r *Repository) RemoveLocalWatchlistItem(ctx context.Context, symbol domain.Symbol) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM watchlist_items WHERE provider=? AND group_id='default' AND market=? AND symbol=?`, domain.BrokerMock, symbol.Market, symbol.Code)
	return err
}

func (r *Repository) ListWatchlist(ctx context.Context) ([]domain.WatchlistItem, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT MIN(w.provider),GROUP_CONCAT(DISTINCT w.group_id),i.market,i.symbol,
COALESCE(NULLIF(MAX(i.display_name),''),NULLIF(MAX(x.display_name),''),i.symbol),MAX(i.currency)
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
		if err := rows.Scan(&item.Provider, &item.GroupID, &item.Symbol.Market, &item.Symbol.Code, &item.Symbol.Name, &item.Symbol.Currency); err != nil {
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
