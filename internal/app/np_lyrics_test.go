package app

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"

	"lilt/internal/lyrics"
	"lilt/internal/qqmusic"
)

// Playback can advance without the hidden window receiving a frame. Resuming
// rebuilds both the clock and the active lyric before the next line's timer.
func TestLyricsResumeBeforeNextLineTimer(t *testing.T) {
	frames := os.Getenv("NM_FRAMES")
	if frames != "" {
		if err := os.MkdirAll(frames, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, reduceMotion := range []bool{false, true} {
		t.Run(fmt.Sprintf("reduce-motion=%t", reduceMotion), func(t *testing.T) {
			a := newTestApp()
			a.settings = defaultSettings()
			a.win = &mygo.Window{}
			a.pl.Cue([]qqmusic.Track{{ID: 7, Duration: 400000}})
			var lrc string
			for i := 0; i < 12; i++ {
				lrc += fmt.Sprintf("[%02d:%02d.00]第%d句歌词\n", i/2, i%2*30, i)
			}
			a.pl.SetLyrics(lyrics.Build(lrc, "", ""))
			a.pl.Hold(time.Second)
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Fill().Background(radixSage(true, 1)).Children(func() {
					ui.Text(c, clock(a.pl.Position())).TextColor(white)
					a.lyricsPane(c, a.pl.Current(), 500)
				})
			}, 600, 500)
			tt.SetPreferences(ui.Preferences{ReduceMotion: reduceMotion})
			tt.Advance(time.Second)
			if a.lyricAt != 0 {
				t.Fatalf("initial line=%d", a.lyricAt)
			}
			if frames != "" {
				saveLyricsEntryFrame(t, frames, fmt.Sprintf("lyrics-resume-%t-before.png", reduceMotion), tt)
			}
			// Move only the playback clock, as audio does while minimized.
			a.pl.Hold(190 * time.Second)
			if tt.HasText("3:10") || a.lyricAt != 0 {
				t.Fatal("fixture did not retain the old frame")
			}
			before := tt.Now()
			oldInvalidate := invalidateWindow
			defer func() { invalidateWindow = oldInvalidate }()
			requests := 0
			invalidateWindow = func(w *mygo.Window) {
				if w != a.win {
					t.Fatal("resume invalidated another window")
				}
				requests++
				tt.Frame()
			}
			a.resumeMainWindow()
			if requests != 1 || a.lyricAt != 6 || !tt.HasText("3:10") || tt.Now() != before {
				t.Fatalf("resume waited for a timer: requests=%d line=%d clock=%v", requests, a.lyricAt, tt.Texts())
			}
			tt.Advance(2 * time.Second)
			if a.lyricScroll.Y <= 0 {
				t.Fatal("resumed lyrics did not scroll to the current line")
			}
			if r, ok := tt.Find("第6句歌词"); !ok || r.Y < 0 || r.Y+r.H > 500 {
				t.Fatalf("current line is outside the resumed pane: %+v, found=%v", r, ok)
			}
			if frames != "" {
				saveLyricsEntryFrame(t, frames, fmt.Sprintf("lyrics-resume-%t-after.png", reduceMotion), tt)
			}
		})
	}
}

// Clicking a QRC line seeks to its absolute start and keeps a paused player
// paused while the lyric selection follows both forward and backwards seeks.
func TestQRCLyricsPausedSeek(t *testing.T) {
	a := newTestApp()
	tr := qqmusic.Track{ID: 7, Name: "逐字测试", Duration: 20000}
	a.pl.Cue([]qqmusic.Track{tr})
	lines := lyrics.Build("", "", "[1000,3000]第一句(1000,3000)\n[5000,3000]第二句(5000,3000)\n[9000,4000]第三句(9000,4000)")
	a.pl.SetLyrics(lines)
	a.pl.Seek(2 * time.Second)
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) {
		builds++
		ui.Column(c).Size(600, 500).Children(func() { a.lyricsPane(c, a.pl.Current(), 500) })
	}, 600, 500)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	for _, step := range []struct {
		text string
		at   time.Duration
		line int
	}{{"第三句", 9 * time.Second, 2}, {"第一句", time.Second, 0}} {
		if err := tt.Click(step.text); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if a.pl.Position() != step.at || a.pl.Playing() || a.lyricAt != step.line {
			t.Fatalf("click %q: position=%v playing=%v line=%d", step.text, a.pl.Position(), a.pl.Playing(), a.lyricAt)
		}
		tt.Advance(time.Second)
		before := builds
		tt.Advance(time.Second)
		if a.pl.Position() != step.at || builds != before {
			t.Fatal("paused word highlighting kept advancing or requesting frames")
		}
	}
}

// QRC snapshots show irregular word durations and a pause without asserting
// on text colours or font pixels, which belong to visual inspection.
func TestQRCLyricsFrames(t *testing.T) {
	dir := os.Getenv("NM_FRAMES")
	if dir == "" {
		t.Skip("set NM_FRAMES to save QRC lyric frames")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := newTestApp()
	tr := qqmusic.Track{ID: 7, Name: "逐字测试", Duration: 20000}
	a.pl.Cue([]qqmusic.Track{tr})
	a.pl.SetLyrics(lyrics.Build("", "[00:01.00]A long note, then a pause", "[1000,7000]慢(1000,2000)慢(3000,500) (3500,1500)唱(5000,3000)\n[9000,4000]下一句歌词(9000,4000)"))
	a.pl.Seek(2 * time.Second)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Size(800, 500).Padding(24).Background(radixSage(true, 1)).Children(func() { a.lyricsPane(c, a.pl.Current(), 500) })
	}, 800, 500)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	for _, frame := range []struct {
		name string
		pos  time.Duration
	}{{"qrc-held-note", 2 * time.Second}, {"qrc-pause", 4 * time.Second}, {"qrc-next-word", 6500 * time.Millisecond}} {
		a.pl.Seek(frame.pos)
		tt.Frame()
		tt.Advance(time.Second)
		f, err := os.Create(filepath.Join(dir, frame.name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tt.Image())
		closeErr := f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}
}

// 浏览时换行不会打断阅读，最后一次滚动满三秒后恢复跟随。
func TestLyricsFollowAfterIdle(t *testing.T) {
	frames := os.Getenv("NM_FRAMES")
	if frames != "" {
		if err := os.MkdirAll(frames, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, reduceMotion := range []bool{false, true} {
		for _, captions := range []bool{false, true} {
			t.Run(fmt.Sprintf("reduce-motion=%t/captions=%t", reduceMotion, captions), func(t *testing.T) {
				a := newTestApp()
				a.settings = defaultSettings()
				a.pl.Cue([]qqmusic.Track{{ID: 7, Duration: 400000}})
				if captions {
					a = captionsTestApp(t, true)
					a.settings.Captions = true
					a.captions.program = 42
					a.captions.model = modelState{checked: true, installed: true}
				}
				var lrc string
				for i := 0; i < 24; i++ {
					lrc += fmt.Sprintf("[%02d:%02d.00]第%d句歌词\n", i/2, i%2*30, i)
				}
				a.pl.SetLyrics(lyrics.Build(lrc, "", ""))
				a.captions.lines = lyrics.Build(lrc, "", "")
				a.pl.Hold(time.Second)
				tt := ui.NewTester(func(c *ui.Context) {
					ui.Column(c).Size(600, 500).Background(radixSage(true, 1)).Children(func() {
						if captions {
							a.captionsPane(c, a.pl.Current(), 500)
						} else {
							a.lyricsPane(c, a.pl.Current(), 500)
						}
					})
				}, 600, 500)
				tt.SetPreferences(ui.Preferences{ReduceMotion: reduceMotion})
				tt.Advance(time.Second)
				if _, ok := tt.Find("回到当前"); ok || !a.lyricBrowseUntil.IsZero() {
					t.Fatal("following lyrics offered to follow again")
				}

				tt.Scroll(300, 250, 0, 400)
				tt.Advance(time.Second)
				browsed := a.lyricScroll.Y
				if a.lyricBrowseUntil.IsZero() || browsed <= 0 {
					t.Fatalf("滚动后未暂停跟随：截止时间=%v y=%v", a.lyricBrowseUntil, browsed)
				}
				if _, ok := tt.Find("回到当前"); ok {
					t.Fatal("浏览时仍显示手动返回按钮")
				}
				if frames != "" {
					saveLyricsEntryFrame(t, frames, fmt.Sprintf("lyrics-browse-up-%t.png", reduceMotion), tt)
				}
				// 播放位置变化不会重置浏览计时，也不会立即拉回当前行。
				a.pl.Hold(5 * time.Minute)
				tt.Frame()
				tt.Advance(time.Second)
				if a.lyricScroll.Y != browsed || a.lyricAt == 10 {
					t.Fatalf("browsed lyrics followed the line sung: y=%v (was %v) line=%d", a.lyricScroll.Y, browsed, a.lyricAt)
				}
				if frames != "" {
					saveLyricsEntryFrame(t, frames, fmt.Sprintf("lyrics-browse-down-%t.png", reduceMotion), tt)
				}
				tt.Scroll(300, 250, 0, 100)
				browsed = a.lyricScroll.Y
				tt.Advance(2999 * time.Millisecond)
				if a.lyricBrowseUntil.IsZero() || a.lyricScroll.Y != browsed {
					t.Fatal("再次滚动未重新计时，或未满三秒就恢复跟随")
				}
				tt.Advance(time.Millisecond)
				if !a.lyricBrowseUntil.IsZero() || a.lyricAt != 10 {
					t.Fatalf("三秒后未恢复跟随：截止时间=%v line=%d", a.lyricBrowseUntil, a.lyricAt)
				}
				tt.Advance(time.Second)
				if r, ok := tt.Find("第10句歌词"); !ok || r.Y < 0 || r.Y+r.H > 500 {
					t.Fatalf("line sung is outside the pane: %+v, found=%v", r, ok)
				}
				if _, ok := tt.Find("回到当前"); ok {
					t.Fatal("button stayed after following again")
				}

				// 当前行尚未变化时，播放中超时也必须回到该行。
				tt.Scroll(300, 250, 0, -200)
				tt.Advance(3 * time.Second)
				if !a.lyricBrowseUntil.IsZero() || a.lyricAt != 10 {
					t.Fatal("当前行未变化时未自动恢复跟随")
				}
				tt.Advance(time.Second)
				if r, ok := tt.Find("第10句歌词"); !ok || r.Y < 0 || r.Y+r.H > 500 {
					t.Fatalf("超时后当前行不在可视范围内：%+v, found=%v", r, ok)
				}

				tt.Scroll(300, 250, 0, -200)
				tt.Advance(time.Second)
				if a.lyricBrowseUntil.IsZero() {
					t.Fatal("scrolling back did not leave the line sung")
				}
				if err := tt.Click("第9句歌词"); err != nil {
					t.Fatal(err)
				}
				tt.Advance(2 * time.Second)
				if !a.lyricBrowseUntil.IsZero() || a.lyricAt != 9 || a.pl.Position() != 270*time.Second {
					t.Fatalf("点击歌词：截止时间=%v line=%d position=%v", a.lyricBrowseUntil, a.lyricAt, a.pl.Position())
				}

				out := audio.NewPlayer(&silence{})
				defer out.Close()
				a.pl.Attach(out)
				a.pl.PlaybackFade = func() bool { return false }
				for _, idle := range []time.Duration{time.Second, 10 * time.Second} {
					a.pl.TogglePlay()
					a.pl.Seek(90 * time.Second)
					tt.Frame()
					tt.Advance(time.Second)
					tt.Scroll(300, 250, 0, 400)
					browsed = a.lyricScroll.Y
					tt.Advance(idle)
					tt.Frame()
					if a.pl.Playing() || a.lyricBrowseUntil.IsZero() || a.lyricScroll.Y != browsed {
						t.Fatal("暂停时浏览歌词仍被自动拉回")
					}

					a.pl.Hold(90 * time.Second)
					tt.Frame()
					if !a.lyricBrowseUntil.IsZero() || a.lyricAt != 3 {
						t.Fatalf("暂停浏览 %v 后恢复播放未立即跟随", idle)
					}
					tt.Advance(time.Second)
				}
			})
		}
	}
}
