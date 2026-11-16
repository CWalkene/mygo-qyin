//go:build !js

package mygo

import (
	"errors"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

type stateCaptureWindow struct {
	platform.Window
	minimized, maximized, fullScreen bool
	bounds                           platform.Rect
}

type stateCaptureStore struct {
	states map[string]WindowState
	saves  int
	err    error
}

func (s *stateCaptureStore) LoadWindowStates() (map[string]WindowState, error) {
	return s.states, nil
}

func (s *stateCaptureStore) SaveWindowStates(states map[string]WindowState) error {
	s.saves++
	if s.err != nil {
		return s.err
	}
	s.states = states
	return nil
}

func TestWindowStateSavesOnMinimize(t *testing.T) {
	previousStates, previousStore := windowStates, App.stateStore
	t.Cleanup(func() { windowStates, App.stateStore = previousStates, previousStore })
	previous := WindowState{
		X: 417, Width: 880, Height: 580, Maximized: true,
		PixelX: 626, PixelWidth: 1320, PixelHeight: 870,
	}
	store := &stateCaptureStore{}
	App.stateStore = store
	windowStates.loaded = true
	windowStates.byKey = map[string]WindowState{"main": previous}
	windowStates.dirty = true
	w := &Window{stateKey: "main", native: stateCaptureWindow{minimized: true}}
	h := &windowHandler{w}
	notified := false
	w.OnMinimize(func() {
		if store.saves != 1 || store.states["main"] != previous {
			t.Fatal("minimize listener ran before the restorable state was saved")
		}
		notified = true
	})
	h.Minimized()
	if !notified || windowStates.dirty {
		t.Fatal("minimized state was not saved")
	}
	h.Minimized()
	if store.saves != 1 {
		t.Fatalf("unchanged state was saved %d times", store.saves)
	}
}

func TestWindowStateMinimizeBeforeBoundsSettle(t *testing.T) {
	previousStates, previousStore := windowStates, App.stateStore
	t.Cleanup(func() { windowStates, App.stateStore = previousStates, previousStore })
	for _, maximized := range []bool{true, false} {
		store := &stateCaptureStore{}
		App.stateStore = store
		windowStates.loaded = true
		windowStates.byKey = map[string]WindowState{
			"main": {Width: 1180, Height: 780, Maximized: !maximized},
		}
		windowStates.dirty = false
		n := &stateCaptureWindow{maximized: maximized}
		w := &Window{stateKey: "main", native: n}
		w.stateChanged()
		n.minimized, n.maximized = true, false
		(&windowHandler{w}).Minimized()
		if store.saves != 1 || store.states["main"].Maximized != maximized {
			t.Fatalf("maximized=%v: saved=%+v, writes=%d", maximized, store.states["main"], store.saves)
		}
		if w.stateTimer != nil {
			t.Fatal("pending capture was not stopped by minimize")
		}
	}
}

func TestWindowStateSaveFailureRetries(t *testing.T) {
	previousStates, previousStore := windowStates, App.stateStore
	t.Cleanup(func() { windowStates, App.stateStore = previousStates, previousStore })
	store := &stateCaptureStore{err: errors.New("save failed")}
	App.stateStore = store
	windowStates.loaded = true
	windowStates.byKey = map[string]WindowState{}
	windowStates.dirty = false
	w := &Window{
		stateKey: "main",
		native: stateCaptureWindow{
			bounds: platform.Rect{X: 263, Y: 119, Width: 1180, Height: 780},
		},
	}
	w.saveState()
	if !windowStates.dirty || store.saves != 1 {
		t.Fatal("failed save was not retained for retry")
	}
	store.err = nil
	w.saveState()
	if windowStates.dirty || store.saves != 2 || store.states["main"].Width != 1180 {
		t.Fatal("state was not saved on retry")
	}
}

func (w stateCaptureWindow) IsMinimized() bool  { return w.minimized }
func (w stateCaptureWindow) IsMaximized() bool  { return w.maximized }
func (w stateCaptureWindow) IsFullScreen() bool { return w.fullScreen }
func (w stateCaptureWindow) Bounds() platform.Rect {
	return w.bounds
}

func TestWindowStateMinimizedKeepsRestorableState(t *testing.T) {
	for _, mode := range []string{"normal", "maximized", "fullscreen"} {
		t.Run(mode, func(t *testing.T) {
			previous := WindowState{
				X: 417, Y: 20, Width: 880, Height: 580,
				PixelX: 626, PixelY: 30, PixelWidth: 1320, PixelHeight: 870,
				Maximized: mode == "maximized", FullScreen: mode == "fullscreen",
			}
			n := stateCaptureWindow{
				minimized: true,
				bounds:    platform.Rect{X: -32000, Y: -32000, Width: 237, Height: 39},
			}
			if got := captureWindowState(previous, n); got != previous {
				t.Fatalf("minimized state = %+v; want %+v", got, previous)
			}
		})
	}
}

func TestWindowStateCaptureAfterRestore(t *testing.T) {
	previous := WindowState{X: 417, Width: 880, Height: 580, Maximized: true}
	bounds := platform.Rect{X: 263, Y: 119, Width: 1180, Height: 780}
	for _, mode := range []string{"normal", "maximized", "fullscreen"} {
		t.Run(mode, func(t *testing.T) {
			n := stateCaptureWindow{
				maximized: mode == "maximized", fullScreen: mode == "fullscreen",
				bounds: bounds,
			}
			want := previous
			want.Maximized, want.FullScreen = n.maximized, n.fullScreen
			if mode == "normal" {
				want.X, want.Y, want.Width, want.Height = bounds.X, bounds.Y, bounds.Width, bounds.Height
			}
			if got := captureWindowState(previous, n); got != want {
				t.Fatalf("restored state = %+v; want %+v", got, want)
			}
		})
	}
}
