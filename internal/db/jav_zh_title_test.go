package db

import (
	"context"
	"encoding/json"
	"testing"

	"javboss/internal/jav/metadata"
	"javboss/internal/models"
)

func TestSaveJavInfoPersistsBothTitles(t *testing.T) {
	gdb := openTestDB(t)
	ctx := context.Background()
	info := &metadata.JavInfo{Code: "MIDE-557", Title: "日本語の原題", ZhTitle: " 中文标题 ", Provider: metadata.ProviderJavDBAPI}
	rec, err := SaveJavInfo(ctx, info)
	if err != nil {
		t.Fatal(err)
	}
	var stored models.Jav
	assertTitles := func(wantTitle, wantZhTitle string) {
		t.Helper()
		if err := gdb.First(&stored, rec.ID).Error; err != nil {
			t.Fatal(err)
		}
		if stored.Title != wantTitle || stored.ZhTitle != wantZhTitle {
			t.Fatalf("stored titles = %q, %q", stored.Title, stored.ZhTitle)
		}
	}
	assertTitles("日本語の原題", "中文标题")
	info.Title, info.ZhTitle = "更新した原題", "更新后的中文标题"
	if _, err := SaveJavInfo(ctx, info); err != nil {
		t.Fatal(err)
	}
	assertTitles("更新した原題", "更新后的中文标题")
	// Providers without a translated title must not erase one already stored.
	info.Provider, info.ZhTitle = metadata.ProviderJavBus, ""
	if _, err := SaveJavInfo(ctx, info); err != nil {
		t.Fatal(err)
	}
	assertTitles("更新した原題", "更新后的中文标题")
	payload, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["title"] != stored.Title || fields["zh_title"] != stored.ZhTitle {
		t.Fatalf("API title fields: %s", payload)
	}
}

func TestChineseTitlesAreSearchableAndVisibleInFavorites(t *testing.T) {
	gdb := openTestDB(t)
	ctx := context.Background()
	item := models.Jav{Code: "ABC-001", Title: "Original", ZhTitle: "宁静的午后"}
	if err := gdb.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	dir := models.Directory{Path: "/tmp/title-translation"}
	if err := gdb.Create(&dir).Error; err != nil {
		t.Fatal(err)
	}
	video := models.Video{DirectoryID: dir.ID, Path: "test.mp4", Filename: "test.mp4", Fingerprint: "translation", JavID: &item.ID}
	if err := gdb.Create(&video).Error; err != nil {
		t.Fatal(err)
	}
	createVideoLocationsForVideos(t, gdb, video)
	for _, search := range []string{"午后", "Original", "ABC-001"} {
		items, total, err := SearchJav(ctx, nil, nil, search, "", 25, 0, nil, nil)
		if err != nil || total != 1 || len(items) != 1 || items[0].ZhTitle != item.ZhTitle {
			t.Fatalf("search=%q total=%d items=%v err=%v", search, total, items, err)
		}
	}
	group := models.JavFavoriteGroup{Name: "Titles", EntityType: JavFavoriteEntityJav}
	if err := gdb.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := AddJavsToFavoriteGroups(ctx, []int64{item.ID}, []int64{group.ID}); err != nil {
		t.Fatal(err)
	}
	favorites, err := ListJavFavoriteGroupItems(ctx, JavFavoriteEntityJav, group.ID, nil)
	if err != nil || len(favorites) != 1 || favorites[0].Name != "ABC-001 宁静的午后" || favorites[0].Title != item.Title || favorites[0].ZhTitle != item.ZhTitle {
		t.Fatalf("favorites=%+v err=%v", favorites, err)
	}
}
