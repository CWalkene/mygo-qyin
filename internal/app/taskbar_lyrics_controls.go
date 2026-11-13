package app

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	taskbarControlsWidth   = 360
	taskbarControlsHeight  = 124
	taskbarControlsHover   = 250 * time.Millisecond
	taskbarControlsMotion  = 180 * time.Millisecond
	taskbarMaterialWhite   = "white"
	taskbarMaterialAcrylic = "acrylic"
)

func (s settings) taskbarControlsVibrancy() mygo.Vibrancy {
	if s.TaskbarLyricsMaterial == taskbarMaterialWhite {
		return mygo.VibrancyNone
	}
	return mygo.VibrancyAcrylic
}

func (a *app) setTaskbarLyricsMaterial(material string) {
	a.settings.TaskbarLyricsMaterial = material
	a.settings.save()
	if d := a.taskbarLyrics; d != nil && d.controls != nil && !d.controls.IsDestroyed() {
		d.controls.SetVibrancy(a.settings.taskbarControlsVibrancy())
	}
	a.redraw()
}

// Only the cover owns an interactive taskbar rectangle; the lyric host stays
// transparent to input, including clicks sent to Explorer's other threads.
func (d *taskbarLyrics) syncCoverWindow() {
	if d.a.win == nil {
		return
	}
	tr := d.a.pl.Current()
	if !d.visible || !d.a.settings.TaskbarLyrics || d.win == nil || d.win.IsDestroyed() || tr == nil || tr.Album.PicURL == "" {
		d.destroyCoverWindow()
		return
	}
	if d.coverWin == nil || d.coverWin.IsDestroyed() {
		p := taskbarLyricsCoverPlacement(d.placement)
		if p.width <= 0 || p.height <= 0 {
			return
		}
		dpi := max(p.dpi, 96)
		w := mygo.NewWindow(mygo.WindowOptions{
			Title: "任务栏歌曲封面", Panel: true, Hidden: true, Frameless: true,
			Transparent: true, DisableShadow: true, DisableResize: true,
			Width: max(1, p.width*96/int(dpi)), Height: max(1, p.height*96/int(dpi)),
			Content: ui.View(d.coverView),
		})
		d.coverWin, d.coverReady = w, false
		w.SetIgnoreMouseEvents(false)
		styleDesktopLyricsWindow(w)
		if err := placeTaskbarLyricsCover(w, d.placement); err != nil {
			d.reportError(err)
			d.destroyCoverWindow()
			return
		}
		w.OnReadyToShow(func() {
			if d.coverWin == w {
				d.coverReady = true
				d.syncCoverWindow()
			}
		})
		w.OnClosed(func() {
			if d.coverWin == w {
				d.coverWin, d.coverReady = nil, false
				d.closeControls()
			}
		})
		return
	}
	if err := placeTaskbarLyricsCover(d.coverWin, d.placement); err != nil {
		d.reportError(err)
		d.destroyCoverWindow()
		return
	}
	if d.coverReady && !d.coverWin.IsVisible() {
		d.coverWin.Invalidate()
		d.coverWin.ShowInactive()
	}
}

func (d *taskbarLyrics) destroyCoverWindow() {
	d.coverHovered = false
	d.coverEntered = time.Time{}
	d.closeControls()
	w := d.coverWin
	d.coverWin, d.coverReady = nil, false
	if w != nil && !w.IsDestroyed() {
		detachTaskbarLyricsWindow(w)
		w.Destroy()
	}
}

func (d *taskbarLyrics) hoverControls(c *ui.Context, hovered, panel bool) {
	entered := hovered && !d.coverHovered && !panel
	if panel {
		d.controlsHovered = hovered
	} else {
		d.coverHovered = hovered
		if entered {
			d.coverEntered = c.Now()
		} else if !hovered {
			d.coverEntered = time.Time{}
		}
		if d.controlsOpen {
			d.coverEntered = time.Time{}
		}
	}
	if !panel && hovered && !d.coverEntered.IsZero() && !d.controlsOpen {
		if wait := taskbarControlsHover - c.Now().Sub(d.coverEntered); wait > 0 {
			c.After(wait)
		} else {
			d.coverEntered = time.Time{}
			if w := d.coverWin; w != nil {
				w.Update(func() {
					if d.coverWin == w && d.coverHovered && d.coverEntered.IsZero() {
						d.openControls()
					}
				})
			} else {
				d.openControls()
			}
		}
	}
	if !d.controlsOpen {
		return
	}
	if d.coverHovered || d.controlsHovered || d.scrub.held {
		d.controlsLeft = time.Time{}
		return
	}
	if d.controlsLeft.IsZero() {
		d.controlsLeft = c.Now()
	}
	// 留出跨越封面与面板间隙的时间，拖动进度条时保持展开。
	if wait := 300*time.Millisecond - c.Now().Sub(d.controlsLeft); wait > 0 {
		c.After(wait)
		return
	}
	if w := d.controls; w != nil {
		left := d.controlsLeft
		w.Update(func() {
			if d.controls == w && d.controlsLeft == left && !d.coverHovered && !d.controlsHovered && !d.scrub.held {
				d.dismissControls()
			}
		})
	} else {
		d.dismissControls()
	}
}

func (d *taskbarLyrics) openControls() {
	if d.controlsOpen {
		return
	}
	tr := d.a.pl.Current()
	if !d.visible || !d.a.settings.TaskbarLyrics || tr == nil || tr.Album.PicURL == "" || d.a.quitting {
		return
	}
	d.controlsOpen = true
	if d.controlsClosing {
		d.controlsClosing = false
		d.a.redraw()
		return
	}
	d.controlsEntering = true
	if d.controls != nil {
		d.showControls()
		d.a.redraw()
		return
	}
	// The same actions work in Tester without creating native windows.
	if d.coverWin == nil {
		d.a.redraw()
		return
	}
	bounds := d.coverWin.Bounds()
	w := mygo.NewFlyout(mygo.FlyoutOptions{
		Parent: d.coverWin, Anchor: mygo.Rectangle{Width: bounds.Width, Height: bounds.Height},
		Placement: mygo.PlacementTopStart, Gap: 16, Width: taskbarControlsWidth, Height: taskbarControlsHeight,
		Shadow: true, Hidden: true, Vibrancy: mygo.VibrancyAcrylic, Content: ui.View(d.controlsView),
	})
	// Windows can enable a backdrop later only when the window was created
	// with material support. Disable it before showing the opaque white panel.
	w.SetVibrancy(d.a.settings.taskbarControlsVibrancy())
	d.controls = w
	w.OnClosed(func() {
		if d.controls == w {
			d.controls, d.controlsOpen, d.scrub = nil, false, scrubState{}
			d.controlsHovered, d.controlsLeft = false, time.Time{}
			d.controlsEntering, d.controlsClosing = false, false
			d.controlsReady = false
			d.controlsBounds, d.controlsFrame = mygo.Rectangle{}, mygo.Rectangle{}
			d.a.redraw()
		}
	})
	w.OnReadyToShow(func() {
		if d.controls == w {
			d.controlsReady = true
			d.showControls()
		}
	})
	d.a.redraw()
}

func (d *taskbarLyrics) showControls() {
	w := d.controls
	if w == nil || !d.controlsReady || !d.controlsOpen || !d.visible || d.a.quitting || d.coverWin == nil {
		return
	}
	bounds := d.coverWin.Bounds()
	w.SetAnchor(mygo.Rectangle{Width: bounds.Width, Height: bounds.Height})
	w.SetSize(taskbarControlsWidth, taskbarControlsHeight)
	d.controlsBounds = w.Bounds()
	d.controlsFrame = taskbarControlsFrame(d.controlsBounds, 0)
	w.SetBounds(d.controlsFrame)
	w.ShowInactive()
	w.Invalidate()
}

func (d *taskbarLyrics) closeControls() {
	open := d.controlsOpen || d.controlsClosing
	w := d.controls
	d.controls, d.controlsOpen, d.scrub = nil, false, scrubState{}
	d.controlsHovered, d.controlsLeft = false, time.Time{}
	d.controlsEntering, d.controlsClosing = false, false
	d.controlsReady = false
	d.controlsBounds, d.controlsFrame = mygo.Rectangle{}, mygo.Rectangle{}
	if w != nil && !w.IsDestroyed() {
		w.Destroy()
	}
	if open {
		d.a.redraw()
	}
}

func (d *taskbarLyrics) hideControls() {
	d.controlsOpen, d.controlsClosing, d.controlsEntering = false, false, false
	d.controlsHovered, d.controlsLeft, d.scrub = false, time.Time{}, scrubState{}
	// 面板保留到歌词窗口关闭，避免与播放中的歌词帧交错释放合成表面。
	if w := d.controls; w != nil {
		w.Hide()
	}
	if d.visible && d.ready {
		d.present()
	}
	d.a.redraw()
}

func (d *taskbarLyrics) dismissControls() {
	if !d.controlsOpen {
		return
	}
	d.controlsOpen, d.controlsClosing, d.scrub = false, true, scrubState{}
	d.controlsHovered, d.controlsLeft = false, time.Time{}
	d.a.redraw()
}

func taskbarControlsFrame(bounds mygo.Rectangle, shown float32) mygo.Rectangle {
	height := max(1, int(float32(bounds.Height)*shown+0.5))
	bounds.Y += bounds.Height - height
	bounds.Height = height
	return bounds
}

func (d *taskbarLyrics) animateControls(c *ui.Context) float32 {
	root := c.Root()
	initialized := ui.Local(root, "controls-motion-initialized", func() bool { return false })
	if !*initialized {
		root.Jump("controls-reveal", b2f(!d.controlsEntering))
		*initialized = true
	}
	target := b2f(d.controlsOpen)
	if d.controls != nil && d.controlsBounds.Height == 0 {
		target = 0
	}
	motion := ui.Animation{Duration: taskbarControlsMotion, Ease: ui.EaseOut}
	if d.controlsClosing {
		motion.Ease = ui.EaseIn
	}
	shown := root.AnimateTo("controls-reveal", target, motion)
	if shown == 1 {
		d.controlsEntering = false
	}
	if w := d.controls; w != nil {
		if d.controlsBounds.Height == 0 {
			if d.controlsClosing {
				w.Update(func() {
					if d.controls == w && d.controlsClosing && !d.controlsOpen {
						d.hideControls()
					}
				})
			}
			return shown
		}
		frame := taskbarControlsFrame(d.controlsBounds, shown)
		if frame != d.controlsFrame || shown == 0 && d.controlsClosing {
			d.controlsFrame = frame
			// 在构建完成后调整原生窗口，让亚克力背景与内容一同展开或收起。
			w.Update(func() {
				if d.controls != w || d.controlsFrame != frame {
					return
				}
				if shown == 0 && d.controlsClosing && !d.controlsOpen {
					d.hideControls()
				} else {
					w.SetBounds(frame)
				}
			})
		}
	} else if shown == 0 && d.controlsClosing {
		d.hideControls()
	}
	return shown
}

func (d *taskbarLyrics) openSongDetails() {
	if d.a.pl.Current() == nil || d.a.quitting {
		return
	}
	d.dismissControls()
	d.a.nowPlaying = true
	d.a.showMain()
	d.a.redraw()
}

func (d *taskbarLyrics) openMainWindow() {
	if d.a.quitting {
		return
	}
	d.dismissControls()
	d.a.showMain()
}
