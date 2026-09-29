package nh

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/security"
	"github.com/shopspring/decimal"
)

func TestAccountsContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test-token", "expires_in": 3600})
		case "/n2/acctinfo":
			if r.Header.Get("authorization") != "Bearer test-token" {
				t.Fatalf("authorization missing")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"rsp_cd": "00000", "rsp_msg": "완료", "Output_0": []map[string]string{{"acct_no": "1234567890", "acct_type": "01"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(server.URL, server.URL, security.Credentials{AppKey: "key", Secret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := client.Accounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || accounts[0].ID != "1234567890" {
		t.Fatalf("unexpected accounts: %#v", accounts)
	}
}

func TestBalanceAndPositionsUseCurrentContractAndShareResponse(t *testing.T) {
	var balanceCalls atomic.Int32
	var foreignBalanceCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth2/token":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test-token", "expires_in": 3600})
		case "/krstock/inquiry/v1/balance":
			balanceCalls.Add(1)
			var body struct {
				Input map[string]any `json:"Input_0"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Input["aly_qut_cd"] != "1" {
				t.Fatalf("missing current-session quote flag: %#v", body.Input)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"rsp_cd": "00000", "rsp_msg": "완료",
				"Output_0": map[string]string{"tot_aet_amt": "123000"},
				"Output_1": []map[string]string{{"iem_cd": "005930", "iem_nm": "삼성전자", "hldg_qty": "2", "evlu_amt": "120000"}},
			})
		case "/gbstock/inquiry/v1/balance":
			foreignBalanceCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"rsp_cd": "00000", "rsp_msg": "완료",
				"Output_0": map[string]string{
					"fc_abk_amt": "500", "fc_eal_amt": "600", "fc_eal_pls_amt": "100", "fc_dca": "400",
					"abk_amt": "690000", "eal_amt_sum": "829470", "eal_pls_sum_amt": "139470", "pft_rt": "20",
				},
				"Output_1": []map[string]string{{
					"iem_cd": "AAPL", "iem_nm": "애플", "cns_bse_bnc_qty": "3",
					"fc_avg_phs_pr": "150", "fc_sec_end_pr": "200", "fc_eal_amt": "600",
					"fc_abk_amt": "500", "fc_eal_pls_amt": "150",
					"krw_abk_amt1": "690000", "krw_eal_amt": "829470", "krw_eal_pls_amt": "139470",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := New(server.URL, server.URL, security.Credentials{AppKey: "key", Secret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Balance(t.Context(), "1234567890"); err != nil {
		t.Fatal(err)
	}
	balances, err := client.Balances(t.Context(), "1234567890")
	if err != nil || len(balances) != 2 {
		t.Fatalf("combined balances: %#v %v", balances, err)
	}
	foreignBalance := balances[1]
	if foreignBalance.Currency != domain.USD || !foreignBalance.ValueTotal.Equal(decimal.NewFromInt(600)) || !foreignBalance.ValueTotalKRW.Equal(decimal.NewFromInt(829470)) || !foreignBalance.ExchangeRate.Equal(decimal.RequireFromString("1382.45")) {
		t.Fatalf("unexpected NH foreign balance: %#v", foreignBalance)
	}
	positions, err := client.Positions(t.Context(), "1234567890")
	if err != nil {
		t.Fatal(err)
	}
	if len(positions) != 2 || positions[0].Symbol.Code != "005930" || positions[1].Symbol.Code != "AAPL" {
		t.Fatalf("unexpected positions: %#v", positions)
	}
	if positions[1].Broker != domain.BrokerNH || !positions[1].ExchangeRate.Equal(decimal.RequireFromString("1382.45")) {
		t.Fatalf("NH exchange rate was not derived from broker KRW valuation: %#v", positions[1])
	}
	if !positions[1].MarketValueKRW.Equal(decimal.NewFromInt(829470)) || !positions[1].ProfitLossKRW.Equal(decimal.NewFromInt(139470)) {
		t.Fatalf("NH broker KRW values were not preserved: %#v", positions[1])
	}
	if balanceCalls.Load() != 1 {
		t.Fatalf("balance endpoint called %d times; want 1", balanceCalls.Load())
	}
	if foreignBalanceCalls.Load() != 1 {
		t.Fatalf("foreign balance endpoint called %d times; want 1", foreignBalanceCalls.Load())
	}
}
