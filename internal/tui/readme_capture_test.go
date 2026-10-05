package tui

import (
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/77romin/minstock-tui/internal/adapters/mock"
	db "github.com/77romin/minstock-tui/internal/adapters/sqlite"
	"github.com/77romin/minstock-tui/internal/app"
	"github.com/77romin/minstock-tui/internal/domain"
	"github.com/77romin/minstock-tui/internal/ports"
	"github.com/charmbracelet/x/ansi"
)

// Opt-in documentation export: only the mock provider and a temporary database
// are used. Never call bootstrap.Build or load credentials in this test.
func TestExportReadmeScreens(t *testing.T) {
	directory := os.Getenv("MINSTOCK_README_CAPTURE_DIR")
	if directory == "" {
		t.Skip("opt-in README screen export")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	repo, err := db.Open(filepath.Join(t.TempDir(), "demo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	p := mock.New()
	s := app.New(repo, []ports.Provider{p}, []ports.InstrumentProvider{p}, []ports.WatchlistReader{p}, []ports.FXProvider{p})
	s.SetInformationProviders([]ports.InformationProvider{p})
	s.SetDividendProvider(p)
	if errs := s.Sync(t.Context()); len(errs) != 0 {
		t.Fatal(errs)
	}
	m := New(s, "demo", time.Minute)
	m.width, m.height, m.loading, m.refreshing = 132, 36, false, false
	m.snapshot = s.Dashboard(t.Context())
	m.dividends = s.DividendPortfolio(t.Context(), m.snapshot.Positions, m.snapshot.FX)
	m.allocation, err = s.AllocationReport(t.Context(), "ALL", m.snapshot)
	if err != nil {
		t.Fatal(err)
	}
	copyCurrentAllocationTargets(&m.allocation)
	m.scanner = s.ScanSurges(t.Context())
	if len(m.scanner.Reports) == 0 {
		t.Fatalf("missing demo scanner results: %#v", m.scanner)
	}
	screens := map[string]screen{"surge": moversScreen, "dividends": dividendScreen, "allocation": allocationScreen}
	for name, screen := range screens {
		m.screen = screen
		writeReadmeScreen(t, directory, name, m.View().Content)
	}
	m.screen, m.previous, m.informationTab = detailScreen, portfolioScreen, true
	m.selected = domain.Symbol{Code: "005930", Name: "삼성전자", Market: domain.MarketKOSPI, Currency: domain.KRW}
	m.information = s.Information(t.Context(), m.selected)
	writeReadmeScreen(t, directory, "news", m.View().Content)
	help, err := exec.Command("go", "run", "../../cmd/minstock", "--help").Output()
	if err != nil {
		t.Fatal(err)
	}
	writeReadmeScreen(t, directory, "cli-help", "$ minstock -h\n\n"+string(help))
}

func writeReadmeScreen(t *testing.T, directory, name, view string) {
	t.Helper()
	content := ReadmeTerminalHTML(view)
	if err := os.WriteFile(filepath.Join(directory, name+".html"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

// Render the actual ANSI view into fixed terminal cells, preserving CJK widths,
// foreground/background colors and selection. The HTML is a capture surface,
// not an independently recreated app UI.
func ReadmeTerminalHTML(view string) string {
	sgr := regexp.MustCompile(`\x1b\[([0-9;]*)m`)
	var out strings.Builder
	out.WriteString(`<!doctype html><meta charset="utf-8"><title>minstock demo capture</title><style>body{margin:0;background:#10141c;color:#d9e1ed}#terminal{width:max-content;padding:24px;background:#10141c}header{font:13px sans-serif;color:#88a0ba;margin-bottom:18px}pre{margin:0;font:16px/24px "SFMono-Regular",Consolas,"Apple SD Gothic Neo",monospace}pre span{display:inline-block;height:24px;vertical-align:top;white-space:pre}</style><main id="terminal"><header>MINSTOCK 0.3.0 · DEMO / 합성 샘플 · 실제 계좌·뉴스가 아닙니다</header><pre>`)
	fg, bg, bold := "#d9e1ed", "transparent", false
	for len(view) > 0 {
		if location := sgr.FindStringSubmatchIndex(view); location != nil && location[0] == 0 {
			codes := strings.Split(view[location[2]:location[3]], ";")
			for i := 0; i < len(codes); i++ {
				n, _ := strconv.Atoi(codes[i])
				switch {
				case n == 0:
					fg, bg, bold = "#d9e1ed", "transparent", false
				case n == 1:
					bold = true
				case n == 22:
					bold = false
				case n == 39:
					fg = "#d9e1ed"
				case n == 49:
					bg = "transparent"
				case (n == 38 || n == 48) && i+2 < len(codes) && codes[i+1] == "5":
					index, _ := strconv.Atoi(codes[i+2])
					color := readmeANSIColor(index)
					if n == 38 {
						fg = color
					} else {
						bg = color
					}
					i += 2
				case (n == 38 || n == 48) && i+4 < len(codes) && codes[i+1] == "2":
					color := fmt.Sprintf("rgb(%s,%s,%s)", codes[i+2], codes[i+3], codes[i+4])
					if n == 38 {
						fg = color
					} else {
						bg = color
					}
					i += 4
				case n >= 30 && n <= 37:
					fg = readmeANSIColor(n - 30)
				case n >= 40 && n <= 47:
					bg = readmeANSIColor(n - 40)
				}
			}
			view = view[location[1]:]
			continue
		}
		r, size := utf8.DecodeRuneInString(view)
		view = view[size:]
		if r == '\n' {
			out.WriteByte('\n')
			continue
		}
		weight := "normal"
		if bold {
			weight = "bold"
		}
		fmt.Fprintf(&out, `<span style="width:%.1fpx;color:%s;background:%s;font-weight:%s">%s</span>`, float64(ansi.StringWidth(string(r)))*9.8, fg, bg, weight, html.EscapeString(string(r)))
	}
	out.WriteString(`</pre></main>`)
	return out.String()
}

func readmeANSIColor(n int) string {
	base := []string{"#10141c", "#d94d58", "#78d897", "#e9c46a", "#75aaff", "#ba9af7", "#72d9d1", "#d9e1ed", "#718096", "#ff7180", "#91f3b0", "#ffdf8a", "#9fc4ff", "#d4b4ff", "#a2eee8", "#ffffff"}
	if n < 0 || n > 255 {
		return base[7]
	}
	if n < 16 {
		return base[n]
	}
	if n >= 232 {
		v := 8 + (n-232)*10
		return fmt.Sprintf("rgb(%d,%d,%d)", v, v, v)
	}
	n -= 16
	levels := []int{0, 95, 135, 175, 215, 255}
	return fmt.Sprintf("rgb(%d,%d,%d)", levels[n/36], levels[(n/6)%6], levels[n%6])
}
