package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/RandomLemon/midori/internal/display"
)

type inspector struct {
	content  *fyne.Container
	listBox  *fyne.Container
	header   *widget.Label
	details  *widget.Label
	outputs  []display.Output
	selected display.OutputID
	onSelect func(display.OutputID)
}

func newInspector(onSelect func(display.OutputID)) *inspector {
	i := &inspector{
		listBox:  container.NewVBox(),
		header:   widget.NewLabelWithStyle("No display selected", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		details:  widget.NewLabel(""),
		onSelect: onSelect,
	}
	i.details.Wrapping = fyne.TextWrapWord
	i.content = container.NewVBox(
		widget.NewLabelWithStyle("Outputs", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		i.listBox,
		widget.NewSeparator(),
		i.header,
		i.details,
	)
	return i
}

func (i *inspector) SetOutputs(outputs []display.Output) {
	i.outputs = append([]display.Output(nil), outputs...)
	if i.selected == "" {
		if len(outputs) > 0 {
			i.selected = outputs[0].ID
		}
	} else if _, ok := i.find(i.selected); !ok {
		i.selected = ""
		if len(outputs) > 0 {
			i.selected = outputs[0].ID
		}
	}
	i.rebuildList()
	i.updateDetails()
}

func (i *inspector) SetSelected(id display.OutputID) {
	i.selected = id
	i.rebuildList()
	i.updateDetails()
}

func (i *inspector) Selected() display.OutputID {
	return i.selected
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

func (i *inspector) updateDetails() {
	output, ok := i.find(i.selected)
	if !ok {
		i.header.SetText("No display selected")
		i.details.SetText("")
		return
	}
	i.header.SetText(output.Name)

	var b strings.Builder
	makeModel := strings.TrimSpace(strings.Join([]string{output.Make, output.Model}, " "))
	if makeModel != "" {
		fmt.Fprintf(&b, "Make / model: %s\n", makeModel)
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
		mode := formatMode(output.Mode)
		if output.Mode.Preferred {
			mode += " (preferred)"
		}
		fmt.Fprintf(&b, "Mode: %s\n", mode)
		fmt.Fprintf(&b, "Scale: %s\n", formatScale(output.Scale))
		fmt.Fprintf(&b, "Rotation: %s\n", output.Transform)
		fmt.Fprintf(&b, "Position: %s\n", formatPosition(output.Position))
		fmt.Fprintf(&b, "Logical size: %d×%d\n", output.LogicalSize.Width, output.LogicalSize.Height)
	} else {
		b.WriteString("Mode: —\n")
		b.WriteString("Position: —\n")
	}
	fmt.Fprintf(&b, "Variable refresh rate: %s\n", formatVRR(output))
	b.WriteString("\nRead-only preview")
	i.details.SetText(b.String())
}

func (i *inspector) find(id display.OutputID) (display.Output, bool) {
	for _, output := range i.outputs {
		if output.ID == id {
			return output, true
		}
	}
	return display.Output{}, false
}
