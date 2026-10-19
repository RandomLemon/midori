package ui

import (
	"errors"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/RandomLemon/midori/internal/display"
)

const (
	defaultWindowWidth  = 1120
	defaultWindowHeight = 760
	inspectorWidth      = 360
)

type Callbacks struct {
	OnRefresh func()
	OnRetry   func()
	OnEdit    func(display.OutputID, display.OutputState)
	OnApply   func()
	OnReset   func()
	OnKeep    func()
	OnRevert  func()
}

type Window struct {
	win       fyne.Window
	callbacks Callbacks

	canvas    *arrangementCanvas
	inspector *inspector

	mainView        *fyne.Container
	loadingView     *fyne.Container
	unsupportedView *fyne.Container

	backendBadge  *widget.Label
	refreshButton *widget.Button
	resetButton   *widget.Button
	applyButton   *widget.Button
	bannerLabel   *widget.Label
	bannerBox     *fyne.Container
	statusLabel   *widget.Label
	errorTitle    *widget.Label
	errorBody     *widget.Label
	retryButton   *widget.Button

	confirmation *widget.PopUp
	countdown    *widget.Label
	revertButton *widget.Button
	keepButton   *widget.Button
	confirming   bool

	busy        bool
	ready       bool
	dirty       bool
	valid       bool
	editing     bool
	hasSnapshot bool
}

func New(fyneApp fyne.App, callbacks Callbacks) *Window {
	w := &Window{
		callbacks: callbacks,
		valid:     true,
	}
	w.win = fyneApp.NewWindow("Display Settings")
	w.win.Resize(fyne.NewSize(defaultWindowWidth, defaultWindowHeight))
	w.win.SetPadded(true)

	w.canvas = newArrangementCanvas(func(id display.OutputID) {
		w.selectOutput(id)
	})
	w.inspector = newInspector(
		func(id display.OutputID) { w.selectOutput(id) },
		func(id display.OutputID, state display.OutputState) { w.editOutput(id, state) },
		func(valid bool) { w.SetValid(valid) },
	)

	headerTitle := widget.NewLabelWithStyle("Display Settings", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	w.backendBadge = widget.NewLabel("")
	w.backendBadge.Hide()
	w.refreshButton = widget.NewButton("Refresh", func() {
		if w.callbacks.OnRefresh != nil {
			w.callbacks.OnRefresh()
		}
	})
	w.refreshButton.Disable()

	header := container.NewHBox(headerTitle, layout.NewSpacer(), w.backendBadge, w.refreshButton)

	w.bannerLabel = widget.NewLabel("")
	w.bannerLabel.Wrapping = fyne.TextWrapWord
	w.bannerBox = container.NewPadded(w.bannerLabel)
	w.bannerBox.Hide()

	w.statusLabel = widget.NewLabel("Loading displays...")
	w.resetButton = widget.NewButton("Reset", func() {
		if w.callbacks.OnReset != nil {
			w.callbacks.OnReset()
		}
	})
	w.applyButton = widget.NewButton("Apply", func() {
		if w.callbacks.OnApply != nil {
			w.callbacks.OnApply()
		}
	})
	footer := container.NewHBox(w.statusLabel, layout.NewSpacer(), w.resetButton, w.applyButton)

	inspectorScroll := container.NewVScroll(w.inspector.content)
	inspectorWrap := newFixedWidth(inspectorScroll, inspectorWidth)
	canvasArea := container.NewStack(w.canvas)

	w.mainView = container.NewBorder(
		container.NewVBox(header, w.bannerBox),
		footer,
		nil,
		inspectorWrap,
		canvasArea,
	)

	loadingLabel := widget.NewLabel("Loading displays...")
	loadingBar := widget.NewProgressBarInfinite()
	w.loadingView = container.NewCenter(container.NewVBox(loadingLabel, loadingBar))

	w.errorTitle = widget.NewLabelWithStyle("No supported compositor detected", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	w.errorBody = widget.NewLabel("")
	w.errorBody.Alignment = fyne.TextAlignCenter
	w.errorBody.Wrapping = fyne.TextWrapWord
	w.retryButton = widget.NewButton("Retry", func() {
		if w.callbacks.OnRetry != nil {
			w.callbacks.OnRetry()
		}
	})
	w.unsupportedView = container.NewCenter(container.NewVBox(w.errorTitle, w.errorBody, w.retryButton))

	confirmTitle := widget.NewLabelWithStyle("Keep this display configuration?", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	w.countdown = widget.NewLabel("Reverting in 15 s")
	w.countdown.Alignment = fyne.TextAlignCenter
	w.revertButton = widget.NewButton("Revert now", func() {
		if w.callbacks.OnRevert != nil {
			w.callbacks.OnRevert()
		}
	})
	w.keepButton = widget.NewButton("Keep changes", func() {
		if w.callbacks.OnKeep != nil {
			w.callbacks.OnKeep()
		}
	})
	confirmContent := container.NewVBox(
		confirmTitle,
		w.countdown,
		container.NewGridWithColumns(2, w.revertButton, w.keepButton),
	)
	w.confirmation = widget.NewModalPopUp(confirmContent, w.win.Canvas())

	w.win.Canvas().SetOnTypedKey(func(event *fyne.KeyEvent) {
		if event.Name != fyne.KeyEscape || !w.confirming {
			return
		}
		if w.callbacks.OnRevert != nil {
			w.callbacks.OnRevert()
		}
	})

	root := container.NewStack(w.mainView, w.loadingView, w.unsupportedView)
	w.win.SetContent(root)
	w.ShowLoading()
	return w
}

func (w *Window) Show() {
	w.win.Show()
}

func (w *Window) Close() {
	w.win.Close()
}

func (w *Window) SetCloseHandler(fn func()) {
	w.win.SetCloseIntercept(fn)
}

func (w *Window) SetBackend(name, version string) {
	if name == "" {
		w.backendBadge.SetText("")
		w.backendBadge.Hide()
		return
	}
	if version == "" || version == "unknown" {
		w.backendBadge.SetText(fmt.Sprintf("%s unknown", name))
	} else {
		w.backendBadge.SetText(fmt.Sprintf("%s %s", name, version))
	}
	w.backendBadge.Show()
}

func (w *Window) SetCapabilities(caps display.Capabilities) {
	w.inspector.SetVRRCapable(caps.VRR)
}

func (w *Window) SetOutputs(outputs []display.Output) {
	w.hasSnapshot = true
	w.inspector.SetOutputs(outputs)
	w.canvas.SetOutputs(w.inspector.Outputs())
	w.canvas.SetSelected(w.inspector.Selected())
	w.updateButtons()
	w.ShowMain()
}

func (w *Window) SetSelected(id display.OutputID) {
	w.inspector.SetSelected(id)
	w.canvas.SetSelected(id)
}

func (w *Window) SetReady(ready bool) {
	w.ready = ready
	w.updateButtons()
}

func (w *Window) SetBusy(busy bool) {
	w.busy = busy
	w.updateButtons()
}

func (w *Window) SetEditable(editable bool) {
	w.editing = editable
	w.inspector.SetEditable(editable)
	w.updateButtons()
}

func (w *Window) SetDirty(dirty bool) {
	w.dirty = dirty
	w.updateButtons()
}

func (w *Window) SetValid(valid bool) {
	w.valid = valid
	w.updateButtons()
}

func (w *Window) SetValidation(err error) {
	if err == nil {
		w.inspector.SetValidation(nil)
		return
	}
	w.inspector.SetValidation(errors.New(validationCopy(err)))
}

func validationCopy(err error) string {
	var validation *display.ValidationError
	if !errors.As(err, &validation) {
		return err.Error()
	}
	switch validation.Field {
	case "enabled":
		return "At least one display must remain enabled."
	case "mode":
		return "Select an available mode."
	case "scale":
		return "Scale must be between 50% and 300%."
	case "position":
		return "Displays must not overlap."
	case "vrr":
		return "Variable refresh rate is not supported by this display."
	default:
		return err.Error()
	}
}

func (w *Window) SetStatus(text string) {
	w.statusLabel.SetText(text)
}

func (w *Window) updateButtons() {
	if w.ready && !w.busy && !w.dirty {
		w.refreshButton.Enable()
	} else {
		w.refreshButton.Disable()
	}
	canReset := w.editing && w.dirty && !w.busy
	if canReset {
		w.resetButton.Enable()
	} else {
		w.resetButton.Disable()
	}
	canApply := canReset && w.valid
	if canApply {
		w.applyButton.Enable()
	} else {
		w.applyButton.Disable()
	}
}

func (w *Window) ShowBanner(text string) {
	w.bannerLabel.SetText(text)
	w.bannerBox.Show()
}

func (w *Window) ClearBanner() {
	w.bannerLabel.SetText("")
	w.bannerBox.Hide()
}

func (w *Window) ShowLoading() {
	w.HideConfirmation()
	w.ready = false
	w.busy = true
	w.dirty = false
	w.editing = false
	w.updateButtons()
	w.SetStatus("Loading displays...")
	w.mainView.Hide()
	w.unsupportedView.Hide()
	w.loadingView.Show()
}

func (w *Window) ShowMain() {
	w.loadingView.Hide()
	w.unsupportedView.Hide()
	w.mainView.Show()
}

func (w *Window) ShowUnsupported() {
	w.HideConfirmation()
	w.ready = false
	w.busy = false
	w.editing = false
	w.updateButtons()
	w.SetStatus("No supported compositor detected")
	w.showError("No supported compositor detected", "Midori currently supports niri. Start it inside a niri session.", true)
}

func (w *Window) ShowLostConnection() {
	w.HideConfirmation()
	w.ready = false
	w.busy = false
	w.editing = false
	w.updateButtons()
	w.SetStatus("Lost connection")
	if !w.hasSnapshot {
		w.showError("Lost connection to niri. Restart Midori.", "Midori could not read the current display configuration.", false)
		return
	}
	w.ShowMain()
	w.ShowBanner("Lost connection to niri. Restart Midori.")
}

func (w *Window) ShowRecovering() {
	w.HideConfirmation()
	w.ready = false
	w.busy = false
	w.editing = false
	w.updateButtons()
	w.SetStatus("Recovery required")
	w.showError("The display configuration could not be restored.", "Restart Midori or fix the configuration manually.", true)
}

func (w *Window) showError(title, body string, retry bool) {
	w.errorTitle.SetText(title)
	w.errorBody.SetText(body)
	if retry {
		w.retryButton.Show()
	} else {
		w.retryButton.Hide()
	}
	w.mainView.Hide()
	w.loadingView.Hide()
	w.unsupportedView.Show()
}

func (w *Window) ShowConfirmation(seconds int) {
	w.countdown.SetText(fmt.Sprintf("Reverting in %d s", seconds))
	w.revertButton.Enable()
	w.keepButton.Enable()
	w.confirming = true
	w.confirmation.Show()
}

func (w *Window) SetCountdown(seconds int) {
	if !w.confirming {
		return
	}
	w.countdown.SetText(fmt.Sprintf("Reverting in %d s", seconds))
}

func (w *Window) ShowReverting() {
	w.countdown.SetText("Reverting...")
	w.revertButton.Disable()
	w.keepButton.Disable()
}

func (w *Window) HideConfirmation() {
	w.confirming = false
	w.confirmation.Hide()
}

func (w *Window) ShowDiscardDialog(onDiscard func()) {
	confirm := dialog.NewConfirm(
		"Discard unsaved changes?",
		"Your pending display changes will be lost.",
		func(discard bool) {
			if discard && onDiscard != nil {
				onDiscard()
			}
		},
		w.win,
	)
	confirm.SetConfirmText("Discard")
	confirm.SetDismissText("Cancel")
	confirm.Show()
}

func (w *Window) editOutput(id display.OutputID, state display.OutputState) {
	w.canvas.SetOutputs(w.inspector.Outputs())
	w.canvas.SetSelected(w.inspector.Selected())
	if w.callbacks.OnEdit != nil {
		w.callbacks.OnEdit(id, state)
	}
}

func (w *Window) selectOutput(id display.OutputID) {
	w.inspector.SetSelected(id)
	w.canvas.SetSelected(id)
	if w.editing {
		w.inspector.EmitSelected()
	}
}

type fixedWidth struct {
	widget.BaseWidget
	child fyne.CanvasObject
	width float32
}

func newFixedWidth(child fyne.CanvasObject, width float32) *fixedWidth {
	f := &fixedWidth{child: child, width: width}
	f.ExtendBaseWidget(f)
	return f
}

func (f *fixedWidth) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(f.child)
}

func (f *fixedWidth) MinSize() fyne.Size {
	return fyne.NewSize(f.width, f.child.MinSize().Height)
}

func (f *fixedWidth) Resize(size fyne.Size) {
	f.BaseWidget.Resize(size)
	f.child.Resize(fyne.NewSize(f.width, size.Height))
	f.child.Move(fyne.NewPos(0, 0))
}
