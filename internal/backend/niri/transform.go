package niri

import (
	"fmt"

	"github.com/RandomLemon/midori/internal/display"
)

func transformString(transform display.Transform) (string, error) {
	switch transform {
	case display.TransformNormal:
		return "Normal", nil
	case display.Transform90:
		return "90", nil
	case display.Transform180:
		return "180", nil
	case display.Transform270:
		return "270", nil
	case display.TransformFlipped:
		return "Flipped", nil
	case display.TransformFlipped90:
		return "Flipped90", nil
	case display.TransformFlipped180:
		return "Flipped180", nil
	case display.TransformFlipped270:
		return "Flipped270", nil
	default:
		return "", fmt.Errorf("niri: transform %d: %w", transform, display.ErrUnsupported)
	}
}

func parseTransform(value string) (display.Transform, error) {
	switch value {
	case "Normal":
		return display.TransformNormal, nil
	case "90":
		return display.Transform90, nil
	case "180":
		return display.Transform180, nil
	case "270":
		return display.Transform270, nil
	case "Flipped":
		return display.TransformFlipped, nil
	case "Flipped90":
		return display.TransformFlipped90, nil
	case "Flipped180":
		return display.TransformFlipped180, nil
	case "Flipped270":
		return display.TransformFlipped270, nil
	default:
		return 0, fmt.Errorf("niri: transform %q: %w", value, display.ErrUnsupported)
	}
}
