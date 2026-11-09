package app

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"lilt/internal/qqmusic"
)

func playerHoverApp() *app {
	a := newTestApp()
	a.settings = defaultSettings()
	a.profile = &qqmusic.Profile{UserID: 1, Nickname: "Amamiya"}
	tr := qqmusic.Track{ID: 7, Name: "红豆", Artists: []qqmusic.Artist{{Name: "王菲"}}, Album: qqmusic.Album{PicURL: "player-hover-cover"}, Duration: 256000}
	a.pl.Cue([]qqmusic.Track{tr})
	a.pl.SetLyrics(nil)
	a.pl.Seek(24 * time.Second)
	a.volume = 0.63
	a.pl.SetVolume(a.volume)
	art := &artwork{bmp: solidBitmap(radixCyan9), tint: radixCyan9}
	for _, size := range []int{96, 120, 240, 600} {
		a.art.art[qqmusic.Sized(tr.Album.PicURL, size)] = art
	}
	return a
}

// 悬停不显示提示文字；跨过间隙和拖动时保持展开，移开 100 毫秒后收起。
func TestPlayerVolumePopoverHover(t *testing.T) {
	a := playerHoverApp()
	tt := ui.NewTester(a.view, 1180, 600)
	tt.Frame()
	button, ok := tt.AccessByRole(ui.RoleButton, "音量")
	if !ok {
		t.Fatal("找不到音量按钮")
	}
	r := button.Bounds
	x, y := r.X+r.W/2, r.Y+r.H/2
	tt.Move(x, y)
	if !a.volumeOpen {
		t.Fatal("悬停音量按钮未开始展开")
	}
	tt.Advance(200 * time.Millisecond)
	slider, ok := tt.AccessByRole(ui.RoleSlider, "音量")
	if !a.volumeOpen || !ok {
		t.Fatal("悬停音量按钮未显示滑块")
	}
	if slider.Focused {
		t.Fatal("悬停展开不应抢占键盘焦点")
	}
	tt.Advance(time.Second)
	if _, shown := tt.AccessByRole(ui.RoleTooltip, "音量"); shown {
		t.Fatal("悬停音量按钮不应显示音量提示文字")
	}
	slider, _ = tt.AccessByRole(ui.RoleSlider, "音量")
	tt.Move(x, r.Y-3)
	tt.Advance(60 * time.Millisecond)
	if !a.volumeOpen {
		t.Fatal("鼠标跨过间隙时浮层提前关闭")
	}
	sr := slider.Bounds
	sx, sy := sr.X+sr.W/2, sr.Y+sr.H/2
	tt.Move(sx, sy)
	tt.Advance(time.Second)
	if !a.volumeOpen {
		t.Fatal("鼠标移入音量条后浮层关闭")
	}
	tt.Press(sx, sy)
	tt.Move(10, 10)
	tt.Advance(time.Second)
	if !a.volumeOpen {
		t.Fatal("拖动滑块离开浮层时不应关闭")
	}
	tt.Release(10, 10)
	tt.Advance(99 * time.Millisecond)
	if !a.volumeOpen {
		t.Fatal("移开不足 100 毫秒时浮层提前收起")
	}
	tt.Advance(2 * time.Millisecond)
	if a.volumeOpen {
		t.Fatal("释放拖动并移开 100 毫秒后浮层未收起")
	}
	tt.Move(x, y)
	tt.Key(0, ui.KeyEscape)
	tt.Advance(time.Second)
	tt.Frame()
	if a.volumeOpen {
		t.Fatal("Escape 关闭后鼠标未移开就重新展开")
	}
	tt.Move(10, 10)
	tt.Move(x, y)
	if !a.volumeOpen {
		t.Fatal("重新悬停未展开音量条")
	}
	tt.Move(10, 10)
	tt.Advance(60 * time.Millisecond)
	tt.Move(x, y)
	tt.Advance(60 * time.Millisecond)
	if !a.volumeOpen {
		t.Fatal("重新悬停未取消待收起状态")
	}
	tt.Move(10, 10)
	tt.Advance(101 * time.Millisecond)
	if a.volumeOpen {
		t.Fatal("鼠标离开按钮和浮层 100 毫秒后未收起")
	}
	if a.pl.Playing() || a.pl.Position() != 24*time.Second || a.nowPlaying {
		t.Fatal("音量交互改变了播放状态或打开了正在播放页面")
	}
}

func TestPlayerVolumePopoverAnimation(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		t.Run(fmt.Sprintf("reduced-%t", reduced), func(t *testing.T) {
			a := playerHoverApp()
			tt := ui.NewTesterWith(a.view, 1180, 600, ui.TesterOptions{
				Preferences: ui.Preferences{ReduceMotion: reduced},
			})
			button, ok := tt.AccessByRole(ui.RoleButton, "音量")
			if !ok {
				t.Fatal("找不到音量按钮")
			}
			x, y := button.Bounds.X+button.Bounds.W/2, button.Bounds.Y+button.Bounds.H/2
			tt.Move(x, y)
			popup := func() ui.Rect {
				t.Helper()
				panel, ok := tt.AccessByRole(ui.RolePopup, "音量")
				if !ok {
					t.Fatal("找不到音量浮层")
				}
				return panel.Bounds
			}
			tt.Advance(time.Millisecond)
			start := popup()
			tt.Advance(29 * time.Millisecond)
			saveTransitionFrame(t, tt, fmt.Sprintf("volume-reduced-%t-opening-early", reduced))
			tt.Advance(60 * time.Millisecond)
			middle := popup()
			saveTransitionFrame(t, tt, fmt.Sprintf("volume-reduced-%t-opening", reduced))
			tt.Advance(120 * time.Millisecond)
			full := popup()
			for _, frame := range []ui.Rect{start, middle} {
				if math.Abs(float64(frame.W-full.W)) > 0.1 {
					t.Fatal("展开过程中浮层宽度发生缩放")
				}
				if math.Abs(float64(frame.H-full.H)) > 0.1 {
					t.Fatal("展开过程中浮层高度发生变化")
				}
				if frame.Y < full.Y-0.1 || frame.Y > full.Y+4.1 {
					t.Fatal("展开位移超过轻微动画的范围")
				}
			}
			if reduced {
				if math.Abs(float64(start.Y-full.Y)) > 0.1 {
					t.Fatal("减少动态效果时浮层仍在移动")
				}
			} else if middle.Y >= (start.Y+full.Y)/2 || middle.Y <= full.Y {
				t.Fatalf("展开未呈现轻微减速位移：起始位置 %v，中间位置 %v，最终位置 %v", start.Y, middle.Y, full.Y)
			}
			capture := func() []byte {
				img := tt.Image()
				scale := float32(img.Bounds().Dx()) / 1180
				crop := image.Rect(int((full.X-8)*scale), int((full.Y-8)*scale),
					int((full.X+full.W+8)*scale), int((full.Y+full.H+8)*scale))
				out := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
				draw.Draw(out, out.Bounds(), img, crop.Min, draw.Src)
				return out.Pix
			}
			tt.Move(10, 10)
			tt.Advance(101 * time.Millisecond)
			if a.volumeOpen {
				t.Fatal("移开 100 毫秒后未开始收起")
			}
			tt.Advance(60 * time.Millisecond)
			closing := capture()
			saveTransitionFrame(t, tt, fmt.Sprintf("volume-reduced-%t-closing", reduced))
			tt.Advance(180 * time.Millisecond)
			if !reduced && bytes.Equal(closing, capture()) {
				t.Fatal("浮层没有播放收起动画就直接消失")
			}
			if _, shown := tt.AccessByRole(ui.RolePopup, "音量"); shown {
				t.Fatal("收起动画结束后浮层未移除")
			}
		})
	}
}

// 点击固定展开后，音量浮层在鼠标移开时保持显示，并可显式关闭。
func TestPlayerVolumePopoverInteraction(t *testing.T) {
	a := playerHoverApp()
	tt := ui.NewTester(a.view, 1180, 600)
	tt.Frame()
	r, ok := tt.Find("音量")
	if !ok {
		t.Fatal("cannot target the volume speaker")
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	tt.ClickAt(x, y)
	if !a.volumeOpen {
		t.Fatal("clicking the speaker did not open its popover")
	}
	openedVolume := a.volume
	tt.Key(0, ui.KeyDown)
	if a.volume >= openedVolume {
		t.Fatal("点击固定展开后，方向键未调低音量")
	}
	tt.Move(10, 10)
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	if !a.volumeOpen {
		t.Fatal("moving away closed the explicitly opened volume popover")
	}
	tt.ClickAt(x, y)
	if a.volumeOpen {
		t.Fatal("clicking the speaker again did not close the popover")
	}
	before := a.volume
	tt.Wheel(x, y, 0, -40)
	if a.volume <= before || a.volumeOpen {
		t.Fatal("scrolling the speaker did not raise the volume locally with the popover closed")
	}
	if a.pl.Playing() || a.pl.Position() != 24*time.Second || a.nowPlaying {
		t.Fatal("using the volume speaker changed playback or opened Now Playing")
	}
	tt.ClickAt(x, y)
	tt.Key(0, ui.KeyEscape)
	if a.volumeOpen {
		t.Fatal("Escape did not dismiss the volume popover")
	}
	tt.ClickAt(x, y)
	tt.ClickAt(10, 400)
	if a.volumeOpen {
		t.Fatal("clicking outside did not dismiss the volume popover")
	}
}

// Dragging and keyboard-adjusting the volume slider changes the volume without seeking or starting playback.
func TestPlayerVolumeSliderInput(t *testing.T) {
	a := playerHoverApp()
	tt := ui.NewTester(func(c *ui.Context) {
		a.pal = paletteFor(c.Theme())
		ui.Column(c).Fill().Center().Children(func() { a.nativeVolumeSlider(c, popoverColors(a.pal.dark)) })
	}, 100, 180)
	tt.Frame()
	r, ok := tt.Find("音量")
	if !ok {
		t.Fatal("cannot target the volume slider")
	}
	x, y := r.X+r.W/2, r.Y+r.H*0.2
	tt.Press(x, r.Y+r.H*0.75)
	tt.Move(x, y)
	tt.Release(x, y)
	tt.Frame()
	if a.volume <= 0.75 || a.volume > 1 {
		t.Fatalf("dragging the slider near its top left volume at %v", a.volume)
	}
	before := a.volume
	tt.Key(0, ui.KeyDown)
	if a.volume >= before {
		t.Fatal("the focused slider no longer accepts keyboard volume changes")
	}
	if a.pl.Playing() || a.pl.Position() != 24*time.Second {
		t.Fatal("adjusting volume changed playback or the current position")
	}
}

// 滚轮每次调节 5%，触控板连续调节，音量始终处于有效范围。
func TestPlayerVolumeScroll(t *testing.T) {
	for _, control := range []string{"speaker", "slider"} {
		t.Run(control, func(t *testing.T) {
			a := playerHoverApp()
			a.volume = 0.5
			a.pl.SetVolume(a.volume)
			tt := ui.NewTester(func(c *ui.Context) {
				a.pal = paletteFor(c.Theme())
				ui.Column(c).Fill().Center().Children(func() {
					if control == "speaker" {
						a.volumeButton(c, a.pal.muted)
					} else {
						a.nativeVolumeSlider(c, popoverColors(a.pal.dark))
					}
				})
			}, 100, 180)
			tt.Frame()
			r, ok := tt.Find("音量")
			if !ok {
				t.Fatal("cannot target the volume control")
			}
			x, y := r.X+r.W/2, r.Y+r.H/2
			for _, step := range []struct {
				dx, dy  float32
				precise bool
				want    float64
			}{
				{dy: 99, want: 0.45}, // Windows' default wheel distance is 3 lines of 33 DIPs.
				{dy: -99, want: 0.5},
				{dy: -50, want: 0.55}, // Other platforms and system scroll settings use different distances.
				{dy: 40, want: 0.5},
				{dy: -165, want: 0.55},
				{dx: 99, want: 0.55},
				{dy: -4, precise: true, want: 0.56},
				{dy: 4, precise: true, want: 0.55},
				{dy: -400, precise: true, want: 1},
				{dy: -99, want: 1},
				{dy: 400, precise: true, want: 0},
				{dy: 99, want: 0},
			} {
				if step.precise {
					tt.Scroll(x, y, step.dx, step.dy)
				} else {
					tt.Wheel(x, y, step.dx, step.dy)
				}
				if math.Abs(a.volume-step.want) > 1e-9 {
					t.Fatalf("scroll (%v, %v), precise=%t: volume=%v, want %v", step.dx, step.dy, step.precise, a.volume, step.want)
				}
			}
			if a.pl.Playing() || a.pl.Position() != 24*time.Second {
				t.Fatal("滚动调节音量改变了播放状态")
			}
		})
	}
}

// With NM_FRAMES set, saves player-bar pointer states in both appearances without loading audio or remote artwork.
func TestPlayerBarHoverFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip()
	}
	for _, width := range []int{880, 1180} {
		for _, dark := range []bool{false, true} {
			for _, scene := range []struct {
				name, label string
				pressed     bool
				opened      bool
			}{
				{name: "normal"},
				{name: "volume", label: "音量"},
				{name: "previous", label: "上一首"},
				{name: "play", label: "播放/暂停"},
				{name: "queue", label: "待播清单"},
				{name: "info", label: "打开正在播放"},
				{name: "like", label: "喜欢"},
				{name: "pressed-play", label: "播放/暂停", pressed: true},
				{name: "opened-volume", label: "音量", opened: true},
			} {
				if width == 880 && scene.name != "normal" && scene.name != "volume" && scene.name != "info" && !scene.opened {
					continue
				}
				a := playerHoverApp()
				tt := ui.NewTester(a.view, width, 600)
				tt.SetDark(dark)
				tt.Frame()
				tt.Move(10, 10)
				tt.Advance(time.Second)
				tt.Frame()
				if scene.label != "" {
					r, ok := tt.Find(scene.label)
					if !ok {
						t.Fatalf("cannot target %s", scene.label)
					}
					x, y := r.X+r.W/2, r.Y+r.H/2
					tt.Move(x, y)
					if scene.pressed {
						tt.Press(x, y)
					}
					if scene.opened {
						tt.ClickAt(x, y)
						tt.Move(10, 10)
					}
					tt.Advance(250 * time.Millisecond)
					tt.Frame()
				}
				img := tt.Image()
				scale := float32(img.Bounds().Dx()) / float32(width)
				height := float32(playerBarHeight + 8)
				if scene.opened || scene.name == "volume" {
					height = 300
				}
				crop := image.Rect(int(float32(sidebarWidth)*scale), int((600-height)*scale), img.Bounds().Dx(), img.Bounds().Dy())
				out := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
				draw.Draw(out, out.Bounds(), img, crop.Min, draw.Src)
				name := fmt.Sprintf("player-bar-%d-dark-%t-%s.png", width, dark, scene.name)
				f, err := os.Create(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, out)
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
