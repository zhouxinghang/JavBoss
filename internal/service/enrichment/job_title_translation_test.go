package enrichment

import (
	"context"
	"errors"
	"testing"

	"javboss/internal/db"
	"javboss/internal/deepseek"
	"javboss/internal/models"
)

func TestTitleTranslationEnrichment(t *testing.T) {
	gdb := openEnrichmentTestDB(t)
	ctx := context.Background()
	items := []models.Jav{
		{Code: "ABC-001", Title: "A quiet afternoon"},
		{Code: "ABC-002", Title: "Original", ZhTitle: "已有中文"},
		{Code: "ABC-003", Title: " "},
	}
	if err := gdb.Create(&items).Error; err != nil {
		t.Fatal(err)
	}
	calls := 0
	translate := func(_ context.Context, settings deepseek.Settings, title string) (string, error) {
		calls++
		if settings.APIKey != "test-key" || settings.Model != deepseek.DefaultModel || title != items[0].Title {
			t.Fatalf("unexpected input settings/model/title")
		}
		return "宁静的午后", nil
	}
	// Neither an absent key nor a disabled switch should issue paid requests.
	if err := enrichTitleTranslations(ctx, translate); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertConfig(ctx, map[string]string{"deepseek_api_key": "test-key"}); err != nil {
		t.Fatal(err)
	}
	if err := enrichTitleTranslations(ctx, translate); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatalf("disabled calls=%d", calls)
	}
	if err := db.UpsertConfig(ctx, map[string]string{"deepseek_auto_translate": "true"}); err != nil {
		t.Fatal(err)
	}
	if err := enrichTitleTranslations(ctx, translate); err != nil {
		t.Fatal(err)
	}
	if err := enrichTitleTranslations(ctx, translate); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("translation calls=%d", calls)
	}
	var stored models.Jav
	if err := gdb.First(&stored, items[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Title != items[0].Title || stored.ZhTitle != "宁静的午后" {
		t.Fatalf("titles=%q/%q", stored.Title, stored.ZhTitle)
	}
	stored = models.Jav{}
	if err := gdb.First(&stored, items[1].ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ZhTitle != "已有中文" {
		t.Fatalf("existing title=%q", stored.ZhTitle)
	}
}

func TestTitleTranslationPreservesConcurrentEditsAndStopsOnFailure(t *testing.T) {
	gdb := openEnrichmentTestDB(t)
	ctx := context.Background()
	if err := db.UpsertConfig(ctx, map[string]string{"deepseek_api_key": "key", "deepseek_auto_translate": "true"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		updates map[string]any
		want    string
	}{
		{"manual translation", map[string]any{"zh_title": "人工中文标题"}, "人工中文标题"},
		{"changed original", map[string]any{"title": "Changed original"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := gdb.Where("1 = 1").Delete(&models.Jav{}).Error; err != nil {
				t.Fatal(err)
			}
			item := models.Jav{Code: "ABC-001", Title: "Original"}
			if err := gdb.Create(&item).Error; err != nil {
				t.Fatal(err)
			}
			err := enrichTitleTranslations(ctx, func(context.Context, deepseek.Settings, string) (string, error) {
				if err := gdb.Model(&item).Updates(tc.updates).Error; err != nil {
					t.Fatal(err)
				}
				return "自动翻译", nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := gdb.First(&item, item.ID).Error; err != nil {
				t.Fatal(err)
			}
			if item.ZhTitle != tc.want {
				t.Fatalf("title=%q, want %q", item.ZhTitle, tc.want)
			}
		})
	}
	wantErr := errors.New("provider unavailable")
	calls := 0
	err := enrichTitleTranslations(ctx, func(context.Context, deepseek.Settings, string) (string, error) { calls++; return "", wantErr })
	if !errors.Is(err, wantErr) || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
