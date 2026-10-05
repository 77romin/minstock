package bootstrap

import (
	"context"
	"fmt"

	"github.com/77romin/minstock-tui/internal/adapters/alphavantage"
	"github.com/77romin/minstock-tui/internal/adapters/dart"
	"github.com/77romin/minstock-tui/internal/adapters/kiwoom"
	"github.com/77romin/minstock-tui/internal/adapters/mock"
	"github.com/77romin/minstock-tui/internal/adapters/naver"
	"github.com/77romin/minstock-tui/internal/adapters/nh"
	db "github.com/77romin/minstock-tui/internal/adapters/sqlite"
	"github.com/77romin/minstock-tui/internal/app"
	"github.com/77romin/minstock-tui/internal/config"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/ports"
	"github.com/77romin/minstock-tui/internal/security"
	"github.com/shopspring/decimal"
)

type Runtime struct {
	Service *app.Service
	Repo    *db.Repository
	Mode    string
}

func Build(ctx context.Context, cfg config.Config) (*Runtime, error) {
	if err := cfg.Scanner.Validate(); err != nil {
		return nil, err
	}
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
	var dividendProvider ports.DividendProvider
	var information []ports.InformationProvider

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
		dividendProvider = demo
		information = append(information, demo)
		mode = "demo"
	}
	if apiKey, _, keyErr := security.LoadAPIKey("alphavantage", "ALPHAVANTAGE_API_KEY"); keyErr == nil {
		client, clientErr := alphavantage.New(cfg.Dividends.BaseURL, apiKey)
		if clientErr != nil {
			repo.Close()
			return nil, clientErr
		}
		dividendProvider = client
		if mode != "demo" {
			information = append(information, client)
		}
	}
	if mode != "demo" {
		if credentials, err := security.Load("naver"); err == nil {
			client, err := naver.New(cfg.News.NaverBaseURL, credentials)
			if err != nil {
				repo.Close()
				return nil, err
			}
			information = append(information, client)
		}
		if key, _, err := security.LoadAPIKey("dart", "DART_API_KEY"); err == nil {
			client, err := dart.New(cfg.News.DARTBaseURL, key, repo)
			if err != nil {
				repo.Close()
				return nil, err
			}
			information = append(information, client)
		}
	}
	service := app.New(repo, providers, instruments, watchlists, fx)
	service.SetDividendProvider(dividendProvider)
	service.SetInformationProviders(information)
	options := app.DefaultScannerOptions()
	options.Query.Market = map[string]domain.Market{"kospi": domain.MarketKOSPI, "kosdaq": domain.MarketKOSDAQ}[cfg.Scanner.Market]
	options.Query.ExcludeETF, options.Query.Limit = cfg.Scanner.ExcludeETF, cfg.Scanner.MaxCandidates
	options.RefreshInterval = cfg.Scanner.RefreshInterval
	options.Policy = domain.SurgePolicy{MinChangeRate: decimal.RequireFromString(cfg.Scanner.MinChangeRate), MinFiveMinuteRate: decimal.RequireFromString(cfg.Scanner.MinFiveMinuteRate), MinVolumeRatio: decimal.RequireFromString(cfg.Scanner.MinVolumeRatio), MinTurnover: decimal.NewFromInt(cfg.Scanner.MinTurnoverKRW)}
	service.SetScannerOptions(options)

	return &Runtime{
		Service: service,
		Repo:    repo,
		Mode:    mode,
	}, nil
}
