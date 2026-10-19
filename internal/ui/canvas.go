package ui

import (
	"math"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/RandomLemon/midori/internal/display"
	"github.com/RandomLemon/midori/internal/layout"
)

const snapScreenPixels = 12

type dragState struct {
	id     display.OutputID
	start  display.Point
	last   display.Point
	deltaX float32
	deltaY float32
}

type arrangementCanvas struct {
	widget.BaseWidget

	content    *fyne.Container
	background *canvas.Rectangle
	empty      *widget.Label
	tiles      map[display.OutputID]*outputTile
	outputs    []display.Output
	selected   display.OutputID
	editable   bool
	drag       *dragState
	zoom       float64
	offsetX    float64
	offsetY    float64

	identify         map[display.OutputID]int
	identifyTimer    *time.Timer
	identifyDuration time.Duration
	identifyGen      uint64

	onSelect   func(display.OutputID)
	onPosition func(display.OutputID, display.Point)
}

func newArrangementCanvas(onSelect func(display.OutputID), onPosition func(display.OutputID, display.Point)) *arrangementCanvas {
	c := &arrangementCanvas{
		content:          container.NewWithoutLayout(),
		empty:            widget.NewLabel("No displays detected"),
		tiles:            make(map[display.OutputID]*outputTile),
		onSelect:         onSelect,
		onPosition:       onPosition,
		zoom:             1,
		identifyDuration: 3 * time.Second,
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
	c.outputs = cloneOutputs(outputs)
	for _, output := range outputs {
		if !output.Enabled {
			continue
		}
		tile := newOutputTile(output)
		id := output.ID
		tile.draggable = c.editable
		tile.onTap = func() {
			if c.onSelect != nil {
				c.onSelect(id)
			}
		}
		tile.onDrag = c.dragTile
		tile.onDragEnd = c.endDrag
		c.tiles[id] = tile
		c.content.Add(tile)
	}
	// Keep the empty label above the tiles and the background below them.
	c.content.Remove(c.empty)
	c.content.Add(c.empty)
	c.SetSelected(c.selected)
	c.applyIdentify()
	c.layoutTiles()
}

func (c *arrangementCanvas) SetEditable(editable bool) {
	c.editable = editable
	for _, tile := range c.tiles {
		tile.draggable = editable
	}
	if !editable {
		c.drag = nil
	}
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

	// Keep the view stable while a tile is under the pointer; refit on drop.
	if c.drag == nil {
		c.zoom = layout.Zoom(int(size.Width), int(size.Height), display.Size{Width: bounds.Width, Height: bounds.Height})
		c.offsetX = (float64(size.Width)-float64(bounds.Width)*c.zoom)/2 - float64(bounds.X)*c.zoom
		c.offsetY = (float64(size.Height)-float64(bounds.Height)*c.zoom)/2 - float64(bounds.Y)*c.zoom
	}
	zoom := c.zoom
	offsetX := c.offsetX
	offsetY := c.offsetY

	for _, output := range c.outputs {
		if !output.Enabled {
			continue
		}
		tile, ok := c.tiles[output.ID]
		if !ok {
			continue
		}
		logical := logicalSize(output)
		width := float32(math.Max(1, math.Round(float64(logical.Width)*zoom)))
		height := float32(math.Max(1, math.Round(float64(logical.Height)*zoom)))
		x := float32(math.Round(offsetX + float64(output.Position.X)*zoom))
		y := float32(math.Round(offsetY + float64(output.Position.Y)*zoom))
		tile.Resize(fyne.NewSize(width, height))
		tile.Move(fyne.NewPos(x, y))
		tile.Show()
	}
}

func (c *arrangementCanvas) dragTile(id display.OutputID, deltaX, deltaY float32) {
	if !c.editable {
		return
	}
	if c.drag == nil || c.drag.id != id {
		if c.selected != id && c.onSelect != nil {
			c.onSelect(id)
		}
		output, ok := c.output(id)
		if !ok || !output.Enabled {
			return
		}
		c.drag = &dragState{id: id, start: output.Position, last: output.Position}
	}
	drag := c.drag
	drag.deltaX += deltaX
	drag.deltaY += deltaY
	if c.zoom <= 0 {
		return
	}

	stepX := int(math.Round(float64(drag.deltaX) / c.zoom))
	stepY := int(math.Round(float64(drag.deltaY) / c.zoom))
	candidate := display.Point{X: drag.start.X + stepX, Y: drag.start.Y + stepY}
	size, ok := c.outputSize(id)
	if !ok {
		return
	}
	threshold := int(math.Ceil(float64(snapScreenPixels) / c.zoom))
	candidate = layout.SnapPosition(candidate, size, c.obstacles(id), threshold)
	if c.overlaps(id, candidate, size) {
		return
	}
	c.setPosition(id, candidate)
	drag.last = candidate
}

func (c *arrangementCanvas) endDrag(id display.OutputID) {
	if c.drag == nil || c.drag.id != id {
		return
	}
	position := c.drag.last
	c.drag = nil
	c.layoutTiles()
	if c.onPosition != nil {
		c.onPosition(id, position)
	}
}

func (c *arrangementCanvas) setPosition(id display.OutputID, position display.Point) {
	for index := range c.outputs {
		if c.outputs[index].ID != id {
			continue
		}
		c.outputs[index].Position = position
		c.layoutTiles()
		return
	}
}

func (c *arrangementCanvas) output(id display.OutputID) (display.Output, bool) {
	for _, output := range c.outputs {
		if output.ID == id {
			return output, true
		}
	}
	return display.Output{}, false
}

func (c *arrangementCanvas) outputSize(id display.OutputID) (display.Size, bool) {
	output, ok := c.output(id)
	if !ok {
		return display.Size{}, false
	}
	return logicalSize(output), true
}

func logicalSize(output display.Output) display.Size {
	if output.LogicalSize.Width > 0 && output.LogicalSize.Height > 0 {
		return output.LogicalSize
	}
	return display.LogicalSize(output.Mode, output.Scale, output.Transform)
}

func (c *arrangementCanvas) obstacles(except display.OutputID) []display.Rect {
	rects := make([]display.Rect, 0, len(c.outputs))
	for _, output := range c.outputs {
		if !output.Enabled || output.ID == except {
			continue
		}
		rects = append(rects, display.StateRect(output.OutputState))
	}
	return rects
}

func (c *arrangementCanvas) overlaps(id display.OutputID, position display.Point, size display.Size) bool {
	rect := display.Rect{X: position.X, Y: position.Y, Width: size.Width, Height: size.Height}
	for _, obstacle := range c.obstacles(id) {
		if rect.Intersects(obstacle) {
			return true
		}
	}
	return false
}

// Identify overlays sequence numbers on enabled tiles for three seconds. It
// performs no backend request.
func (c *arrangementCanvas) Identify() {
	c.identifyGen++
	generation := c.identifyGen
	if c.identifyTimer != nil {
		c.identifyTimer.Stop()
		c.identifyTimer = nil
	}
	c.identify = make(map[display.OutputID]int)
	number := 1
	for _, output := range c.outputs {
		if !output.Enabled {
			continue
		}
		c.identify[output.ID] = number
		number++
	}
	c.applyIdentify()
	if len(c.identify) == 0 {
		c.identify = nil
		return
	}
	c.identifyTimer = time.AfterFunc(c.identifyDuration, func() {
		fyne.Do(func() {
			if c.identifyGen != generation {
				return
			}
			c.clearIdentify()
		})
	})
}

func (c *arrangementCanvas) clearIdentify() {
	c.identifyGen++
	if c.identifyTimer != nil {
		c.identifyTimer.Stop()
		c.identifyTimer = nil
	}
	c.identify = nil
	for _, tile := range c.tiles {
		tile.hideIdentify()
	}
}

func (c *arrangementCanvas) applyIdentify() {
	for id, tile := range c.tiles {
		if number, ok := c.identify[id]; ok {
			tile.showIdentify(number)
		} else {
			tile.hideIdentify()
		}
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
	identify   *canvas.Text
	output     display.Output
	onTap      func()
	onDrag     func(display.OutputID, float32, float32)
	onDragEnd  func(display.OutputID)
	draggable  bool
}

func newOutputTile(output display.Output) *outputTile {
	t := &outputTile{
		content:    container.NewWithoutLayout(),
		background: canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground)),
		name:       widget.NewLabelWithStyle(output.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		mode:       widget.NewLabel(formatMode(output.Mode)),
		scale:      widget.NewLabel(formatScale(output.Scale)),
		rotation:   widget.NewLabel(output.Transform.String()),
		identify:   canvas.NewText("", theme.Color(theme.ColorNamePrimary)),
		output:     output,
	}
	t.background.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	t.background.StrokeWidth = 1
	t.identify.TextSize = theme.TextSize() * 3
	t.identify.TextStyle = fyne.TextStyle{Bold: true}
	t.identify.Alignment = fyne.TextAlignCenter
	t.identify.Hide()
	t.content.Add(t.background)
	for _, label := range []*widget.Label{t.name, t.mode, t.scale, t.rotation} {
		label.Truncation = fyne.TextTruncateEllipsis
		t.content.Add(label)
	}
	t.content.Add(t.identify)
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

func (t *outputTile) Dragged(event *fyne.DragEvent) {
	if !t.draggable || t.onDrag == nil {
		return
	}
	t.onDrag(t.output.ID, event.Dragged.DX, event.Dragged.DY)
}

func (t *outputTile) DragEnd() {
	if !t.draggable || t.onDragEnd == nil {
		return
	}
	t.onDragEnd(t.output.ID)
}

func (t *outputTile) showIdentify(number int) {
	t.identify.Text = strconv.Itoa(number)
	t.identify.Show()
	t.identify.Refresh()
	t.layoutLabels()
}

func (t *outputTile) hideIdentify() {
	t.identify.Hide()
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
		t.layoutIdentify()
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
	t.layoutIdentify()
}

func (t *outputTile) layoutIdentify() {
	if !t.identify.Visible() {
		return
	}
	minimum := t.identify.MinSize()
	t.identify.Resize(minimum)
	t.identify.Move(fyne.NewPos(
		(t.Size().Width-minimum.Width)/2,
		(t.Size().Height-minimum.Height)/2,
	))
}
