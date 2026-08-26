# minstock

NH투자증권(NAMUH PLUG)과 키움증권 OpenAPI의 계좌·시세를 한 터미널에서 조회하는
Go 기반 주식 관리 TUI입니다. 주문 권한은 포함하지 않은 조회 전용 MVP이며, API 키가
없어도 내장된 샘플 데이터로 모든 화면을 실행해 볼 수 있습니다.

## 제공 기능

- NH·키움 계좌의 잔고와 보유 종목 통합 조회
- 종목코드·한글 종목명 로컬 검색
- OHLC 캔들 차트와 MA5·MA20·MA60·MA120 이동평균선
- 틱, 1·5·15·60분, 일, 주, 월, 년 차트 전환
- 키움 관심종목과 minstock 로컬 관심종목 통합 조회
- 규칙과 근거가 보이는 급등 후보 분석
- 키움 미국주식 기준환율을 이용한 USD/KRW 표시
- OAuth 토큰 자동 갱신, 요청 제한, SQLite 캔들 캐시
- API 키가 없을 때 자동 demo 모드

> 급등 분석은 정량 조건을 설명하는 관찰 도구이며 투자 추천이 아닙니다.

## 빠른 시작

Go 1.27 이상이 필요합니다.

```sh
git clone https://github.com/mink/stock-min-tui.git
cd stock-min-tui
make build
./bin/minstock
```

시스템 어디서나 `minstock`으로 실행하려면 다음처럼 설치합니다.

```sh
make install
```

`$(go env GOPATH)/bin`이 `PATH`에 포함되어 있어야 합니다. zsh에서는 필요할 경우
`~/.zshrc`에 아래 줄을 추가한 뒤 새 터미널을 여십시오.

```sh
export PATH="$(go env GOPATH)/bin:$PATH"
```

이후 실행 명령은 간단히 `minstock`입니다.

## 명령과 옵션

TUI 안에서 대부분의 작업을 하므로 옵션은 운영에 필요한 최소 범위만 제공합니다.

| 명령 | 설명 |
|---|---|
| `minstock` | TUI 실행 |
| `minstock sync` | 종목 인덱스와 지원되는 증권사 관심종목 동기화 |
| `minstock setup kiwoom` | 키움 App Key/Secret을 OS 키링에 저장 |
| `minstock setup nh` | NH App Key/Secret을 OS 키링에 저장 |
| `minstock --diagnose` | 설정 경로, DB 경로, 자격증명 준비 상태 확인 |
| `minstock --version` | 버전 출력 |
| `minstock --config ./my.toml` | 지정한 설정 파일로 TUI 실행 |
| `minstock help` | CLI 사용법 출력 |

전역 옵션은 하위 명령보다 앞에 둡니다. 예: `minstock --config ./my.toml sync`.

`--broker`, `--symbol`, `--interval` 같은 조회 옵션은 만들지 않았습니다. 대화형 TUI에서
필터와 종목·차트 단위를 바꾸는 편이 더 빠르고, CLI는 설정·진단·동기화처럼 자동화에
유용한 기능만 담당합니다.

## TUI 사용법

| 키 | 동작 |
|---|---|
| `1`~`5` | 현황, 내 주식, 검색, 관심종목, 급등 분석 이동 |
| `↑`/`↓`, `j`/`k` | 행 선택 |
| `Enter` | 선택 종목 상세 차트 |
| `/` | 종목 검색 |
| 상세 화면 `←`/`→`, `h`/`l` | 틱 → 분 → 일 → 주 → 월 → 년 단위 전환 |
| 상세 화면 `a` | minstock 로컬 관심종목에 추가 |
| 관심종목 화면 `d` | 선택 종목이 로컬 항목이면 삭제 |
| `r` | 새로고침 |
| `Esc` | 이전 화면 |
| `?` | 도움말 |
| `q`, `Ctrl+C` | 종료 |

차트 색상은 MA5 노랑, MA20 마젠타, MA60 초록, MA120 시안입니다. 여기서 MA5는
현재 선택한 봉 5개의 평균입니다. 따라서 일봉에서는 5일선이고, 5분봉에서는
25분 범위의 5개 봉 이동평균입니다.

## 증권사 API 연결

먼저 각 증권사의 개발자 페이지에서 조회 권한의 App Key와 App Secret을 발급합니다.
키는 설정 파일이나 SQLite에 평문 저장하지 않고 macOS Keychain/Linux Secret Service에
보관합니다.

```sh
minstock setup kiwoom
minstock setup nh
minstock --diagnose
minstock sync
minstock
```

CI나 일시적인 셸에서는 환경변수도 사용할 수 있습니다.

```sh
export KIWOOM_APP_KEY="..."
export KIWOOM_APP_SECRET="..."
export NHPLUG_APP_KEY="..."
export NHPLUG_APP_SECRET="..."
minstock
```

자격증명은 환경변수가 OS 키링보다 우선합니다. 로그와 진단 출력에는 비밀값과 접근
토큰을 출력하지 않습니다. 키움 자격증명이 있으면 종목 마스터, 앱 관심종목 및
USD/KRW 기준환율도 동기화합니다. 현재 공개된 NH API에는 앱 관심종목 조회 API가
없어 NH 관심종목은 minstock의 로컬 관심종목으로 관리합니다.

### 설정 파일

기본 설정 경로는 `minstock --diagnose`로 확인합니다. macOS 기본값은
`~/Library/Application Support/minstock/config.toml`입니다.

```sh
mkdir -p "$HOME/Library/Application Support/minstock"
cp configs/minstock.example.toml "$HOME/Library/Application Support/minstock/config.toml"
```

예시:

```toml
[app]
refresh_interval = "5s"
theme = "auto"

[kiwoom]
enabled = false
mode = "mock"
base_url = "https://mockapi.kiwoom.com"

[nh]
enabled = false
mode = "mock"
base_url = "https://moapi.nhplug.com:8443"
auth_url = "https://api.nhplug.com:8443"
```

`setup`으로 자격증명을 저장하면 `enabled = false`여도 해당 증권사를 자동 활성화합니다.
실서버를 사용하려면 각 증권사의 공식 문서에 맞춰 `mode`와 `base_url`을 변경하십시오.
NH 모의 시세/계좌 API는 모의 도메인을 사용하지만 OAuth 발급은 공식 SDK 기준 인증
도메인을 사용합니다.

## SQLite를 쓰는 이유

SQLite에는 모든 시세와 기업정보를 복제하지 않습니다. 검색에 필요한 종목코드·이름·
시장, 관심종목 관계, 이미 받은 확정 캔들만 저장합니다. 현재가와 계좌 잔고는 화면을
갱신할 때 API에서 가져옵니다.

이 방식의 장점은 다음과 같습니다.

- 글자를 입력할 때마다 증권사 API를 호출하지 않아 검색이 즉시 반응합니다.
- 호출 횟수 제한과 네트워크 장애의 영향을 줄입니다.
- 관심종목과 과거 캔들이 재실행 후에도 유지됩니다.
- 별도 DB 서버 없이 파일 하나로 백업·삭제할 수 있습니다.

국내 종목의 최소 메타데이터는 보통 매우 작고, 용량 대부분은 사용자가 열어 본 캔들
캐시입니다. DB 위치는 `minstock --diagnose`에 표시됩니다. 스키마와 마이그레이션은
프로그램에 포함되어 있어 첫 실행에 자동 생성되고, `minstock sync`가 API에서 받은
종목과 관심종목을 채웁니다. 즉, 사용자는 API 연결과 동기화만 하면 됩니다.

## 아키텍처

```text
cmd/minstock
    │
    ├── Bubble Tea TUI ── ntcharts/lipgloss
    │         │
    │    Application Service
    │         │
    ├─────────┼──────────────┐
    │         │              │
Kiwoom      NH PLUG       SQLite
Adapter     Adapter        Repository
    │         │              │
  REST/OAuth APIs       종목·관심·캔들 캐시
```

TUI와 애플리케이션은 증권사 응답 구조를 직접 알지 않습니다. 공통 도메인과 작은 포트
인터페이스 뒤에 어댑터를 두어, 향후 주문 기능은 별도의 `OrderExecutor` 구현과 추가
승인 절차로 격리할 수 있습니다.

주요 기술은 Go, Bubble Tea v2, Lip Gloss v2, ntcharts v2, `shopspring/decimal`,
CGO가 필요 없는 `modernc.org/sqlite`, OS 키링입니다. API 호출은 타임아웃, 호출 속도
제한, 토큰 만료 시 1회 재인증을 적용합니다.

## 현재 MVP의 범위와 제약

- 조회 전용입니다. 매수·매도·정정·취소 코드는 실행 경로에 없습니다.
- v0.1은 5초 REST 폴링 방식입니다. WebSocket 실시간 구독은 후속 릴리스 범위입니다.
- 키움은 틱/분/일/주/월/년 캔들을 제공합니다. NH 공개 국내 API의 분·틱봉이
  지원되지 않는 경우 키움 어댑터 또는 기존 캐시로 폴백하며, 둘 다 없으면 화면에
  명시적인 오류를 표시합니다.
- NH 공개 API에서 앱 관심종목 조회 기능을 확인할 수 없어 자동 가져오기는 키움만
  지원합니다.
- 실 증권사 스모크 테스트는 본인의 발급 키와 계좌로 진행해야 합니다. 먼저 모의
  도메인에서 조회 결과를 대조한 뒤 실서버로 전환하십시오.
- 급등 점수는 변화율·단기 상승률·거래량 비율·거래대금·고점 거리·체결강도의 규칙
  기반 결과이며 수익을 보장하지 않습니다.

## 개발 및 검증

```sh
make fmt
make test
make build
make run
```

전체 검증:

```sh
make check
```

테스트는 이동평균/봉 집계/급등 점수, SQLite 마이그레이션·검색·관심종목·캔들 캐시,
mock 공급자를 통한 전체 서비스 흐름, 터미널 차트 렌더링을 포함합니다.

상세 설계와 후속 로드맵은 [planV1.md](planV1.md)를 참고하십시오.

## 공식 참고 자료

- [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- [Kiwoom REST API](https://github.com/Kiwoom-Securities/Kiwoom-REST-API)
- [NAMUH PLUG SDK](https://github.com/PLUG-OpenAPI/nhplug-sdk)

## 라이선스

라이선스는 아직 지정하지 않았습니다. 외부 공개 전에 `LICENSE` 파일을 추가하십시오.
