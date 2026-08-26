package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
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
	MinChangeRate     string `toml:"min_change_rate"`
	MinFiveMinuteRate string `toml:"min_five_minute_rate"`
	MinVolumeRatio    string `toml:"min_volume_ratio"`
	MinTurnoverKRW    int64  `toml:"min_turnover_krw"`
}

type Config struct {
	App     App     `toml:"app"`
	Kiwoom  Broker  `toml:"kiwoom"`
	NH      Broker  `toml:"nh"`
	Scanner Scanner `toml:"scanner"`
	Path    string  `toml:"-"`
	DataDir string  `toml:"-"`
}

func Default() Config {
	return Config{
		App:     App{RefreshInterval: 5 * time.Second, RefreshRaw: "5s", Theme: "auto"},
		Kiwoom:  Broker{Mode: "mock", BaseURL: "https://mockapi.kiwoom.com"},
		NH:      Broker{Mode: "mock", BaseURL: "https://moapi.nhplug.com:8443", AuthURL: "https://api.nhplug.com:8443"},
		Scanner: Scanner{MinChangeRate: "5", MinFiveMinuteRate: "2", MinVolumeRatio: "2", MinTurnoverKRW: 3_000_000_000},
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
	return cfg, nil
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
}

func (c Config) DBPath() string { return filepath.Join(c.DataDir, "minstock.db") }
