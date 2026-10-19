package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/RandomLemon/midori/internal/display"
)

func testApp() fyne.App {
	return test.NewApp()
}

func uiOutputs() []display.Output {
	edpMode := display.Mode{Width: 1920, Height: 1200, RefreshMilliHz: 60000, Preferred: true}
	hdmiMode := display.Mode{Width: 2560, Height: 1440, RefreshMilliHz: 60000}
	return []display.Output{
		{
			ID:          "HDMI-A-1",
			Name:        "HDMI-A-1",
			Make:        "Dell Inc.",
			Model:       "U2723QE",
			LogicalSize: display.Size{Width: 1707, Height: 960},
			OutputState: display.OutputState{
				Enabled:   true,
				Mode:      hdmiMode,
				Scale:     1.5,
				Transform: display.TransformNormal,
				Position:  display.Point{X: 1920, Y: 0},
			},
		},
		{
			ID:           "eDP-1",
			Name:         "eDP-1",
			Make:         "AU Optronics",
			Model:        "0x1234",
			VRRSupported: true,
			LogicalSize:  display.Size{Width: 1920, Height: 1200},
			OutputState: display.OutputState{
				Enabled:   true,
				Mode:      edpMode,
				Scale:     1,
				Transform: display.Transform90,
				Position:  display.Point{X: 0, Y: 0},
			},
		},
		{
			ID:          "DP-2",
			Name:        "DP-2",
			Make:        "Dell Inc.",
			Model:       "P2419H",
			LogicalSize: display.Size{},
			OutputState: display.OutputState{Enabled: false},
		},
	}
}

func TestWindowSnapshotAndSelection(t *testing.T) {
	fyneApp := testApp()
	defer fyneApp.Quit()
	window := New(fyneApp, Callbacks{})
	window.SetBackend("niri", "26.04")
	if got := window.backendBadge.Text; got != "niri 26.04" {
		t.Fatalf("backend badge = %q", got)
	}

	window.SetSnapshot(display.Snapshot{Outputs: uiOutputs()})
	window.SetBusy(false)
	window.SetReady(true)

	if len(window.canvas.tiles) != 2 {
		t.Fatalf("canvas tiles = %d, want 2 enabled outputs", len(window.canvas.tiles))
	}
	if window.inspector.Selected() != "HDMI-A-1" {
		t.Fatalf("selected = %q, want HDMI-A-1", window.inspector.Selected())
	}
	if window.refreshButton.Disabled() {
		t.Fatal("refresh button should be enabled in Ready")
	}
	if strings.Contains(window.statusLabel.Text, "Loading") {
		t.Fatalf("status = %q, want ready status", window.statusLabel.Text)
	}

	test.Tap(window.canvas.tiles["eDP-1"])
	if window.inspector.Selected() != "eDP-1" {
		t.Fatalf("tile tap selection = %q, want eDP-1", window.inspector.Selected())
	}

	window.selectOutput("DP-2")
	if window.canvas.selected != "DP-2" {
		t.Fatalf("canvas selection = %q", window.canvas.selected)
	}
	if !strings.Contains(window.inspector.details.Text, "Disabled") {
		t.Fatalf("details do not show disabled state: %q", window.inspector.details.Text)
	}
}

func TestWindowUnsupportedAndLostConnection(t *testing.T) {
	fyneApp := testApp()
	defer fyneApp.Quit()
	window := New(fyneApp, Callbacks{})

	window.ShowUnsupported()
	if window.mainView.Visible() || window.loadingView.Visible() || !window.unsupportedView.Visible() {
		t.Fatal("unsupported state visibility is wrong")
	}
	if window.errorTitle.Text != "No supported compositor detected" {
		t.Fatalf("unsupported title = %q", window.errorTitle.Text)
	}
	if window.retryButton.Disabled() {
		t.Fatal("retry button should be enabled")
	}

	window.SetSnapshot(display.Snapshot{Outputs: uiOutputs()})
	window.ShowLostConnection()
	if !window.mainView.Visible() || window.unsupportedView.Visible() {
		t.Fatal("lost connection should keep the main view")
	}
	if window.bannerLabel.Text != "Lost connection to niri. Restart Midori." {
		t.Fatalf("banner = %q", window.bannerLabel.Text)
	}
	if !window.refreshButton.Disabled() {
		t.Fatal("refresh should be disabled while unavailable")
	}
}

func TestCanvasLayoutUsesLogicalCoordinates(t *testing.T) {
	fyneApp := testApp()
	defer fyneApp.Quit()
	window := New(fyneApp, Callbacks{})
	window.canvas.SetOutputs(uiOutputs())
	window.canvas.Resize(fyne.NewSize(800, 600))

	edp := window.canvas.tiles["eDP-1"]
	hdmi := window.canvas.tiles["HDMI-A-1"]
	if edp == nil || hdmi == nil {
		t.Fatal("missing canvas tiles")
	}
	if !(hdmi.Position().X > edp.Position().X) {
		t.Fatalf("HDMI should be to the right: eDP=%+v HDMI=%+v", edp.Position(), hdmi.Position())
	}
	if edp.Size().Width <= 0 || edp.Size().Height <= 0 {
		t.Fatalf("eDP tile size = %+v", edp.Size())
	}
}

func TestFormatting(t *testing.T) {
	mode := display.Mode{Width: 2560, Height: 1440, RefreshMilliHz: 143912}
	if got := formatMode(mode); got != "2560×1440 @ 143.912 Hz" {
		t.Fatalf("formatMode() = %q", got)
	}
	if got := formatRefresh(display.Mode{RefreshMilliHz: 60000}); got != "60 Hz" {
		t.Fatalf("formatRefresh() = %q", got)
	}
	if got := formatScale(1.5); got != "150%" {
		t.Fatalf("formatScale() = %q", got)
	}
	if got := formatPosition(display.Point{X: -1920, Y: 10}); got != "-1920, 10" {
		t.Fatalf("formatPosition() = %q", got)
	}
}
