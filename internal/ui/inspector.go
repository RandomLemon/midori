package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/RandomLemon/midori/internal/display"
)

var transformLabels = []string{
	"Normal",
	"90°",
	"180°",
	"270°",
	"Flipped",
	"Flipped 90°",
	"Flipped 180°",
	"Flipped 270°",
}

type inspector struct {
	content *fyne.Container
	listBox *fyne.Container

	header     *widget.Label
	enabled    *widget.Check
	resolution *widget.Select
	refresh    *widget.Select
	scale      *widget.Slider
	scaleLabel *widget.Label
	rotation   *widget.Select
	vrr        *widget.Check
	posX       *widget.Entry
	posY       *widget.Entry
	custom     *widget.Label
	details    *widget.Label
	validation *widget.Label

	outputs         []display.Output
	selected        display.OutputID
	editing         bool
	vrrBackend      bool
	hasSelection    bool
	positionInvalid bool
	updating        bool

	resolutions    []display.Size
	resolutionOpts []string
	refreshModes   []display.Mode
	refreshOpts    []string

	onSelect   func(display.OutputID)
	onEdit     func(display.OutputID, display.OutputState)
	onValidity func(bool)
}

func newInspector(onSelect func(display.OutputID), onEdit func(display.OutputID, display.OutputState), onValidity func(bool)) *inspector {
	i := &inspector{
		listBox:    container.NewVBox(),
		header:     widget.NewLabelWithStyle("No display selected", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		enabled:    widget.NewCheck("Enabled", nil),
		resolution: widget.NewSelect(nil, nil),
		refresh:    widget.NewSelect(nil, nil),
		scale:      widget.NewSlider(0.5, 3.0),
		scaleLabel: widget.NewLabel("100%"),
		rotation:   widget.NewSelect(transformLabels, nil),
		vrr:        widget.NewCheck("Variable refresh rate", nil),
		posX:       widget.NewEntry(),
		posY:       widget.NewEntry(),
		custom:     widget.NewLabel(""),
		details:    widget.NewLabel(""),
		validation: widget.NewLabel(""),
		onSelect:   onSelect,
		onEdit:     onEdit,
		onValidity: onValidity,
	}
	i.details.Wrapping = fyne.TextWrapWord
	i.resolution.PlaceHolder = "Select a resolution"
	i.refresh.PlaceHolder = "Select a refresh rate"
	i.validation.Wrapping = fyne.TextWrapWord
	i.scale.Step = 0.05
	i.enabled.OnChanged = i.enabledChanged
	i.resolution.OnChanged = i.resolutionChanged
	i.refresh.OnChanged = i.refreshChanged
	i.scale.OnChanged = i.scaleChanged
	i.rotation.OnChanged = i.rotationChanged
	i.vrr.OnChanged = i.vrrChanged
	i.posX.OnChanged = i.positionChanged
	i.posY.OnChanged = i.positionChanged

	i.content = container.NewVBox(
		widget.NewLabelWithStyle("Outputs", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		i.listBox,
		widget.NewSeparator(),
		i.header,
		i.enabled,
		widget.NewLabel("Resolution"),
		i.resolution,
		widget.NewLabel("Refresh rate"),
		i.refresh,
		widget.NewLabel("Scale"),
		container.NewBorder(nil, nil, nil, i.scaleLabel, i.scale),
		widget.NewLabel("Rotation"),
		i.rotation,
		i.vrr,
		widget.NewLabel("Position"),
		container.NewGridWithColumns(2,
			container.NewBorder(nil, nil, widget.NewLabel("X"), nil, i.posX),
			container.NewBorder(nil, nil, widget.NewLabel("Y"), nil, i.posY),
		),
		i.custom,
		widget.NewSeparator(),
		i.details,
		i.validation,
	)
	i.applyEditable()
	return i
}

func (i *inspector) SetOutputs(outputs []display.Output) {
	i.outputs = cloneOutputs(outputs)
	if len(i.outputs) == 0 {
		i.selected = ""
	} else if _, ok := i.find(i.selected); !ok {
		i.selected = i.outputs[0].ID
	}
	i.rebuildList()
	i.syncControls()
}

func (i *inspector) Outputs() []display.Output {
	return cloneOutputs(i.outputs)
}

func (i *inspector) SetSelected(id display.OutputID) {
	i.selected = id
	i.rebuildList()
	i.syncControls()
}

func (i *inspector) Selected() display.OutputID {
	return i.selected
}

// SetPosition records a canvas drag result in the draft and refreshes the
// numeric position entries only when the drag ends.
func (i *inspector) SetPosition(id display.OutputID, position display.Point) {
	if !i.editing {
		return
	}
	for index := range i.outputs {
		if i.outputs[index].ID != id {
			continue
		}
		i.outputs[index].Position = position
		if i.outputs[index].Enabled {
			i.outputs[index].LogicalSize = display.LogicalSize(
				i.outputs[index].Mode,
				i.outputs[index].Scale,
				i.outputs[index].Transform,
			)
		}
		if id == i.selected {
			i.positionInvalid = false
			i.updating = true
			i.posX.SetText(strconv.Itoa(position.X))
			i.posY.SetText(strconv.Itoa(position.Y))
			i.updating = false
		}
		if i.onEdit != nil {
			i.onEdit(id, i.outputs[index].OutputState)
		}
		return
	}
}

func (i *inspector) SetEditable(editable bool) {
	i.editing = editable
	i.applyEditable()
}

func (i *inspector) SetVRRCapable(capable bool) {
	if i.vrrBackend == capable {
		return
	}
	i.vrrBackend = capable
	i.syncControls()
}

func (i *inspector) SetValidation(err error) {
	if err == nil {
		i.validation.SetText("")
		return
	}
	i.validation.SetText(err.Error())
}

func (i *inspector) applyEditable() {
	editable := i.editing && i.hasSelection
	controls := []fyne.Disableable{
		i.enabled,
		i.resolution,
		i.refresh,
		i.scale,
		i.rotation,
		i.vrr,
		i.posX,
		i.posY,
	}
	for _, control := range controls {
		if editable {
			control.Enable()
		} else {
			control.Disable()
		}
	}
}

func (i *inspector) rebuildList() {
	i.listBox.RemoveAll()
	if len(i.outputs) == 0 {
		i.listBox.Add(widget.NewLabel("No displays detected"))
		return
	}
	for _, output := range i.outputs {
		id := output.ID
		status := "enabled"
		if !output.Enabled {
			status = "disabled"
		}
		button := widget.NewButton(fmt.Sprintf("%s  ·  %s", output.Name, status), func() {
			if i.onSelect != nil {
				i.onSelect(id)
			}
		})
		if id == i.selected {
			button.Importance = widget.HighImportance
		} else {
			button.Importance = widget.MediumImportance
		}
		i.listBox.Add(button)
	}
}

func (i *inspector) syncControls() {
	output, ok := i.find(i.selected)
	if !ok {
		i.hasSelection = false
		i.updating = true
		i.header.SetText("No display selected")
		i.details.SetText("")
		i.validation.SetText("")
		i.enabled.SetChecked(false)
		i.resolution.ClearSelected()
		i.refresh.ClearSelected()
		i.rotation.ClearSelected()
		i.vrr.SetChecked(false)
		i.posX.SetText("")
		i.posY.SetText("")
		i.custom.Hide()
		i.updating = false
		i.applyEditable()
		return
	}

	i.hasSelection = true
	i.updating = true
	defer func() {
		i.updating = false
		i.applyEditable()
	}()

	i.header.SetText(output.Name)
	i.enabled.SetChecked(output.Enabled)

	i.buildResolutionOptions(output)
	i.buildRefreshOptions(output)

	if output.Scale < 0.5 {
		i.scale.Min = output.Scale
		i.scale.Max = 3.0
	} else if output.Scale > 3.0 {
		i.scale.Min = 0.5
		i.scale.Max = output.Scale
	} else {
		i.scale.Min = 0.5
		i.scale.Max = 3.0
	}
	i.scale.SetValue(output.Scale)
	i.scaleLabel.SetText(formatScale(output.Scale))

	i.rotation.SetSelectedIndex(int(output.Transform))
	if i.vrrBackend && output.VRRSupported {
		i.vrr.SetChecked(output.VRR)
		i.vrr.Show()
	} else {
		i.vrr.SetChecked(false)
		i.vrr.Hide()
	}
	i.posX.SetText(strconv.Itoa(output.Position.X))
	i.posY.SetText(strconv.Itoa(output.Position.Y))

	if output.IsCustomMode {
		i.custom.SetText("Current (custom): " + formatMode(output.Mode))
		i.custom.Show()
	} else {
		i.custom.Hide()
	}

	i.details.SetText(inspectorDetails(output))
}

func inspectorDetails(output display.Output) string {
	var b strings.Builder
	if output.Make != "" || output.Model != "" {
		fmt.Fprintf(&b, "Make / model: %s %s\n", output.Make, output.Model)
	}
	if output.Serial != "" {
		fmt.Fprintf(&b, "Serial: %s\n", output.Serial)
	}
	if physical := formatPhysicalSize(output.PhysicalSize); physical != "" {
		fmt.Fprintf(&b, "Physical size: %s\n", physical)
	}
	status := "Enabled"
	if !output.Enabled {
		status = "Disabled"
	}
	fmt.Fprintf(&b, "Status: %s\n", status)
	if output.Enabled {
		fmt.Fprintf(&b, "Logical size: %d×%d\n", output.LogicalSize.Width, output.LogicalSize.Height)
	} else {
		preview := display.LogicalSize(output.Mode, output.Scale, output.Transform)
		fmt.Fprintf(&b, "Draft size: %d×%d\n", preview.Width, preview.Height)
	}
	fmt.Fprintf(&b, "Variable refresh rate: %s", formatVRR(output))
	return b.String()
}

func (i *inspector) buildResolutionOptions(output display.Output) {
	seen := make(map[display.Size]bool)
	i.resolutions = i.resolutions[:0]
	for _, mode := range output.Modes {
		size := display.Size{Width: mode.Width, Height: mode.Height}
		if seen[size] {
			continue
		}
		seen[size] = true
		i.resolutions = append(i.resolutions, size)
	}
	sort.Slice(i.resolutions, func(a, b int) bool {
		areaA := i.resolutions[a].Width * i.resolutions[a].Height
		areaB := i.resolutions[b].Width * i.resolutions[b].Height
		if areaA != areaB {
			return areaA > areaB
		}
		if i.resolutions[a].Width != i.resolutions[b].Width {
			return i.resolutions[a].Width > i.resolutions[b].Width
		}
		return i.resolutions[a].Height > i.resolutions[b].Height
	})
	i.resolutionOpts = make([]string, 0, len(i.resolutions))
	for _, size := range i.resolutions {
		i.resolutionOpts = append(i.resolutionOpts, fmt.Sprintf("%d×%d", size.Width, size.Height))
	}
	i.resolution.SetOptions(i.resolutionOpts)
	if output.IsCustomMode {
		i.resolution.ClearSelected()
		return
	}
	for index, size := range i.resolutions {
		if size.Width == output.Mode.Width && size.Height == output.Mode.Height {
			i.resolution.SetSelectedIndex(index)
			return
		}
	}
	i.resolution.ClearSelected()
}

func (i *inspector) buildRefreshOptions(output display.Output) {
	var modes []display.Mode
	if size, ok := i.selectedResolution(); ok {
		for _, mode := range output.Modes {
			if mode.Width == size.Width && mode.Height == size.Height {
				modes = append(modes, mode)
			}
		}
	}
	sort.Slice(modes, func(a, b int) bool {
		return modes[a].RefreshMilliHz > modes[b].RefreshMilliHz
	})
	i.refreshModes = modes
	i.refreshOpts = make([]string, 0, len(modes))
	for _, mode := range modes {
		label := formatRefresh(mode)
		if mode.Preferred {
			label += " (Preferred)"
		}
		i.refreshOpts = append(i.refreshOpts, label)
	}
	i.refresh.SetOptions(i.refreshOpts)
	for index, mode := range modes {
		if sameMode(mode, output.Mode) {
			i.refresh.SetSelectedIndex(index)
			return
		}
	}
	if len(modes) > 0 {
		i.refresh.SetSelectedIndex(0)
	} else {
		i.refresh.ClearSelected()
	}
}

func (i *inspector) selectedResolution() (display.Size, bool) {
	if i.resolution.Selected == "" {
		return display.Size{}, false
	}
	for index, label := range i.resolutionOpts {
		if label == i.resolution.Selected && index < len(i.resolutions) {
			return i.resolutions[index], true
		}
	}
	return display.Size{}, false
}

func (i *inspector) enabledChanged(checked bool) {
	if i.updating {
		return
	}
	i.updateSelected(func(output *display.Output) {
		output.Enabled = checked
	})
}

func (i *inspector) resolutionChanged(label string) {
	if i.updating {
		return
	}
	size, ok := i.selectedResolution()
	if !ok {
		return
	}
	output, ok := i.find(i.selected)
	if !ok {
		return
	}
	var chosen *display.Mode
	var best display.Mode
	found := false
	for _, mode := range output.Modes {
		if mode.Width != size.Width || mode.Height != size.Height {
			continue
		}
		if !found || mode.RefreshMilliHz > best.RefreshMilliHz {
			best = mode
			found = true
		}
		if mode.Preferred {
			modeCopy := mode
			chosen = &modeCopy
			break
		}
	}
	if chosen == nil && found {
		chosen = &best
	}
	if chosen == nil {
		return
	}
	i.updateSelected(func(output *display.Output) {
		output.Mode = *chosen
	})
	i.custom.Hide()
	i.updating = true
	i.buildRefreshOptions(*i.outputsForSelected())
	i.updating = false
}

func (i *inspector) refreshChanged(label string) {
	if i.updating {
		return
	}
	for index, option := range i.refreshOpts {
		if option == label && index < len(i.refreshModes) {
			mode := i.refreshModes[index]
			i.updateSelected(func(output *display.Output) {
				output.Mode = mode
			})
			return
		}
	}
}

func (i *inspector) scaleChanged(value float64) {
	if i.updating {
		return
	}
	i.scaleLabel.SetText(formatScale(value))
	i.updateSelected(func(output *display.Output) {
		output.Scale = value
	})
}

func (i *inspector) rotationChanged(label string) {
	if i.updating {
		return
	}
	for index, option := range transformLabels {
		if option == label {
			transform := display.Transform(index)
			i.updateSelected(func(output *display.Output) {
				output.Transform = transform
			})
			return
		}
	}
}

func (i *inspector) vrrChanged(checked bool) {
	if i.updating {
		return
	}
	i.updateSelected(func(output *display.Output) {
		output.VRR = checked
	})
}

func (i *inspector) positionChanged(string) {
	if i.updating {
		return
	}
	x, errX := strconv.Atoi(i.posX.Text)
	y, errY := strconv.Atoi(i.posY.Text)
	if errX != nil || errY != nil {
		i.positionInvalid = true
		i.validation.SetText("Position must be an integer.")
		if i.onValidity != nil {
			i.onValidity(false)
		}
		return
	}
	i.positionInvalid = false
	i.updateSelected(func(output *display.Output) {
		output.Position = display.Point{X: x, Y: y}
	})
}

func (i *inspector) updateSelected(update func(*display.Output)) {
	output, ok := i.find(i.selected)
	if !ok {
		return
	}
	update(&output)
	if output.Enabled {
		output.LogicalSize = display.LogicalSize(output.Mode, output.Scale, output.Transform)
	} else {
		output.LogicalSize = display.Size{}
	}
	for index := range i.outputs {
		if i.outputs[index].ID == output.ID {
			i.outputs[index] = output
			break
		}
	}
	i.rebuildList()
	if i.onEdit != nil {
		i.onEdit(output.ID, output.OutputState)
	}
	if i.positionInvalid {
		i.validation.SetText("Position must be an integer.")
		if i.onValidity != nil {
			i.onValidity(false)
		}
	}
}

func (i *inspector) outputsForSelected() *display.Output {
	if output, ok := i.find(i.selected); ok {
		return &output
	}
	return nil
}

func (i *inspector) find(id display.OutputID) (display.Output, bool) {
	for _, output := range i.outputs {
		if output.ID == id {
			return output, true
		}
	}
	return display.Output{}, false
}

func sameMode(a, b display.Mode) bool {
	return a.Width == b.Width && a.Height == b.Height && a.RefreshMilliHz == b.RefreshMilliHz
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
