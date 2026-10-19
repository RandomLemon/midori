package ui

import (
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/RandomLemon/midori/internal/display"
	"github.com/RandomLemon/midori/internal/layout"
)

type arrangementCanvas struct {
	widget.BaseWidget

	content    *fyne.Container
	background *canvas.Rectangle
	empty      *widget.Label
	tiles      map[display.OutputID]*outputTile
	outputs    []display.Output
	selected   display.OutputID
	onSelect   func(display.OutputID)
}

func newArrangementCanvas(onSelect func(display.OutputID)) *arrangementCanvas {
	c := &arrangementCanvas{
		content:  container.NewWithoutLayout(),
		empty:    widget.NewLabel("No displays detected"),
		tiles:    make(map[display.OutputID]*outputTile),
		onSelect: onSelect,
	}
	c.background = canvas.NewRectangle(theme.Color(theme.ColorNameBackground))
	c.empty.Alignment = fyne.TextAlignCenter
	c.content.Add(c.background)
	c.content.Add(c.empty)
	c.ExtendBaseWidget(c)
	return c
}

func (c *arrangementCanvas) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(c.content)
}

func (c *arrangementCanvas) MinSize() fyne.Size {
	return fyne.NewSize(320, 240)
}

func (c *arrangementCanvas) SetOutputs(outputs []display.Output) {
	for _, tile := range c.tiles {
		c.content.Remove(tile)
	}
	c.tiles = make(map[display.OutputID]*outputTile, len(outputs))
	c.outputs = append([]display.Output(nil), outputs...)
	for _, output := range outputs {
		if !output.Enabled {
			continue
		}
		tile := newOutputTile(output)
		id := output.ID
		tile.onTap = func() {
			if c.onSelect != nil {
				c.onSelect(id)
			}
		}
		c.tiles[id] = tile
		c.content.Add(tile)
	}
	// Keep the empty label above the tiles and the background below them.
	c.content.Remove(c.empty)
	c.content.Add(c.empty)
	c.SetSelected(c.selected)
	c.layoutTiles()
}

func (c *arrangementCanvas) SetSelected(id display.OutputID) {
	c.selected = id
	for tileID, tile := range c.tiles {
		tile.SetSelected(tileID == id)
	}
}

func (c *arrangementCanvas) Resize(size fyne.Size) {
	c.BaseWidget.Resize(size)
	c.background.Resize(size)
	c.background.Move(fyne.NewPos(0, 0))
	c.layoutTiles()
}

func (c *arrangementCanvas) layoutTiles() {
	size := c.Size()
	if size.Width <= 0 || size.Height <= 0 {
		return
	}

	bounds, ok := layout.Bounds(c.outputs)
	if !ok {
		for _, tile := range c.tiles {
			tile.Hide()
		}
		c.empty.Show()
		c.empty.Resize(fyne.NewSize(size.Width, c.empty.MinSize().Height))
		c.empty.Move(fyne.NewPos(0, (size.Height-c.empty.MinSize().Height)/2))
		return
	}
	c.empty.Hide()

	zoom := layout.Zoom(int(size.Width), int(size.Height), display.Size{Width: bounds.Width, Height: bounds.Height})
	offsetX := (float64(size.Width)-float64(bounds.Width)*zoom)/2 - float64(bounds.X)*zoom
	offsetY := (float64(size.Height)-float64(bounds.Height)*zoom)/2 - float64(bounds.Y)*zoom

	for _, output := range c.outputs {
		if !output.Enabled {
			continue
		}
		tile, ok := c.tiles[output.ID]
		if !ok {
			continue
		}
		logical := output.LogicalSize
		if logical.Width <= 0 || logical.Height <= 0 {
			logical = display.LogicalSize(output.Mode, output.Scale, output.Transform)
		}
		width := float32(math.Max(1, math.Round(float64(logical.Width)*zoom)))
		height := float32(math.Max(1, math.Round(float64(logical.Height)*zoom)))
		x := float32(math.Round(offsetX + float64(output.Position.X)*zoom))
		y := float32(math.Round(offsetY + float64(output.Position.Y)*zoom))
		tile.Resize(fyne.NewSize(width, height))
		tile.Move(fyne.NewPos(x, y))
		tile.Show()
	}
}

type outputTile struct {
	widget.BaseWidget

	content    *fyne.Container
	background *canvas.Rectangle
	name       *widget.Label
	mode       *widget.Label
	scale      *widget.Label
	rotation   *widget.Label
	output     display.Output
	onTap      func()
}

func newOutputTile(output display.Output) *outputTile {
	t := &outputTile{
		content:    container.NewWithoutLayout(),
		background: canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground)),
		name:       widget.NewLabelWithStyle(output.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		mode:       widget.NewLabel(formatMode(output.Mode)),
		scale:      widget.NewLabel(formatScale(output.Scale)),
		rotation:   widget.NewLabel(output.Transform.String()),
		output:     output,
	}
	t.background.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	t.background.StrokeWidth = 1
	t.content.Add(t.background)
	for _, label := range []*widget.Label{t.name, t.mode, t.scale, t.rotation} {
		label.Truncation = fyne.TextTruncateEllipsis
		t.content.Add(label)
	}
	t.ExtendBaseWidget(t)
	return t
}

func (t *outputTile) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.content)
}

func (t *outputTile) MinSize() fyne.Size {
	return fyne.NewSize(40, 30)
}

func (t *outputTile) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

func (t *outputTile) SetSelected(selected bool) {
	if selected {
		t.background.StrokeColor = theme.Color(theme.ColorNamePrimary)
		t.background.StrokeWidth = 2
	} else {
		t.background.StrokeColor = theme.Color(theme.ColorNameInputBorder)
		t.background.StrokeWidth = 1
	}
	t.background.Refresh()
}

func (t *outputTile) Resize(size fyne.Size) {
	t.BaseWidget.Resize(size)
	t.background.Resize(size)
	t.background.Move(fyne.NewPos(0, 0))
	t.layoutLabels()
}

func (t *outputTile) layoutLabels() {
	size := t.Size()
	padding := float32(8)
	width := float32(math.Max(0, float64(size.Width-2*padding)))
	t.name.Resize(fyne.NewSize(width, t.name.MinSize().Height))
	t.name.Move(fyne.NewPos(padding, padding))
	if size.Width < 100 {
		t.mode.Hide()
		t.scale.Hide()
		t.rotation.Hide()
		return
	}

	y := padding + t.name.MinSize().Height + 2
	labels := []*widget.Label{t.mode, t.scale}
	if t.output.Transform != display.TransformNormal {
		labels = append(labels, t.rotation)
	} else {
		t.rotation.Hide()
	}
	for _, label := range labels {
		if y+label.MinSize().Height > size.Height-padding {
			label.Hide()
			continue
		}
		label.Resize(fyne.NewSize(width, label.MinSize().Height))
		label.Move(fyne.NewPos(padding, y))
		label.Show()
		y += label.MinSize().Height + 2
	}
}
