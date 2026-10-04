package manager

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// videoCoverFileName is the base name of the re-stored custom cover file.
const videoCoverFileName = "cover"

// VideoCoverDir returns the directory that holds a video's re-stored cover file.
func VideoCoverDir(dataDir string, videoID int64) string {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" || videoID <= 0 {
		return ""
	}
	return filepath.Join(dataDir, "video", strconv.FormatInt(videoID, 10), "cover")
}

// FindVideoCoverPath returns the re-stored custom cover file for a video, if any.
func FindVideoCoverPath(dataDir string, videoID int64) (string, bool) {
	dir := VideoCoverDir(dataDir, videoID)
	if dir == "" {
		return "", false
	}
	for _, ext := range knownExts {
		p := filepath.Join(dir, videoCoverFileName+ext)
		if isValidCoverFile(p) {
			return p, true
		}
	}
	return "", false
}

// StoreVideoCoverFromFile re-stores srcPath as the video's custom cover so the
// cover no longer depends on the source screenshot file.
func StoreVideoCoverFromFile(dataDir string, videoID int64, srcPath string) error {
	dir := VideoCoverDir(dataDir, videoID)
	if dir == "" {
		return fmt.Errorf("invalid video cover target")
	}
	src := strings.TrimSpace(srcPath)
	if src == "" {
		return fmt.Errorf("cover source path is required")
	}
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat cover source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("cover source is not a regular file")
	}

	ext := strings.ToLower(filepath.Ext(src))
	if ext == "" || len(ext) > 5 {
		ext = ".jpg"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ensure cover dir: %w", err)
	}

	target := filepath.Join(dir, videoCoverFileName+ext)
	tmp := target + ".tmp"
	if err := copyFileBytes(src, tmp); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("copy cover: %w", err)
	}
	if !validCoverImageFile(tmp) {
		_ = os.Remove(tmp)
		return errInvalidCover
	}
	removeVideoCoverFiles(dir)
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("finalize cover: %w", err)
	}
	return nil
}

// RemoveVideoCover deletes any re-stored custom cover file for the video.
func RemoveVideoCover(dataDir string, videoID int64) {
	dir := VideoCoverDir(dataDir, videoID)
	if dir == "" {
		return
	}
	removeVideoCoverFiles(dir)
	_ = os.Remove(dir)
}

func removeVideoCoverFiles(dir string) {
	if dir == "" {
		return
	}
	for _, ext := range knownExts {
		_ = os.Remove(filepath.Join(dir, videoCoverFileName+ext))
	}
}

func copyFileBytes(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
