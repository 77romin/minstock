# README 스크린샷 가이드

README의 화면 미리보기에는 아래 이미지를 사용합니다. 기존 자산·손익·보유 종목
이미지는 합성 샘플입니다. 새 JPG는 mock 공급자와 임시 DB로 현재 앱의 실제 `View()`
출력을 렌더링한 뒤 브라우저에서 캡처했습니다. 화면 구성을 별도로 그린 목업이 아닙니다.

| 파일 | 권장 화면 | 보여줄 포인트 |
|---|---|---|
| `dashboard.png` | 현황 화면 | 증권사 연결 상태, 자산 요약, 데이터 갱신 시각 |
| `portfolio.png` | 내 주식 화면 | 한국/미국 탭, 다양한 자산 열, 손익 색상과 합계 |
| `chart.png` | 종목 상세 화면 | 캔들, MA5·20·60·120, 종목 정보와 분석 근거 |
| `discovery.png` | 종목 검색 | 한글·영문·티커 로컬 검색 |
| `diagnose.png` | 연결 진단 화면 | 공급자 연결 상태와 데이터 기준 시각 |
| `cli-help.jpg` | 최신 CLI 도움말 | --sync/-sy, NAVER API HUB·DART 등록 |
| `surge.jpg` | 시장 급등 | 조건, 당일·5분 등락률, 거래량 배수와 분석 근거 |
| `news.jpg` | 종목 뉴스·공시 | 탭 전환, 메타데이터, 원문 링크와 조회 상태 |
| `news-feed.jpg` | 통합 뉴스 피드 (0.4.0) | 보유/관심·종류·종목·읽음 필터와 데이터 기준 시각 |
| `dividends.jpg` | 배당 | 종목별 세전·세후 연간 예상액과 원화 환산 |
| `allocation.jpg` | 목표 비중 | 범위, 목표/현재/편차와 조회 전용 안내 |

## 촬영 권장 조건

- 터미널 가로 폭 120~150 columns, 동일한 창 크기
- 다크 테마, 글자가 읽히는 배율
- 데모 모드 또는 종목·금액·수익률을 모두 교체한 합성 데이터
- 창 테두리와 불필요한 바탕화면은 최소화
- PNG 또는 JPG 형식, TUI 캡처는 가로 1200px 이상 권장

## 새 캡처 재생성

Go와 Node.js가 필요합니다. 프로젝트 루트에서 실행합니다.

```sh
capture_dir=$(mktemp -d /tmp/minstock-readme-XXXXXX)
MINSTOCK_README_CAPTURE_DIR="$capture_dir" go test ./internal/tui -run '^TestExportReadmeScreens$' -count=1 -v
node scripts/serve-readme-captures.mjs "$capture_dir"
```

표시된 localhost 주소에서 `/surge.html`, `/news.html`, `/dividends.html`,
`/allocation.html`, `/news-feed.html`, `/cli-help.html`을 열고 `#terminal` 전체를 캡처합니다. 브라우저 캡처는
JPG로 저장합니다. 현재 터미널 출력의 ANSI 색상·선택 배경·한글 2칸 폭을 유지합니다.
미리보기 서버는 loopback에서 생성된 여섯 HTML만 제공하며 저장소 전체를 공개하지 않습니다.
캡처가 끝나면 `Ctrl+C`로 서버를 종료합니다.

이 테스트는 정상 테스트 실행에서는 건너뜁니다. 자격증명을 읽는 bootstrap 경로를
호출하지 않고 mock 공급자와 임시 SQLite만 사용합니다. 날짜·시각은 생성 시점 기준으로
바뀔 수 있습니다. 기사 제목·URL도 예시이며 `example.com` 링크는 실제 기사로 대체하지 않습니다.

검수 시 실제 계좌번호·사용자 이름·키·토큰이 없는지, 한글이 깨지지 않는지,
하단 키 안내와 선택 행이 잘리지 않는지 확인합니다. 데모 표시를 지우지 마세요.

## README에 이미지 적용

캡처를 이 디렉터리에 위 파일명으로 저장하면 README의 이미지 표에 바로 반영됩니다.

```html
<table>
  <tr>
    <td width="50%"><img src="docs/images/dashboard.png" alt="통합 현황 대시보드"></td>
    <td width="50%"><img src="docs/images/portfolio.png" alt="내 주식 포트폴리오"></td>
  </tr>
  <tr>
    <td width="50%"><img src="docs/images/chart.png" alt="종목 상세 차트"></td>
    <td width="50%"><img src="docs/images/discovery.png" alt="종목 검색"></td>
  </tr>
  <tr>
    <td width="50%"><img src="docs/images/diagnose.png" alt="연결 진단"></td>
    <td width="50%"><img src="docs/images/cli-help.jpg" alt="CLI 도움말"></td>
  </tr>
</table>
```
