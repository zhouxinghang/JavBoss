package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"javboss/internal/mpv"
	"javboss/internal/subtitle"
)

func TestClientDownloadsUploadedSubtitlesForSingleAndPlaylistPlayback(t *testing.T) {
	const content = "1\n00:00:00,000 --> 00:00:10,000\n远端字幕\n"
	var selected atomic.Bool
	selected.Store(true)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "session=secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/videos/42/stream", "/videos/43/stream":
			w.WriteHeader(http.StatusOK)
		case "/videos/42/subtitle":
			var file *subtitle.File
			if selected.Load() {
				file = &subtitle.File{Name: "远端.srt", Size: int64(len(content))}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"subtitle": file})
		case "/videos/42/subtitle/file":
			_, _ = w.Write([]byte(content))
		case "/videos/43/subtitle":
			_ = json.NewEncoder(w).Encode(map[string]any{"subtitle": nil})
		case "/videos/42/play", "/videos/43/play", "/videos/42/screenshots", "/videos/43/screenshots":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer remote.Close()
	var played mpv.PlayOptions
	c := newTestClient(t, remote.URL, func(_ string, options mpv.PlayOptions) error { played = options; return nil })
	var playlist []mpv.PlaylistItem
	c.playPlaylist = func(items []mpv.PlaylistItem) error { playlist = items; return nil }
	play := func(path, body string) {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+path, strings.NewReader(body))
		r.Header.Set("Cookie", "session=secret")
		w := httptest.NewRecorder()
		c.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("play: %d %s", w.Code, w.Body.String())
		}
	}
	play("/videos/play", `{"video_id":42,"location_id":7}`)
	data, err := os.ReadFile(played.SubtitlePath)
	if err != nil || string(data) != content {
		t.Fatalf("local subtitle = %q, %v", data, err)
	}
	if strings.Contains(played.SubtitlePath, "secret") {
		t.Fatal("subtitle path exposed an authentication cookie")
	}
	play("/videos/playlist", `{"items":[{"video_id":42},{"video_id":43}]}`)
	if len(playlist) != 2 || playlist[0].Options.SubtitlePath == "" || playlist[1].Options.SubtitlePath != "" {
		t.Fatalf("playlist subtitles = %#v", playlist)
	}
	selected.Store(false)
	play("/videos/play", `{"video_id":42,"location_id":7}`)
	if played.SubtitlePath != "" {
		t.Fatal("a removed remote subtitle was reused from cache")
	}
}

func TestClientDoesNotStartPlaybackWhenSubtitleDownloadFails(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/videos/42/stream" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "failed", http.StatusInternalServerError)
	}))
	defer remote.Close()
	played := false
	c := newTestClient(t, remote.URL, func(string, mpv.PlayOptions) error { played = true; return nil })
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/videos/play", strings.NewReader(`{"video_id":42,"location_id":7}`))
	r.Header.Set("Cookie", "session=secret")
	w := httptest.NewRecorder()
	c.ServeHTTP(w, r)
	if w.Code != http.StatusBadGateway || played {
		t.Fatalf("status = %d, played = %v", w.Code, played)
	}
}
