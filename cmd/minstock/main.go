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
	"github.com/mink/stock-min-tui/internal/bootstrap"
	"github.com/mink/stock-min-tui/internal/config"
	"github.com/mink/stock-min-tui/internal/security"
	"github.com/mink/stock-min-tui/internal/tui"
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
	showVersion := flags.Bool("version", false, "버전 출력")
	setupFlag := flags.Bool("setup", false, "store broker credentials in the OS keychain")
	kiwoomFlag := flags.Bool("kiwoom", false, "select Kiwoom for --setup")
	nhFlag := flags.Bool("nh", false, "select NH for --setup")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *setupFlag {
		if *kiwoomFlag == *nhFlag {
			return errors.New("--setup requires exactly one broker flag: --kiwoom or --nh")
		}
		if *kiwoomFlag {
			return setup("kiwoom")
		}
		return setup("nh")
	}
	if *showVersion {
		fmt.Println("minstock", version)
		return nil
	}

	remaining := flags.Args()
	if len(remaining) > 0 && remaining[0] == "setup" {
		if len(remaining) != 2 {
			return errors.New("사용법: minstock setup <kiwoom|nh>")
		}
		return setup(remaining[1])
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

	if len(remaining) > 0 {
		switch remaining[0] {
		case "sync":
			errs := rt.Service.Sync(context.Background())
			if len(errs) > 0 {
				return errors.Join(errs...)
			}
			fmt.Println("종목 및 관심종목 동기화 완료")
			return nil
		case "help":
			printUsage()
			return nil
		default:
			return fmt.Errorf("알 수 없는 명령 %q", remaining[0])
		}
	}

	program := tea.NewProgram(tui.New(rt.Service, rt.Mode, cfg.App.RefreshInterval))
	_, err = program.Run()
	return err
}

func setup(provider string) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider != "kiwoom" && provider != "nh" {
		return fmt.Errorf("지원하지 않는 증권사 %q", provider)
	}
	fmt.Printf("%s App Key (입력 숨김): ", strings.ToUpper(provider))
	appKey, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return err
	}
	fmt.Print("App Secret (입력 숨김): ")
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
		}
		fmt.Printf("%-7s: %-24s mode=%-4s endpoint=%s\n", item.name, state, item.broker.Mode, item.broker.BaseURL)
	}
	return nil
}

func printUsage() {
	fmt.Println(`minstock - terminal stock portfolio viewer (read-only)

Usage:
  minstock                         Start the TUI
  minstock sync                    Sync instrument and watchlist data
  minstock setup <kiwoom|nh>       Store API credentials in the OS keychain
  minstock --setup --kiwoom        Same as "minstock setup kiwoom"
  minstock --setup --nh            Same as "minstock setup nh"
  minstock --diagnose              Check configuration and credential status
  minstock --config <path>         Use an alternate configuration file
  minstock --version               Print the version
  minstock --help, -h              Show this help

Options:
  --setup --kiwoom|--nh             Select a broker when using --setup

This program is read-only. Order APIs are not enabled.`)
}
