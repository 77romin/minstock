# README 스크린샷 가이드

README의 화면 미리보기에는 아래 네 장을 권장합니다. 실제 계좌번호와 자산 금액이
노출되지 않도록 가능하면 자격증명 없이 실행되는 데모 모드에서 촬영합니다.

| 파일 | 권장 화면 | 보여줄 포인트 |
|---|---|---|
| `dashboard.png` | 현황 화면 | 증권사 연결 상태, 자산 요약, 데이터 갱신 시각 |
| `portfolio.png` | 내 주식 화면 | 한국/미국 탭, 다양한 자산 열, 손익 색상과 합계 |
| `chart.png` | 종목 상세 화면 | 캔들, MA5·20·60·120, 종목 정보와 분석 근거 |
| `discovery.png` | 검색 또는 급등 분석 | 한글·영문·티커 검색이나 조건별 분석 근거 |

## 촬영 권장 조건

- 터미널 가로 폭 120~150 columns, 동일한 창 크기
- 다크 테마, 글자가 읽히는 배율
- 데모 모드 또는 계좌번호·평가금액을 마스킹한 데이터
- 창 테두리와 불필요한 바탕화면은 최소화
- PNG 형식, 가로 1200px 이상

## README에 이미지 적용

캡처 네 장을 이 디렉터리에 위 파일명으로 저장한 뒤 README의 `<table>` 플레이스홀더를
아래와 같은 이미지 표로 교체합니다.

```html
<table>
  <tr>
    <td width="50%"><img src="docs/images/dashboard.png" alt="통합 현황 대시보드"></td>
    <td width="50%"><img src="docs/images/portfolio.png" alt="내 주식 포트폴리오"></td>
  </tr>
  <tr>
    <td width="50%"><img src="docs/images/chart.png" alt="종목 상세 차트"></td>
    <td width="50%"><img src="docs/images/discovery.png" alt="검색과 급등 분석"></td>
  </tr>
</table>
```
