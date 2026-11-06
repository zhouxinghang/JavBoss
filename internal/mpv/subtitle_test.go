package mpv

import (
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"javboss/internal/subtitle"
)

func TestUploadedSubtitleOptionsAcrossPlaybackModes(t *testing.T) {
	name := "中文,; subtitles"
	if runtime.GOOS != "windows" {
		name += ":"
	}
	dir := filepath.Join(t.TempDir(), name)
	file, err := subtitle.Save(dir, 42, "movie.srt", strings.NewReader("1\n00:00:00,000 --> 00:00:10,000\nHello\n"))
	if err != nil {
		t.Fatal(err)
	}
	options := PlayOptions{DataDir: dir, VideoID: 42}
	args, err := buildPlaybackSubtitleArgs(options)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(args, []string{"--sub-file=" + file.Path, "--sid=auto", "--sub-visibility=yes"}) {
		t.Fatalf("startup args = %#v", args)
	}
	for _, mode := range []string{"replace", "append"} {
		command, err := buildPlaylistLoadFileCommand(PlaylistItem{Path: "movie.mp4", Options: options}, mode)
		if err != nil {
			t.Fatal(err)
		}
		settings := command[4].(map[string]string)
		if settings["sub-files-append"] != file.Path || settings["sid"] != "auto" || settings["sub-visibility"] != "yes" {
			t.Fatalf("%s options = %#v", mode, settings)
		}
	}
	command, err := buildLoadFileCommand("other.mp4", PlayOptions{DataDir: dir, VideoID: 43})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := command[4].(map[string]string)["sub-files-append"]; exists {
		t.Fatal("subtitle leaked into another video")
	}
	args, err = buildPlaybackSubtitleArgs(PlayOptions{SubtitlePath: file.Path})
	if err != nil || len(args) != 3 || args[0] != "--sub-file="+file.Path {
		t.Fatalf("client args = %v, %v", args, err)
	}
}
