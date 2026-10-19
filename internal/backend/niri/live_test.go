//go:build integration

package niri

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/RandomLemon/midori/internal/display"
)

func TestLiveOutputs(t *testing.T) {
	if os.Getenv("MIDORI_INTEGRATION") != "1" {
		t.Skip("set MIDORI_INTEGRATION=1 to run live niri tests")
	}
	backend := NewFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	version, err := backend.Version(ctx)
	if err != nil {
		t.Fatalf("Version() error = %v", err)
	}
	outputs, err := backend.Outputs(ctx)
	if err != nil {
		t.Fatalf("Outputs() error = %v", err)
	}
	if len(outputs) == 0 {
		t.Fatal("Outputs() returned no outputs")
	}
	t.Logf("niri %s: %d outputs", version, len(outputs))
	for _, output := range outputs {
		t.Logf("%s enabled=%v mode=%dx%d@%.3f scale=%v transform=%s position=%+v",
			output.ID, output.Enabled, output.Mode.Width, output.Mode.Height,
			output.Mode.RefreshHz(), output.Scale, output.Transform, output.Position)
	}
}

func TestLiveApplyRevert(t *testing.T) {
	if os.Getenv("MIDORI_INTEGRATION") != "1" {
		t.Skip("set MIDORI_INTEGRATION=1 to run live niri tests")
	}
	backend := NewFromEnv()
	service := display.NewService(backend)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	snapshot, err := service.Load(ctx)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	var target display.Output
	for _, output := range snapshot.Outputs {
		if output.Enabled && output.VRRSupported {
			target = output
			break
		}
	}
	if target.ID == "" {
		t.Skip("no enabled VRR-capable output")
	}

	desired := target.OutputState
	desired.VRR = !desired.VRR
	tx, err := service.Apply(ctx, map[display.OutputID]display.OutputState{target.ID: desired})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	reverted := false
	defer func() {
		if reverted {
			return
		}
		revertCtx, revertCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer revertCancel()
		if err := service.Revert(revertCtx, tx.Before); err != nil {
			t.Errorf("deferred Revert() error = %v", err)
		}
	}()

	applied, err := service.Load(ctx)
	if err != nil {
		t.Fatalf("Load() after apply error = %v", err)
	}
	got, _ := applied.Output(target.ID)
	if got.VRR != desired.VRR {
		t.Fatalf("VRR after apply = %v, want %v", got.VRR, desired.VRR)
	}

	if err := service.Revert(ctx, tx.Before); err != nil {
		t.Fatalf("Revert() error = %v", err)
	}
	reverted = true
	restored, err := service.Load(ctx)
	if err != nil {
		t.Fatalf("Load() after revert error = %v", err)
	}
	got, _ = restored.Output(target.ID)
	if got.VRR != target.VRR {
		t.Fatalf("VRR after revert = %v, want %v", got.VRR, target.VRR)
	}
}
