package app

import (
	"context"
	"errors"
	"testing"
	"time"

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
		Modes:       []display.Mode{mode},
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
		controller.applyPoll(snapshot, snapshot, errors.New("poll failed"))
	}
	if got := controllerState(controller); got != StateUnavailable {
		t.Fatalf("state after three failures = %v, want Unavailable", got)
	}

	controller.applyPoll(snapshot, snapshot, nil)
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

func newTestController(t *testing.T) (*Controller, *fakebackend.Backend) {
	t.Helper()
	return newTestControllerWithOutputs(t, controllerTestOutputs()...)
}

func newTestControllerWithOutputs(t *testing.T, outputs ...display.Output) (*Controller, *fakebackend.Backend) {
	t.Helper()
	fyneApp := test.NewApp()
	t.Cleanup(fyneApp.Quit)
	view := ui.New(fyneApp, ui.Callbacks{})
	backend := fakebackend.New(outputs...)
	service := display.NewService(backend)
	controller := New(backend, service, view)
	snapshot, err := service.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	controller.applyLoad(snapshot, "test", nil)
	if got := controllerState(controller); got != StateReady {
		t.Fatalf("initial state = %v, want Ready", got)
	}
	return controller, backend
}

func controllerPairOutputs() []display.Output {
	mode := display.Mode{Width: 1920, Height: 1080, RefreshMilliHz: 60000, Preferred: true}
	return []display.Output{
		{
			ID:          "DP-1",
			Name:        "DP-1",
			Modes:       []display.Mode{mode},
			LogicalSize: display.Size{Width: 1920, Height: 1080},
			OutputState: display.OutputState{
				Enabled:   true,
				Mode:      mode,
				Scale:     1,
				Transform: display.TransformNormal,
				Position:  display.Point{X: 0, Y: 0},
			},
		},
		{
			ID:          "DP-2",
			Name:        "DP-2",
			Modes:       []display.Mode{mode},
			LogicalSize: display.Size{Width: 1920, Height: 1080},
			OutputState: display.OutputState{
				Enabled:   true,
				Mode:      mode,
				Scale:     1,
				Transform: display.TransformNormal,
				Position:  display.Point{X: 2500, Y: 40},
			},
		},
	}
}

func TestAutoArrangeBuildsGapFreeRow(t *testing.T) {
	controller, _ := newTestControllerWithOutputs(t, controllerPairOutputs()...)
	controller.AutoArrange()
	if got := controllerState(controller); got != StateDirty {
		t.Fatalf("state = %v, want Dirty", got)
	}

	first, ok := findDraft(controller, "DP-1")
	if !ok {
		t.Fatal("DP-1 missing from draft")
	}
	second, ok := findDraft(controller, "DP-2")
	if !ok {
		t.Fatal("DP-2 missing from draft")
	}
	if first.Position != (display.Point{X: 0, Y: 0}) {
		t.Fatalf("DP-1 position = %+v, want 0,0", first.Position)
	}
	if second.Position != (display.Point{X: 1920, Y: 0}) {
		t.Fatalf("DP-2 position = %+v, want 1920,0", second.Position)
	}
	validation := display.ValidateDraft(controller.backend.Capabilities(), controller.snapshot, controller.draft)
	if validation != nil {
		t.Fatalf("arranged draft is invalid: %v", validation)
	}
}

func TestArrangedPositionsApplyAndRevert(t *testing.T) {
	controller, backend := newTestControllerWithOutputs(t, controllerPairOutputs()...)
	controller.AutoArrange()

	targets := map[display.OutputID]display.OutputState{}
	for _, output := range controller.draft {
		targets[output.ID] = output.OutputState
	}
	tx, err := controller.service.Apply(context.Background(), targets)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	controller.finishApply(tx, nil, display.Snapshot{}, nil)
	if got := controllerState(controller); got != StateConfirming {
		t.Fatalf("state = %v, want Confirming", got)
	}

	applied, err := backend.Outputs(context.Background())
	if err != nil {
		t.Fatalf("Outputs() error = %v", err)
	}
	if applied[1].Position != (display.Point{X: 1920, Y: 0}) {
		t.Fatalf("applied position = %+v, want 1920,0", applied[1].Position)
	}

	controller.RevertNow()
	waitForState(t, controller, StateReverting, 3*time.Second)
	waitForState(t, controller, StateReady, 3*time.Second)

	reverted, err := backend.Outputs(context.Background())
	if err != nil {
		t.Fatalf("Outputs() error = %v", err)
	}
	if reverted[1].Position != (display.Point{X: 2500, Y: 40}) {
		t.Fatalf("reverted position = %+v, want 2500,40", reverted[1].Position)
	}
}

func waitForState(t *testing.T, controller *Controller, want State, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case got := <-controller.stateChanged:
			if got == want {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for state %v; current %v", want, controllerState(controller))
		}
	}
}

func TestEditMarksDirty(t *testing.T) {
	controller, _ := newTestController(t)
	state := controllerTestOutputs()[0].OutputState
	state.Scale = 1.25
	controller.EditOutput("eDP-1", state)
	if got := controllerState(controller); got != StateDirty {
		t.Fatalf("state = %v, want Dirty", got)
	}
	if !controller.isDirty(controller.draft, controller.snapshot) {
		t.Fatal("draft should differ from snapshot")
	}
}

func TestKeepAfterApply(t *testing.T) {
	controller, _ := newTestController(t)
	state := controllerTestOutputs()[0].OutputState
	state.Scale = 1.25
	controller.EditOutput("eDP-1", state)
	targets := map[display.OutputID]display.OutputState{"eDP-1": state}
	tx, err := controller.service.Apply(context.Background(), targets)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	controller.finishApply(tx, nil, display.Snapshot{}, nil)
	if got := controllerState(controller); got != StateConfirming {
		t.Fatalf("state = %v, want Confirming", got)
	}
	controller.Keep()
	if got := controllerState(controller); got != StateReady {
		t.Fatalf("state after Keep = %v, want Ready", got)
	}
	got, _ := controller.snapshot.Output("eDP-1")
	if got.Scale != 1.25 {
		t.Fatalf("kept scale = %v, want 1.25", got.Scale)
	}
}

func TestRevertNowRestoresPreviousState(t *testing.T) {
	controller, backend := newTestController(t)
	state := controllerTestOutputs()[0].OutputState
	state.Scale = 1.25
	controller.EditOutput("eDP-1", state)
	tx, err := controller.service.Apply(context.Background(), map[display.OutputID]display.OutputState{"eDP-1": state})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	controller.finishApply(tx, nil, display.Snapshot{}, nil)
	controller.RevertNow()
	waitForState(t, controller, StateReverting, 3*time.Second)
	waitForState(t, controller, StateReady, 3*time.Second)

	outputs, err := backend.Outputs(context.Background())
	if err != nil {
		t.Fatalf("Outputs() error = %v", err)
	}
	if outputs[0].Scale != 1 {
		t.Fatalf("scale after revert = %v, want 1", outputs[0].Scale)
	}
}

func TestCountdownAutomaticallyReverts(t *testing.T) {
	controller, _ := newTestController(t)
	controller.confirmAfter = 1
	state := controllerTestOutputs()[0].OutputState
	state.Scale = 1.25
	controller.EditOutput("eDP-1", state)
	tx, err := controller.service.Apply(context.Background(), map[display.OutputID]display.OutputState{"eDP-1": state})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	controller.finishApply(tx, nil, display.Snapshot{}, nil)
	if got := controllerState(controller); got != StateConfirming {
		t.Fatalf("state = %v, want Confirming", got)
	}
	waitForState(t, controller, StateReverting, 3*time.Second)
	waitForState(t, controller, StateReady, 3*time.Second)
}

func TestRollbackFailureEntersRecovering(t *testing.T) {
	controller, backend := newTestController(t)
	state := controllerTestOutputs()[0].OutputState
	state.Scale = 1.25
	controller.EditOutput("eDP-1", state)
	backend.SetApplyError("eDP-1", errors.New("rejected"))
	_, err := controller.service.Apply(context.Background(), map[display.OutputID]display.OutputState{"eDP-1": state})
	if err == nil {
		t.Fatal("Apply() error = nil, want failure")
	}
	controller.finishApply(nil, err, display.Snapshot{}, nil)
	if got := controllerState(controller); got != StateRecovering {
		t.Fatalf("state = %v, want Recovering", got)
	}
}

func TestBuildDraftDefaultsDisabledOutput(t *testing.T) {
	controller := &Controller{}
	mode := display.Mode{Width: 1920, Height: 1080, RefreshMilliHz: 60000, Preferred: true}
	outputs := []display.Output{
		{
			ID:          "eDP-1",
			Name:        "eDP-1",
			LogicalSize: display.Size{Width: 1920, Height: 1080},
			OutputState: display.OutputState{
				Enabled: true,
				Mode:    display.Mode{Width: 1920, Height: 1080, RefreshMilliHz: 60000},
				Scale:   1,
			},
		},
		{
			ID:    "DP-2",
			Name:  "DP-2",
			Modes: []display.Mode{mode},
		},
	}
	draft := controller.buildDraft(display.Snapshot{Outputs: outputs})
	if len(draft) != 2 {
		t.Fatalf("draft length = %d, want 2", len(draft))
	}
	disabled := draft[1].OutputState
	if disabled.Enabled || disabled.Scale != 1 || disabled.Transform != display.TransformNormal || disabled.VRR {
		t.Fatalf("disabled defaults = %+v", disabled)
	}
	if disabled.Mode != mode {
		t.Fatalf("default mode = %+v, want %+v", disabled.Mode, mode)
	}
	if disabled.Position.X != 1920 || disabled.Position.Y != 0 {
		t.Fatalf("default position = %+v, want right of enabled output", disabled.Position)
	}
}

func TestCloseDuringConfirmationReverts(t *testing.T) {
	controller, _ := newTestController(t)
	state := controllerTestOutputs()[0].OutputState
	state.Scale = 1.25
	controller.EditOutput("eDP-1", state)
	tx, err := controller.service.Apply(context.Background(), map[display.OutputID]display.OutputState{"eDP-1": state})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	controller.finishApply(tx, nil, display.Snapshot{}, nil)
	controller.RequestClose()
	waitForState(t, controller, StateReverting, 3*time.Second)
	waitForState(t, controller, StateReady, 3*time.Second)
}

func TestApplyFailureWithSuccessfulRollbackKeepsDraft(t *testing.T) {
	controller, backend := newTestController(t)
	state := controllerTestOutputs()[0].OutputState
	state.Scale = 1.25
	controller.EditOutput("eDP-1", state)
	backend.SetApplyErrorOnce("eDP-1", errors.New("rejected once"))

	_, applyErr := controller.service.Apply(context.Background(), map[display.OutputID]display.OutputState{"eDP-1": state})
	if applyErr == nil {
		t.Fatal("Apply() error = nil, want failure")
	}
	if display.IsRollbackFailure(applyErr) {
		t.Fatalf("unexpected rollback failure: %v", applyErr)
	}
	fresh, err := controller.loadSnapshot()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	controller.finishApply(nil, applyErr, fresh, nil)
	if got := controllerState(controller); got != StateDirty {
		t.Fatalf("state = %v, want Dirty", got)
	}
	got, _ := controller.snapshot.Output("eDP-1")
	if got.Scale != 1 {
		t.Fatalf("snapshot scale = %v, want restored 1", got.Scale)
	}
	draft, _ := findDraft(controller, "eDP-1")
	if draft.Scale != 1.25 {
		t.Fatalf("draft scale = %v, want retained 1.25", draft.Scale)
	}
}

func findDraft(controller *Controller, id display.OutputID) (display.OutputState, bool) {
	controller.mu.Lock()
	defer controller.mu.Unlock()
	for _, output := range controller.draft {
		if output.ID == id {
			return output.OutputState, true
		}
	}
	return display.OutputState{}, false
}
