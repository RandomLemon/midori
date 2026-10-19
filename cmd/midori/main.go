package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
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
