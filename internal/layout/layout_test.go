package layout

import (
	"testing"

	"github.com/RandomLemon/midori/internal/display"
)

func TestBounds(t *testing.T) {
	outputs := []display.Output{
		{
			OutputState: display.OutputState{
				Enabled:   true,
				Mode:      display.Mode{Width: 1920, Height: 1080},
				Scale:     1,
				Transform: display.TransformNormal,
				Position:  display.Point{X: -1920, Y: 0},
			},
		},
		{
			OutputState: display.OutputState{
				Enabled:   true,
				Mode:      display.Mode{Width: 2560, Height: 1440},
				Scale:     2,
				Transform: display.TransformNormal,
				Position:  display.Point{X: 0, Y: 100},
			},
		},
		{
			OutputState: display.OutputState{
				Enabled:  false,
				Position: display.Point{X: 9999, Y: 9999},
			},
		},
	}
	got, ok := Bounds(outputs)
	if !ok {
		t.Fatal("Bounds() found no outputs")
	}
	want := display.Rect{X: -1920, Y: 0, Width: 3200, Height: 1080}
	if got != want {
		t.Fatalf("Bounds() = %+v, want %+v", got, want)
	}
}

func TestBoundsEmpty(t *testing.T) {
	if _, ok := Bounds(nil); ok {
		t.Fatal("Bounds() found bounds for empty layout")
	}
}

func TestSnapPosition(t *testing.T) {
	size := display.Size{Width: 1920, Height: 1080}
	tests := []struct {
		name      string
		position  display.Point
		size      display.Size
		obstacles []display.Rect
		threshold int
		want      display.Point
	}{
		{
			name:      "snap to obstacle right edge",
			position:  display.Point{X: 1915, Y: 0},
			size:      size,
			obstacles: []display.Rect{{X: 0, Y: 0, Width: 1920, Height: 1080}},
			threshold: 12,
			want:      display.Point{X: 1920, Y: 0},
		},
		{
			name:      "snap to obstacle left edge",
			position:  display.Point{X: 5, Y: 0},
			size:      size,
			obstacles: []display.Rect{{X: 1920, Y: 0, Width: 1920, Height: 1080}},
			threshold: 12,
			want:      display.Point{X: 0, Y: 0},
		},
		{
			name:      "outside threshold",
			position:  display.Point{X: 1907, Y: 0},
			size:      size,
			obstacles: []display.Rect{{X: 0, Y: 0, Width: 1920, Height: 1080}},
			threshold: 12,
			want:      display.Point{X: 1907, Y: 0},
		},
		{
			name:      "snap vertical edge",
			position:  display.Point{X: 500, Y: 105},
			size:      display.Size{Width: 100, Height: 100},
			obstacles: []display.Rect{{X: 0, Y: 0, Width: 1000, Height: 100}},
			threshold: 12,
			want:      display.Point{X: 500, Y: 100},
		},
		{
			name:      "reject overlapping snap",
			position:  display.Point{X: 205, Y: 0},
			size:      display.Size{Width: 100, Height: 100},
			obstacles: []display.Rect{{X: 100, Y: 0, Width: 200, Height: 100}},
			threshold: 12,
			want:      display.Point{X: 205, Y: 0},
		},
		{
			name:      "disabled threshold",
			position:  display.Point{X: 1915, Y: 0},
			size:      size,
			obstacles: []display.Rect{{X: 1920, Y: 0, Width: 1920, Height: 1080}},
			threshold: 0,
			want:      display.Point{X: 1915, Y: 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SnapPosition(tt.position, tt.size, tt.obstacles, tt.threshold)
			if got != tt.want {
				t.Fatalf("SnapPosition() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestAutoArrange(t *testing.T) {
	outputs := []display.Output{
		{
			ID:   "HDMI-A-1",
			Name: "HDMI-A-1",
			OutputState: display.OutputState{
				Enabled:   true,
				Mode:      display.Mode{Width: 1920, Height: 1080},
				Scale:     1,
				Transform: display.TransformNormal,
				Position:  display.Point{X: 500, Y: 10},
			},
		},
		{
			ID:   "DP-2",
			Name: "DP-2",
			OutputState: display.OutputState{
				Enabled:   true,
				Mode:      display.Mode{Width: 2560, Height: 1440},
				Scale:     2,
				Transform: display.TransformNormal,
				Position:  display.Point{X: 0, Y: 10},
			},
		},
		{
			ID:   "eDP-1",
			Name: "eDP-1",
			OutputState: display.OutputState{
				Enabled:  false,
				Position: display.Point{X: 77, Y: 88},
			},
		},
	}
	got := AutoArrange(outputs)

	wantPositions := map[display.OutputID]display.Point{
		"DP-2":     {X: 0, Y: 0},
		"HDMI-A-1": {X: 1280, Y: 0},
		"eDP-1":    {X: 77, Y: 88},
	}
	for _, output := range got {
		if want := wantPositions[output.ID]; output.Position != want {
			t.Errorf("%s position = %+v, want %+v", output.ID, output.Position, want)
		}
	}
	dp := got[1]
	hdmi := got[0]
	if dp.Position.X+dp.LogicalSize.Width != hdmi.Position.X {
		t.Fatalf("row has a gap: DP ends at %d, HDMI starts at %d",
			dp.Position.X+dp.LogicalSize.Width, hdmi.Position.X)
	}
	if outputs[0].Position.X != 500 || outputs[1].Position.X != 0 {
		t.Fatal("AutoArrange mutated its input")
	}
}

func TestZoom(t *testing.T) {
	tests := []struct {
		name                      string
		canvasWidth, canvasHeight int
		bounds                    display.Size
		want                      float64
	}{
		{
			name:         "fit width",
			canvasWidth:  1120,
			canvasHeight: 760,
			bounds:       display.Size{Width: 2000, Height: 1000},
			want:         0.528,
		},
		{
			name:         "capped at one",
			canvasWidth:  2000,
			canvasHeight: 1000,
			bounds:       display.Size{Width: 100, Height: 100},
			want:         1,
		},
		{
			name:         "minimum zoom",
			canvasWidth:  100,
			canvasHeight: 100,
			bounds:       display.Size{Width: 10000, Height: 10000},
			want:         0.1,
		},
		{
			name:         "empty",
			canvasWidth:  1120,
			canvasHeight: 760,
			bounds:       display.Size{},
			want:         1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Zoom(tt.canvasWidth, tt.canvasHeight, tt.bounds)
			if diff := got - tt.want; diff > 1e-9 || diff < -1e-9 {
				t.Fatalf("Zoom() = %v, want %v", got, tt.want)
			}
		})
	}
}
