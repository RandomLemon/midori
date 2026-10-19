package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/RandomLemon/midori/internal/display"
)

func formatMode(mode display.Mode) string {
	return fmt.Sprintf("%d×%d @ %s", mode.Width, mode.Height, formatRefresh(mode))
}

func formatRefresh(mode display.Mode) string {
	value := strconv.FormatFloat(mode.RefreshHz(), 'f', 3, 64)
	value = strings.TrimRight(value, "0")
	value = strings.TrimRight(value, ".")
	return value + " Hz"
}

func formatScale(scale float64) string {
	value := strconv.FormatFloat(scale*100, 'f', 2, 64)
	value = strings.TrimRight(value, "0")
	value = strings.TrimRight(value, ".")
	return value + "%"
}

func formatPosition(position display.Point) string {
	return fmt.Sprintf("%d, %d", position.X, position.Y)
}

func formatPhysicalSize(size *display.Size) string {
	if size == nil {
		return ""
	}
	return fmt.Sprintf("%d×%d mm", size.Width, size.Height)
}

func formatVRR(output display.Output) string {
	if !output.VRRSupported {
		return "not supported"
	}
	if output.VRR {
		return "supported, on"
	}
	return "supported, off"
}
