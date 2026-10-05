package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/shopspring/decimal"
)

type App struct {
	RefreshInterval time.Duration `toml:"-"`
	RefreshRaw      string        `toml:"refresh_interval"`
	Theme           string        `toml:"theme"`
}

type Broker struct {
	Enabled bool   `toml:"enabled"`
	Mode    string `toml:"mode"`
	BaseURL string `toml:"base_url"`
	AuthURL string `toml:"auth_url"`
}

type Scanner struct {
	Market            string        `toml:"market"`
	ExcludeETF        bool          `toml:"exclude_etf"`
	MaxCandidates     int           `toml:"max_candidates"`
	RefreshRaw        string        `toml:"refresh_interval"`
	RefreshInterval   time.Duration `toml:"-"`
	MinChangeRate     string        `toml:"min_change_rate"`
	MinFiveMinuteRate string        `toml:"min_five_minute_rate"`
	MinVolumeRatio    string        `toml:"min_volume_ratio"`
	MinTurnoverKRW    int64         `toml:"min_turnover_krw"`
}

type Dividends struct {
	Provider string `toml:"provider"`
	BaseURL  string `toml:"base_url"`
}

type News struct {
	NaverBaseURL string `toml:"naver_base_url"`
	DARTBaseURL  string `toml:"dart_base_url"`
}

const NaverNewsURL = "https://naverapihub.apigw.ntruss.com/search/v1/news"

type Config struct {
	App       App       `toml:"app"`
	Kiwoom    Broker    `toml:"kiwoom"`
	NH        Broker    `toml:"nh"`
	Dividends Dividends `toml:"dividends"`
	Scanner   Scanner   `toml:"scanner"`
	News      News      `toml:"news"`
	Path      string    `toml:"-"`
	DataDir   string    `toml:"-"`
}

func Default() Config {
	return Config{
		App:       App{RefreshInterval: 5 * time.Second, RefreshRaw: "5s", Theme: "auto"},
		Kiwoom:    Broker{Mode: "mock", BaseURL: "https://mockapi.kiwoom.com"},
		NH:        Broker{Mode: "mock", BaseURL: "https://moapi.nhplug.com:8443", AuthURL: "https://api.nhplug.com:8443"},
		Dividends: Dividends{Provider: "alphavantage", BaseURL: "https://www.alphavantage.co/query"},
		Scanner:   Scanner{Market: "all", ExcludeETF: true, MaxCandidates: 20, RefreshRaw: "60s", RefreshInterval: time.Minute, MinChangeRate: "5", MinFiveMinuteRate: "2", MinVolumeRatio: "2", MinTurnoverKRW: 3_000_000_000},
		News:      News{NaverBaseURL: NaverNewsURL, DARTBaseURL: "https://opendart.fss.or.kr/api"},
	}
}

func DefaultPaths() (configPath, dataDir string, err error) {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return "", "", fmt.Errorf("config directory: %w", err)
	}
	dataRoot, err := os.UserCacheDir()
	if err != nil {
		return "", "", fmt.Errorf("cache directory: %w", err)
	}
	return filepath.Join(configRoot, "minstock", "config.toml"), filepath.Join(dataRoot, "minstock"), nil
}

func Load(path string) (Config, error) {
	cfg := Default()
	defaultPath, dataDir, err := DefaultPaths()
	if err != nil {
		return cfg, err
	}
	if path == "" {
		path = defaultPath
	}
	cfg.Path, cfg.DataDir = path, dataDir

	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return cfg, fmt.Errorf("read config %s: %w", path, err)
	}
	if err == nil {
		if err := toml.Unmarshal(b, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config %s: %w", path, err)
		}
	}
	if cfg.App.RefreshRaw != "" {
		cfg.App.RefreshInterval, err = time.ParseDuration(cfg.App.RefreshRaw)
		if err != nil {
			return cfg, fmt.Errorf("app.refresh_interval: %w", err)
		}
	}
	if cfg.App.RefreshInterval < time.Second {
		return cfg, fmt.Errorf("app.refresh_interval must be at least 1s")
	}
	applyEnv(&cfg)
	// Upgrade the old generated/default URL in memory. API HUB credentials are
	// required; legacy developer-center keys cannot authenticate against API HUB.
	if cfg.News.NaverBaseURL == "https://openapi.naver.com/v1/search/news.json" {
		cfg.News.NaverBaseURL = NaverNewsURL
	}
	if err := cfg.Scanner.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (s *Scanner) Validate() error {
	if s.Market != "all" && s.Market != "kospi" && s.Market != "kosdaq" {
		return fmt.Errorf("scanner.market must be all, kospi or kosdaq")
	}
	if s.MaxCandidates < 1 || s.MaxCandidates > 50 {
		return fmt.Errorf("scanner.max_candidates must be between 1 and 50")
	}
	interval, err := time.ParseDuration(s.RefreshRaw)
	if err != nil || interval < time.Minute {
		return fmt.Errorf("scanner.refresh_interval must be at least 60s")
	}
	s.RefreshInterval = interval
	for name, raw := range map[string]string{"min_change_rate": s.MinChangeRate, "min_five_minute_rate": s.MinFiveMinuteRate, "min_volume_ratio": s.MinVolumeRatio} {
		value, err := decimal.NewFromString(raw)
		if err != nil || !value.IsPositive() {
			return fmt.Errorf("scanner.%s must be a positive number", name)
		}
	}
	if s.MinTurnoverKRW <= 0 {
		return fmt.Errorf("scanner.min_turnover_krw must be positive")
	}
	return nil
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("MINSTOCK_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("KIWOOM_MODE"); v != "" {
		cfg.Kiwoom.Mode = v
	}
	if v := os.Getenv("KIWOOM_BASE_URL"); v != "" {
		cfg.Kiwoom.BaseURL = v
	}
	if v := os.Getenv("NHPLUG_BASE_URL"); v != "" {
		cfg.NH.BaseURL = v
	}
	if v := os.Getenv("NHPLUG_AUTH_URL"); v != "" {
		cfg.NH.AuthURL = v
	}
	if v := os.Getenv("ALPHAVANTAGE_BASE_URL"); v != "" {
		cfg.Dividends.BaseURL = v
	}
}

func (c Config) DBPath() string { return filepath.Join(c.DataDir, "minstock.db") }
