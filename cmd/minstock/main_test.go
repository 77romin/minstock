package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpListsNewsSetupAndFlagOnlySync(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"help"}} {
		func() {
			original := os.Stdout
			read, write, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer read.Close()
			os.Stdout = write
			defer func() { os.Stdout = original }()
			err = run(args)
			write.Close()
			data, readErr := io.ReadAll(read)
			if err != nil || readErr != nil {
				t.Fatalf("help failed: %v %v", err, readErr)
			}
			text := string(data)
			for _, want := range []string{"--naver", "NAVER API HUB", "--dart", "DART", "--sync, -sy"} {
				if !strings.Contains(text, want) {
					t.Fatalf("missing %q in help", want)
				}
			}
			if strings.Contains(text, "minstock sync") {
				t.Fatal("removed sync subcommand is advertised")
			}
		}()
	}
}

func TestSyncSubcommandRejectedBeforeLoadingConfiguration(t *testing.T) {
	err := run([]string{"--config", "/does/not/exist/config.toml", "sync"})
	if err == nil || !strings.Contains(err.Error(), "minstock --sync") || !strings.Contains(err.Error(), "minstock -sy") {
		t.Fatalf("missing migration guidance: %v", err)
	}
}

func TestBothSyncFlagsReachSameConfigurationPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.toml")
	if err := os.WriteFile(path, []byte("[invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--sync", "-sy"} {
		err := run([]string{flag, "--config", path})
		if err == nil || !strings.Contains(err.Error(), "parse config") {
			t.Fatalf("%s did not reach sync path: %v", flag, err)
		}
		err = run([]string{flag, "extra"})
		if err == nil || !strings.Contains(err.Error(), "positional") {
			t.Fatalf("%s accepted an argument: %v", flag, err)
		}
	}
}

func TestNewsSetupFlagsRequireExactlyOneProvider(t *testing.T) {
	for _, args := range [][]string{{"--setup", "--naver", "--dart"}, {"-s", "--naver", "--nh"}, {"-s", "--dart", "--kiwoom"}} {
		err := run(args)
		if err == nil || !strings.Contains(err.Error(), "exactly one flag") {
			t.Fatalf("setup provider selection: %v", err)
		}
	}
}
