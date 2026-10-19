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

	window.SetOutputs(uiOutputs())
	window.SetBusy(false)
	window.SetEditable(true)
	window.SetDirty(false)
	window.SetValid(true)
	window.SetReady(true)
	window.SetStatus("Session-only changes")

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

	window.SetOutputs(uiOutputs())
	window.SetBusy(false)
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

func TestEditingControlsAndApplyBar(t *testing.T) {
	fyneApp := testApp()
	defer fyneApp.Quit()
	var edited display.OutputState
	window := New(fyneApp, Callbacks{
		OnEdit: func(id display.OutputID, state display.OutputState) {
			if id == "HDMI-A-1" {
				edited = state
			}
		},
	})
	window.SetOutputs(uiOutputs())
	window.SetBusy(false)
	window.SetEditable(true)
	window.SetDirty(false)
	window.SetValid(true)
	window.SetReady(true)

	window.canvas.Resize(fyne.NewSize(800, 600))
	beforeWidth := window.canvas.tiles["HDMI-A-1"].Size().Width
	window.inspector.scaleChanged(2.0)
	afterWidth := window.canvas.tiles["HDMI-A-1"].Size().Width
	if afterWidth >= beforeWidth {
		t.Fatalf("tile width did not shrink after scale change: %v -> %v", beforeWidth, afterWidth)
	}
	window.SetDirty(true)
	if edited.Scale != 2.0 {
		t.Fatalf("edited scale = %v, want 2.0", edited.Scale)
	}
	if window.applyButton.Disabled() {
		t.Fatal("apply should be enabled for a valid dirty draft")
	}

	window.SetValid(false)
	if !window.applyButton.Disabled() {
		t.Fatal("apply should be disabled for an invalid draft")
	}
	if window.resetButton.Disabled() {
		t.Fatal("reset should remain enabled for an invalid dirty draft")
	}

	window.SetValid(true)
	window.inspector.posX.SetText("invalid")
	window.inspector.positionChanged("invalid")
	if !window.applyButton.Disabled() {
		t.Fatal("apply should be disabled for an invalid position entry")
	}

	window.SetEditable(false)
	if !window.inspector.enabled.Disabled() || !window.inspector.scale.Disabled() {
		t.Fatal("inspector controls should be disabled while busy")
	}
}

func TestConfirmationPopupCallbacks(t *testing.T) {
	fyneApp := testApp()
	defer fyneApp.Quit()
	kept := 0
	reverted := 0
	window := New(fyneApp, Callbacks{
		OnKeep:   func() { kept++ },
		OnRevert: func() { reverted++ },
	})
	window.ShowConfirmation(15)
	if !window.confirming {
		t.Fatal("confirmation should be visible")
	}
	if got := window.countdown.Text; got != "Reverting in 15 s" {
		t.Fatalf("countdown = %q", got)
	}
	window.SetCountdown(3)
	if got := window.countdown.Text; got != "Reverting in 3 s" {
		t.Fatalf("countdown = %q", got)
	}
	test.Tap(window.keepButton)
	if kept != 1 {
		t.Fatalf("keep callback count = %d", kept)
	}
	window.HideConfirmation()
	if window.confirming {
		t.Fatal("confirmation should be hidden")
	}

	window.ShowConfirmation(15)
	test.Tap(window.revertButton)
	if reverted != 1 {
		t.Fatalf("revert callback count = %d", reverted)
	}
	window.ShowConfirmation(15)
	window.win.Canvas().OnTypedKey()(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if reverted != 2 {
		t.Fatalf("escape revert callback count = %d", reverted)
	}
	window.ShowReverting()
	if !window.revertButton.Disabled() || !window.keepButton.Disabled() {
		t.Fatal("confirmation buttons should be disabled while reverting")
	}
	window.HideConfirmation()
}

func TestValidationCopy(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{&display.ValidationError{OutputID: "eDP-1", Field: "mode"}, "Select an available mode."},
		{&display.ValidationError{OutputID: "eDP-1", Field: "scale"}, "Scale must be between 50% and 300%."},
		{&display.ValidationError{OutputID: "eDP-1", Field: "position"}, "Displays must not overlap."},
		{&display.ValidationError{Field: "enabled"}, "At least one display must remain enabled."},
	}
	for _, tt := range tests {
		if got := validationCopy(tt.err); got != tt.want {
			t.Errorf("validationCopy(%v) = %q, want %q", tt.err, got, tt.want)
		}
	}
}
