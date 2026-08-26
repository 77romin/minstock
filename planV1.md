# Stock Min TUI 개발 계획 V1

> 문서 상태: 승인 완료 · v0.2 조회 전용 구현 기준선<br>
> 작성일: 2026-08-26<br>
> 대상 릴리스: `v0.2.0` 한국·미국주식 조회 전용 MVP<br>
> 구현 시작: 2026-08-26

## 1. 요약

Stock Min TUI는 NH투자증권과 키움증권 계좌 및 시장 데이터를 하나의
터미널에서 조회하는 Go 기반 주식 관리 프로그램이다. 첫 릴리스에서는 실제
자금을 움직이지 않고 다음 문제에 집중한다.

- 여러 증권사에 흩어진 보유 자산을 한 화면에서 조회한다.
- 종목을 빠르게 검색하고 관심 종목과 상세 차트를 확인한다.
- 급등 종목을 정량 기준으로 탐지하고 판단 근거를 설명한다.
- 원/달러 환율과 데이터 출처 및 갱신 상태를 명확하게 보여준다.
- 향후 주문 기능을 추가하더라도 조회 코드와 주문 권한이 섞이지 않게 한다.

TUI 프레임워크는 Bubble Tea v2를 사용한다. 증권사 연동은 공통 도메인과
포트(인터페이스) 뒤에 NH·키움 어댑터를 각각 구현한다. v0.1 시세와 과거 데이터는
호출 제한이 적용된 REST 폴링으로 조회하고, 로컬 검색·캐시는 SQLite를 사용한다.
WebSocket 실시간 구독은 안정적인 조회 MVP가 검증된 뒤 v0.2 이후에 추가한다.

## 2. 목표와 범위

### 2.1 MVP 목표

1. NH 및 키움 API 인증 상태를 확인할 수 있다.
2. 두 증권사의 계좌, 잔고, 보유 종목을 개별 또는 통합해서 볼 수 있다.
3. 국내 종목을 코드 또는 이름으로 검색할 수 있다.
4. 종목의 현재가, 등락률, OHLCV 차트와 이동평균선을 볼 수 있다.
5. 급등주 후보와 후보 선정 근거를 확인할 수 있다.
6. USD/KRW 환율, 출처, 기준 시각을 볼 수 있다.
7. 연결 장애, 호출 제한, 오래된 데이터를 사용자가 즉시 식별할 수 있다.
8. macOS와 Linux에서 단일 바이너리로 실행할 수 있다.

### 2.2 MVP 제외 범위

- 매수, 매도, 정정, 취소 및 예약 주문
- 자동매매와 조건 주문
- AI가 작성하는 종목 추천 또는 매매 판단
- 뉴스, 공시, 커뮤니티 감성 분석
- 세금 및 수수료 정산
- 완전한 성과 분석, 벤치마크 비교 및 백테스트
- 모바일 또는 웹 UI
- 토스증권 연동

제외 기능은 확장 포인트만 마련하고 MVP 코드 경로에는 주문 권한을 넣지 않는다.

## 3. 핵심 설계 원칙

### 3.1 조회와 주문 권한 분리

`Broker`라는 거대한 인터페이스 하나에 모든 기능을 넣지 않는다. 조회 포트와
주문 포트를 분리하고, MVP 실행 파일에는 주문 포트 구현체를 주입하지 않는다.

```go
type PortfolioReader interface {
    Accounts(ctx context.Context) ([]Account, error)
    Balance(ctx context.Context, accountID string) (Balance, error)
    Positions(ctx context.Context, accountID string) ([]Position, error)
}

type MarketDataProvider interface {
    Quote(ctx context.Context, symbol Symbol) (Quote, error)
    Candles(ctx context.Context, query CandleQuery) ([]Candle, error)
}

type RealtimeProvider interface {
    SubscribeQuotes(ctx context.Context, symbols []Symbol) (<-chan QuoteEvent, error)
}

// 증권사가 관심종목 조회 API를 제공할 때만 구현하는 선택 기능이다.
type WatchlistReader interface {
    WatchlistGroups(ctx context.Context) ([]WatchlistGroup, error)
    WatchlistItems(ctx context.Context, groupID string) ([]WatchlistItem, error)
}

// v0.1 실행 경로에는 연결하지 않는다.
type OrderExecutor interface {
    PlaceOrder(ctx context.Context, order NewOrder) (Order, error)
    CancelOrder(ctx context.Context, orderID string) error
}
```

### 3.2 증권사 응답 격리

NH 및 키움의 요청·응답 DTO는 각 어댑터 패키지 밖으로 노출하지 않는다.
애플리케이션과 TUI는 `Position`, `Quote`, `Candle` 같은 공통 도메인 모델만
사용한다.

### 3.3 데이터의 출처와 시각 표시

시세, 환율 및 분석 결과에는 다음 메타데이터를 함께 보관한다.

- 공급자: NH, Kiwoom 등
- 시장 기준 시각
- 애플리케이션 수신 시각
- 실시간, 지연, 캐시 여부
- 마지막 정상 갱신 시각

### 3.4 설명 가능한 급등 분석

MVP의 리포트는 생성형 AI 문장이 아니라 계산 근거가 재현 가능한 규칙 기반
결과다. 동일한 입력에는 동일한 점수와 설명이 나와야 한다.

### 3.5 정확한 금융 수치

- 원화 정수 금액은 `int64`를 사용한다.
- 환율, 평균단가, 해외주식 가격 등 소수점 값은 decimal 타입을 사용한다.
- 금액 계산에는 이진 부동소수점 `float64`를 사용하지 않는다.
- 통화가 다른 자산은 원통화 값과 원화 환산값을 함께 저장한다.

## 4. 기능 명세

기능 우선순위는 `P0`가 MVP 필수, `P1`이 MVP 품질 향상, `P2`가 후속 기능이다.

### F-001 애플리케이션 시작 및 설정 진단 — P0

- 설정 파일과 환경변수를 읽는다.
- NH·키움 자격증명의 존재 여부만 표시하고 실제 값은 출력하지 않는다.
- SQLite 마이그레이션과 종목 마스터 상태를 검사한다.
- 연결 실패가 있어도 가능한 화면은 오프라인/부분 연결 상태로 실행한다.
- `--diagnose` 옵션으로 비밀값을 제외한 진단 결과를 출력한다.

완료 조건:

- 키가 없는 상태에서도 프로그램이 패닉 없이 실행된다.
- 어느 설정이 누락됐는지 TUI와 진단 명령에서 알 수 있다.
- 로그에 App Key, App Secret, 접근 토큰, 계좌번호 전체가 남지 않는다.

### F-002 증권사 인증 및 연결 상태 — P0

- NH 및 키움 OAuth 토큰을 발급하고 만료 전에 갱신한다.
- 토큰은 메모리에 유지하고 평문으로 DB에 저장하지 않는다.
- REST 인증 및 조회 상태를 증권사별로 표시한다.
- 설정 가능한 주기로 폴링하며 모든 요청은 rate limiter를 통과한다.
- 일시적인 조회 실패에도 성공한 공급자의 결과와 기존 캐시를 유지한다.

완료 조건:

- 정상, 인증 실패, 호출 제한, 네트워크 장애 상태가 구분된다.
- 종료 시 진행 중인 네트워크 요청이 정상적으로 정리된다.

### F-003 통합 대시보드 — P0

표시 항목:

- 총 평가금액
- 총 매입금액
- 평가손익 및 수익률
- 예수금
- 증권사별 평가금액
- 주요 보유 종목
- 급등주 상위 후보
- KOSPI/KOSDAQ 요약
- USD/KRW
- 공급자 연결 상태와 마지막 갱신 시각

동작:

- `1` 또는 전역 메뉴로 진입한다.
- 터미널 크기에 따라 2열과 1열 레이아웃을 전환한다.
- 데이터가 일부 실패하면 성공한 패널은 유지한다.

### F-004 내 주식 현황 — P0

- 한국주식과 미국주식을 별도 탭으로 구분한다.
- 표 열은 종목명, 평가손익, 수익률, 잔고수량, 평가금액, 매입가, 현재가,
  매입금액, 보유비중 순으로 제공한다.
- 평가손익과 수익률은 양수이면 빨간색, 음수이면 파란색으로 표시한다.
- 보유비중은 탭별 총 평가금액을 분모로 계산한다.
- 좁은 터미널에서는 열 구간 이동을 제공하고 행 선택 상태를 유지한다.

- 전체, NH, 키움 계좌 필터를 제공한다.
- 키움 미국주식 원장잔고와 외화예수금을 조회해 국내 포트폴리오와 함께 표시한다.
- 미국주식은 원통화 USD와 적용 환율 기준의 KRW 환산값을 함께 유지하며, 환산 전
  USD를 KRW 금액에 직접 더하지 않는다.
- 종목명, 시장, 통화, 수량, 평균단가, 현재가, 평가금액, 평가손익,
  수익률을 표시한다.
- 종목명, 평가금액, 평가손익, 수익률로 정렬한다.
- 원화와 외화 합산 시 적용 환율과 환산 기준 시각을 표시한다.
- 종목 선택 후 `Enter`로 상세 화면에 진입한다.

완료 조건:

- API의 합계 값과 로컬 합산 값 차이가 허용 오차를 넘으면 경고한다.
- 0보유, 거래정지, 가격 미수신 종목도 화면이 깨지지 않는다.

### F-005 종목 검색 — P0

- 국내 종목 코드 및 한글 종목명 부분 검색을 제공한다.
- KOSPI, KOSDAQ, ETF 등 시장 필터를 제공한다.
- 로컬에는 종목코드, 종목명, 시장, 상품 유형 등 검색에 필요한 최소 인덱스만
  저장한다. 전 종목 시세나 상세 기업정보를 저장하는 방식이 아니다.
- 검색어 입력과 필터링은 로컬 인덱스에서 처리해 키 입력마다 외부 API를
  호출하지 않는다.
- 사용자가 결과를 선택했을 때만 현재가와 상세 정보를 증권사 API에서 가져온다.
- 로컬 인덱스에 없는 정확한 종목코드가 입력되면 원격 조회 후 결과를 보완한다.
- 종목 인덱스는 하루 1회 또는 사용자가 요청할 때 갱신한다.
- 검색 결과에 종목명, 코드, 시장, 현재가, 등락률을 표시한다.
- 결과에서 상세 화면 진입과 관심 종목 추가가 가능하다.

MVP 이후 후보:

- 초성 검색
- 미국주식 심볼 및 영문명 검색
- 최근 검색 기록

### F-006 관심 종목 — P0

- TUI에서 관리하는 로컬 통합 관심 종목 추가 및 삭제
- 사용자 지정 그룹인 기본 Watchlist 제공
- 관심종목 조회 API를 제공하는 증권사는 그룹과 종목을 read-only로 가져온다.
- 키움 국내 관심종목 그룹 목록(`ka01300`)과 그룹 상세(`ka01301`)를 동기화한다.
- 동일 시장·종목코드는 한 행으로 합치고 `키움:성장주`, `LOCAL:기본`처럼 원본
  그룹과 출처를 함께 표시한다.
- 키움 앱에서 변경된 내용은 시작 시 또는 수동 동기화 시 다시 가져온다.
- MVP에서는 TUI의 변경을 증권사 앱으로 역동기화하지 않는다.
- 현재 공개된 NH NAMUH PLUG API 목록에서는 관심종목 조회 기능을 확인할 수
  없으므로 NH 앱 관심종목 자동 가져오기는 지원 대상에서 제외한다. NH 종목은
  TUI에서 로컬 관심종목으로 추가한다.
- 향후 NH가 해당 API를 제공하면 `WatchlistReader` 어댑터만 추가해 동일 화면에
  병합한다.
- 현재가, 등락률, 거래량, 갱신 시각 표시
- 실행 후에도 SQLite에 유지
- 실시간 구독 가능 수를 넘으면 화면에 보이는 종목과 우선순위 종목부터 구독

완료 조건:

- 키움 관심종목 그룹 및 종목을 가져와 로컬 관심종목과 한 화면에서 볼 수 있다.
- 같은 종목이 여러 그룹에 있어도 시세는 한 번만 요청하고 모든 소속 그룹을
  표시한다.
- 증권사가 관심종목 API를 제공하지 않는 상태는 오류가 아니라 `미지원`으로
  표시한다.

### F-007 종목 상세 및 차트 — P0

상세 정보:

- 현재가, 전일 대비, 등락률
- 시가, 고가, 저가, 거래량, 거래대금
- 시장, 종목코드, 데이터 공급자, 갱신 시각
- OHLC 캔들 차트
- 하단 거래량 차트
- 이동평균선 MA5, MA20, MA60, MA120 동시 표시
- 좌우 이동, 확대·축소 및 선택 캔들 OHLCV 조회

봉 단위와 조회 기간은 서로 다른 개념으로 제공한다.

```text
봉 단위: 틱 | 1분 | 5분 | 15분 | 60분(1시간) | 일 | 주 | 월 | 년
조회 기간: 1D | 5D | 1M | 3M | 1Y | 5Y
```

이동평균선 표기 규칙:

- `MA(n)`은 현재 차트의 최근 `n개 봉` 종가에 대한 단순이동평균이다.
- 따라서 일봉의 MA5·20·60·120은 `5일선`, `20일선`, `60일선`,
  `120일선`으로 표시한다.
- 주봉에서는 `5주선`, `20주선`, `60주선`, `120주선`으로 표시한다.
- 월봉에서는 `5개월선`, `20개월선`, `60개월선`, `120개월선`으로 표시한다.
- 틱·분·시간봉에서는 `MA(5)`, `MA(20)`, `MA(60)`, `MA(120)`으로 표시해
  일수로 오해하지 않게 한다.
- 이동평균은 수정주가 적용 여부를 데이터 메타데이터와 함께 표시한다.

이동평균선 시각 규칙:

- MA5: 노랑
- MA20: 자홍
- MA60: 초록
- MA120: 청록/파랑
- 밝은/어두운 터미널 테마에서 각각 대비가 확보되는 색상을 사용한다.
- 색각 이상 및 흑백 터미널을 위해 범례에 기간 숫자를 항상 표시하고 선 모양도
  구분한다.

데이터 처리:

- 공급자가 직접 지원하는 캔들을 우선 사용한다.
- 직접 지원하지 않는 5분, 15분, 60분 등은 더 작은 단위 OHLCV를 집계한다.
- 집계 규칙은 `open=첫 값`, `high=최댓값`, `low=최솟값`, `close=마지막 값`,
  `volume=합계`다.
- 진행 중인 캔들과 확정 캔들을 구분한다.
- 휴장, 장전, 장후 및 KRX/NXT 세션 경계를 고려한다.

### F-008 급등주 탐지 — P0

초기 후보 기준은 설정 파일로 조정 가능하게 한다.

```text
당일 등락률       >= +5%
최근 5분 등락률   >= +2%
거래량 비율       >= 전일 동시간 2배
누적 거래대금     >= 30억원
당일 고점 근접도  >= 98%
```

기본 점수:

```text
단기 가격 모멘텀  30%
거래량 급증       25%
거래대금          20%
고점 돌파         15%
체결 강도         10%
```

출력:

- 후보 순위와 0~100점 점수
- 당일 및 단기 등락률
- 거래량 배수와 거래대금 순위
- 고점 대비 거리
- 체결 강도
- 점수에 기여한 조건
- 변동성 확대 등 위험 문구
- 기준 시각 및 공급자

이 결과는 매수 추천이 아니라 시장 데이터 요약임을 화면에 명시한다.

### F-009 환율 — P0

- USD/KRW 값, 전일 대비, 등락률을 표시한다.
- 공급자, 시장 기준 시각, 지연 여부를 표시한다.
- 증권사 시장 환율을 우선 사용한다.
- 공식 일별 기준 데이터가 추가되면 시장 참고 환율과 별도 항목으로 구분한다.
- 외화 자산 환산에는 어떤 환율을 사용했는지 기록한다.

### F-010 캐시 및 오프라인 표시 — P1

- 종목 마스터, 관심 종목, 확정 캔들 및 마지막 정상 시세를 SQLite에 저장한다.
- 데이터 종류별 TTL을 적용한다.
- 캐시 데이터에는 `CACHED`와 기준 시각을 표시한다.
- 장중 오래된 시세를 현재 시세처럼 표시하지 않는다.

초기 TTL 기본값:

| 데이터 | 장중 TTL | 장 마감 후 |
|---|---:|---:|
| 현재가 | 3초 | 다음 거래일 전까지 |
| 계좌 잔고 | 10초 | 5분 |
| 확정 분봉 | 해당 봉 마감까지 | 영구 캐시 가능 |
| 확정 일봉 | 당일 장 마감까지 | 영구 캐시 가능 |
| 종목 마스터 | 24시간 | 24시간 |

TTL은 설정 가능하게 하고 실제 공급자 정책에 맞춰 연동 과정에서 조정한다.

### F-011 도움말과 키보드 조작 — P1

기본 키맵:

| 키 | 동작 |
|---|---|
| `1`~`4` | 주요 화면 이동 |
| `↑` / `↓`, `j` / `k` | 항목 이동 |
| `Enter` | 선택/상세 보기 |
| `Esc` | 이전 화면 |
| `/` | 종목 검색 |
| `gg` / `G` | 목록 처음/끝 |
| `Ctrl+u` / `Ctrl+d` | 반 페이지 위/아래 |
| `gt` / `gT` | 다음/이전 화면 탭 |
| `m` | 로컬 관심 종목 추가/해제 |
| `f` | 전체/한국/미국 시장 필터 |
| `c` | USD/KRW 표시 모드 전환 |
| `:r` | 현재 화면 새로고침 |
| `:s` | 전체 동기화 |
| `:d` | 연결 진단 |
| `:?` | 도움말 |
| `:q` | 종료 |
| `ctrl+c` | 안전 종료 |

입력 포커스가 검색창에 있을 때 전역 단축키와 문자가 충돌하지 않게 한다.

## 5. 증권사 연동 방침

### 5.1 키움증권

키움 REST API를 사용한다. 공식 가이드에서 주식 틱, 분, 일, 주, 월, 년봉
차트 조회를 제공하므로 차트의 우선 공급자로 사용한다.

공식 안내 기준 고려 사항:

- 국내주식 조회 TR은 계좌/토큰별 초당 호출 제한을 적용한다.
- 계좌별 세션 수와 실시간 구독 종목 수 제한을 중앙에서 관리한다.
- 모의투자 서버가 있으므로 향후 주문 개발의 첫 검증 환경으로 사용한다.
- API 오류 코드와 연속조회 필드를 어댑터에서 해석한다.
- 국내 관심종목 그룹 목록(`ka01300`) 및 그룹 상세(`ka01301`)를 가져와 로컬
  통합 관심종목에 병합한다.
- 미국주식 원장잔고(`ust21070`), 외화예수금(`ust21110`), 현재가(`usa20100`)와
  틱·분·일·주·월·년 차트(`usa06010`~`usa06015`)를 조회한다.
- 허용된 조회 API ID와 URL을 코드의 allowlist로 제한해 주문 계열 호출을 네트워크
  요청 전에 차단한다.

### 5.2 NH투자증권

NAMUH PLUG OpenAPI의 REST를 사용한다. WebSocket 시세는 후속 릴리스에서 추가한다.

- OAuth `client_credentials` 토큰을 중앙 토큰 매니저가 관리한다.
- 계좌 목록, 잔고, 보유 종목, 국내·해외 시세를 연동한다.
- 국내 시장 구분은 KRX, NXT, 통합 구분을 도메인 메타데이터로 보존한다.
- 최신 공식 SDK의 모의투자 도메인 `https://moapi.nhplug.com:8443`을 조회 검증에
  사용하고, 토큰은 공식 인증 도메인에서 발급한다.
- NH에 없는 차트 단위는 지원되는 원시 데이터로 집계하거나 키움 공급자로
  전환하되 출처를 화면에 표시한다.
- 현재 공개 API 목록에서 관심종목 조회 기능을 확인할 수 없으므로 NH 앱의
  관심종목을 자동 동기화하지 않는다. API가 추가되면 선택 포트로 확장한다.

### 5.3 공급자 선택과 장애 처리

```text
계좌/보유 정보 → 해당 계좌 증권사만 사용
현재가         → 계좌 증권사 우선, 실패 시 허용된 대체 공급자
차트           → 직접 지원 단위 우선, 필요 시 로컬 집계
환율           → 키움 미국주식 종목정보의 `base_exrt`, 실패 시 미수신 표시
```

공급자 전환이 발생해도 조용히 값을 바꾸지 않고 출처 변경을 표시한다.

## 6. TUI 화면 구조

```text
AppModel
├── GlobalHeader
│   ├── 시장 지수
│   ├── 현재 시각
│   └── 연결 상태
├── Navigation
├── DashboardScreen
├── PortfolioScreen
├── SearchScreen
├── WatchlistScreen
├── MoversScreen
├── StockDetailScreen
│   ├── QuoteSummary
│   ├── CandleToolbar
│   ├── OHLCChart
│   └── VolumeChart
├── SettingsScreen
├── HelpOverlay
└── StatusBar
```

Bubble Tea의 루트 모델은 화면 전환과 전역 메시지만 담당한다. 각 화면 모델이
자체 선택 상태, 로딩 상태, viewport와 키맵을 관리한다. HTTP나 DB 작업은
`Update` 안에서 직접 실행하지 않고 `tea.Cmd`로 실행한 뒤 결과 메시지를 받는다.

주요 메시지 예:

```go
type PortfolioLoadedMsg struct { Portfolio Portfolio }
type QuoteUpdatedMsg struct { Quote Quote }
type CandlesLoadedMsg struct { Query CandleQuery; Candles []Candle }
type BrokerStatusMsg struct { Broker BrokerID; Status ConnectionStatus }
type AppErrorMsg struct { Op string; Err error; Retryable bool }
```

## 7. 기술 스택

버전은 구현 시작일의 안정 버전을 확인한 뒤 `go.mod`에 정확히 고정한다.

| 영역 | 기술 | 선택 이유 |
|---|---|---|
| 언어 | Go 안정 버전 | 동시성, 단일 바이너리, 크로스 플랫폼 |
| TUI | `charm.land/bubbletea/v2` | 이벤트 기반 전체 화면 TUI |
| 컴포넌트 | `charm.land/bubbles/v2` | table, list, viewport, input, spinner |
| 스타일 | `charm.land/lipgloss/v2` | 반응형 레이아웃과 터미널 색상 |
| 차트 | `github.com/NimbleMarkets/ntcharts/v2` | Bubble Tea v2용 OHLC/캔들 차트 기반 |
| REST | 표준 `net/http` | 의존성 최소화와 세밀한 제어 |
| 실시간(후속) | `github.com/coder/websocket` 후보 | v0.2 이후 context 기반 구독 |
| 호출 제한 | `golang.org/x/time/rate` | 증권사/엔드포인트별 rate limit |
| 저장소 | SQLite | 경량 종목 인덱스, 캐시, 관심 종목 |
| SQLite 드라이버 | `modernc.org/sqlite` | CGO 없는 빌드 |
| Decimal | `github.com/shopspring/decimal` | 금액과 환율 계산 |
| 로그 | 표준 `log/slog` | 구조화 로그와 민감정보 필터 |
| 설정 | TOML + 환경변수 | 사람이 읽기 쉬운 설정과 배포 override |
| 비밀정보 | OS Keychain 우선 | 평문 설정 파일 노출 방지 |
| 테스트 | `testing`, `httptest` | 표준 도구 중심 테스트 |
| 린트 | `go vet`, `golangci-lint` | 정적 검사 |
| 릴리스 | GoReleaser | macOS/Linux 단일 바이너리 배포 |

`ntcharts`는 캔들 렌더링 기반으로 우선 검증한다. 터미널별 글리프 차이나
성능 문제가 크면 캔들 렌더러만 내부 포트로 감싸 자체 구현으로 교체할 수 있게
한다.

## 8. 아키텍처

### 8.1 전체 구조

```text
┌──────────────────────── Bubble Tea TUI ────────────────────────┐
│ Dashboard │ Portfolio │ Search │ Movers │ Stock Detail         │
└──────────────────────────────┬──────────────────────────────────┘
                               │ Query / Command
┌──────────────────── Application Services ──────────────────────┐
│ PortfolioService │ MarketService │ ChartService │ Scanner      │
└───────────┬──────────────────┬───────────────────┬──────────────┘
            │ ports            │                   │
     ┌──────┴──────┐    ┌──────┴──────┐     ┌──────┴──────┐
     │ NH Adapter  │    │Kiwoom Adapter│     │ SQLite Repo │
     │ REST/OAuth  │    │ REST/OAuth   │     │Cache/Search │
     └──────┬──────┘    └──────┬───────┘     └─────────────┘
            │                  │
       NAMUH PLUG         Kiwoom REST API
```

### 8.2 계층 책임

#### Domain

- 금융 및 시장 데이터 모델
- 이동평균, OHLCV 집계, 손익 계산 규칙
- 외부 라이브러리와 증권사 DTO에 의존하지 않음

#### Application

- 사용 사례 조합
- 공급자 선택
- 캐시 정책
- 부분 실패 처리
- 급등 스캐너 실행

#### Ports

- 계좌 조회
- 시장 데이터 조회
- 실시간 구독 포트(후속 릴리스)
- 저장소
- 시계 및 비밀정보 제공자

#### Adapters

- NH REST/OAuth
- 키움 REST/OAuth
- SQLite
- OS Keychain
- TUI

### 8.3 프로젝트 디렉터리

```text
stock-min-tui/
├── cmd/
│   └── minstock/
│       └── main.go
├── internal/
│   ├── app/
│   │   ├── portfolio_service.go
│   │   ├── market_service.go
│   │   ├── chart_service.go
│   │   └── scanner_service.go
│   ├── domain/
│   │   ├── account.go
│   │   ├── money.go
│   │   ├── position.go
│   │   ├── quote.go
│   │   ├── candle.go
│   │   └── surge.go
│   ├── ports/
│   │   ├── portfolio.go
│   │   ├── market_data.go
│   │   ├── realtime.go
│   │   ├── repository.go
│   │   └── secrets.go
│   ├── adapters/
│   │   ├── nh/
│   │   ├── kiwoom/
│   │   ├── sqlite/
│   │   └── keychain/
│   ├── market/
│   │   ├── aggregate.go
│   │   ├── moving_average.go
│   │   └── scanner.go
│   ├── platform/
│   │   ├── ratelimit/
│   │   ├── retry/
│   │   └── clock/
│   └── tui/
│       ├── model.go
│       ├── messages.go
│       ├── keys.go
│       ├── styles.go
│       └── screens/
├── migrations/
├── testdata/
│   ├── nh/
│   └── kiwoom/
├── configs/
│   └── stock-min.example.toml
├── scripts/
├── go.mod
├── Makefile
├── README.md
└── planV1.md
```

## 9. 주요 도메인 모델

```go
type Symbol struct {
    Code     string
    Name     string
    Market   Market
    Currency Currency
}

type Quote struct {
    Symbol       Symbol
    Price        decimal.Decimal
    Change       decimal.Decimal
    ChangeRate   decimal.Decimal
    Volume       int64
    MarketTime   time.Time
    ReceivedAt   time.Time
    Provider     BrokerID
    Freshness    Freshness
}

type Candle struct {
    Symbol      Symbol
    Interval    CandleInterval
    OpenTime    time.Time
    CloseTime   time.Time
    Open        decimal.Decimal
    High        decimal.Decimal
    Low         decimal.Decimal
    Close       decimal.Decimal
    Volume      int64
    Adjusted    bool
    Complete    bool
    Provider    BrokerID
}
```

모든 시간은 내부적으로 `time.Time`과 명시적 location을 사용한다. DB에는 UTC로
저장하고 화면에서는 시장 시간 또는 KST로 표시한다.

## 10. 로컬 저장소

초기 테이블:

```text
schema_migrations
instruments
watchlist_sources
watchlists
watchlist_items
watchlist_memberships
candles
quote_snapshots
fx_rates
broker_sync_state
app_preferences
```

### 10.1 SQLite를 사용하는 이유

SQLite를 선택하는 목적은 증권사 데이터를 전부 복제하는 것이 아니라 다음의
작은 로컬 상태를 안전하고 일관되게 관리하기 위해서다.

- 종목명 부분 검색을 위한 최소 종목 인덱스
- 로컬 및 증권사별 관심종목 그룹과 소속 관계
- 이미 받은 확정 캔들의 재사용과 API 호출 절감
- 마지막 정상 시세 및 환율의 오프라인/장애 표시
- 증권사별 마지막 동기화 시각과 애플리케이션 설정

파일 여러 개에 JSON으로 저장하는 방식보다 트랜잭션, 고유 키, upsert,
인덱스와 스키마 마이그레이션을 일관되게 사용할 수 있다. 별도 서버 프로세스가
필요하지 않고 애플리케이션 데이터 디렉터리의 단일 파일로 동작한다. 검색 결과
선택 후의 현재가·호가·기업 상세정보는 API에서 가져오므로 SQLite가 시장 전체
데이터 저장소로 커지는 것을 방지한다.

SQLite 사용으로 실행 바이너리에 드라이버 용량이 추가되는 비용은 있지만,
관심종목·캐시·마이그레이션을 각각 별도 파일 형식으로 구현하는 복잡도보다 작다고
판단한다. DB 파일에는 보관 기간과 최대 크기 정책을 적용하고 오래된 미확정 캐시를
정리한다.

### 10.2 종목 검색 저장 범위

`instruments`는 전 종목의 시세 이력을 복제하는 테이블이 아니다. 검색에 필요한
다음 최소 필드만 보관한다.

```text
provider | market | symbol | display_name | instrument_type | active | updated_at
```

국내 상장 종목 수준의 문자열 메타데이터는 작은 데이터셋이며, 현재가·호가·재무
정보는 검색 인덱스에 넣지 않는다. SQLite는 관심 종목과 캔들 캐시에도 필요하므로
검색 인덱스를 추가한다고 별도 데이터베이스 엔진이 늘어나지 않는다.

검색 흐름은 하이브리드 방식으로 고정한다.

```text
검색어 입력 → 로컬 최소 인덱스에서 즉시 필터링
결과 선택   → 증권사 API에서 현재가/상세정보 조회
정확한 코드가 로컬에 없음 → 원격 조회 후 인덱스 보완
```

이 구조는 입력할 때마다 발생하는 네트워크 지연과 API 호출 제한을 피하면서도
로컬에 불필요한 시장 데이터를 쌓지 않는다.

### 10.3 스키마 생성과 자동 동기화

스키마만 생성한다고 데이터가 자동으로 채워지는 것은 아니다. 애플리케이션이
스키마와 다음 동기화 로직을 함께 제공한다.

```text
최초 실행
  → SQLite 파일 생성 및 마이그레이션
  → 사용자가 등록한 API 자격증명으로 인증
  → 종목정보 파일/API에서 최소 종목 인덱스 적재
  → 지원되는 증권사의 관심종목 그룹 및 항목 적재
  → 계좌·시세·캔들은 화면 요청과 TTL 정책에 따라 적재
```

사용자가 해야 할 일은 각 증권사에서 API 사용을 신청하고 발급된 자격증명을
Stock Min 설정에 등록하는 것이다. 그 이후 테이블 생성, 초기 적재, 갱신 및
upsert는 프로그램이 수행한다. 인증 정보가 없거나 일부 증권사 연결이 실패하면
해당 공급자만 건너뛰고 로컬 기능과 정상 공급자는 계속 동작한다.

### 10.4 관심종목 병합 모델

- `watchlist_sources`: `LOCAL`, `KIWOOM` 등 원본 공급자
- `watchlists`: 공급자별 그룹 이름과 외부 그룹 ID
- `watchlist_items`: `(market, symbol)` 기준으로 중복 제거된 종목
- `watchlist_memberships`: 한 종목이 여러 그룹에 속하는 관계

키움 관심종목은 API에서 read-only로 가져오고, 로컬 관심종목은 TUI에서 직접
편집한다. 같은 종목이 키움의 여러 그룹과 로컬 그룹에 동시에 있어도 화면에서는
한 번만 표시하되 모든 출처와 그룹을 확인할 수 있다.

저장하지 않는 값:

- App Secret
- 접근 토큰
- 계좌 비밀번호
- 전체 계좌번호가 포함된 로그

캔들 고유 키는 `(provider, market, symbol, interval, open_time)`으로 잡고
upsert한다. 미완성 캔들은 갱신할 수 있고 확정 캔들은 원칙적으로 불변으로
취급하되 수정주가 재동기화는 별도 버전으로 처리한다.

## 11. 비동기 처리와 데이터 흐름

### 11.1 초기 실행

```text
설정/DB 로드
  → TUI 즉시 표시
  → 증권사 인증 병렬 실행
  → 종목 인덱스와 지원 증권사 관심종목 동기화
  → 계좌 및 캐시 데이터 표시
  → 현재가 갱신
  → 설정 주기에 따라 REST 폴링
```

### 11.2 상세 차트

```text
종목 선택 + Enter
  → 캐시 차트 즉시 표시
  → 필요한 캔들 범위 계산
  → Rate Limiter를 거쳐 REST 조회
  → DTO를 공통 Candle로 변환
  → SQLite upsert
  → 이동평균 계산
  → CandlesLoadedMsg로 TUI 갱신
  → 다음 폴링에서 진행 중 캔들 갱신
```

### 11.3 호출 제한

- 증권사별 limiter와 엔드포인트 그룹별 limiter를 둔다.
- 동일 종목·동일 범위 요청은 singleflight로 합친다.
- 화면 이동으로 필요 없어졌으면 context를 취소한다.
- `429` 또는 공급자 유량 오류는 서버 지시와 지수 백오프를 따른다.
- 사용자의 수동 새로고침도 limiter를 우회하지 않는다.

## 12. 오류 및 상태 모델

사용자에게 원시 오류 코드만 노출하지 않는다.

| 상태 | UI 표현 | 동작 |
|---|---|---|
| 인증 필요 | `AUTH REQUIRED` | 설정 안내 |
| 연결 중 | `CONNECTING` | 캐시 표시 가능 |
| 정상 | `LIVE` | 정상 갱신 |
| 지연 | `DELAYED` | 기준 시각 강조 |
| 캐시 | `CACHED` | 데이터 나이 표시 |
| 호출 제한 | `RATE LIMITED` | 재시도 예정 시각 표시 |
| 부분 장애 | `DEGRADED` | 정상 공급자/패널 유지 |
| 오프라인 | `OFFLINE` | 캐시만 표시 |

로그는 사용자 메시지와 개발자 진단 정보를 분리한다. 네트워크 응답 본문을
무조건 기록하지 않으며 민감 필드는 중앙 redactor로 제거한다.

## 13. 보안 요구사항

- 자격증명은 OS Keychain을 우선 사용한다.
- 개발 시 환경변수를 허용하지만 `.env`는 Git에서 제외한다.
- 설정 예제에는 가짜 값만 둔다.
- API 요청/응답 로깅 기본값은 비활성화한다.
- 계좌번호는 끝 4자리 외 마스킹한다.
- panic 및 오류 리포트에서도 토큰과 헤더를 제거한다.
- 주문 기능 추가 전 별도의 threat model과 사용자 승인 절차를 설계한다.
- 의존성 취약점은 `govulncheck`로 검사한다.

## 14. 테스트 전략

### 14.1 단위 테스트

- 금액 및 수익률 계산
- 통화 환산
- OHLCV 리샘플링
- MA5/MA20/MA60/MA120 계산
- 급등 조건 및 점수
- TTL과 데이터 freshness
- 오류 코드 매핑

### 14.2 어댑터 계약 테스트

- 실제 키가 필요 없는 `httptest.Server` 사용
- 공식 응답 예제를 익명화해 `testdata` fixture로 관리
- 정상, 빈 결과, 연속조회, 토큰 만료, rate limit, 잘못된 JSON 테스트
- WebSocket 도입 시 재연결과 중복 이벤트 테스트

### 14.3 TUI 테스트

- 화면별 golden test
- 80×24, 120×38, 160×50 터미널 크기 검증
- 좁은 화면 레이아웃 검증
- 키 입력과 화면 전환 상태 테스트
- 한글 폭, ANSI 색상 비지원 터미널 검증

### 14.4 통합 테스트

- 키움 모의 서버 또는 허용된 테스트 환경에서 read-only smoke test
- NH 실계좌는 조회 전용 opt-in smoke test
- 실제 계좌 테스트는 기본 CI에서 실행하지 않는다.

### 14.5 품질 게이트

```text
go test ./...
go test -race ./...
go vet ./...
golangci-lint run
govulncheck ./...
```

## 15. 구현 단계

### Phase 0 — 프로젝트 골격과 Mock TUI

- Go 모듈과 디렉터리 생성
- Bubble Tea 전역 모델과 화면 라우팅
- 공통 스타일과 키맵
- Mock Provider
- 대시보드, 내 주식, 검색, 급등주, 종목 상세 화면
- SQLite 마이그레이션 기반

산출물: 실제 키 없이 모든 화면을 둘러볼 수 있는 실행 파일

### Phase 1 — 키움 조회 연동

- 인증과 토큰 관리
- 종목 마스터
- 키움 관심종목 그룹 및 항목 read-only 동기화
- 계좌, 잔고, 보유 종목
- 현재가 및 차트
- rate limit, 연속조회, 오류 매핑
- fixture 기반 계약 테스트

산출물: 키움 데이터를 사용하는 조회형 TUI

### Phase 2 — NH 조회 연동과 통합 포트폴리오

- OAuth와 계좌 목록
- 잔고 및 보유 종목
- 현재가와 주기적 REST 갱신
- 공통 모델 정규화
- 다중 계좌 및 다중 통화 합산

산출물: NH·키움 통합 자산 화면

### Phase 3 — 차트 완성

- 틱·분·일·주·월·년봉
- 조회 기간 선택
- OHLCV 로컬 집계
- MA5/MA20/MA60/MA120
- 거래량, 확대·축소, 캔들 선택은 후속 개선
- 캐시 및 폴링 기반 진행 캔들

산출물: 종목 상세 분석 화면

### Phase 4 — 급등주와 환율

- 시장 후보 수집
- 정량 스코어와 설명
- 급등주 순위 및 상세 진입
- USD/KRW
- 데이터 freshness와 장 상태 처리

산출물: `v0.1.0-rc1`

### Phase 5 — 안정화와 배포

- 전체 테스트 및 race test
- 민감정보 로그 감사
- 작은 터미널 및 macOS/Linux 검증
- README, 설정 가이드, API 발급 가이드
- GoReleaser 빌드

산출물: `v0.1.0`

## 16. MVP 완료 정의

다음 항목을 모두 충족해야 `v0.1.0`으로 완료 처리한다.

- [ ] NH와 키움 인증 및 read-only 조회 성공
- [ ] 통합/증권사별 포트폴리오 표시
- [ ] 키움 미국주식 잔고·예수금·현재가와 USD/KRW 환산 표시
- [ ] 종목 코드·이름 검색
- [ ] 관심 종목 영구 저장
- [ ] 키움 관심종목과 로컬 관심종목 통합 표시
- [ ] 캔들 차트 표시(거래량 보조 차트는 후속)
- [ ] 틱·분·시간·일·주·월·년봉 선택
- [ ] 조회 기간 선택
- [ ] MA5·MA20·MA60·MA120을 서로 구분되는 색으로 표시
- [ ] 급등주 순위와 근거 리포트
- [ ] USD/KRW와 출처·기준 시각 표시
- [ ] 연결/지연/캐시/오류 상태 표시
- [ ] API 키 및 토큰 로그 미노출
- [ ] 80×24 이상 터미널에서 핵심 기능 사용 가능
- [ ] 단위·계약·TUI 테스트 통과
- [ ] `go test -race`, lint, 취약점 검사 통과
- [ ] macOS 및 Linux 바이너리 실행 검증
- [ ] 주문 API 호출 경로가 빌드에 연결되지 않음

## 17. 후속 로드맵

### 조회 기능 고도화

- 뉴스·공시 등 라이선스가 명확한 데이터 소스 추가
- AI는 주문이 아닌 구조화된 조회·분석 결과만 생성
- 근거 데이터와 기준 시각 첨부
- 미국주식 전체 종목 마스터와 영문명 검색
- WebSocket 실시간 시세와 REST reconciliation

### 주문 기능 보류

매수·매도·정정·취소 및 자동매매는 현재 로드맵에서 제외한다. 별도의 명시적인 사용자
승인과 독립된 보안 설계 없이는 주문 포트, 주문 API ID, 주문 화면을 추가하지 않는다.

## 18. 알려진 리스크와 대응

| 리스크 | 영향 | 대응 |
|---|---|---|
| 증권사 API 정책 변경 | 연동 중단 | 어댑터 격리, 계약 테스트, 버전/공지 확인 |
| 호출 제한 | 화면 갱신 지연 | 중앙 limiter, 캐시, singleflight |
| 폴링 사이 시세 변동 | 틱 단위 반영 지연 | 기준 시각 표시, 후속 WebSocket 및 REST reconciliation |
| 증권사별 값 정의 차이 | 통합 합계 오류 | 공통 정의 문서화, 원본/정규화 값 테스트 |
| 수정주가 차이 | 이동평균 불일치 | adjusted 메타데이터, 공급자 혼합 금지 |
| 장 세션 차이 | 캔들 집계 오류 | 거래 캘린더와 KRX/NXT 세션 모델 |
| 터미널 해상도 | 차트 가독성 저하 | 최소 크기, 반응형 축약, viewport |
| 민감정보 유출 | 계좌 보안 위험 | Keychain, redaction, 로그 감사 |
| 라이브러리 API 변경 | 빌드 실패 | 버전 고정, 차트 포트 추상화 |

## 19. 구현 착수 결과

승인 후 v0.1 조회 MVP 전체 골격과 다음 작업을 구현했다.

1. Go 모듈과 기본 디렉터리 구성
2. Bubble Tea v2, Lip Gloss v2, ntcharts v2 버전 고정
3. 도메인 모델과 조회 포트 정의
4. Mock Provider 구현
5. 대시보드 및 화면 전환 구현
6. 종목 상세 캔들 차트 기술 검증
7. SQLite 마이그레이션과 관심 종목 저장
8. 테스트와 개발 명령 정리

여기에 키움/NH REST 어댑터, 안전한 키링 설정, CLI, 로컬 관심종목, 자동화 테스트와
README까지 구현했다. 실제 증권사 smoke test와 필드별 계좌 대조는 사용자가 발급한
API 키를 `minstock setup`으로 연결한 뒤 진행한다.

## 20. 참고 자료

- Bubble Tea: <https://github.com/charmbracelet/bubbletea>
- Bubble Tea v2 릴리스: <https://github.com/charmbracelet/bubbletea/releases>
- Bubbles: <https://github.com/charmbracelet/bubbles>
- Lip Gloss: <https://github.com/charmbracelet/lipgloss>
- ntcharts: <https://github.com/NimbleMarkets/ntcharts>
- 키움 REST API: <https://openapi.kiwoom.com>
- NH NAMUH PLUG OpenAPI: <https://www.nhplug.com/apiservice>

---

이 문서는 구현 승인을 위한 V1 기준선이다. 승인 이후 범위가 바뀌면 변경 이유,
영향 범위 및 완료 조건을 함께 갱신한다.
