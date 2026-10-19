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
