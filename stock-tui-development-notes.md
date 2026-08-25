# Stock TUI 개발 대화 정리

> 목적: 토스증권, NH투자증권, 키움증권 등의 Open API를 연결해 계좌/주식
> 현황, 주문, 급등주 탐지·분석, 원달러 환율 등을 터미널 UI(TUI)에서
> 통합하고, 이후 AI Agent와 제한적 자동매매까지 확장하는 프로젝트의 초기
> 설계 기록.

## 1. 만들고 싶은 것

핵심 요구사항:

-   증권계좌 보유주식 및 평가손익 조회
-   TUI에서 매수/매도 및 주문 관리
-   조건 기반 자동 매수/매도
-   급등주 탐지 및 알림
-   급등 후보에 대한 AI 분석
-   USD/KRW 환율 현황
-   국내주식 및 가능하면 미국주식 지원
-   여러 증권사를 하나의 인터페이스에서 다룰 수 있는 구조
-   AI API 또는 Pi Agent 계열 Agent 연결
-   장기적으로 제한적인 자동매매

## 2. 전체 아키텍처 방향

``` text
                     ┌─ Toss Securities
                     ├─ NH Investment
TUI ─ Broker Layer ──┤
                     └─ Kiwoom
          │
          ├─ Portfolio
          ├─ Orders
          ├─ Market Data
          └─ Realtime / WebSocket
                 │
                 ▼
           급등주 Scanner
                 │
                 ▼
             AI Agent
                 │
                 ▼
            Risk Engine
                 │
                 ▼
         Execution Engine
```

핵심 원칙은 **AI와 실제 주문 실행을 분리**하는 것이다.

AI가 Broker API의 `buy()` / `sell()`을 직접 자유롭게 호출하도록 하지
않고:

``` text
Market Data
    ↓
Scanner / Strategy
    ↓
AI Analysis
    ↓
Trade Proposal
    ↓
Risk Manager
    ↓
Execution Engine
    ↓
Broker API
```

구조로 만든다.

예를 들어 AI 출력은 주문 자체가 아니라 다음과 같은 제안이어야 한다.

``` json
{
  "action": "BUY",
  "symbol": "005930",
  "reason": "volume breakout",
  "confidence": 0.76
}
```

Risk Engine이 주문 한도, 종목 비중, 손실 제한 등의 정책을 검사한 후에만
주문을 실행한다.

## 3. Broker 추상화

처음부터 증권사별 구현을 분리한다.

``` python
from abc import ABC, abstractmethod

class Broker(ABC):

    @abstractmethod
    async def get_balance(self):
        ...

    @abstractmethod
    async def get_positions(self):
        ...

    @abstractmethod
    async def get_quote(self, symbol):
        ...

    @abstractmethod
    async def buy(self, symbol, qty, price=None):
        ...

    @abstractmethod
    async def sell(self, symbol, qty, price=None):
        ...

    @abstractmethod
    async def cancel_order(self, order_id):
        ...

    @abstractmethod
    async def stream_quotes(self, symbols):
        ...
```

구현체 예:

``` text
TossBroker
NHBroker
KiwoomBroker
```

이렇게 해두면 TUI와 전략 엔진은 어느 증권사인지 알 필요가 없다.

## 4. 토스증권

대화에서 확인한 방향:

-   공식 Open API 사용
-   계좌/보유종목 조회
-   주문 생성
-   주문 정정/취소
-   조건주문
-   시세/랭킹 조회
-   KRW/USD 환율 조회
-   REST 중심

따라서 토스 계좌를 사용하는 경우 Portfolio 및 주문 실행 Provider 중
하나로 붙일 수 있다.

## 5. NH투자증권

NH투자증권도 Open API를 제공하는 것으로 확인했다.

대화에서 검토한 주요 특징:

-   NAMUH PLUG OpenAPI
-   REST
-   WebSocket 실시간 데이터
-   국내/해외주식
-   주문/정정/취소
-   계좌/잔고/손익
-   시세 및 차트
-   App Key / App Secret 기반 인증
-   Python SDK 활용 가능

따라서 멀티 증권사 Broker Layer에 `NHBroker`로 구현하기 좋은 후보이다.

## 6. 키움증권

키움도 기존 Windows OCX 방식 OpenAPI+ 외에 REST API를 제공한다.

검토한 기능:

-   REST API
-   WebSocket
-   OAuth 계열 인증
-   국내/미국주식
-   계좌
-   시세
-   주문
-   차트
-   순위정보
-   조건검색
-   실시간 조건검색
-   모의투자

특히 **급등주 Scanner 개발 및 자동매매 테스트**에는 키움의 조건검색과
모의투자가 유용하다.

예:

``` text
등락률 > +5%
AND
거래량 > 전일 300%
AND
거래대금 > 50억원
AND
시가총액 > 1000억원
```

조건검색 결과를 WebSocket 이벤트로 받고 추가 분석을 수행하는 구조를
고려할 수 있다.

## 7. 급등주 Scanner

단순 현재 등락률만 보는 대신 여러 신호를 조합한다.

예시:

``` python
score = (
    price_change_5m * 0.30
    + volume_ratio * 0.30
    + turnover_ratio * 0.20
    + breakout_score * 0.10
    + volatility_score * 0.10
)
```

TUI 표시 예:

``` text
급등 감지

005930 삼성전자
+7.8%

5분 상승률      +3.1%
거래량 증가     4.8x
거래대금 순위   #12
전고점 돌파      YES
VI               근접

Risk: HIGH
```

향후 고려할 신호:

-   1분/3분/5분 가격 변화
-   거래량 급증
-   거래대금
-   시가총액
-   전고점 돌파
-   당일 고점 돌파
-   VI 근접/발동
-   호가 잔량 변화
-   체결강도
-   변동성
-   시장/업종 대비 상대강도

## 8. AI Agent

Pi Agent를 포크하거나 별도 LLM API를 연결할 수 있다.

Agent에 직접 Broker 객체를 넘기는 대신 제한된 Tool API를 제공한다.

``` text
get_portfolio()
get_quote(symbol)
get_candles(symbol)
get_rankings()
get_exchange_rate()

analyze_stock(symbol)

propose_buy(symbol, amount)
propose_sell(symbol, quantity)
```

실제 주문 권한은 `ExecutionEngine`만 가진다.

## 9. Risk Engine

자동매매 전에 반드시 별도의 정책 계층을 둔다.

예:

``` text
1회 최대 주문금액
1일 최대 주문금액
일일 최대 손실
종목별 최대 투자 비중
전체 포트폴리오 최대 노출
시장가 주문 허용 여부
거래량 부족 종목 제외
투자경고/위험 종목 제외
연속 주문 제한
API 오류 시 거래 중단
Kill Switch
```

초기 버전에서는 AI가 자동으로 주문하기보다:

``` text
AI 분석
→ 주문 제안
→ 사용자 승인
→ 주문
```

방식으로 시작한다.

충분히 검증된 뒤:

``` text
Rule + AI
→ Risk Engine
→ 제한 범위 내 자동주문
```

으로 확장한다.

## 10. TUI

Python 기반이라면 Textual을 우선 후보로 고려한다.

예상 기술 스택:

``` text
Python 3.12+
Textual          TUI
httpx            REST
websockets       WebSocket
pydantic         데이터 모델
asyncio          비동기 처리
SQLite/DuckDB    로컬 데이터
Polars/Pandas    분석
LLM / Pi Agent   AI 분석
```

화면 예:

``` text
┌─────────────────────────────────────────────────────────┐
│                     STOCK TUI                           │
├───────────────┬──────────────────┬──────────────────────┤
│ Portfolio     │ Market / 급등주   │ USD/KRW              │
│               │                  │                      │
│ 삼성 +3.2%    │ ABC +18.2%       │ 1,3xx.xx             │
│ NVDA -1.1%    │ XYZ +13.5%       │ ▲ 0.xx%              │
├───────────────┴──────────────────┴──────────────────────┤
│ AI Analysis                                             │
│ 거래량 급증 / 변동성 증가 / 전고점 돌파 등             │
├─────────────────────────────────────────────────────────┤
│ [B] Buy [S] Sell [C] Condition [A] AI [R] Refresh      │
└─────────────────────────────────────────────────────────┘
```

멀티 증권사 포트폴리오 예:

``` text
┌ Portfolio ─────────────────────────┐
│ Broker     Asset        P/L        │
│ NH         삼성전자     +4.2%       │
│ NH         NVDA         +8.1%       │
│ Kiwoom     SK하이닉스   -1.2%       │
│ Toss       AAPL         +3.7%       │
└────────────────────────────────────┘
```

## 11. 프로젝트 디렉터리 초안

``` text
stock-tui/
├── app/
│   ├── tui/
│   │   ├── dashboard.py
│   │   ├── portfolio.py
│   │   ├── scanner.py
│   │   ├── orders.py
│   │   └── ai_panel.py
│   │
│   ├── brokers/
│   │   ├── base.py
│   │   ├── toss.py
│   │   ├── nh.py
│   │   └── kiwoom.py
│   │
│   ├── market/
│   │   ├── scanner.py
│   │   ├── indicators.py
│   │   ├── ranking.py
│   │   └── fx.py
│   │
│   ├── trading/
│   │   ├── strategy.py
│   │   ├── risk.py
│   │   ├── execution.py
│   │   └── orders.py
│   │
│   ├── agent/
│   │   ├── tools.py
│   │   ├── analyst.py
│   │   └── prompts.py
│   │
│   ├── models/
│   └── storage/
│
├── config/
├── data/
├── tests/
├── scripts/
├── pyproject.toml
└── README.md
```

## 12. 개발 단계

### v0.1 - Read Only

목표: 실제 돈을 움직이지 않는 통합 Dashboard.

``` text
Broker 인증
↓
계좌 조회
↓
보유주식 조회
↓
현재가
↓
USD/KRW
↓
TUI Dashboard
```

### v0.2 - Market Scanner

``` text
실시간/주기적 Market Data
↓
급등주 후보
↓
Ranking
↓
TUI Alert
```

### v0.3 - Manual Trading

``` text
TUI
↓
Buy / Sell
↓
Risk Check
↓
Broker API
```

처음에는 실제 계좌보다 모의투자 환경을 우선 사용한다.

### v0.4 - AI Analyst

``` text
Scanner
↓
Candidate
↓
AI Analysis
↓
TUI
```

AI는 아직 주문하지 않는다.

### v0.5 - Assisted Trading

``` text
AI
↓
Trade Proposal
↓
Risk Engine
↓
사용자 승인
↓
Execution
```

### v1.0 - Limited Automation

검증 후 제한된 자동매매를 추가한다.

``` text
Strategy
+
AI Signal
↓
Risk Engine
↓
Execution Engine
↓
Broker
```

## 13. 권장 첫 개발 순서

대화에서 나온 방향을 종합하면 다음 순서가 적합하다.

``` text
1. Python 프로젝트 생성
2. Broker ABC 정의
3. 키움 모의투자 API 연결
4. 계좌/잔고 조회
5. Textual Dashboard
6. WebSocket 실시간 시세
7. 급등주 Scanner
8. 수동 주문
9. Risk Engine
10. NH Broker 연결
11. Toss Broker 연결
12. AI Analyst
13. 주문 제안 기능
14. 충분한 검증 후 제한적 자동매매
```

키움 모의투자를 먼저 사용하는 이유는 실제 자금을 건드리지 않고
주문/취소/WebSocket/Scanner를 검증하기 좋기 때문이다.

## 14. 지금부터의 구현 목표

본격적인 개발의 첫 milestone:

**`stock-tui v0.1`**

완료 조건:

-   Python 프로젝트 실행 가능
-   Textual TUI 실행
-   Broker 인터페이스 존재
-   최소 1개 Broker 연결
-   계좌 잔고 표시
-   보유종목 표시
-   현재가 표시
-   USD/KRW 표시
-   API 키를 코드에 하드코딩하지 않음
-   `.env` / 환경변수 사용
-   기본 로깅
-   테스트 구조 마련

그 이후 WebSocket과 Scanner를 붙인다.

------------------------------------------------------------------------

## 참고한 공식 서비스

-   Toss Securities Open API
-   NH투자증권 NAMUH PLUG OpenAPI
-   키움증권 REST API

API 엔드포인트와 정책은 실제 구현 시작 시 각 증권사의 최신 공식 개발자
문서를 다시 확인한다.
