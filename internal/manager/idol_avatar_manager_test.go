package manager

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"javboss/internal/jav/gfriends"
)

func testAvatarJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for i := range img.Pix {
		img.Pix[i] = byte(i * 7)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	if int64(buf.Len()) < minValidIdolAvatarSizeBytes {
		t.Fatalf("fixture jpeg too small: %d bytes", buf.Len())
	}
	return buf.Bytes()
}

func newTestAvatarManager(t *testing.T) (*IdolAvatarManager, *httptest.Server, []byte) {
	t.Helper()
	imageBytes := testAvatarJPEG(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/Filetree.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Content":{"1":{"三上悠亜.jpg":"abc.jpg"}}}`))
	})
	mux.HandleFunc("/Content/1/abc.jpg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(imageBytes)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	dir := t.TempDir()
	client := gfriends.New(
		gfriends.WithRepositoryURL(server.URL),
		gfriends.WithHTTPClient(server.Client()),
		gfriends.WithCachePath(filepath.Join(dir, "Filetree.json")),
	)
	return NewIdolAvatarManager(dir, client), server, imageBytes
}

func TestIdolAvatarManagerCandidatesAndEnsure(t *testing.T) {
	manager, server, _ := newTestAvatarManager(t)
	ctx := context.Background()

	candidates := manager.Candidates(ctx, "三上悠亜")
	if len(candidates) != 1 || candidates[0].Source != "1" {
		t.Fatalf("candidates = %#v", candidates)
	}
	key := CandidateKey(candidates[0].URL)
	if key == "" {
		t.Fatal("candidate key must not be empty")
	}

	avatarPath, err := manager.Ensure(ctx, 42, candidates[0].URL)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if filepath.Dir(avatarPath) != manager.Dir() {
		t.Fatalf("avatar path %q is not inside %q", avatarPath, manager.Dir())
	}
	if filepath.Base(avatarPath) != "42-"+key+".jpg" {
		t.Fatalf("avatar file name = %q", filepath.Base(avatarPath))
	}
	if _, ok := manager.LocalPath(42, candidates[0].URL); !ok {
		t.Fatal("LocalPath did not report the downloaded avatar")
	}
	if !manager.Exists(42) {
		t.Fatal("Exists should report the downloaded avatar")
	}

	manager.Remove(42)
	if manager.Exists(42) {
		t.Fatal("Remove should delete the cached avatars")
	}

	// EnsureAuto picks and downloads the first candidate.
	autoPath, autoURL, err := manager.EnsureAuto(ctx, 43, []string{"三上悠亜"})
	if err != nil {
		t.Fatalf("EnsureAuto: %v", err)
	}
	if !strings.HasPrefix(autoURL, server.URL+"/Content/1/") {
		t.Fatalf("auto url = %q", autoURL)
	}
	if _, err := os.Stat(autoPath); err != nil {
		t.Fatalf("auto avatar missing: %v", err)
	}
}

func TestIdolAvatarManagerEnsureAutoReturnsNotFoundForUnknownName(t *testing.T) {
	manager, _, _ := newTestAvatarManager(t)
	if _, _, err := manager.EnsureAuto(context.Background(), 7, []string{"不存在"}); err == nil {
		t.Fatal("EnsureAuto should fail for an unknown idol name")
	}
}

func TestIdolAvatarManagerFindCandidateUsesKey(t *testing.T) {
	manager, _, _ := newTestAvatarManager(t)
	ctx := context.Background()

	candidates := manager.Candidates(ctx, "三上悠亜")
	if len(candidates) == 0 {
		t.Fatal("expected at least one candidate")
	}
	key := CandidateKey(candidates[0].URL)
	image, ok := manager.FindCandidate(ctx, key, "三上悠亜")
	if !ok || image.URL != candidates[0].URL {
		t.Fatalf("FindCandidate = %#v ok=%v", image, ok)
	}
	if _, ok := manager.FindCandidate(ctx, "deadbeef", "三上悠亜"); ok {
		t.Fatal("FindCandidate should reject an unknown key")
	}
}
