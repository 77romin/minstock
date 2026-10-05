package informationhttp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

func Endpoint(raw, host string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("invalid information API endpoint")
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"
	if (!local && (u.Hostname() != host || u.Scheme != "https")) || (local && u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("untrusted information API endpoint")
	}
	return u.String(), nil
}
func Client() *http.Client {
	return &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("API redirect refused") }}
}

func Get(ctx context.Context, client *http.Client, target string, headers map[string]string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid API request")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("API 네트워크 요청 실패")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API HTTP %d", resp.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("API 응답 읽기 실패")
	}
	if int64(len(payload)) > limit {
		return nil, fmt.Errorf("API 응답 크기 초과")
	}
	return payload, nil
}
