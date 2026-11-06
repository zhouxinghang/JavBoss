package subtitle

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

const sampleSRT = "1\n00:00:00,000 --> 00:00:10,000\n测试字幕\n"

func TestStoreUploadReplaceAndRemove(t *testing.T) {
	dir := t.TempDir()
	file, err := Save(dir, 42, "中文 [subtitle].SRT", strings.NewReader(sampleSRT))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir, 42)
	if err != nil || loaded == nil || *loaded != *file {
		t.Fatalf("load = %#v, %v", loaded, err)
	}
	if !filepath.IsAbs(file.Path) {
		t.Fatal("mpv requires an absolute subtitle path")
	}
	if other, err := Load(dir, 43); err != nil || other != nil {
		t.Fatalf("subtitle leaked to other video: %#v, %v", other, err)
	}
	if _, err := Save(dir, 42, "invalid.srt", strings.NewReader("invalid")); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	loaded, _ = Load(dir, 42)
	if loaded.Path != file.Path {
		t.Fatal("invalid upload replaced the valid subtitle")
	}
	replacement, err := Save(dir, 42, "other.vtt", strings.NewReader("WEBVTT\n\n00:00.000 --> 00:10.000\nNew subtitle\n"))
	if err != nil {
		t.Fatal(err)
	}
	loaded, _ = Load(dir, 42)
	if loaded.Path != replacement.Path || loaded.Path == file.Path {
		t.Fatalf("replacement = %#v", loaded)
	}
	if _, err := os.Stat(file.Path); err != nil {
		t.Fatal("replacement removed a subtitle used by a queued playlist entry")
	}
	if err := Remove(dir, 42); err != nil {
		t.Fatal(err)
	}
	loaded, err = Load(dir, 42)
	if err != nil || loaded != nil {
		t.Fatalf("deleted selection = %#v, %v", loaded, err)
	}
}

func TestUploadValidationAndEncodings(t *testing.T) {
	utf16Data := []byte{0xff, 0xfe}
	for _, unit := range utf16.Encode([]rune(sampleSRT)) {
		utf16Data = binary.LittleEndian.AppendUint16(utf16Data, unit)
	}
	for _, tc := range []struct {
		name string
		data []byte
		want error
	}{
		{"valid.srt", []byte(sampleSRT), nil},
		{"bom.srt", append([]byte{0xef, 0xbb, 0xbf}, []byte(sampleSRT)...), nil},
		{"utf16.srt", utf16Data, nil},
		{"valid.ass", []byte("[Script Info]\nScriptType: v4.00+\n[Events]\nDialogue: 0,0:00:00.00,0:00:10.00,Default,,0,0,0,,Hello"), nil},
		{"valid.ssa", []byte("[Events]\nDialogue: Marked=0,0:00:00.00,0:00:10.00,Default,,0,0,0,,Hello"), nil},
		{"valid.vtt", []byte("WEBVTT\n\n00:00.000 --> 00:10.000\nHi\n"), nil},
		{"../escape.srt", []byte(sampleSRT), ErrInvalid},
		{`..\escape.srt`, []byte(sampleSRT), ErrInvalid},
		{"script.lua", []byte(sampleSRT), ErrInvalid},
		{"empty.srt", nil, ErrInvalid},
		{"fake.srt", []byte("not a subtitle"), ErrInvalid},
		{"fake.vtt", []byte(sampleSRT), ErrInvalid},
		{"large.srt", bytes.Repeat([]byte{'a'}, int(MaxSize)+1), ErrTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := Save(t.TempDir(), 1, tc.name, bytes.NewReader(tc.data))
			if !errors.Is(err, tc.want) {
				t.Fatalf("save error = %v, want %v", err, tc.want)
			}
			if tc.want == nil {
				data, err := os.ReadFile(file.Path)
				if err != nil || bytes.IndexByte(data, 0) >= 0 {
					t.Fatalf("saved content = %q, %v", data, err)
				}
			}
		})
	}
}
