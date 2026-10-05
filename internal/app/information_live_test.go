package app_test

import (
	"context"
	"github.com/77romin/minstock-tui/internal/adapters/alphavantage"
	"github.com/77romin/minstock-tui/internal/adapters/dart"
	"github.com/77romin/minstock-tui/internal/adapters/naver"
	db "github.com/77romin/minstock-tui/internal/adapters/sqlite"
	"github.com/77romin/minstock-tui/internal/app"
	"github.com/77romin/minstock-tui/internal/config"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/ports"
	"github.com/77romin/minstock-tui/internal/security"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Explicit opt-in: consumes API quota, reads credentials, and never writes keys.
func TestDARTLiveSmoke(t *testing.T) {
	if os.Getenv("MINSTOCK_DART_SMOKE") != "1" {
		t.Skip("opt-in DART-only live test")
	}
	key, _, err := security.LoadAPIKey("dart", "DART_API_KEY")
	if err != nil {
		t.Fatal("DART key unavailable")
	}
	repo, err := db.Open(filepath.Join(t.TempDir(), "dart.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	provider, err := dart.New("https://opendart.fss.or.kr/api", key, repo)
	if err != nil {
		t.Fatal(err)
	}
	s := app.New(repo, nil, nil, nil, nil)
	s.SetInformationProviders([]ports.InformationProvider{provider})
	symbol := domain.Symbol{Code: "005930", Name: "삼성전자", Currency: domain.KRW}
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	report := s.Information(ctx, symbol)
	if len(report.Sources) != 1 || report.Sources[0].Warning != "" || len(report.Items) == 0 {
		t.Fatalf("DART live failed: sources=%#v items=%d", report.Sources, len(report.Items))
	}
	if _, _, err := repo.LoadCache(ctx, "dart-corporations:v1"); err != nil {
		t.Fatal("new corporation mapping not persisted")
	}
	feed := s.NewsFeed(ctx, []domain.Symbol{symbol}, 0, true)
	if len(feed.Entries) != len(report.Items) {
		t.Fatal("DART disclosures lost in unified feed")
	}
	for _, entry := range feed.Entries {
		if entry.Item.Kind != domain.InformationDisclosure {
			t.Fatal("unexpected non-disclosure item")
		}
	}
	t.Logf("DART fresh mapping downloaded; %d disclosures; %d unified feed entries", len(report.Items), len(feed.Entries))
}

// Explicit opt-in: consumes API quota, reads credentials, and never writes keys.
func TestInformationLiveSmoke(t *testing.T) {
	if os.Getenv("MINSTOCK_INFORMATION_SMOKE") != "1" {
		t.Skip("opt-in live information test")
	}
	repo, err := db.Open(filepath.Join(t.TempDir(), "information.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err = repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	var providers []ports.InformationProvider
	if creds, err := security.Load("naver"); err == nil {
		p, err := naver.New(config.NaverNewsURL, creds)
		if err != nil {
			t.Fatal(err)
		}
		providers = append(providers, p)
	} else {
		t.Log("NAVER key unavailable")
	}
	if key, _, err := security.LoadAPIKey("dart", "DART_API_KEY"); err == nil {
		p, err := dart.New("https://opendart.fss.or.kr/api", key, repo)
		if err != nil {
			t.Fatal(err)
		}
		providers = append(providers, p)
	} else {
		t.Log("DART key unavailable")
	}
	if key, _, err := security.LoadAPIKey("alphavantage", "ALPHAVANTAGE_API_KEY"); err == nil {
		p, err := alphavantage.New("https://www.alphavantage.co/query", key)
		if err != nil {
			t.Fatal(err)
		}
		providers = append(providers, p)
	} else {
		t.Log("Alpha Vantage key unavailable")
	}
	if len(providers) == 0 {
		t.Skip("no configured providers")
	}
	s := app.New(repo, nil, nil, nil, nil)
	s.SetInformationProviders(providers)
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	symbols := []domain.Symbol{{Code: "005930", Name: "삼성전자", Currency: domain.KRW}, {Code: "AAPL", Ticker: "AAPL", Currency: domain.USD}}
	for _, symbol := range symbols {
		var report app.InformationReport
		if symbol.Currency == domain.USD {
			report = s.InformationUS(ctx, symbol)
		} else {
			report = s.Information(ctx, symbol)
		}
		for _, source := range report.Sources {
			t.Logf("%s %s: items=%d warning=%s", symbol.Code, source.Source, len(report.Items), source.Warning)
			if source.Warning != "" {
				t.Errorf("%s live request did not succeed", source.Source)
			}
		}
	}
	// Validate the feed against the same actual responses without extra API calls.
	feed := s.NewsFeed(t.Context(), symbols, 0, false)
	t.Logf("combined cache-only feed: %d items / %d targets", len(feed.Entries), feed.Total)
	if len(feed.Entries) > 0 {
		id := feed.Entries[0].ID
		if _, err := s.SetNewsFeedRead(t.Context(), id, true); err != nil {
			t.Fatal(err)
		}
		restored := s.NewsFeed(t.Context(), symbols, 0, false)
		if restored.Entries[0].ID != id || restored.Entries[0].ReadAt.IsZero() {
			t.Fatal("live article read state was not restored")
		}
	}
}
