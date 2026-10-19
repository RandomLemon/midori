package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"github.com/RandomLemon/midori/internal/display"
	"github.com/RandomLemon/midori/internal/ui"
)

type blockingApplyBackend struct {
	mu      sync.Mutex
	outputs []display.Output
	blockID display.OutputID
	started chan struct{}
	blocked bool
}

func (b *blockingApplyBackend) ID() string          { return "blocking" }
func (b *blockingApplyBackend) DisplayName() string { return "Blocking" }
func (b *blockingApplyBackend) Version(context.Context) (string, error) {
	return "test", nil
}
func (b *blockingApplyBackend) Capabilities() display.Capabilities {
	return display.Capabilities{Apply: true, VRR: true}
}
func (b *blockingApplyBackend) Close() error { return nil }

func (b *blockingApplyBackend) Outputs(ctx context.Context) ([]display.Output, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return cloneOutputs(b.outputs), nil
}

func (b *blockingApplyBackend) Apply(ctx context.Context, change display.Change) error {
	b.mu.Lock()
	block := change.Before.ID == b.blockID && !b.blocked
	if block {
		b.blocked = true
	}
	b.mu.Unlock()
	if block {
		close(b.started)
		<-ctx.Done()
		return ctx.Err()
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	for index := range b.outputs {
		if b.outputs[index].ID != change.Before.ID {
			continue
		}
		if change.After.Enabled {
			b.outputs[index].OutputState = change.After
			b.outputs[index].LogicalSize = display.LogicalSize(change.After.Mode, change.After.Scale, change.After.Transform)
		} else {
			b.outputs[index].OutputState = display.OutputState{Enabled: false}
			b.outputs[index].LogicalSize = display.Size{}
		}
		return nil
	}
	return display.ErrOutputMissing
}

func TestCloseDuringRevertWaitsForRestore(t *testing.T) {
	controller, backend := newTestController(t)
	state := controllerTestOutputs()[0].OutputState
	state.Scale = 1.25
	controller.EditOutput("eDP-1", state)
	tx, err := controller.service.Apply(context.Background(), map[display.OutputID]display.OutputState{"eDP-1": state})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	controller.finishApply(tx, nil, display.Snapshot{}, nil)

	backend.SetDelay(20 * time.Millisecond)
	controller.RevertNow()
	controller.RequestClose()

	select {
	case <-controller.stop:
	case <-time.After(5 * time.Second):
		t.Fatalf("window did not exit; state = %v", controllerState(controller))
	}
	outputs, err := backend.Outputs(context.Background())
	if err != nil {
		t.Fatalf("Outputs() error = %v", err)
	}
	if outputs[0].Scale != 1 {
		t.Fatalf("scale after close during revert = %v, want 1", outputs[0].Scale)
	}
}

func TestCloseDuringApplyRollsBackBeforeExit(t *testing.T) {
	fyneApp := test.NewApp()
	defer fyneApp.Quit()

	outputs := controllerPairOutputs()
	backend := &blockingApplyBackend{
		outputs: outputs,
		blockID: "DP-2",
		started: make(chan struct{}),
	}
	service := display.NewService(backend)
	view := ui.New(fyneApp, ui.Callbacks{})
	controller := New(backend, service, view)

	snapshot, err := service.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	controller.applyLoad(snapshot, "test", nil)

	for _, output := range outputs {
		state := output.OutputState
		state.Scale = 1.25
		controller.EditOutput(output.ID, state)
	}
	controller.Apply()
	<-backend.started
	controller.RequestClose()

	select {
	case <-controller.stop:
	case <-time.After(5 * time.Second):
		t.Fatalf("window did not exit; state = %v", controllerState(controller))
	}

	restored, err := backend.Outputs(context.Background())
	if err != nil {
		t.Fatalf("Outputs() error = %v", err)
	}
	for _, id := range []display.OutputID{"DP-1", "DP-2"} {
		got, _ := display.Snapshot{Outputs: restored}.Output(id)
		want, _ := display.Snapshot{Outputs: outputs}.Output(id)
		if got.Scale != want.Scale {
			t.Fatalf("%s scale after close = %v, want restored %v", id, got.Scale, want.Scale)
		}
	}
}
