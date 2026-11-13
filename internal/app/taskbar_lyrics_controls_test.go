package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"
	"lilt/internal/qqmusic"
)

func taskbarControlsFixture() (*app, *taskbarLyrics) {
	a, d := taskbarLyricsViewFixture()
	a.taskbarLyrics = d
	a.settings.setProgressEffect(progressStandard)
	tracks := []qqmusic.Track{
		{ID: 7, Name: "我们一起听音乐", Artists: []qqmusic.Artist{{Name: "Amamiya"}}, Duration: 200000},
		{ID: 8, Name: "下一首", Duration: 200000},
		{ID: 9, Name: "上一首", Duration: 200000},
	}
	for i := range tracks {
		tracks[i].Album.PicURL = "taskbar-controls-cover"
	}
	a.pl.Cue(tracks)
	a.pl.SetLyrics(nil)
	a.pl.Seek(40 * time.Second)
	art := &artwork{bmp: solidBitmap(radixCyan9)}
	for _, size := range []int{96, 120} {
		a.art.art[qqmusic.Sized(tracks[0].Album.PicURL, size)] = art
	}
	return a, d
}

// 连续悬停封面 0.25 秒后展开面板，短暂划过不展开，移出后延迟关闭。
func TestTaskbarLyricsControlsCoverHover(t *testing.T) {
	for _, surface := range []string{"lyrics-host", "cover-island"} {
		t.Run(surface, func(t *testing.T) {
			a, d := taskbarControlsFixture()
			view, width, height := d.view, 280, 42
			if surface == "cover-island" {
				view, width, height = d.coverView, 34, 34
			}
			tt := ui.NewTester(view, width, height)
			tt.Move(-10, -10)
			tt.Frame()
			cover, ok := tt.Find("打开主窗口")
			if !ok {
				t.Fatal("cannot target taskbar cover")
			}
			tt.Move(cover.X+cover.W/2, cover.Y+cover.H/2)
			tt.Frame()
			tt.Advance(200 * time.Millisecond)
			tt.Move(-10, -10)
			tt.Frame()
			tt.Advance(500 * time.Millisecond)
			if d.controlsOpen {
				t.Fatal("短暂划过封面后展开了面板")
			}
			tt.Move(cover.X+cover.W/2, cover.Y+cover.H/2)
			tt.Frame()
			tt.Advance(249 * time.Millisecond)
			if d.controlsOpen {
				t.Fatal("连续悬停不足 0.25 秒就展开了面板")
			}
			tt.Advance(time.Millisecond)
			if !d.controlsOpen {
				t.Fatal("hovering the cover did not open song controls")
			}
			if err := tt.Click("打开主窗口"); err != nil {
				t.Fatal(err)
			}
			if d.controlsOpen || a.nowPlaying {
				t.Fatal("点击封面未收起面板，或意外切换到了歌曲详情")
			}
			tt.Move(-10, -10)
			tt.Frame()
			tt.Move(cover.X+cover.W/2, cover.Y+cover.H/2)
			tt.Frame()
			tt.Advance(taskbarControlsHover)
			if !d.controlsOpen {
				t.Fatal("hovering the cover again did not reopen song controls")
			}
			tt.Move(-10, -10)
			tt.Frame()
			tt.Advance(150 * time.Millisecond)
			if !d.controlsOpen {
				t.Fatal("controls closed before the pointer could cross the gap")
			}
			panel := ui.NewTester(d.controlsView, taskbarControlsWidth, taskbarControlsHeight)
			panel.Move(100, 40)
			panel.Frame()
			panel.Advance(taskbarControlsMotion)
			tt.Advance(350 * time.Millisecond)
			if !d.controlsOpen {
				t.Fatal("controls closed while hovering the panel")
			}
			panel.Move(-10, -10)
			panel.Frame()
			panel.Advance(350 * time.Millisecond)
			if d.controlsOpen {
				t.Fatal("controls stayed open after leaving both surfaces")
			}
			tt.Move(cover.X+cover.W/2, cover.Y+cover.H/2)
			tt.Frame()
			tt.Advance(500 * time.Millisecond)
			panel.Frame()
			panel.Key(0, ui.KeyEscape)
			if d.controlsOpen || a.nowPlaying || a.pl.Playing() || a.pl.Position() != 40*time.Second {
				t.Fatal("Escape did not dismiss locally or cover interaction changed playback")
			}
		})
	}
}

func TestTaskbarLyricsControlsMotion(t *testing.T) {
	if dir := os.Getenv("NM_FRAMES"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, reduced := range []bool{false, true} {
		t.Run(map[bool]string{false: "animated", true: "reduced-motion"}[reduced], func(t *testing.T) {
			_, d := taskbarControlsFixture()
			d.coverHovered = true
			d.openControls()
			tt := ui.NewTesterWith(d.controlsView, taskbarControlsWidth, taskbarControlsHeight,
				ui.TesterOptions{Preferences: ui.Preferences{ReduceMotion: reduced}})
			top := func() int {
				img := tt.Image()
				for y := 0; y < img.Bounds().Dy(); y++ {
					_, _, _, alpha := img.At(taskbarControlsWidth/2, y).RGBA()
					if alpha > 0 {
						return y
					}
				}
				return taskbarControlsHeight
			}
			if !reduced {
				if y := top(); y != taskbarControlsHeight {
					t.Fatalf("展开前面板已可见：top=%d", y)
				}
				tt.Advance(taskbarControlsMotion / 2)
				saveTransitionFrame(t, tt, "taskbar-controls-opening")
				if y := top(); y <= 0 || y >= taskbarControlsHeight {
					t.Fatalf("展开中间帧没有从下方向上进入：top=%d", y)
				}
			}
			tt.Advance(taskbarControlsMotion)
			tt.Frame()
			if y := top(); y != 0 {
				t.Fatalf("展开完成后面板未完整显示：top=%d", y)
			}
			d.dismissControls()
			if !reduced {
				saveTransitionFrame(t, tt, "taskbar-controls-expanded")
			}
			tt.Frame()
			if !reduced {
				tt.Advance(taskbarControlsMotion / 2)
				saveTransitionFrame(t, tt, "taskbar-controls-closing")
				if y := top(); y <= 0 || y >= taskbarControlsHeight || !d.controlsClosing {
					t.Fatalf("收起中间帧没有向下退出：top=%d closing=%t", y, d.controlsClosing)
				}
				d.openControls()
				tt.Frame()
				tt.Advance(taskbarControlsMotion)
				tt.Frame()
				if !d.controlsOpen || d.controlsClosing || top() != 0 {
					t.Fatal("收起过程中重新展开未恢复面板")
				}
				d.dismissControls()
				tt.Frame()
			}
			tt.Advance(taskbarControlsMotion)
			tt.Frame()
			if d.controlsOpen || d.controlsClosing || top() != taskbarControlsHeight {
				t.Fatalf("收起动画完成后仍保留面板：open=%t closing=%t top=%d", d.controlsOpen, d.controlsClosing, top())
			}
		})
	}
	bounds := mygo.Rectangle{X: 20, Y: 600, Width: taskbarControlsWidth, Height: taskbarControlsHeight}
	for _, shown := range []float32{0, 0.5, 1} {
		frame := taskbarControlsFrame(bounds, shown)
		if frame.Y+frame.Height != bounds.Y+bounds.Height || frame.X != bounds.X || frame.Width != bounds.Width {
			t.Fatalf("原生面板动画未固定底边：shown=%f frame=%+v", shown, frame)
		}
	}
}

func TestTaskbarLyricsControlsOpenSongDetails(t *testing.T) {
	a, d := taskbarControlsFixture()
	d.controlsOpen = true
	tt := ui.NewTester(d.controlsView, taskbarControlsWidth, taskbarControlsHeight)
	tt.Frame()
	if err := tt.Click("打开歌曲详情"); err != nil {
		t.Fatal(err)
	}
	if !a.nowPlaying || d.controlsOpen || a.pl.Current().ID != 7 || a.pl.Position() != 40*time.Second {
		t.Fatal("artwork did not open song details and dismiss controls while preserving playback")
	}
}

// Taskbar playback controls pause and resume the same audio player independently of the main window.
func TestTaskbarLyricsControlsPlayback(t *testing.T) {
	a, d := taskbarControlsFixture()
	d.controlsOpen = true
	out := audio.NewPlayer(&silence{})
	out.SetVolume(0)
	a.pl.Attach(out)
	defer out.Close()
	tt := ui.NewTester(d.controlsView, taskbarControlsWidth, taskbarControlsHeight)
	tt.Frame()
	if err := tt.Click("播放/暂停"); err != nil {
		t.Fatal(err)
	}
	if a.pl.Playing() || a.nowPlaying || !d.controlsOpen {
		t.Fatal("pause did not act on the shared player while keeping local controls open")
	}
	if err := tt.Click("播放/暂停"); err != nil {
		t.Fatal(err)
	}
	if !a.pl.Playing() || a.nowPlaying || a.pl.Current().ID != 7 {
		t.Fatal("resume changed the track or failed to resume the shared player")
	}
}

// Previous and next buttons choose the corresponding queue tracks rather than navigating the main app.
func TestTaskbarLyricsControlsQueue(t *testing.T) {
	a, d := taskbarControlsFixture()
	d.controlsOpen = true
	tt := ui.NewTester(d.controlsView, taskbarControlsWidth, taskbarControlsHeight)
	tt.Frame()
	for _, action := range []struct {
		label string
		id    int64
	}{{"下一首", 8}, {"上一首", 7}, {"上一首", 9}} {
		if err := tt.Click(action.label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.pl.Current().ID != action.id || a.nowPlaying || !d.controlsOpen {
			t.Fatalf("%s chose track %d, want %d, or left the local controls", action.label, a.pl.Current().ID, action.id)
		}
	}
}

// Dragging the taskbar progress bar must seek only on release and preserve a paused player.
func TestTaskbarLyricsControlsSeekOnRelease(t *testing.T) {
	a, d := taskbarControlsFixture()
	d.controlsOpen = true
	tt := ui.NewTester(d.controlsView, taskbarControlsWidth, taskbarControlsHeight)
	tt.Frame()
	bar, ok := tt.Find("播放进度")
	if !ok {
		t.Fatal("cannot target taskbar playback progress")
	}
	y := bar.Y + bar.H/2
	at := func(f float32) float32 { return bar.X + bar.W*f }
	tt.Press(at(0.25), y)
	tt.Frame()
	tt.Move(at(1)+30, y+30)
	tt.Frame()
	if a.pl.Position() != 40*time.Second || a.pl.Current().ID != 7 {
		t.Fatal("dragging committed a seek or advanced the queue before release")
	}
	tt.Move(at(0.6), y-20)
	tt.Release(at(0.6), y-20)
	tt.Frame()
	tt.Frame()
	// Slider coordinates are float32, so a 60% pointer position rounds by a few microseconds.
	if position := a.pl.Position(); position < 120*time.Second-time.Millisecond || position > 120*time.Second+time.Millisecond || a.pl.Playing() || a.pl.Current().ID != 7 {
		t.Fatalf("release did not seek the paused current track: position=%s playing=%t id=%d", a.pl.Position(), a.pl.Playing(), a.pl.Current().ID)
	}
}

// Changing song identity during a taskbar drag must discard its old seek value before the pointer is released.
func TestTaskbarLyricsControlsTrackChangeCancelsSeek(t *testing.T) {
	for _, identity := range []string{"id", "mid"} {
		t.Run(identity, func(t *testing.T) {
			a, d := taskbarControlsFixture()
			d.controlsOpen = true
			tt := ui.NewTester(d.controlsView, taskbarControlsWidth, taskbarControlsHeight)
			tt.Frame()
			bar, ok := tt.Find("播放进度")
			if !ok {
				t.Fatal("cannot target taskbar playback progress")
			}
			y := bar.Y + bar.H/2
			tt.Press(bar.X+bar.W*0.2, y)
			tt.Move(bar.X+bar.W*0.8, y)
			tt.Frame()
			if !d.scrub.held || !d.scrub.active {
				t.Fatal("fixture did not start a progress drag")
			}
			track := *a.pl.Current()
			if identity == "id" {
				track.ID++
			} else {
				track.MID = "another-song-mid"
			}
			a.pl.Cue([]qqmusic.Track{track})
			a.pl.Seek(15 * time.Second)
			tt.Frame()
			tt.Release(bar.X+bar.W*0.8, y)
			tt.Frame()
			if d.scrub.held || d.scrub.active || a.pl.Position() != 15*time.Second {
				t.Fatalf("old drag sought into the new song: held=%t active=%t position=%s", d.scrub.held, d.scrub.active, a.pl.Position())
			}
		})
	}
}

// Hiding, destroying or disabling taskbar lyrics must dismiss its controls and discard unfinished scrubbing.
func TestTaskbarLyricsControlsOwnerDismissal(t *testing.T) {
	for _, action := range []string{"hide", "no-space", "cover-close", "destroy", "disable"} {
		t.Run(action, func(t *testing.T) {
			a, d := taskbarControlsFixture()
			d.controlsOpen = true
			d.scrub = scrubState{v: 120, held: true, active: true}
			switch action {
			case "hide":
				d.hide()
			case "no-space":
				d.updatePlacement(taskbarLyricsPlacement{})
			case "cover-close":
				d.destroyCoverWindow()
			case "destroy":
				d.destroy()
			case "disable":
				if !supportsTaskbarLyrics() {
					t.Skip("taskbar settings are Windows-only")
				}
				a.setTaskbarLyricsEnabled(false)
			}
			if d.controlsOpen || d.scrub.held || d.scrub.active || a.pl.Position() != 40*time.Second {
				t.Fatalf("%s left song controls open or committed the discarded seek", action)
			}
		})
	}
}

// Export light and dark taskbar controls, including transparent material content, for visual inspection.
func TestTaskbarLyricsControlsFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES to inspect taskbar song controls")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []struct {
		name     string
		dark     bool
		vibrancy bool
		material string
	}{{"light", false, false, ""}, {"dark", true, false, ""}, {"acrylic-light", false, true, ""}, {"acrylic-dark", true, true, ""}, {"white-light", false, false, taskbarMaterialWhite}, {"white-dark", true, true, taskbarMaterialWhite}} {
		t.Run(mode.name, func(t *testing.T) {
			a, d := taskbarControlsFixture()
			a.settings.TaskbarLyricsMaterial = mode.material
			d.controlsOpen, d.dark = true, mode.dark
			if mode.vibrancy {
				d.dark = !mode.dark // Exercise differing app and taskbar appearances.
			}
			a.pl.Seek(72 * time.Second)
			tt := ui.NewTester(d.controlsView, taskbarControlsWidth, taskbarControlsHeight)
			tt.SetDark(mode.dark)
			tt.SetVibrancy(mode.vibrancy)
			tt.Move(-10, -10)
			tt.Advance(200 * time.Millisecond)
			tt.Frame()
			f, err := os.Create(filepath.Join(dir, "taskbar-controls-"+mode.name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, tt.Image())
			closeErr := f.Close()
			if err != nil || closeErr != nil {
				t.Fatalf("save controls frame: %v / %v", err, closeErr)
			}
		})
	}
}
