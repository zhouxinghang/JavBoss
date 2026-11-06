//go:build !windows

package mpv

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"javboss/internal/subtitle"
)

func TestUploadedSubtitlesWithMPV(t *testing.T) {
	path := os.Getenv("JAVBOSS_TEST_MPV")
	if path == "" {
		t.Skip("set JAVBOSS_TEST_MPV to run real subtitle playback")
	}
	socketDir, err := os.MkdirTemp("", "jb-sub-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	dir := filepath.Join(t.TempDir(), "中文,;: subtitles")
	file, err := subtitle.Save(dir, 42, "上传.srt", strings.NewReader("1\n00:00:00,000 --> 00:00:10,000\n测试字幕\n"))
	if err != nil {
		t.Fatal(err)
	}
	media, err := filepath.Abs("../../web/public/icon-192.png")
	if err != nil {
		t.Fatal(err)
	}
	options := PlayOptions{DataDir: dir, VideoID: 42}
	for _, reuse := range []bool{false, true} {
		name := "startup"
		if reuse {
			name = "ipc"
		}
		t.Run(name, func(t *testing.T) {
			endpoint := filepath.Join(socketDir, name+".sock")
			args := []string{"--no-config", "--vo=null", "--ao=null", "--pause=yes", "--image-display-duration=inf", "--idle=yes", "--input-ipc-server=" + endpoint, "--watch-later-directory=" + filepath.Join(dir, "watch-later", name)}
			if !reuse {
				subArgs, err := buildPlaybackSubtitleArgs(options)
				if err != nil {
					t.Fatal(err)
				}
				args = append(args, subArgs...)
				args = append(args, "--", media)
			}
			cmd := exec.Command(path, args...)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
			if err := waitForIPCReady(endpoint); err != nil {
				t.Fatal(err)
			}
			if reuse {
				command, err := buildLoadFileCommand(media, options)
				if err != nil {
					t.Fatal(err)
				}
				if err := runIPCCommand(endpoint, command); err != nil {
					t.Fatal(err)
				}
			}
			assertTrack := func(wantPath string) {
				t.Helper()
				// mpv's file-loaded event follows the IPC command response.
				waitMPVTestProperty(t, endpoint, "path", media)
				var tracks []struct {
					Type             string `json:"type"`
					ExternalFilename string `json:"external-filename"`
					Selected         bool   `json:"selected"`
				}
				var data []byte
				for attempt := 0; attempt < 100; attempt++ {
					data, err = queryIPCCommand(endpoint, []any{"get_property", "track-list"})
					if err == nil {
						_ = json.Unmarshal(data, &tracks)
					}
					for _, track := range tracks {
						if track.Type == "sub" && track.ExternalFilename == wantPath && track.Selected {
							return
						}
					}
					time.Sleep(20 * time.Millisecond)
				}
				t.Fatalf("uploaded subtitle was not selected: %s, err=%v", data, err)
			}
			assertTrack(file.Path)
			if !reuse {
				return
			}
			// Saved playback settings must not keep a newly uploaded subtitle hidden.
			for _, command := range [][]any{{"set_property", "sub-visibility", false}, {"set_property", "sid", "no"}, {"write-watch-later-config"}, {"stop"}} {
				if err := runIPCCommand(endpoint, command); err != nil {
					t.Fatal(err)
				}
			}
			load, err := buildLoadFileCommand(media, options)
			if err != nil {
				t.Fatal(err)
			}
			if err := runIPCCommand(endpoint, load); err != nil {
				t.Fatal(err)
			}
			assertTrack(file.Path)
			waitMPVTestProperty(t, endpoint, "sub-visibility", true)
			// Append a distinct subtitle to the next entry, then a video without
			// uploads. Neither entry may inherit the first one's subtitle.
			other, err := subtitle.Save(dir, 43, "next.vtt", strings.NewReader("WEBVTT\n\n00:00.000 --> 00:10.000\nNext\n"))
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []int64{43, 44} {
				command, err := buildPlaylistLoadFileCommand(PlaylistItem{Path: media, Options: PlayOptions{DataDir: dir, VideoID: id}}, "append")
				if err != nil {
					t.Fatal(err)
				}
				if err := runIPCCommand(endpoint, command); err != nil {
					t.Fatal(err)
				}
			}
			if err := runIPCCommand(endpoint, []any{"playlist-play-index", 1}); err != nil {
				t.Fatal(err)
			}
			assertTrack(other.Path)
			if err := runIPCCommand(endpoint, []any{"playlist-play-index", 2}); err != nil {
				t.Fatal(err)
			}
			waitMPVTestProperty(t, endpoint, "playlist-pos", float64(2))
			waitMPVTestProperty(t, endpoint, "track-list/count", float64(1))
			data, err := queryIPCCommand(endpoint, []any{"get_property", "track-list"})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), `"type":"sub"`) {
				t.Fatalf("subtitle leaked to another video: %s", data)
			}
		})
	}
}
