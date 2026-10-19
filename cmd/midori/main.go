package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"syscall"
	"time"

	"fyne.io/fyne/v2/app"

	midoriapp "github.com/RandomLemon/midori/internal/app"
	backendpkg "github.com/RandomLemon/midori/internal/backend"
	"github.com/RandomLemon/midori/internal/display"
	"github.com/RandomLemon/midori/internal/ui"
)

const applicationID = "io.github.randomlemon.midori"

var version = "dev"

func main() {
	ensureEnglishLocale()
	backendName := flag.String("backend", "auto", "backend to use: auto, niri, or hyprland")
	debug := flag.Bool("debug", false, "enable debug logging")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("midori %s\n", version)
		return
	}

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	switch *backendName {
	case "auto", "niri", "hyprland":
	default:
		fmt.Fprintf(os.Stderr, "midori: unknown backend %q; supported values: auto, niri, hyprland\n", *backendName)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	selected, err := backendpkg.Open(ctx, *backendName)
	cancel()
	if selected != nil {
		slog.Debug("backend selected", "backend", selected.ID())
	}
	if err != nil {
		if *backendName != "auto" {
			fmt.Fprintf(os.Stderr, "midori: %v\n", err)
			os.Exit(1)
		}
		selected = nil
	}

	service := display.NewService(selected)
	fyneApp := app.NewWithID(applicationID)

	var controller *midoriapp.Controller
	view := ui.New(fyneApp, ui.Callbacks{
		OnEdit: func(id display.OutputID, state display.OutputState) {
			if controller != nil {
				controller.EditOutput(id, state)
			}
		},
		OnApply: func() {
			if controller != nil {
				controller.Apply()
			}
		},
		OnReset: func() {
			if controller != nil {
				controller.Reset()
			}
		},
		OnAutoArrange: func() {
			if controller != nil {
				controller.AutoArrange()
			}
		},
		OnKeep: func() {
			if controller != nil {
				controller.Keep()
			}
		},
		OnRevert: func() {
			if controller != nil {
				controller.RevertNow()
			}
		},
		OnRefresh: func() {
			if controller != nil {
				controller.Refresh()
			}
		},
		OnRetry: func() {
			if controller != nil {
				controller.Retry()
			}
		},
	})
	controller = midoriapp.New(selected, service, view)
	controller.Start()
	view.Show()
	fyneApp.Run()
	controller.Stop()
	if selected != nil {
		_ = selected.Close()
	}
}

// ensureEnglishLocale re-executes the process once with an English locale.
// Fyne resolves its UI locale during package initialization, before main runs,
// so setting environment variables here is too late.
func ensureEnglishLocale() {
	if os.Getenv("MIDORI_LOCALE_READY") == "1" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	env := os.Environ()
	overrides := map[string]string{
		"LANG":                "en_US.UTF-8",
		"LC_ALL":              "en_US.UTF-8",
		"LANGUAGE":            "en",
		"MIDORI_LOCALE_READY": "1",
	}
	for index, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if value, ok := overrides[key]; ok {
			env[index] = key + "=" + value
			delete(overrides, key)
		}
	}
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}
	_ = syscall.Exec(exe, os.Args, env)
}
