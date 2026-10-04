package manager

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writeTestCoverImage(t *testing.T, path string, width, height int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test image: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode test image: %v", err)
	}
}

func TestStoreVideoCoverFromFileReStoresCover(t *testing.T) {
	dataDir := t.TempDir()
	source := filepath.Join(t.TempDir(), "screenshot.png")
	writeTestCoverImage(t, source, 320, 180)

	if err := StoreVideoCoverFromFile(dataDir, 42, source); err != nil {
		t.Fatalf("StoreVideoCoverFromFile() error = %v", err)
	}

	path, ok := FindVideoCoverPath(dataDir, 42)
	if !ok {
		t.Fatal("FindVideoCoverPath() did not find the re-stored cover")
	}
	if filepath.Ext(path) != ".png" {
		t.Fatalf("cover extension = %q, want .png", filepath.Ext(path))
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cover file missing: %v", err)
	}

	// The cover must not depend on the source screenshot.
	if err := os.Remove(source); err != nil {
		t.Fatalf("remove source: %v", err)
	}
	if _, ok := FindVideoCoverPath(dataDir, 42); !ok {
		t.Fatal("cover disappeared after the source screenshot was removed")
	}

	RemoveVideoCover(dataDir, 42)
	if _, ok := FindVideoCoverPath(dataDir, 42); ok {
		t.Fatal("cover still present after RemoveVideoCover")
	}
}

func TestStoreVideoCoverFromFileReplacesOtherExtensions(t *testing.T) {
	dataDir := t.TempDir()
	dir := VideoCoverDir(dataDir, 7)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir cover dir: %v", err)
	}
	writeTestCoverImage(t, filepath.Join(dir, "cover.jpg"), 200, 200)

	source := filepath.Join(t.TempDir(), "screenshot.png")
	writeTestCoverImage(t, source, 300, 300)
	if err := StoreVideoCoverFromFile(dataDir, 7, source); err != nil {
		t.Fatalf("StoreVideoCoverFromFile() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cover.jpg")); !os.IsNotExist(err) {
		t.Fatalf("stale cover.jpg still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cover.png")); err != nil {
		t.Fatalf("new cover.png missing: %v", err)
	}
}

func TestStoreVideoCoverFromFileRejectsInvalidImage(t *testing.T) {
	dataDir := t.TempDir()
	source := filepath.Join(t.TempDir(), "screenshot.jpg")
	if err := os.WriteFile(source, []byte("not an image"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := StoreVideoCoverFromFile(dataDir, 9, source); err == nil {
		t.Fatal("StoreVideoCoverFromFile() accepted an invalid image")
	}
	if _, ok := FindVideoCoverPath(dataDir, 9); ok {
		t.Fatal("invalid cover was stored")
	}
}
