package service

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"javboss/internal/common"
	"javboss/internal/db"
	"javboss/internal/models"
)

func encodeTestPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func encodeTestJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func encodeTestGIF(t *testing.T, w, h int) []byte {
	t.Helper()
	palette := color.Palette{color.Black, color.White}
	img := image.NewPaletted(image.Rect(0, 0, w, h), palette)
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode gif: %v", err)
	}
	return buf.Bytes()
}

func TestScanDirectoryCleanupCategorizesAllSixJunkCategoriesAndProtectsMedia(t *testing.T) {
	root := t.TempDir()

	// Real videos & protected sidecar files (MUST NOT be flagged).
	writeTestFile(t, filepath.Join(root, "IPX-001", "hhd800.com@IPX-001.mp4"), make([]byte, 1024))
	writeTestFile(t, filepath.Join(root, "IPX-001", "hhd800.com@IPX-001.nfo"), []byte("<movie/>"))
	writeTestFile(t, filepath.Join(root, "IPX-001", "hhd800.com@IPX-001.zh.srt"), []byte("1\n00:00:01,000 --> 00:00:02,000\nhi"))
	writeTestFile(t, filepath.Join(root, "IPX-001", "hhd800.com@IPX-001-poster.jpg"), encodeTestJPEG(t, 400, 600))
	writeTestFile(t, filepath.Join(root, "IPX-001", "poster.jpg"), encodeTestJPEG(t, 400, 600))
	writeTestFile(t, filepath.Join(root, "IPX-001", "IPX-001.jpg"), encodeTestJPEG(t, 800, 538))
	writeTestFile(t, filepath.Join(root, "IPX-001", "extrafanart", "01.jpg"), encodeTestJPEG(t, 800, 538))
	writeTestFile(t, filepath.Join(root, "IPX-001", "sample_01.jpg"), append(encodeTestJPEG(t, 800, 538), make([]byte, 40*1024)...))

	// 1. System junk
	writeTestFile(t, filepath.Join(root, ".DS_Store"), []byte("ds"))
	writeTestFile(t, filepath.Join(root, "IPX-001", "Thumbs.db"), []byte("th"))
	writeTestFile(t, filepath.Join(root, "IPX-001", "._hhd800.com@IPX-001.mp4"), []byte("appledouble"))
	writeTestFile(t, filepath.Join(root, "IPX-001", "@eaDir", "IPX-001.mp4", "SYNOFILE_THUMB_M.jpg"), []byte("syno"))

	// 2. Text & web archives
	writeTestFile(t, filepath.Join(root, "IPX-001", "最新地址.txt"), []byte("promo"))
	writeTestFile(t, filepath.Join(root, "IPX-001", "永久地址.mhtml"), []byte("<html></html>"))
	writeTestFile(t, filepath.Join(root, "IPX-001", "点击观看.chm"), []byte("chm"))

	// 3. Torrent files
	writeTestFile(t, filepath.Join(root, "IPX-001", "IPX-001.torrent"), []byte("d8:announce"))

	// 4. Windows shortcuts
	writeTestFile(t, filepath.Join(root, "IPX-001", "免费看片.url"), []byte("[InternetShortcut]\nURL=https://example.com"))
	writeTestFile(t, filepath.Join(root, "IPX-001", "最新入口.lnk"), []byte("lnk"))

	// 5. Suspected ad images
	writeTestFile(t, filepath.Join(root, "IPX-001", "扫码进群.png"), encodeTestPNG(t, 500, 500))
	writeTestFile(t, filepath.Join(root, "IPX-001", "hhd800.com.jpg"), encodeTestJPEG(t, 600, 400))
	writeTestFile(t, filepath.Join(root, "IPX-001", "promo.gif"), encodeTestGIF(t, 400, 300))
	writeTestFile(t, filepath.Join(root, "IPX-001", "01.png"), encodeTestPNG(t, 960, 120))
	writeTestFile(t, filepath.Join(root, "IPX-001", "small_badge.png"), encodeTestPNG(t, 120, 120))

	// 6. Download leftovers
	writeTestFile(t, filepath.Join(root, "IPX-001", "IPX-002.mp4.!qb"), []byte("partial"))
	writeTestFile(t, filepath.Join(root, "IPX-001", "IPX-003.mp4.aria2"), []byte("aria2"))
	writeTestFile(t, filepath.Join(root, "IPX-001", "movie.xltd.cfg"), []byte("cfg"))
	writeTestFile(t, filepath.Join(root, "IPX-001", "_____padding_file_0_BitComet____"), []byte("pad"))
	writeTestFile(t, filepath.Join(root, "IPX-001", ".unwanted", "skip.txt"), []byte("unwanted"))

	result, err := ScanDirectoryCleanup(t.Context(), models.Directory{Path: root}, DirectoryCleanupScanOptions{
		Categories:        AllJunkCategories(),
		IncludeDuplicates: false,
	})
	if err != nil {
		t.Fatalf("ScanDirectoryCleanup failed: %v", err)
	}

	gotByPath := make(map[string]DirectoryCleanupJunkItem, len(result.JunkItems))
	for _, item := range result.JunkItems {
		gotByPath[item.RelativePath] = item
	}

	expected := map[string]string{
		".DS_Store":                        JunkCategorySystemJunk,
		"IPX-001/Thumbs.db":                JunkCategorySystemJunk,
		"IPX-001/._hhd800.com@IPX-001.mp4": JunkCategorySystemJunk,
		"IPX-001/@eaDir/IPX-001.mp4/SYNOFILE_THUMB_M.jpg": JunkCategorySystemJunk,
		"IPX-001/最新地址.txt":                                JunkCategoryTextWeb,
		"IPX-001/永久地址.mhtml":                              JunkCategoryTextWeb,
		"IPX-001/点击观看.chm":                                JunkCategoryTextWeb,
		"IPX-001/IPX-001.torrent":                         JunkCategoryTorrent,
		"IPX-001/免费看片.url":                                JunkCategoryWindowsShortcut,
		"IPX-001/最新入口.lnk":                                JunkCategoryWindowsShortcut,
		"IPX-001/扫码进群.png":                                JunkCategorySuspectedAdImage,
		"IPX-001/hhd800.com.jpg":                          JunkCategorySuspectedAdImage,
		"IPX-001/promo.gif":                               JunkCategorySuspectedAdImage,
		"IPX-001/01.png":                                  JunkCategorySuspectedAdImage,
		"IPX-001/small_badge.png":                         JunkCategorySuspectedAdImage,
		"IPX-001/IPX-002.mp4.!qb":                         JunkCategoryDownloadLeftover,
		"IPX-001/IPX-003.mp4.aria2":                       JunkCategoryDownloadLeftover,
		"IPX-001/movie.xltd.cfg":                          JunkCategoryDownloadLeftover,
		"IPX-001/_____padding_file_0_BitComet____":        JunkCategoryDownloadLeftover,
		"IPX-001/.unwanted/skip.txt":                      JunkCategoryDownloadLeftover,
	}
	for rel, wantCat := range expected {
		item, ok := gotByPath[rel]
		if !ok {
			t.Errorf("expected junk item %q (%s) to be detected", rel, wantCat)
			continue
		}
		if item.Category != wantCat {
			t.Errorf("junk item %q category = %q, want %q", rel, item.Category, wantCat)
		}
	}

	mustNotMatch := []string{
		"IPX-001/hhd800.com@IPX-001.mp4",
		"IPX-001/hhd800.com@IPX-001.nfo",
		"IPX-001/hhd800.com@IPX-001.zh.srt",
		"IPX-001/hhd800.com@IPX-001-poster.jpg",
		"IPX-001/poster.jpg",
		"IPX-001/IPX-001.jpg",
		"IPX-001/extrafanart/01.jpg",
		"IPX-001/sample_01.jpg",
	}
	for _, rel := range mustNotMatch {
		if item, ok := gotByPath[rel]; ok {
			t.Errorf("protected file %q was falsely flagged as %q (%s)", rel, item.Category, item.ReasonEN)
		}
	}

	if result.JunkTotalCount != len(expected) {
		t.Fatalf("JunkTotalCount = %d, want %d; items=%+v", result.JunkTotalCount, len(expected), result.JunkItems)
	}

	// Category filtering: requesting only torrent + windows_shortcut returns only those.
	filtered, err := ScanDirectoryCleanup(t.Context(), models.Directory{Path: root}, DirectoryCleanupScanOptions{
		Categories: []string{JunkCategoryTorrent, JunkCategoryWindowsShortcut},
	})
	if err != nil {
		t.Fatalf("filtered ScanDirectoryCleanup failed: %v", err)
	}
	if filtered.JunkTotalCount != 3 {
		t.Fatalf("filtered JunkTotalCount = %d, want 3", filtered.JunkTotalCount)
	}
}

func TestScanDirectoryCleanupDetectsDuplicatesByJavCodeAndMultiPart(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "cleanup.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	prevDB := common.DB
	common.DB = database
	t.Cleanup(func() {
		common.DB = prevDB
		if sqlDB, dbErr := database.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	root := t.TempDir()
	dir := models.Directory{Path: root}
	if err := database.Create(&dir).Error; err != nil {
		t.Fatalf("create directory: %v", err)
	}

	// Duplicate pair 1: IPX-001 (one scraped in DB, one unscraped with domain prefix + -C suffix).
	writeTestFile(t, filepath.Join(root, "a", "IPX-001.mp4"), make([]byte, 5000))
	writeTestFile(t, filepath.Join(root, "b", "hhd800.com@IPX-001-C.mkv"), make([]byte, 9000))
	// AppleDouble file for IPX-001 must NOT be counted as a duplicate video.
	writeTestFile(t, filepath.Join(root, "b", "._hhd800.com@IPX-001-C.mkv"), make([]byte, 128))

	// Duplicate pair 2: Multi-part SSIS-002 (CD1 & CD2).
	writeTestFile(t, filepath.Join(root, "SSIS-002-CD1.mp4"), make([]byte, 3000))
	writeTestFile(t, filepath.Join(root, "SSIS-002-CD2.mp4"), make([]byte, 3500))

	// Distinct works sharing the same website prefix must NOT collide on HHD-800!
	writeTestFile(t, filepath.Join(root, "hhd800.com@ABP-111.mp4"), make([]byte, 2000))
	writeTestFile(t, filepath.Join(root, "hhd800.com@ABP-222.mp4"), make([]byte, 2000))

	// DB record for a/IPX-001.mp4
	javRec := models.Jav{Code: "IPX-001", Title: "Japanese Title", ZhTitle: "中文标题"}
	if err := database.Create(&javRec).Error; err != nil {
		t.Fatalf("create jav: %v", err)
	}
	videoRec := models.Video{
		Size:        5000,
		DurationSec: 7200,
		Fingerprint: "1920x1080|5000000|4800000|192000|7200000|5000",
	}
	if err := database.Create(&videoRec).Error; err != nil {
		t.Fatalf("create video: %v", err)
	}
	loc, err := db.UpsertVideoLocation(t.Context(), videoRec.ID, dir.ID, "a/IPX-001.mp4", time.Now().UTC())
	if err != nil {
		t.Fatalf("upsert location: %v", err)
	}
	if err := database.Model(&models.VideoLocation{}).Where("id = ?", loc.ID).Update("jav_id", javRec.ID).Error; err != nil {
		t.Fatalf("link jav: %v", err)
	}

	result, err := ScanDirectoryCleanup(t.Context(), dir, DirectoryCleanupScanOptions{
		IncludeDuplicates: true,
	})
	if err != nil {
		t.Fatalf("ScanDirectoryCleanup failed: %v", err)
	}

	if result.DuplicateGroupCount != 2 {
		t.Fatalf("DuplicateGroupCount = %d, want 2; groups=%+v", result.DuplicateGroupCount, result.DuplicateGroups)
	}

	groupsByCode := make(map[string]DirectoryDuplicateGroup, len(result.DuplicateGroups))
	for _, g := range result.DuplicateGroups {
		groupsByCode[g.Code] = g
	}

	ipx, ok := groupsByCode["IPX-001"]
	if !ok {
		t.Fatalf("missing IPX-001 duplicate group: %+v", result.DuplicateGroups)
	}
	if ipx.Count != 2 || ipx.Title != "中文标题" || ipx.HasMultiPart || ipx.ReclaimableSize != 5000 {
		t.Fatalf("unexpected IPX-001 group: %+v", ipx)
	}
	if ipx.Files[0].RelativePath != "b/hhd800.com@IPX-001-C.mkv" || ipx.Files[1].RelativePath != "a/IPX-001.mp4" {
		t.Fatalf("expected largest file first in IPX-001 group: %+v", ipx.Files)
	}
	if ipx.Files[1].Resolution != "1920x1080" || ipx.Files[1].DurationSec != 7200 || ipx.Files[1].CodeSource != "jav" {
		t.Fatalf("expected DB metadata on a/IPX-001.mp4: %+v", ipx.Files[1])
	}

	ssis, ok := groupsByCode["SSIS-002"]
	if !ok {
		t.Fatalf("missing SSIS-002 group: %+v", result.DuplicateGroups)
	}
	if !ssis.HasMultiPart {
		t.Fatalf("expected SSIS-002 CD1/CD2 to be flagged as HasMultiPart: %+v", ssis)
	}
}

func TestDeleteDirectoryCleanupFilesRemovesFilesHidesDBLocationsAndPrunesEmptyDirs(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "cleanup-delete.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	prevDB := common.DB
	common.DB = database
	t.Cleanup(func() {
		common.DB = prevDB
		if sqlDB, dbErr := database.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	root := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	writeTestFile(t, outsideFile, []byte("keep"))

	dir := models.Directory{Path: root}
	if err := database.Create(&dir).Error; err != nil {
		t.Fatalf("create directory: %v", err)
	}

	junkRel := "nested/empty/ad.txt"
	dupRel := "nested/empty/IPX-001.mp4"
	writeTestFile(t, filepath.Join(root, filepath.FromSlash(junkRel)), []byte("junk"))
	writeTestFile(t, filepath.Join(root, filepath.FromSlash(dupRel)), []byte("video-bytes"))

	// Create symlink pointing outside root (must be rejected).
	symlinkRel := "escape.txt"
	if err := os.Symlink(outsideFile, filepath.Join(root, symlinkRel)); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	videoRec := models.Video{Size: 11, Fingerprint: "cleanup-delete-fp"}
	if err := database.Create(&videoRec).Error; err != nil {
		t.Fatalf("create video: %v", err)
	}
	loc, err := db.UpsertVideoLocation(t.Context(), videoRec.ID, dir.ID, dupRel, time.Now().UTC())
	if err != nil {
		t.Fatalf("upsert location: %v", err)
	}

	res, err := DeleteDirectoryCleanupFiles(
		t.Context(),
		dir,
		[]string{junkRel, dupRel, "../outside.txt", symlinkRel},
		true,
	)
	if err != nil {
		t.Fatalf("DeleteDirectoryCleanupFiles failed: %v", err)
	}

	if res.DeletedCount != 2 || res.HiddenVideoLocations != 1 || res.EmptyDirectoriesMoved != 2 {
		t.Fatalf("unexpected delete result: %+v", res)
	}
	if len(res.Failed) != 2 {
		t.Fatalf("expected 2 rejected paths (traversal + symlink), got: %+v", res.Failed)
	}
	if _, err := os.Stat(outsideFile); err != nil {
		t.Fatalf("outside file must remain untouched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "nested")); !os.IsNotExist(err) {
		t.Fatalf("expected empty nested directories to be removed, stat err=%v", err)
	}

	var refreshed models.VideoLocation
	if err := database.First(&refreshed, loc.ID).Error; err != nil {
		t.Fatalf("reload video location: %v", err)
	}
	if !refreshed.IsDelete {
		t.Fatalf("expected deleted video location to be marked is_delete=true")
	}
}
