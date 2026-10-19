// Package layout contains pure display-arrangement math. It imports only
// display types and never imports Fyne.
package layout

import (
	"math"
	"sort"

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

// SnapPosition snaps position to the nearest edge of an obstacle when the
// adjustment is within threshold logical pixels. Candidates that would overlap
// an obstacle are rejected. The original position is returned when no valid
// snap exists.
func SnapPosition(position display.Point, size display.Size, obstacles []display.Rect, threshold int) display.Point {
	if threshold <= 0 || size.Width <= 0 || size.Height <= 0 {
		return position
	}
	xDelta, xFound, xAbs := bestSnap(position.X, threshold,
		edgeCandidates(size.Width, obstacles, true)...)
	yDelta, yFound, yAbs := bestSnap(position.Y, threshold,
		edgeCandidates(size.Height, obstacles, false)...)

	if xFound && yFound {
		combined := display.Point{X: position.X + xDelta, Y: position.Y + yDelta}
		if !overlaps(combined, size, obstacles) {
			return combined
		}
	}
	var best display.Point
	bestAbs := threshold + 1
	found := false
	if xFound {
		candidate := display.Point{X: position.X + xDelta, Y: position.Y}
		if !overlaps(candidate, size, obstacles) && xAbs < bestAbs {
			best, bestAbs, found = candidate, xAbs, true
		}
	}
	if yFound {
		candidate := display.Point{X: position.X, Y: position.Y + yDelta}
		if !overlaps(candidate, size, obstacles) && yAbs < bestAbs {
			best, bestAbs, found = candidate, yAbs, true
		}
	}
	if found {
		return best
	}
	return position
}

// AutoArrange places enabled outputs in a single row at y=0, ordered by
// current Y, then X, then connector name. Logical widths accumulate with no
// gap. Disabled outputs keep their state.
func AutoArrange(outputs []display.Output) []display.Output {
	arranged := cloneOutputs(outputs)
	order := make([]int, 0, len(arranged))
	for index, output := range arranged {
		if output.Enabled {
			order = append(order, index)
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		a := arranged[order[i]]
		b := arranged[order[j]]
		if a.Position.Y != b.Position.Y {
			return a.Position.Y < b.Position.Y
		}
		if a.Position.X != b.Position.X {
			return a.Position.X < b.Position.X
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
	x := 0
	for _, index := range order {
		size := display.LogicalSize(arranged[index].Mode, arranged[index].Scale, arranged[index].Transform)
		arranged[index].Position = display.Point{X: x, Y: 0}
		arranged[index].LogicalSize = size
		x += size.Width
	}
	return arranged
}

func edgeCandidates(extent int, obstacles []display.Rect, horizontal bool) []int {
	candidates := make([]int, 0, len(obstacles)*4)
	for _, obstacle := range obstacles {
		if horizontal {
			candidates = append(candidates,
				obstacle.X,
				obstacle.X+obstacle.Width-extent,
				obstacle.X+obstacle.Width,
				obstacle.X-extent,
			)
			continue
		}
		candidates = append(candidates,
			obstacle.Y,
			obstacle.Y+obstacle.Height-extent,
			obstacle.Y+obstacle.Height,
			obstacle.Y-extent,
		)
	}
	return candidates
}

func bestSnap(current, threshold int, candidates ...int) (delta int, found bool, distance int) {
	bestDistance := threshold + 1
	for _, candidate := range candidates {
		adjustment := candidate - current
		if adjustment < 0 {
			adjustment = -adjustment
		}
		if adjustment > threshold || adjustment >= bestDistance {
			continue
		}
		delta = candidate - current
		distance = adjustment
		bestDistance = adjustment
		found = true
	}
	return delta, found, distance
}

func overlaps(position display.Point, size display.Size, obstacles []display.Rect) bool {
	rect := display.Rect{X: position.X, Y: position.Y, Width: size.Width, Height: size.Height}
	for _, obstacle := range obstacles {
		if rect.Intersects(obstacle) {
			return true
		}
	}
	return false
}

func cloneOutputs(outputs []display.Output) []display.Output {
	cloned := make([]display.Output, len(outputs))
	for index, output := range outputs {
		cloned[index] = output
		cloned[index].Modes = append([]display.Mode(nil), output.Modes...)
		if output.PhysicalSize != nil {
			size := *output.PhysicalSize
			cloned[index].PhysicalSize = &size
		}
	}
	return cloned
}
