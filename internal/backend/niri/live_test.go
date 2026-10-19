//go:build integration

package niri

import (
	"context"
	"os"
	"testing"
	"time"
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
