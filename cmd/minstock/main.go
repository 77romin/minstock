package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/77romin/minstock-tui/internal/bootstrap"
	"github.com/77romin/minstock-tui/internal/config"
	"github.com/77romin/minstock-tui/internal/security"
	"github.com/77romin/minstock-tui/internal/tui"
	"golang.org/x/term"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "minstock:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("minstock", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	flags.Usage = printUsage
	configPath := flags.String("config", "", "설정 파일 경로")
	diagnose := flags.Bool("diagnose", false, "설정 및 연결 준비 상태 진단")
	diagnoseShort := flags.Bool("d", false, "same as --diagnose")
	showVersion := flags.Bool("version", false, "버전 출력")
	versionShort := flags.Bool("v", false, "same as --version")
	syncFlag := flags.Bool("sync", false, "sync instruments and watchlists")
	syncShort := flags.Bool("sy", false, "same as --sync")
	setupFlag := flags.Bool("setup", false, "store API credentials in the OS keychain")
	setupShort := flags.Bool("s", false, "same as --setup")
	kiwoomFlag := flags.Bool("kiwoom", false, "select Kiwoom for --setup")
	nhFlag := flags.Bool("nh", false, "select NH for --setup")
	dividendFlag := flags.Bool("dividend", false, "select dividend data provider for --setup")
	naverFlag := flags.Bool("naver", false, "select NAVER API HUB for --setup")
	dartFlag := flags.Bool("dart", false, "select DART for --setup")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *versionShort {
		*showVersion = true
	}
	if *diagnoseShort {
		*diagnose = true
	}
	if *syncShort {
		*syncFlag = true
	}
	if *setupShort {
		*setupFlag = true
	}
	if *setupFlag {
		selected := 0
		for _, enabled := range []bool{*kiwoomFlag, *nhFlag, *dividendFlag, *naverFlag, *dartFlag} {
			if enabled {
				selected++
			}
		}
		if selected != 1 {
			return errors.New("--setup requires exactly one flag: --kiwoom, --nh, --dividend, --naver, or --dart")
		}
		if *kiwoomFlag {
			return setup("kiwoom")
		}
		if *dividendFlag {
			return setup("dividend")
		}
		if *naverFlag {
			return setup("naver")
		}
		if *dartFlag {
			return setup("dart")
		}
		return setup("nh")
	}
	if *syncFlag {
		remaining := flags.Args()
		if len(remaining) > 0 {
			return errors.New("--sync does not accept positional arguments")
		}
		cfg, err := config.Load(*configPath)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		rt, err := bootstrap.Build(ctx, cfg)
		if err != nil {
			return err
		}
		defer rt.Repo.Close()
		errs := rt.Service.Sync(ctx)
		if len(errs) > 0 {
			return errors.Join(errs...)
		}
		fmt.Println("Instrument and watchlist sync complete")
		return nil
	}
	if *showVersion {
		fmt.Println("minstock", version)
		return nil
	}

	remaining := flags.Args()
	if len(remaining) > 0 && remaining[0] == "setup" {
		if len(remaining) != 2 {
			return errors.New("사용법: minstock setup <kiwoom|nh|dividend|naver|dart>")
		}
		return setup(remaining[1])
	}
	if len(remaining) > 0 {
		if len(remaining) == 1 && remaining[0] == "help" {
			printUsage()
			return nil
		}
		if remaining[0] == "sync" {
			return errors.New("minstock sync는 지원하지 않습니다. minstock --sync 또는 minstock -sy를 사용하세요")
		}
		return fmt.Errorf("알 수 없는 명령 %q", remaining[0])
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if *diagnose {
		return printDiagnosis(cfg)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rt, err := bootstrap.Build(ctx, cfg)
	if err != nil {
		return err
	}
	defer rt.Repo.Close()

	program := tea.NewProgram(tui.New(rt.Service, rt.Mode, cfg.App.RefreshInterval))
	_, err = program.Run()
	return err
}

func setup(provider string) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "dart" {
		fmt.Print("DART API Key (입력 숨김): ")
		value, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return err
		}
		if err := security.SaveAPIKey("dart", string(value)); err != nil {
			return err
		}
		fmt.Println("DART 공시 API 키를 OS 보안 키링에 저장하고 재확인했습니다.")
		return nil
	}
	if provider == "dividend" || provider == "alphavantage" {
		fmt.Print("Alpha Vantage API Key (입력 숨김): ")
		apiKey, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return err
		}
		if err := security.SaveAPIKey("alphavantage", string(apiKey)); err != nil {
			return err
		}
		fmt.Println("배당 데이터 API 키를 OS 보안 키링에 저장하고 재확인했습니다.")
		fmt.Println("확인: minstock --diagnose")
		return nil
	}
	if provider != "kiwoom" && provider != "nh" && provider != "naver" {
		return fmt.Errorf("지원하지 않는 증권사 %q", provider)
	}
	keyLabel, secretLabel := "App Key", "App Secret"
	if provider == "naver" {
		keyLabel, secretLabel = "Client ID", "Client Secret"
		fmt.Println("NAVER API HUB의 Application > 인증 정보에서 발급된 키를 입력하세요.")
	}
	fmt.Printf("%s %s (입력 숨김): ", strings.ToUpper(provider), keyLabel)
	appKey, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return err
	}
	fmt.Printf("%s (입력 숨김): ", secretLabel)
	secret, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return err
	}
	if err := security.Save(provider, string(appKey), string(secret)); err != nil {
		return err
	}
	fmt.Println("OS 보안 키링에 저장했습니다. 다음 실행부터 자동 연결됩니다.")
	return nil
}

func printDiagnosis(cfg config.Config) error {
	fmt.Println("minstock", version)
	fmt.Println("Go:", runtime.Version(), runtime.GOOS+"/"+runtime.GOARCH)
	fmt.Println("설정:", cfg.Path)
	fmt.Println("데이터:", cfg.DataDir)
	fmt.Println("SQLite:", cfg.DBPath())
	brokers := []struct {
		name   string
		broker config.Broker
	}{{"kiwoom", cfg.Kiwoom}, {"nh", cfg.NH}}
	for _, item := range brokers {
		state := "미설정"
		if creds, err := security.Load(item.name); err == nil {
			state = "설정됨 (" + creds.Source + ")"
		} else {
			state = "미설정 (" + err.Error() + ")"
		}
		fmt.Printf("%-7s: %-24s mode=%-4s endpoint=%s\n", item.name, state, item.broker.Mode, item.broker.BaseURL)
	}
	dividendState := "미설정"
	if _, source, err := security.LoadAPIKey("alphavantage", "ALPHAVANTAGE_API_KEY"); err == nil {
		dividendState = "설정됨 (" + source + ")"
	} else {
		dividendState = "미설정 (" + err.Error() + ")"
	}
	fmt.Printf("%-7s: %-24s provider=%s endpoint=%s\n", "배당", dividendState, cfg.Dividends.Provider, cfg.Dividends.BaseURL)
	naverState := "미설정 · minstock setup naver"
	if credentials, err := security.Load("naver"); err == nil {
		naverState = "설정됨 (" + credentials.Source + ")"
	}
	dartState := "미설정 · minstock setup dart"
	if _, source, err := security.LoadAPIKey("dart", "DART_API_KEY"); err == nil {
		dartState = "설정됨 (" + source + ")"
	}
	fmt.Printf("NAVER API HUB: %s endpoint=%s\n", naverState, cfg.News.NaverBaseURL)
	fmt.Printf("DART  : %s endpoint=%s\n", dartState, cfg.News.DARTBaseURL)
	return nil
}

func printUsage() {
	fmt.Println(`minstock - terminal stock portfolio viewer (read-only)

Usage:
  minstock                         Start the TUI
  minstock --sync, -sy             Sync instrument and watchlist data
  minstock setup <kiwoom|nh|dividend|naver|dart> Store API credentials in the OS keychain
  minstock --setup, -s --kiwoom    Store Kiwoom credentials
  minstock --setup, -s --nh        Store NH credentials
  minstock --setup, -s --dividend  Store Alpha Vantage API key
  minstock --setup, -s --naver     Store NAVER API HUB Client ID / Secret
  minstock --setup, -s --dart      Store DART disclosure API key
  minstock --diagnose, -d          Check configuration and credential status
  minstock --config <path>         Use an alternate configuration file
  minstock --version, -v           Print the version
  minstock --help, -h              Show this help

This program is read-only. Order APIs are not enabled.`)
}
