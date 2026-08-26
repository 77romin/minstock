package bootstrap

import (
	"context"
	"fmt"

	"github.com/mink/stock-min-tui/internal/adapters/kiwoom"
	"github.com/mink/stock-min-tui/internal/adapters/mock"
	"github.com/mink/stock-min-tui/internal/adapters/nh"
	db "github.com/mink/stock-min-tui/internal/adapters/sqlite"
	"github.com/mink/stock-min-tui/internal/app"
	"github.com/mink/stock-min-tui/internal/config"
	"github.com/mink/stock-min-tui/internal/ports"
	"github.com/mink/stock-min-tui/internal/security"
)

type Runtime struct {
	Service *app.Service
	Repo    *db.Repository
	Mode    string
}

func Build(ctx context.Context, cfg config.Config) (*Runtime, error) {
	repo, err := db.Open(cfg.DBPath())
	if err != nil {
		return nil, err
	}
	if err := repo.Migrate(ctx); err != nil {
		repo.Close()
		return nil, err
	}

	var providers []ports.Provider
	var instruments []ports.InstrumentProvider
	var watchlists []ports.WatchlistReader
	var fx []ports.FXProvider

	if cfg.Kiwoom.Enabled || security.Configured("kiwoom") {
		creds, loadErr := security.Load("kiwoom")
		if loadErr != nil {
			repo.Close()
			return nil, fmt.Errorf("Kiwoom enabled: %w", loadErr)
		}
		client, clientErr := kiwoom.New(cfg.Kiwoom.BaseURL, creds)
		if clientErr != nil {
			repo.Close()
			return nil, clientErr
		}
		providers = append(providers, client)
		instruments = append(instruments, client)
		watchlists = append(watchlists, client)
		fx = append(fx, client)
	}

	if cfg.NH.Enabled || security.Configured("nh") {
		creds, loadErr := security.Load("nh")
		if loadErr != nil {
			repo.Close()
			return nil, fmt.Errorf("NH enabled: %w", loadErr)
		}
		client, clientErr := nh.New(cfg.NH.BaseURL, cfg.NH.AuthURL, creds)
		if clientErr != nil {
			repo.Close()
			return nil, clientErr
		}
		providers = append(providers, client)
	}

	mode := "connected"
	if len(providers) == 0 {
		demo := mock.New()
		providers = append(providers, demo)
		instruments = append(instruments, demo)
		watchlists = append(watchlists, demo)
		fx = append(fx, demo)
		mode = "demo"
	}

	return &Runtime{
		Service: app.New(repo, providers, instruments, watchlists, fx),
		Repo:    repo,
		Mode:    mode,
	}, nil
}
