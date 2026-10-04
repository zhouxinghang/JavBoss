package db

import (
	"context"
	"slices"
	"testing"

	"javboss/internal/models"
)

func TestGetJavIdolAvatarNamesPrefersJapaneseNameAndDedupes(t *testing.T) {
	gdb := openTestDB(t)
	ctx := context.Background()

	idol := models.JavIdol{
		Name:         "三上悠亜",
		JapaneseName: "三上悠亜",
		ChineseName:  "三上悠亚",
		RomanName:    "Yua Mikami",
	}
	if err := gdb.Create(&idol).Error; err != nil {
		t.Fatalf("create idol: %v", err)
	}
	aliases := []models.JavIdolAlias{
		{JavIdolID: idol.ID, Alias: "みかみゆあ"},
		{JavIdolID: idol.ID, Alias: "三上悠亚"},
	}
	if err := gdb.Create(&aliases).Error; err != nil {
		t.Fatalf("create aliases: %v", err)
	}

	names, err := GetJavIdolAvatarNames(ctx, idol.ID)
	if err != nil {
		t.Fatalf("GetJavIdolAvatarNames: %v", err)
	}
	want := []string{"三上悠亜", "三上悠亚", "Yua Mikami", "みかみゆあ"}
	if !slices.Equal(names, want) {
		t.Fatalf("names = %#v, want %#v", names, want)
	}
}

func TestGetJavIdolAvatarNamesRejectsInvalidID(t *testing.T) {
	openTestDB(t)
	if _, err := GetJavIdolAvatarNames(context.Background(), 0); err == nil {
		t.Fatal("expected an error for a non-positive idol id")
	}
}

func TestUpdateJavIdolAvatarURL(t *testing.T) {
	gdb := openTestDB(t)
	ctx := context.Background()

	idol := models.JavIdol{Name: "Avatar Idol"}
	if err := gdb.Create(&idol).Error; err != nil {
		t.Fatalf("create idol: %v", err)
	}

	if avatarURL, err := GetJavIdolAvatarURL(ctx, idol.ID); err != nil || avatarURL != "" {
		t.Fatalf("initial avatar url = %q err=%v, want empty", avatarURL, err)
	}

	selected := "https://example.com/Content/1/abc.jpg?t=123"
	if err := UpdateJavIdolAvatarURL(ctx, idol.ID, selected); err != nil {
		t.Fatalf("UpdateJavIdolAvatarURL: %v", err)
	}
	if avatarURL, err := GetJavIdolAvatarURL(ctx, idol.ID); err != nil || avatarURL != selected {
		t.Fatalf("avatar url = %q err=%v, want %q", avatarURL, err, selected)
	}

	// Saving the same value must not be reported as a missing idol.
	if err := UpdateJavIdolAvatarURL(ctx, idol.ID, selected); err != nil {
		t.Fatalf("UpdateJavIdolAvatarURL (unchanged): %v", err)
	}

	if err := UpdateJavIdolAvatarURL(ctx, idol.ID, ""); err != nil {
		t.Fatalf("reset avatar url: %v", err)
	}
	if avatarURL, err := GetJavIdolAvatarURL(ctx, idol.ID); err != nil || avatarURL != "" {
		t.Fatalf("reset avatar url = %q err=%v, want empty", avatarURL, err)
	}

	if err := UpdateJavIdolAvatarURL(ctx, idol.ID+9999, "x"); err == nil {
		t.Fatal("expected ErrRecordNotFound for a missing idol")
	}
}

func TestJavIdolAvatarCollection(t *testing.T) {
	gdb := openTestDB(t)
	ctx := context.Background()

	idol := models.JavIdol{Name: "Collection Idol"}
	if err := gdb.Create(&idol).Error; err != nil {
		t.Fatalf("create idol: %v", err)
	}

	uploadURL := "upload:upload-abc123.jpg"
	manualURL := "https://example.com/custom.jpg"
	for _, url := range []string{uploadURL, manualURL, uploadURL} {
		if err := AddJavIdolAvatar(ctx, idol.ID, url); err != nil {
			t.Fatalf("AddJavIdolAvatar(%q): %v", url, err)
		}
	}

	urls, err := ListJavIdolAvatars(ctx, idol.ID)
	if err != nil {
		t.Fatalf("ListJavIdolAvatars: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("avatars = %#v, want 2 unique entries", urls)
	}

	if err := DeleteJavIdolAvatar(ctx, idol.ID, uploadURL); err != nil {
		t.Fatalf("DeleteJavIdolAvatar: %v", err)
	}
	urls, err = ListJavIdolAvatars(ctx, idol.ID)
	if err != nil || len(urls) != 1 || urls[0] != manualURL {
		t.Fatalf("avatars after delete = %#v err=%v", urls, err)
	}

	if _, err := ListJavIdolAvatars(ctx, 0); err == nil {
		t.Fatal("expected an error for a non-positive idol id")
	}
}

func TestMergeJavIdolsMovesAvatarCollection(t *testing.T) {
	gdb := openTestDB(t)
	ctx := context.Background()

	canonical := models.JavIdol{Name: "Canonical Idol"}
	source := models.JavIdol{Name: "Source Idol"}
	if err := gdb.Create(&canonical).Error; err != nil {
		t.Fatalf("create canonical: %v", err)
	}
	if err := gdb.Create(&source).Error; err != nil {
		t.Fatalf("create source: %v", err)
	}

	shared := "https://example.com/shared.jpg"
	if err := AddJavIdolAvatar(ctx, canonical.ID, shared); err != nil {
		t.Fatalf("add canonical avatar: %v", err)
	}
	for _, url := range []string{shared, "upload:upload-source.jpg"} {
		if err := AddJavIdolAvatar(ctx, source.ID, url); err != nil {
			t.Fatalf("add source avatar %q: %v", url, err)
		}
	}

	if _, err := MergeJavIdols(ctx, canonical.ID, []int64{source.ID}, nil); err != nil {
		t.Fatalf("MergeJavIdols: %v", err)
	}

	urls, err := ListJavIdolAvatars(ctx, canonical.ID)
	if err != nil {
		t.Fatalf("ListJavIdolAvatars: %v", err)
	}
	slices.Sort(urls)
	want := []string{"upload:upload-source.jpg", shared}
	slices.Sort(want)
	if !slices.Equal(urls, want) {
		t.Fatalf("merged avatars = %#v, want %#v", urls, want)
	}

	// The merged idol is gone, so its collection lives on only via the canonical idol.
	var sourceCount int64
	if err := gdb.Model(&models.JavIdol{}).Where("id = ?", source.ID).Count(&sourceCount).Error; err != nil {
		t.Fatalf("count source idol: %v", err)
	}
	if sourceCount != 0 {
		t.Fatalf("source idol still exists")
	}
}
