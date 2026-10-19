package app

import (
	"context"
	"errors"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/RandomLemon/midori/internal/display"
	"github.com/RandomLemon/midori/internal/testutil/fakebackend"
	"github.com/RandomLemon/midori/internal/ui"
)

func controllerTestOutputs() []display.Output {
	mode := display.Mode{Width: 1920, Height: 1080, RefreshMilliHz: 60000, Preferred: true}
	return []display.Output{{
		ID:          "eDP-1",
		Name:        "eDP-1",
		LogicalSize: display.Size{Width: 1920, Height: 1080},
		OutputState: display.OutputState{
			Enabled:   true,
			Mode:      mode,
			Scale:     1,
			Transform: display.TransformNormal,
		},
	}}
}

func TestApplyLoadAndPollTransitions(t *testing.T) {
	fyneApp := test.NewApp()
	defer fyneApp.Quit()
	view := ui.New(fyneApp, ui.Callbacks{})
	backend := fakebackend.New(controllerTestOutputs()...)
	service := display.NewService(backend)
	controller := New(backend, service, view)

	snapshot, err := service.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	controller.applyLoad(snapshot, "26.04", nil)
	if got := controllerState(controller); got != StateReady {
		t.Fatalf("state = %v, want Ready", got)
	}

	for range 3 {
		controller.applyPoll(display.Snapshot{}, errors.New("poll failed"))
	}
	if got := controllerState(controller); got != StateUnavailable {
		t.Fatalf("state after three failures = %v, want Unavailable", got)
	}

	controller.applyPoll(snapshot, nil)
	if got := controllerState(controller); got != StateReady {
		t.Fatalf("state after recovery = %v, want Ready", got)
	}
}

func TestFingerprintTracksState(t *testing.T) {
	outputs := controllerTestOutputs()
	first := fingerprint(display.Snapshot{Outputs: outputs})
	if first != fingerprint(display.Snapshot{Outputs: outputs}) {
		t.Fatal("identical snapshots have different fingerprints")
	}
	outputs[0].Scale = 1.25
	second := fingerprint(display.Snapshot{Outputs: outputs})
	if first == second {
		t.Fatal("scale change did not change fingerprint")
	}
}

func controllerState(c *Controller) State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}
