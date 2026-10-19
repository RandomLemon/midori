package display

import (
	"context"
	"fmt"
	"time"
)

// OutputID identifies an output by its connector name for one compositor session.
type OutputID string

type Point struct {
	X int
	Y int
}

type Size struct {
	Width  int
	Height int
}

type Mode struct {
	Width          int
	Height         int
	RefreshMilliHz uint32
	Preferred      bool
}

// RefreshHz returns the mode refresh rate in hertz.
func (m Mode) RefreshHz() float64 {
	return float64(m.RefreshMilliHz) / 1000.0
}

type Transform uint8

const (
	TransformNormal Transform = iota
	Transform90
	Transform180
	Transform270
	TransformFlipped
	TransformFlipped90
	TransformFlipped180
	TransformFlipped270
)

// String returns the rotation label shown in the UI.
func (t Transform) String() string {
	switch t {
	case TransformNormal:
		return "Normal"
	case Transform90:
		return "90°"
	case Transform180:
		return "180°"
	case Transform270:
		return "270°"
	case TransformFlipped:
		return "Flipped"
	case TransformFlipped90:
		return "Flipped 90°"
	case TransformFlipped180:
		return "Flipped 180°"
	case TransformFlipped270:
		return "Flipped 270°"
	default:
		return fmt.Sprintf("Transform(%d)", uint8(t))
	}
}

// Rotates reports whether the transform swaps width and height.
func (t Transform) Rotates() bool {
	switch t {
	case Transform90, Transform270, TransformFlipped90, TransformFlipped270:
		return true
	default:
		return false
	}
}

type OutputState struct {
	Enabled   bool
	Mode      Mode
	Scale     float64
	Transform Transform
	Position  Point
	VRR       bool
}

type Output struct {
	ID           OutputID
	Name         string
	Make         string
	Model        string
	Serial       string
	PhysicalSize *Size // millimetres; nil when unknown
	IsCustomMode bool
	VRRSupported bool
	Modes        []Mode
	LogicalSize  Size
	OutputState
}

type Snapshot struct {
	Backend    string
	CapturedAt time.Time
	Outputs    []Output
}

// Output returns the output with the given ID.
func (s Snapshot) Output(id OutputID) (Output, bool) {
	for _, o := range s.Outputs {
		if o.ID == id {
			return o, true
		}
	}
	return Output{}, false
}

type Change struct {
	Before Output
	After  OutputState
}

type Capabilities struct {
	Apply       bool
	VRR         bool
	Persistence bool
}

type Backend interface {
	ID() string
	DisplayName() string
	Version(ctx context.Context) (string, error)
	Capabilities() Capabilities
	Outputs(ctx context.Context) ([]Output, error)
	Apply(ctx context.Context, change Change) error
	Close() error
}
