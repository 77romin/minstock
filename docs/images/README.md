# README 스크린샷 가이드

README의 화면 미리보기에는 아래 여섯 장을 사용합니다. 현재 자산·손익·보유 종목이
표시되는 세 이미지는 실제 화면을 기반으로 합성 샘플 데이터로 교체했습니다.

| 파일 | 권장 화면 | 보여줄 포인트 |
|---|---|---|
| `dashboard.png` | 현황 화면 | 증권사 연결 상태, 자산 요약, 데이터 갱신 시각 |
| `portfolio.png` | 내 주식 화면 | 한국/미국 탭, 다양한 자산 열, 손익 색상과 합계 |
| `chart.png` | 종목 상세 화면 | 캔들, MA5·20·60·120, 종목 정보와 분석 근거 |
| `discovery.png` | 검색 또는 급등 분석 | 한글·영문·티커 검색이나 조건별 분석 근거 |
| `diagnose.png` | 연결 진단 화면 | 공급자 연결 상태와 데이터 기준 시각 |
| `manual.png` | CLI 도움말 | 실행·동기화·설정·진단 명령과 조회 전용 안내 |

## 촬영 권장 조건

- 터미널 가로 폭 120~150 columns, 동일한 창 크기
- 다크 테마, 글자가 읽히는 배율
- 데모 모드 또는 종목·금액·수익률을 모두 교체한 합성 데이터
- 창 테두리와 불필요한 바탕화면은 최소화
- PNG 형식, 가로 1200px 이상

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
    <td width="50%"><img src="docs/images/discovery.png" alt="검색과 급등 분석"></td>
  </tr>
  <tr>
    <td width="50%"><img src="docs/images/diagnose.png" alt="연결 진단"></td>
    <td width="50%"><img src="docs/images/manual.png" alt="CLI 도움말"></td>
  </tr>
</table>
```
