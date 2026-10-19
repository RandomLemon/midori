package display

import "math"

// Rect is a rectangle in compositor logical pixels.
type Rect struct {
	X      int
	Y      int
	Width  int
	Height int
}

// Intersects reports whether two rectangles overlap. Touching edges do not overlap.
func (r Rect) Intersects(other Rect) bool {
	return r.X < other.X+other.Width &&
		r.X+r.Width > other.X &&
		r.Y < other.Y+other.Height &&
		r.Y+r.Height > other.Y
}

// LogicalSize computes the logical size for a physical mode after scale and transform.
func LogicalSize(mode Mode, scale float64, t Transform) Size {
	if scale <= 0 {
		scale = 1
	}
	w := float64(mode.Width)
	h := float64(mode.Height)
	if t.Rotates() {
		w, h = h, w
	}
	return Size{
		Width:  clampDimension(int(math.Round(w / scale))),
		Height: clampDimension(int(math.Round(h / scale))),
	}
}

// StateRect returns the logical rectangle occupied by an enabled or disabled state.
func StateRect(state OutputState) Rect {
	size := LogicalSize(state.Mode, state.Scale, state.Transform)
	return Rect{
		X:      state.Position.X,
		Y:      state.Position.Y,
		Width:  size.Width,
		Height: size.Height,
	}
}

func clampDimension(v int) int {
	if v < 1 {
		return 1
	}
	return v
}
