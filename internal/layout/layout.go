// Package layout contains pure display-arrangement math. It imports only
// display types and never imports Fyne.
package layout

import (
	"math"

	"github.com/RandomLemon/midori/internal/display"
)

// Bounds returns the bounding rectangle of enabled outputs.
func Bounds(outputs []display.Output) (display.Rect, bool) {
	var bounds display.Rect
	found := false
	for _, output := range outputs {
		if !output.Enabled {
			continue
		}
		rect := display.StateRect(output.OutputState)
		if !found {
			bounds = rect
			found = true
			continue
		}
		minX := min(bounds.X, rect.X)
		minY := min(bounds.Y, rect.Y)
		maxX := max(bounds.X+bounds.Width, rect.X+rect.Width)
		maxY := max(bounds.Y+bounds.Height, rect.Y+rect.Height)
		bounds = display.Rect{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}
	}
	return bounds, found
}

// Zoom returns the auto-fit zoom for the canvas and layout bounds.
func Zoom(canvasWidth, canvasHeight int, bounds display.Size) float64 {
	if bounds.Width <= 0 || bounds.Height <= 0 || canvasWidth <= 0 || canvasHeight <= 0 {
		return 1.0
	}
	x := (float64(canvasWidth) - 64) / float64(bounds.Width)
	y := (float64(canvasHeight) - 64) / float64(bounds.Height)
	return math.Min(1.0, math.Max(0.1, math.Min(x, y)))
}
