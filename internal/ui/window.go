package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
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
	bannerLabel   *widget.Label
	bannerBox     *fyne.Container
	statusLabel   *widget.Label
	errorTitle    *widget.Label
	errorBody     *widget.Label
	retryButton   *widget.Button

	busy        bool
	ready       bool
	hasSnapshot bool
}

func New(fyneApp fyne.App, callbacks Callbacks) *Window {
	w := &Window{
		callbacks: callbacks,
	}
	w.win = fyneApp.NewWindow("Display Settings")
	w.win.Resize(fyne.NewSize(defaultWindowWidth, defaultWindowHeight))
	w.win.SetPadded(true)

	w.canvas = newArrangementCanvas(func(id display.OutputID) {
		w.selectOutput(id)
	})
	w.inspector = newInspector(func(id display.OutputID) {
		w.selectOutput(id)
	})

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
	footer := container.NewHBox(w.statusLabel, layout.NewSpacer(), widget.NewLabel("Session-only changes"))

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

	root := container.NewStack(w.mainView, w.loadingView, w.unsupportedView)
	w.win.SetContent(root)
	w.ShowLoading()
	return w
}

func (w *Window) Show() {
	w.win.Show()
}

func (w *Window) SetOnClosed(fn func()) {
	w.win.SetCloseIntercept(func() {
		if fn != nil {
			fn()
		}
		w.win.Close()
	})
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

func (w *Window) SetSnapshot(snapshot display.Snapshot) {
	w.hasSnapshot = true
	w.inspector.SetOutputs(snapshot.Outputs)
	w.canvas.SetOutputs(snapshot.Outputs)
	w.canvas.SetSelected(w.inspector.Selected())
	w.ShowMain()
	if len(snapshot.Outputs) == 0 {
		w.ShowBanner("No displays detected")
		w.statusLabel.SetText("No displays detected")
	} else {
		w.ClearBanner()
		w.statusLabel.SetText("Read-only preview")
	}
}

func (w *Window) SetReady(ready bool) {
	w.ready = ready
	w.updateRefresh()
}

func (w *Window) SetBusy(busy bool) {
	w.busy = busy
	w.updateRefresh()
}

func (w *Window) updateRefresh() {
	if w.ready && !w.busy {
		w.refreshButton.Enable()
	} else {
		w.refreshButton.Disable()
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
	w.ready = false
	w.busy = true
	w.updateRefresh()
	w.statusLabel.SetText("Loading displays...")
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
	w.ready = false
	w.busy = false
	w.updateRefresh()
	w.statusLabel.SetText("No supported compositor detected")
	w.errorTitle.SetText("No supported compositor detected")
	w.errorBody.SetText("Midori currently supports niri. Start it inside a niri session.")
	w.retryButton.Show()
	w.mainView.Hide()
	w.loadingView.Hide()
	w.unsupportedView.Show()
}

func (w *Window) ShowLostConnection() {
	w.ready = false
	w.busy = false
	w.updateRefresh()
	w.statusLabel.SetText("Lost connection")
	if !w.hasSnapshot {
		w.errorTitle.SetText("Lost connection to niri. Restart Midori.")
		w.errorBody.SetText("Midori could not read the current display configuration.")
		w.retryButton.Show()
		w.mainView.Hide()
		w.loadingView.Hide()
		w.unsupportedView.Show()
		return
	}
	w.ShowMain()
	w.ShowBanner("Lost connection to niri. Restart Midori.")
}

func (w *Window) selectOutput(id display.OutputID) {
	w.inspector.SetSelected(id)
	w.canvas.SetSelected(id)
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
