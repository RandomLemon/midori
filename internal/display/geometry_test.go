package display

import "testing"

func TestLogicalSize(t *testing.T) {
	tests := []struct {
		name      string
		mode      Mode
		scale     float64
		transform Transform
		want      Size
	}{
		{
			name:  "normal",
			mode:  Mode{Width: 1920, Height: 1200},
			scale: 1,
			want:  Size{Width: 1920, Height: 1200},
		},
		{
			name:  "scaled",
			mode:  Mode{Width: 2560, Height: 1440},
			scale: 1.5,
			want:  Size{Width: 1707, Height: 960},
		},
		{
			name:      "rotated",
			mode:      Mode{Width: 1920, Height: 1200},
			scale:     2,
			transform: Transform90,
			want:      Size{Width: 600, Height: 960},
		},
		{
			name:      "flipped rotated",
			mode:      Mode{Width: 1920, Height: 1200},
			scale:     1,
			transform: TransformFlipped270,
			want:      Size{Width: 1200, Height: 1920},
		},
		{
			name:  "clamped",
			mode:  Mode{Width: 1, Height: 1},
			scale: 3,
			want:  Size{Width: 1, Height: 1},
		},
		{
			name:  "invalid scale",
			mode:  Mode{Width: 10, Height: 10},
			scale: 0,
			want:  Size{Width: 10, Height: 10},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LogicalSize(tt.mode, tt.scale, tt.transform); got != tt.want {
				t.Fatalf("LogicalSize() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRectIntersects(t *testing.T) {
	base := Rect{X: 0, Y: 0, Width: 100, Height: 100}
	tests := []struct {
		name  string
		other Rect
		want  bool
	}{
		{name: "overlap", other: Rect{X: 50, Y: 50, Width: 100, Height: 100}, want: true},
		{name: "touching right", other: Rect{X: 100, Y: 0, Width: 100, Height: 100}, want: false},
		{name: "touching bottom", other: Rect{X: 0, Y: 100, Width: 100, Height: 100}, want: false},
		{name: "disjoint", other: Rect{X: 200, Y: 200, Width: 10, Height: 10}, want: false},
		{name: "contained", other: Rect{X: 10, Y: 10, Width: 10, Height: 10}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := base.Intersects(tt.other); got != tt.want {
				t.Fatalf("Intersects() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransformLabels(t *testing.T) {
	tests := map[Transform]string{
		TransformNormal:     "Normal",
		Transform90:         "90°",
		TransformFlipped90:  "Flipped 90°",
		TransformFlipped180: "Flipped 180°",
		TransformFlipped270: "Flipped 270°",
		Transform(200):      "Transform(200)",
	}
	for transform, want := range tests {
		if got := transform.String(); got != want {
			t.Errorf("Transform(%d).String() = %q, want %q", transform, got, want)
		}
	}
}
