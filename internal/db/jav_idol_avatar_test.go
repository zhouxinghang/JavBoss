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
