package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/ports"
)

type informationCacheStub struct {
	ports.Repository
	mu      sync.Mutex
	entries map[string]struct {
		data []byte
		at   time.Time
	}
}

func (r *informationCacheStub) SaveCache(_ context.Context, key string, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[key] = struct {
		data []byte
		at   time.Time
	}{append([]byte(nil), data...), time.Now()}
	return nil
}
func (r *informationCacheStub) LoadCache(_ context.Context, key string) ([]byte, time.Time, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[key]
	if !ok {
		return nil, time.Time{}, errors.New("missing")
	}
	return append([]byte(nil), entry.data...), entry.at, nil
}

type informationStub struct {
	source  string
	calls   atomic.Int32
	err     error
	items   []domain.InformationItem
	started chan struct{}
	release chan struct{}
}

func (p *informationStub) InformationSource() string              { return p.source }
func (p *informationStub) SupportsInformation(domain.Symbol) bool { return true }
func (p *informationStub) Information(ctx context.Context, _ domain.Symbol) ([]domain.InformationItem, error) {
	p.calls.Add(1)
	if p.started != nil {
		close(p.started)
		select {
		case <-p.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return p.items, p.err
}
func informationTestService(providers ...ports.InformationProvider) (*Service, *informationCacheStub) {
	repo := &informationCacheStub{entries: make(map[string]struct {
		data []byte
		at   time.Time
	})}
	s := New(repo, nil, nil, nil, nil)
	s.SetInformationProviders(providers)
	return s, repo
}
func TestInformationCacheFallbackPartialFailureAndEmpty(t *testing.T) {
	symbol := domain.Symbol{Code: "005930", Name: "삼성전자", Currency: domain.KRW}
	news := &informationStub{source: "NAVER", items: []domain.InformationItem{{Kind: domain.InformationNews, Title: "삼성전자 뉴스", URL: "https://example.com/news", PublishedAt: time.Now()}}}
	dart := &informationStub{source: "DART", err: errors.New("offline")}
	s, repo := informationTestService(news, dart)
	first := s.Information(t.Context(), symbol)
	if len(first.Items) != 1 || first.Sources[1].Warning == "" {
		t.Fatalf("partial failure: %#v", first)
	}
	restarted := New(repo, nil, nil, nil, nil)
	restarted.SetInformationProviders([]ports.InformationProvider{news, dart})
	cached := restarted.Information(t.Context(), symbol)
	if news.calls.Load() != 1 || dart.calls.Load() != 1 || cached.Sources[0].Freshness != domain.FreshCached {
		t.Fatal("persistent cache or backoff ignored")
	}
	repo.mu.Lock()
	for key, entry := range repo.entries {
		entry.at = time.Now().Add(-time.Hour)
		repo.entries[key] = entry
	}
	repo.mu.Unlock()
	news.err = errors.New("news offline")
	failed := s.Information(t.Context(), symbol)
	if len(failed.Items) != 1 || failed.Sources[0].Warning == "" || failed.Sources[0].Freshness != domain.FreshCached {
		t.Fatalf("stale fallback: %#v", failed)
	}
	// Success with no articles must replace stale articles, not mimic a failure.
	repo.mu.Lock()
	for key := range repo.entries {
		if key == "failure:information:v1:NAVER:"+symbol.Key()+":"+symbol.Name {
			delete(repo.entries, key)
		}
	}
	repo.mu.Unlock()
	news.err = nil
	news.items = nil
	empty := s.Information(t.Context(), symbol)
	if len(empty.Items) != 0 || empty.Sources[0].Warning != "" {
		t.Fatalf("empty success: %#v", empty)
	}
}
func TestInformationSingleflightCancellationAndQuota(t *testing.T) {
	provider := &informationStub{source: "NAVER", started: make(chan struct{}), release: make(chan struct{})}
	s, _ := informationTestService(provider)
	symbol := domain.Symbol{Code: "005930", Currency: domain.KRW}
	done := make(chan InformationReport, 1)
	go func() { done <- s.Information(t.Context(), symbol) }()
	<-provider.started
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	report := s.Information(ctx, symbol)
	if report.Sources[0].Warning == "" || provider.calls.Load() != 1 {
		t.Fatal("cancelled waiter started duplicate request")
	}
	cached := s.CachedInformation(t.Context(), symbol)
	if !cached.Sources[0].AsOf.IsZero() {
		t.Fatal("cache-only read performed network request")
	}
	close(provider.release)
	<-done
	quota := &informationStub{source: "NAVER", err: errors.New("HTTP 429")}
	limited, _ := informationTestService(quota)
	limited.Information(t.Context(), symbol)
	symbol.Code = "000660"
	limited.Information(t.Context(), symbol)
	if quota.calls.Load() != 1 {
		t.Fatal("provider-wide quota backoff ignored")
	}
}
