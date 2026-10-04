package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJavJSONUsesEmptySampleImageArray(t *testing.T) {
	data, err := json.Marshal(Jav{Code: "ABC-001"})
	if err != nil {
		t.Fatalf("marshal jav: %v", err)
	}
	if !strings.Contains(string(data), `"sample_images":[]`) {
		t.Fatalf("sample_images is not an empty array: %s", data)
	}
}

func TestJavIdolAvatarsValueAndScan(t *testing.T) {
	avatars := JavIdolAvatars{"upload:upload-a.jpg", "https://example.com/b.jpg"}
	value, err := avatars.Value()
	if err != nil {
		t.Fatalf("value: %v", err)
	}
	if value != `["upload:upload-a.jpg","https://example.com/b.jpg"]` {
		t.Fatalf("value = %v", value)
	}

	for _, stored := range []any{value, []byte(value.(string)), "", nil, "null"} {
		var scanned JavIdolAvatars
		if err := scanned.Scan(stored); err != nil {
			t.Fatalf("scan %#v: %v", stored, err)
		}
		if scanned == nil {
			t.Fatalf("scan %#v produced a nil slice", stored)
		}
	}

	var scanned JavIdolAvatars
	if err := scanned.Scan(value); err != nil {
		t.Fatalf("scan round-trip: %v", err)
	}
	if len(scanned) != 2 || scanned[0] != avatars[0] {
		t.Fatalf("scanned = %#v, want %#v", scanned, avatars)
	}
}

func TestJavSampleImagesNotFoundSentinel(t *testing.T) {
	images := NewJavSampleImagesNotFound()
	if !images.IsNotFound() {
		t.Fatalf("not-found sentinel was not recognized: %#v", images)
	}
	data, err := json.Marshal(images)
	if err != nil {
		t.Fatalf("marshal sentinel: %v", err)
	}
	if string(data) != `[{"thumbnail_url":":not_found","detail_url":":not_found"}]` {
		t.Fatalf("unexpected sentinel JSON: %s", data)
	}
}
