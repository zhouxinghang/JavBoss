// Package subtitle stores one selected upload per video outside the media library.
package subtitle

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

const MaxSize int64 = 10 << 20

var (
	ErrInvalid        = errors.New("invalid subtitle file")
	ErrTooLarge       = errors.New("subtitle exceeds 10 MB")
	storedNamePattern = regexp.MustCompile(`^[a-f0-9]{64}\.(srt|ass|ssa|vtt)$`)
	srtTiming         = regexp.MustCompile(`(?m)^\s*\d+:\d{2}:\d{2}[,.]\d{3}\s*-->\s*\d+:\d{2}:\d{2}[,.]\d{3}`)
	vttTiming         = regexp.MustCompile(`(?m)^\s*(\d+:)?\d{2}:\d{2}\.\d{3}\s*-->\s*(\d+:)?\d{2}:\d{2}\.\d{3}`)
)

type File struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Path string `json:"-"`
}

type record struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	StoredName string `json:"stored_name"`
}

func directory(dataDir string, videoID int64) (string, error) {
	if strings.TrimSpace(dataDir) == "" || videoID <= 0 {
		return "", errors.New("subtitle storage requires a data directory and video ID")
	}
	return filepath.Abs(filepath.Join(dataDir, "video", strconv.FormatInt(videoID, 10), "subtitle"))
}

func Load(dataDir string, videoID int64) (*File, error) {
	dir, err := directory(dataDir, videoID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "current.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read subtitle metadata: %w", err)
	}
	var saved record
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, fmt.Errorf("decode subtitle metadata: %w", err)
	}
	if !storedNamePattern.MatchString(saved.StoredName) {
		return nil, ErrInvalid
	}
	path := filepath.Join(dir, saved.StoredName)
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat uploaded subtitle: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > MaxSize || info.Size() == 0 {
		return nil, ErrInvalid
	}
	return &File{Name: saved.Name, Size: info.Size(), Path: path}, nil
}

func Save(dataDir string, videoID int64, name string, reader io.Reader) (*File, error) {
	// Both separators are rejected regardless of the server's operating system.
	if name == "" || len(name) > 1024 || strings.ContainsAny(name, "/\\\x00\r\n") {
		return nil, ErrInvalid
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".srt" && ext != ".ass" && ext != ".ssa" && ext != ".vtt" {
		return nil, ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(reader, MaxSize+1))
	if err != nil {
		return nil, fmt.Errorf("read subtitle upload: %w", err)
	}
	if int64(len(data)) > MaxSize {
		return nil, ErrTooLarge
	}
	data = normalizeText(data)
	if len(data) == 0 || bytes.IndexByte(data, 0) >= 0 {
		return nil, ErrInvalid
	}
	if int64(len(data)) > MaxSize {
		return nil, ErrTooLarge
	}
	valid := false
	switch ext {
	case ".srt":
		valid = srtTiming.Match(data)
	case ".vtt":
		valid = bytes.HasPrefix(data, []byte("WEBVTT")) && vttTiming.Match(data)
	case ".ass", ".ssa":
		lower := bytes.ToLower(data)
		valid = bytes.Contains(lower, []byte("[events]")) && bytes.Contains(lower, []byte("dialogue:"))
	}
	if !valid {
		return nil, ErrInvalid
	}
	dir, err := directory(dataDir, videoID)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create subtitle directory: %w", err)
	}
	storedName := fmt.Sprintf("%x%s", sha256.Sum256(data), ext)
	path := filepath.Join(dir, storedName)
	if err := writeAtomic(path, data); err != nil {
		return nil, err
	}
	metadata, err := json.Marshal(record{Name: name, Size: int64(len(data)), StoredName: storedName})
	if err != nil {
		return nil, err
	}
	if err := writeAtomic(filepath.Join(dir, "current.json"), metadata); err != nil {
		return nil, err
	}
	// Keep immutable versions for entries already queued in an MPV playlist.
	return &File{Name: name, Size: int64(len(data)), Path: path}, nil
}

func Remove(dataDir string, videoID int64) error {
	dir, err := directory(dataDir, videoID)
	if err != nil {
		return err
	}
	// Existing players may still be using a version; only clear the selection.
	err = os.Remove(filepath.Join(dir, "current.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func normalizeText(data []byte) []byte {
	if len(data) >= 2 && (bytes.HasPrefix(data, []byte{0xff, 0xfe}) || bytes.HasPrefix(data, []byte{0xfe, 0xff})) {
		if len(data)%2 != 0 {
			return nil
		}
		var order binary.ByteOrder = binary.LittleEndian
		if data[0] == 0xfe {
			order = binary.BigEndian
		}
		units := make([]uint16, (len(data)-2)/2)
		for i := range units {
			units[i] = order.Uint16(data[2+i*2:])
		}
		data = []byte(string(utf16.Decode(units)))
	}
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}

func writeAtomic(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".subtitle-*")
	if err != nil {
		return fmt.Errorf("create subtitle file: %w", err)
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write subtitle file: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close subtitle file: %w", closeErr)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace subtitle file: %w", err)
	}
	return nil
}
