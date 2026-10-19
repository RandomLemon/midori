package display_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/RandomLemon/midori/internal/display"
	"github.com/RandomLemon/midori/internal/testutil/fakebackend"
)

type cancelRollbackBackend struct {
	mu      sync.Mutex
	outputs []display.Output
	blockID display.OutputID
	started chan struct{}
	blocked bool
}

func (b *cancelRollbackBackend) ID() string          { return "cancel-test" }
func (b *cancelRollbackBackend) DisplayName() string { return "Cancel test" }
func (b *cancelRollbackBackend) Version(context.Context) (string, error) {
	return "test", nil
}
func (b *cancelRollbackBackend) Capabilities() display.Capabilities {
	return display.Capabilities{Apply: true, VRR: true}
}
func (b *cancelRollbackBackend) Close() error { return nil }

func (b *cancelRollbackBackend) Outputs(context.Context) ([]display.Output, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return cloneTestOutputs(b.outputs), nil
}

func (b *cancelRollbackBackend) Apply(ctx context.Context, change display.Change) error {
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

func testOutputs() []display.Output {
	edpMode := display.Mode{Width: 1920, Height: 1200, RefreshMilliHz: 60000, Preferred: true}
	hdmiMode := display.Mode{Width: 2560, Height: 1440, RefreshMilliHz: 143912, Preferred: true}
	return []display.Output{
		{
			ID:           "eDP-1",
			Name:         "eDP-1",
			Make:         "AU Optronics",
			Model:        "0x1234",
			VRRSupported: true,
			Modes:        []display.Mode{edpMode, {Width: 1920, Height: 1200, RefreshMilliHz: 48000}},
			LogicalSize:  display.Size{Width: 1920, Height: 1200},
			OutputState: display.OutputState{
				Enabled:   true,
				Mode:      edpMode,
				Scale:     1,
				Transform: display.TransformNormal,
				Position:  display.Point{X: 0, Y: 0},
			},
		},
		{
			ID:           "HDMI-A-1",
			Name:         "HDMI-A-1",
			Make:         "Dell Inc.",
			Model:        "U2723QE",
			VRRSupported: true,
			Modes:        []display.Mode{hdmiMode, {Width: 1920, Height: 1080, RefreshMilliHz: 60000}},
			LogicalSize:  display.Size{Width: 1707, Height: 960},
			OutputState: display.OutputState{
				Enabled:   true,
				Mode:      hdmiMode,
				Scale:     1.5,
				Transform: display.TransformNormal,
				Position:  display.Point{X: 1920, Y: 0},
			},
		},
	}
}

func target(outputs []display.Output, id display.OutputID) display.OutputState {
	for _, output := range outputs {
		if output.ID == id {
			return output.OutputState
		}
	}
	panic("missing test output")
}

func TestLoadSortsOutputs(t *testing.T) {
	outputs := testOutputs()
	outputs[0], outputs[1] = outputs[1], outputs[0]
	backend := fakebackend.New(outputs...)
	service := display.NewService(backend)

	snapshot, err := service.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := []display.OutputID{snapshot.Outputs[0].ID, snapshot.Outputs[1].ID}; got[0] != "HDMI-A-1" || got[1] != "eDP-1" {
		t.Fatalf("outputs not sorted by name: %v", got)
	}
	if snapshot.Backend != "fake" {
		t.Fatalf("Backend = %q, want fake", snapshot.Backend)
	}
}

func TestApplySuccess(t *testing.T) {
	outputs := testOutputs()
	backend := fakebackend.New(outputs...)
	service := display.NewService(backend)
	after := target(outputs, "HDMI-A-1")
	after.Scale = 1.25

	tx, err := service.Apply(context.Background(), map[display.OutputID]display.OutputState{
		"HDMI-A-1": after,
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(tx.Changed) != 1 || tx.Changed[0] != "HDMI-A-1" {
		t.Fatalf("Changed = %v, want [HDMI-A-1]", tx.Changed)
	}
	got, ok := tx.After.Output("HDMI-A-1")
	if !ok {
		t.Fatal("HDMI-A-1 missing from After")
	}
	if got.Scale != 1.25 {
		t.Fatalf("After scale = %v, want 1.25", got.Scale)
	}
	calls := backend.Calls()
	applies := 0
	for _, call := range calls {
		if call.Operation == "apply" {
			applies++
		}
	}
	if len(calls) != 3 || applies != 1 {
		t.Fatalf("backend calls = %d (apply %d), want 3 (apply 1)", len(calls), applies)
	}
}

func TestApplyRejectsUnadvertisedMode(t *testing.T) {
	outputs := testOutputs()
	service := display.NewService(fakebackend.New(outputs...))
	after := target(outputs, "HDMI-A-1")
	after.Mode = display.Mode{Width: 800, Height: 600, RefreshMilliHz: 60000}

	_, err := service.Apply(context.Background(), map[display.OutputID]display.OutputState{
		"HDMI-A-1": after,
	})
	if !errors.Is(err, display.ErrInvalidConfiguration) {
		t.Fatalf("Apply() error = %v, want ErrInvalidConfiguration", err)
	}
	if !strings.Contains(err.Error(), "HDMI-A-1") || !strings.Contains(err.Error(), "mode") {
		t.Fatalf("error does not identify output and field: %v", err)
	}
}

func TestApplyRejectsOverlap(t *testing.T) {
	outputs := testOutputs()
	service := display.NewService(fakebackend.New(outputs...))
	after := target(outputs, "eDP-1")
	after.Position = display.Point{X: 1000, Y: 0}

	_, err := service.Apply(context.Background(), map[display.OutputID]display.OutputState{
		"eDP-1": after,
	})
	if !errors.Is(err, display.ErrInvalidConfiguration) {
		t.Fatalf("Apply() error = %v, want ErrInvalidConfiguration", err)
	}
	if !strings.Contains(err.Error(), "overlap") {
		t.Fatalf("error = %v, want overlap", err)
	}
}

func TestApplyRejectsAllDisabled(t *testing.T) {
	outputs := testOutputs()
	service := display.NewService(fakebackend.New(outputs...))

	_, err := service.Apply(context.Background(), map[display.OutputID]display.OutputState{
		"eDP-1":    {Enabled: false},
		"HDMI-A-1": {Enabled: false},
	})
	if !errors.Is(err, display.ErrInvalidConfiguration) {
		t.Fatalf("Apply() error = %v, want ErrInvalidConfiguration", err)
	}
}

func TestApplyRejectsUnsupportedVRR(t *testing.T) {
	outputs := testOutputs()
	outputs[0].VRRSupported = false
	service := display.NewService(fakebackend.New(outputs...))
	after := target(outputs, "eDP-1")
	after.VRR = true

	_, err := service.Apply(context.Background(), map[display.OutputID]display.OutputState{
		"eDP-1": after,
	})
	if !errors.Is(err, display.ErrInvalidConfiguration) {
		t.Fatalf("Apply() error = %v, want ErrInvalidConfiguration", err)
	}
	if !strings.Contains(err.Error(), "vrr") {
		t.Fatalf("error = %v, want vrr field", err)
	}
}

func TestApplyRejectsReadOnlyBackend(t *testing.T) {
	backend := fakebackend.New(testOutputs()...)
	backend.SetCapabilities(display.Capabilities{VRR: true})
	service := display.NewService(backend)

	_, err := service.Apply(context.Background(), map[display.OutputID]display.OutputState{
		"eDP-1": {Enabled: false},
	})
	if !errors.Is(err, display.ErrUnsupported) {
		t.Fatalf("Apply() error = %v, want ErrUnsupported", err)
	}
}

func TestApplyRollsBackPartialFailure(t *testing.T) {
	outputs := testOutputs()
	backend := fakebackend.New(outputs...)
	backend.SetApplyError("eDP-1", errors.New("rejected by compositor"))
	service := display.NewService(backend)

	edp := target(outputs, "eDP-1")
	edp.Scale = 1.25
	hdmi := target(outputs, "HDMI-A-1")
	hdmi.Scale = 1.75

	_, err := service.Apply(context.Background(), map[display.OutputID]display.OutputState{
		"eDP-1":    edp,
		"HDMI-A-1": hdmi,
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want failure")
	}
	if !display.IsRollbackFailure(err) {
		t.Fatalf("Apply() error = %v, want rollback failure", err)
	}
	if !strings.Contains(err.Error(), "eDP-1") {
		t.Fatalf("error = %v, want failing output", err)
	}

	snapshot, loadErr := service.Load(context.Background())
	if loadErr != nil {
		t.Fatalf("Load() error = %v", loadErr)
	}
	got, _ := snapshot.Output("HDMI-A-1")
	if got.Scale != 1.5 {
		t.Fatalf("HDMI-A-1 scale = %v, want rollback to 1.5", got.Scale)
	}
	calls := backend.Calls()
	applies := 0
	for _, call := range calls {
		if call.Operation == "apply" {
			applies++
		}
	}
	if applies != 4 {
		t.Fatalf("apply calls = %d, want 4 (two attempts plus rollback of both): %+v", applies, calls)
	}
}

func TestApplyRollbackSurvivesCancellation(t *testing.T) {
	outputs := testOutputs()
	backend := &cancelRollbackBackend{
		outputs: outputs,
		blockID: "eDP-1",
		started: make(chan struct{}),
	}
	service := display.NewService(backend)

	edp := target(outputs, "eDP-1")
	edp.Scale = 1.25
	hdmi := target(outputs, "HDMI-A-1")
	hdmi.Scale = 1.75
	targets := map[display.OutputID]display.OutputState{
		"eDP-1":    edp,
		"HDMI-A-1": hdmi,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := service.Apply(ctx, targets)
		done <- err
	}()
	<-backend.started
	cancel()

	err := <-done
	if err == nil {
		t.Fatal("Apply() error = nil, want cancellation failure")
	}
	if display.IsRollbackFailure(err) {
		t.Fatalf("rollback failed after cancellation: %v", err)
	}
	restored, loadErr := backend.Outputs(context.Background())
	if loadErr != nil {
		t.Fatalf("Outputs() error = %v", loadErr)
	}
	for _, id := range []display.OutputID{"eDP-1", "HDMI-A-1"} {
		got, _ := display.Snapshot{Outputs: restored}.Output(id)
		want, _ := display.Snapshot{Outputs: outputs}.Output(id)
		if got.Scale != want.Scale {
			t.Fatalf("%s scale after canceled apply = %v, want %v", id, got.Scale, want.Scale)
		}
	}
}

func TestApplyRejectsNoChanges(t *testing.T) {
	outputs := testOutputs()
	service := display.NewService(fakebackend.New(outputs...))

	_, err := service.Apply(context.Background(), map[display.OutputID]display.OutputState{
		"eDP-1": target(outputs, "eDP-1"),
	})
	if !errors.Is(err, display.ErrInvalidConfiguration) {
		t.Fatalf("Apply() error = %v, want ErrInvalidConfiguration", err)
	}
}

func TestRevertRestoresSnapshot(t *testing.T) {
	outputs := testOutputs()
	backend := fakebackend.New(outputs...)
	service := display.NewService(backend)
	before, err := service.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	after := target(outputs, "HDMI-A-1")
	after.Scale = 1.25
	if _, err := service.Apply(context.Background(), map[display.OutputID]display.OutputState{"HDMI-A-1": after}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if err := service.Revert(context.Background(), before); err != nil {
		t.Fatalf("Revert() error = %v", err)
	}

	current, err := service.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got, _ := current.Output("HDMI-A-1")
	if got.Scale != 1.5 {
		t.Fatalf("HDMI-A-1 scale = %v, want 1.5", got.Scale)
	}
}

func TestServiceWithoutBackend(t *testing.T) {
	service := display.NewService(nil)
	if _, err := service.Load(context.Background()); !errors.Is(err, display.ErrNoBackend) {
		t.Fatalf("Load() error = %v, want ErrNoBackend", err)
	}
	if _, err := service.Apply(context.Background(), nil); !errors.Is(err, display.ErrNoBackend) {
		t.Fatalf("Apply() error = %v, want ErrNoBackend", err)
	}
}

func TestApplyRejectsConcurrentTransaction(t *testing.T) {
	outputs := testOutputs()
	backend := fakebackend.New(outputs...)
	started := make(chan struct{})
	release := make(chan struct{})
	backend.SetApplyFunc(func(display.Change) error {
		close(started)
		<-release
		return nil
	})
	service := display.NewService(backend)

	after := target(outputs, "eDP-1")
	after.Scale = 1.25
	done := make(chan error, 1)
	go func() {
		_, err := service.Apply(context.Background(), map[display.OutputID]display.OutputState{"eDP-1": after})
		done <- err
	}()
	<-started

	_, err := service.Apply(context.Background(), map[display.OutputID]display.OutputState{"eDP-1": after})
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("second Apply() error = %v, want already running", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first Apply() error = %v", err)
	}
}

func TestApplyRollbackSucceedsAfterTransientFailure(t *testing.T) {
	outputs := testOutputs()
	backend := fakebackend.New(outputs...)
	backend.SetApplyErrorOnce("eDP-1", errors.New("reject once"))
	service := display.NewService(backend)

	edp := target(outputs, "eDP-1")
	edp.Scale = 1.25
	hdmi := target(outputs, "HDMI-A-1")
	hdmi.Scale = 1.75

	_, err := service.Apply(context.Background(), map[display.OutputID]display.OutputState{
		"eDP-1":    edp,
		"HDMI-A-1": hdmi,
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want failure")
	}
	if display.IsRollbackFailure(err) {
		t.Fatalf("Apply() reported rollback failure after successful rollback: %v", err)
	}
	snapshot, loadErr := service.Load(context.Background())
	if loadErr != nil {
		t.Fatalf("Load() error = %v", loadErr)
	}
	for _, id := range []display.OutputID{"eDP-1", "HDMI-A-1"} {
		got, _ := snapshot.Output(id)
		want, _ := display.Snapshot{Outputs: outputs}.Output(id)
		if got.Scale != want.Scale {
			t.Fatalf("%s scale = %v, want restored %v", id, got.Scale, want.Scale)
		}
	}
}

func TestValidateDraft(t *testing.T) {
	outputs := testOutputs()
	caps := display.Capabilities{Apply: true, VRR: true}
	if err := display.ValidateDraft(caps, display.Snapshot{Outputs: outputs}, outputs); err != nil {
		t.Fatalf("ValidateDraft() error = %v", err)
	}

	overlap := cloneTestOutputs(outputs)
	overlap[0].Position = display.Point{X: 1000, Y: 0}
	if err := display.ValidateDraft(caps, display.Snapshot{Outputs: outputs}, overlap); !errors.Is(err, display.ErrInvalidConfiguration) {
		t.Fatalf("ValidateDraft(overlap) error = %v, want ErrInvalidConfiguration", err)
	}

	allDisabled := cloneTestOutputs(outputs)
	allDisabled[0].Enabled = false
	allDisabled[1].Enabled = false
	if err := display.ValidateDraft(caps, display.Snapshot{Outputs: outputs}, allDisabled); !errors.Is(err, display.ErrInvalidConfiguration) {
		t.Fatalf("ValidateDraft(all disabled) error = %v, want ErrInvalidConfiguration", err)
	}
}

func cloneTestOutputs(outputs []display.Output) []display.Output {
	cloned := make([]display.Output, len(outputs))
	copy(cloned, outputs)
	for index := range cloned {
		cloned[index].Modes = append([]display.Mode(nil), outputs[index].Modes...)
	}
	return cloned
}

func TestValidateDraftCustomMode(t *testing.T) {
	custom := display.Mode{Width: 2560, Height: 1440, RefreshMilliHz: 143912}
	advertised := display.Mode{Width: 1920, Height: 1080, RefreshMilliHz: 60000, Preferred: true}
	current := display.Output{
		ID:           "DP-1",
		Name:         "DP-1",
		IsCustomMode: true,
		Modes:        []display.Mode{advertised},
		OutputState: display.OutputState{
			Enabled: true,
			Mode:    custom,
			Scale:   1,
		},
	}
	snapshot := display.Snapshot{Outputs: []display.Output{current}}
	caps := display.Capabilities{Apply: true, VRR: true}

	unchanged := cloneTestOutputs([]display.Output{current})
	if err := display.ValidateDraft(caps, snapshot, unchanged); err != nil {
		t.Fatalf("unchanged custom mode rejected: %v", err)
	}

	changed := cloneTestOutputs([]display.Output{current})
	changed[0].Mode = advertised
	if err := display.ValidateDraft(caps, snapshot, changed); err != nil {
		t.Fatalf("advertised mode rejected: %v", err)
	}

	arbitrary := cloneTestOutputs([]display.Output{current})
	arbitrary[0].Mode = display.Mode{Width: 800, Height: 600, RefreshMilliHz: 60000}
	if err := display.ValidateDraft(caps, snapshot, arbitrary); !errors.Is(err, display.ErrInvalidConfiguration) {
		t.Fatalf("arbitrary custom mode error = %v, want ErrInvalidConfiguration", err)
	}
}
