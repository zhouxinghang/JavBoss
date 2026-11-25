package service

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"javboss/internal/common/logging"
	"javboss/internal/db"
	"javboss/internal/models"
	"javboss/internal/util"
)

const (
	JunkCategorySystemJunk       = "system_junk"
	JunkCategoryTextWeb          = "text_web"
	JunkCategoryTorrent          = "torrent"
	JunkCategoryWindowsShortcut  = "windows_shortcut"
	JunkCategorySuspectedAdImage = "suspected_ad_image"
	JunkCategoryDownloadLeftover = "download_leftover"
)

var allJunkCategories = []string{
	JunkCategorySystemJunk,
	JunkCategoryTextWeb,
	JunkCategoryTorrent,
	JunkCategoryWindowsShortcut,
	JunkCategorySuspectedAdImage,
	JunkCategoryDownloadLeftover,
}

var (
	ErrInvalidCleanupCategory = errors.New("invalid cleanup category")
	ErrCleanupFileUnavailable = errors.New("cleanup file is unavailable")
	ErrCleanupFileUnsupported = errors.New("cleanup file is not previewable")
)

// DirectoryCleanupScanOptions configures one directory cleanup scan.
type DirectoryCleanupScanOptions struct {
	Categories        []string
	IncludeDuplicates bool
}

// DirectoryCleanupJunkItem describes one matched junk file.
type DirectoryCleanupJunkItem struct {
	RelativePath     string `json:"relative_path"`
	Filename         string `json:"filename"`
	Category         string `json:"category"`
	Size             int64  `json:"size"`
	ModifiedAtUnixMS int64  `json:"modified_at_unix_ms"`
	ReasonZH         string `json:"reason_zh"`
	ReasonEN         string `json:"reason_en"`
	Width            int    `json:"width,omitempty"`
	Height           int    `json:"height,omitempty"`
	Previewable      bool   `json:"previewable,omitempty"`
}

// DirectoryCleanupCategorySummary aggregates matches for one junk category.
type DirectoryCleanupCategorySummary struct {
	Category  string `json:"category"`
	Count     int    `json:"count"`
	TotalSize int64  `json:"total_size"`
}

// DirectoryDuplicateFile describes one video file belonging to a duplicate JAV code group.
type DirectoryDuplicateFile struct {
	RelativePath     string `json:"relative_path"`
	Filename         string `json:"filename"`
	Size             int64  `json:"size"`
	ModifiedAtUnixMS int64  `json:"modified_at_unix_ms"`
	Code             string `json:"code"`
	CodeSource       string `json:"code_source"`
	PartLabel        string `json:"part_label,omitempty"`
	LocationID       int64  `json:"location_id,omitempty"`
	VideoID          int64  `json:"video_id,omitempty"`
	DurationSec      int64  `json:"duration_sec,omitempty"`
	Resolution       string `json:"resolution,omitempty"`
	Fingerprint      string `json:"fingerprint,omitempty"`
}

// DirectoryDuplicateGroup groups video files sharing the same JAV code.
type DirectoryDuplicateGroup struct {
	Code            string                   `json:"code"`
	Title           string                   `json:"title,omitempty"`
	Count           int                      `json:"count"`
	TotalSize       int64                    `json:"total_size"`
	ReclaimableSize int64                    `json:"reclaimable_size"`
	HasMultiPart    bool                     `json:"has_multi_part"`
	Files           []DirectoryDuplicateFile `json:"files"`
}

// DirectoryCleanupScanResult summarizes junk files and duplicate video groups for a directory.
type DirectoryCleanupScanResult struct {
	DirectoryID                 int64                             `json:"directory_id"`
	ScannedFiles                int64                             `json:"scanned_files"`
	DurationMS                  int64                             `json:"duration_ms"`
	SelectedCategories          []string                          `json:"selected_categories"`
	IncludeDuplicates           bool                              `json:"include_duplicates"`
	JunkTotalCount              int                               `json:"junk_total_count"`
	JunkTotalSize               int64                             `json:"junk_total_size"`
	CategorySummaries           []DirectoryCleanupCategorySummary `json:"category_summaries"`
	JunkItems                   []DirectoryCleanupJunkItem        `json:"junk_items"`
	DuplicateGroupCount         int                               `json:"duplicate_group_count"`
	DuplicateFileCount          int                               `json:"duplicate_file_count"`
	DuplicateTotalSize          int64                             `json:"duplicate_total_size"`
	DuplicateReclaimableSize    int64                             `json:"duplicate_reclaimable_size"`
	DuplicateGroups             []DirectoryDuplicateGroup         `json:"duplicate_groups"`
	SkippedUnreadableEntryCount int                               `json:"skipped_unreadable_entry_count,omitempty"`
}

// DirectoryCleanupDeleteFailure records one file that could not be deleted.
type DirectoryCleanupDeleteFailure struct {
	RelativePath string `json:"relative_path"`
	ReasonZH     string `json:"reason_zh"`
	ReasonEN     string `json:"reason_en"`
}

// DirectoryCleanupDeleteResult reports the outcome of deleting selected files.
type DirectoryCleanupDeleteResult struct {
	DeletedCount          int                             `json:"deleted_count"`
	DeletedSize           int64                           `json:"deleted_size"`
	DeletedPaths          []string                        `json:"deleted_paths"`
	HiddenVideoLocations  int64                           `json:"hidden_video_locations"`
	EmptyDirectoriesMoved int                             `json:"empty_directories_removed"`
	Failed                []DirectoryCleanupDeleteFailure `json:"failed"`
}

// NormalizeCleanupCategories validates and deduplicates requested categories in canonical order.
func NormalizeCleanupCategories(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	selected := make(map[string]struct{}, len(raw))
	for _, value := range raw {
		cat := strings.TrimSpace(value)
		if cat == "" {
			continue
		}
		if !isValidJunkCategory(cat) {
			return nil, fmt.Errorf("%w: %s", ErrInvalidCleanupCategory, cat)
		}
		selected[cat] = struct{}{}
	}
	out := make([]string, 0, len(selected))
	for _, cat := range allJunkCategories {
		if _, ok := selected[cat]; ok {
			out = append(out, cat)
		}
	}
	return out, nil
}

// AllJunkCategories returns all supported junk categories in canonical order.
func AllJunkCategories() []string {
	out := make([]string, len(allJunkCategories))
	copy(out, allJunkCategories)
	return out
}

func isValidJunkCategory(category string) bool {
	for _, candidate := range allJunkCategories {
		if candidate == category {
			return true
		}
	}
	return false
}

type cleanupImageCandidate struct {
	parentDir  string
	relPath    string
	absPath    string
	filename   string
	size       int64
	modifiedAt time.Time
}

type cleanupDirVideoInfo struct {
	stems map[string]struct{}
	codes map[string]struct{}
}

type cleanupDuplicateCandidate struct {
	relPath    string
	filename   string
	size       int64
	modifiedAt time.Time
	meta       *db.DirectoryCleanupVideoMeta
}

// ScanDirectoryCleanup scans one directory for selected junk categories and/or duplicate JAV videos.
func ScanDirectoryCleanup(
	ctx context.Context,
	directory models.Directory,
	options DirectoryCleanupScanOptions,
) (*DirectoryCleanupScanResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	root := filepath.Clean(strings.TrimSpace(directory.Path))
	if root == "" || !filepath.IsAbs(root) {
		return nil, errors.New("directory root must be absolute")
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("directory path is not a directory")
	}

	categories, err := NormalizeCleanupCategories(options.Categories)
	if err != nil {
		return nil, err
	}
	selectedSet := make(map[string]struct{}, len(categories))
	for _, cat := range categories {
		selectedSet[cat] = struct{}{}
	}
	wantImages := hasKey(selectedSet, JunkCategorySuspectedAdImage)
	wantDuplicates := options.IncludeDuplicates

	var dbVideoMeta map[string]db.DirectoryCleanupVideoMeta
	knownDirCodes := make(map[string]string)
	if (wantDuplicates || wantImages) && directory.ID > 0 {
		loaded, dbErr := db.ListDirectoryCleanupVideoMetadata(ctx, directory.ID)
		if dbErr != nil {
			logging.Error("load directory cleanup video metadata failed id=%d err=%v", directory.ID, dbErr)
		} else {
			dbVideoMeta = loaded
			for _, meta := range loaded {
				if code := normalizeDuplicateCode(meta.JavCode); code != "" {
					knownDirCodes[code] = code
				}
			}
		}
	}

	startedAt := time.Now()
	result := &DirectoryCleanupScanResult{
		DirectoryID:        directory.ID,
		SelectedCategories: categories,
		IncludeDuplicates:  wantDuplicates,
		JunkItems:          make([]DirectoryCleanupJunkItem, 0),
		DuplicateGroups:    make([]DirectoryDuplicateGroup, 0),
	}

	var imageCandidates []cleanupImageCandidate
	dirVideos := make(map[string]*cleanupDirVideoInfo)
	var duplicateCandidates []cleanupDuplicateCandidate

	walkErr := filepath.WalkDir(root, func(candidatePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			result.SkippedUnreadableEntryCount++
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if entry.IsDir() {
			if candidatePath != root && shouldSkipSystemProtectedDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}

		relOS, relErr := filepath.Rel(root, filepath.Clean(candidatePath))
		if relErr != nil || relOS == "." || relOS == ".." || strings.HasPrefix(relOS, ".."+string(filepath.Separator)) {
			return nil
		}
		relSlash := filepath.ToSlash(cleanRelativePath(relOS))
		if relSlash == "" {
			return nil
		}

		fileInfo, infoErr := entry.Info()
		if infoErr != nil {
			result.SkippedUnreadableEntryCount++
			return nil
		}
		if !fileInfo.Mode().IsRegular() {
			return nil
		}

		result.ScannedFiles++
		filename := entry.Name()
		size := fileInfo.Size()
		modTime := fileInfo.ModTime().UTC()

		// 1. Classify mutually exclusive non-image junk categories first so AppleDouble
		// files like "._IPX-001.mp4" or download leftovers like "IPX-001.mp4.!qb" are never
		// misclassified as videos or other file types.
		if category, reasonZH, reasonEN, matched := classifyNonImageJunk(relSlash, filename); matched {
			if hasKey(selectedSet, category) {
				result.JunkItems = append(result.JunkItems, DirectoryCleanupJunkItem{
					RelativePath:     relSlash,
					Filename:         filename,
					Category:         category,
					Size:             size,
					ModifiedAtUnixMS: modTime.UnixMilli(),
					ReasonZH:         reasonZH,
					ReasonEN:         reasonEN,
				})
			}
			return nil
		}

		// 2. Track real videos for sidecar image protection and duplicate detection.
		meta, inDB := dbVideoMeta[relSlash]
		if inDB || util.HasVideoExtension(filepath.Ext(filename)) {
			if wantImages {
				parentDir := filepath.Dir(relSlash)
				dv := dirVideos[parentDir]
				if dv == nil {
					dv = &cleanupDirVideoInfo{
						stems: make(map[string]struct{}),
						codes: make(map[string]struct{}),
					}
					dirVideos[parentDir] = dv
				}
				stem := strings.ToLower(strings.TrimSuffix(filename, filepath.Ext(filename)))
				if stem != "" {
					dv.stems[stem] = struct{}{}
				}
				if inDB && strings.TrimSpace(meta.JavCode) != "" {
					dv.codes[strings.ToLower(normalizeDuplicateCode(meta.JavCode))] = struct{}{}
				} else if code := extractPrimaryDuplicateCode(filename, knownDirCodes); code != "" {
					dv.codes[strings.ToLower(code)] = struct{}{}
				}
			}
			if wantDuplicates {
				var metaPtr *db.DirectoryCleanupVideoMeta
				if inDB {
					copyMeta := meta
					metaPtr = &copyMeta
				}
				duplicateCandidates = append(duplicateCandidates, cleanupDuplicateCandidate{
					relPath:    relSlash,
					filename:   filename,
					size:       size,
					modifiedAt: modTime,
					meta:       metaPtr,
				})
			}
			return nil
		}

		// 3. Collect image candidates for post-walk suspected ad image classification.
		if wantImages && isSupportedCleanupImageExt(filepath.Ext(filename)) {
			imageCandidates = append(imageCandidates, cleanupImageCandidate{
				parentDir:  filepath.Dir(relSlash),
				relPath:    relSlash,
				absPath:    candidatePath,
				filename:   filename,
				size:       size,
				modifiedAt: modTime,
			})
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	if wantImages {
		for _, candidate := range imageCandidates {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			item, matched := classifySuspectedAdImage(candidate, dirVideos[candidate.parentDir])
			if matched {
				result.JunkItems = append(result.JunkItems, item)
			}
		}
	}

	sort.Slice(result.JunkItems, func(i, j int) bool {
		if result.JunkItems[i].Category != result.JunkItems[j].Category {
			return junkCategoryOrder(result.JunkItems[i].Category) < junkCategoryOrder(result.JunkItems[j].Category)
		}
		if result.JunkItems[i].Size != result.JunkItems[j].Size {
			return result.JunkItems[i].Size > result.JunkItems[j].Size
		}
		return result.JunkItems[i].RelativePath < result.JunkItems[j].RelativePath
	})

	summaryByCategory := make(map[string]*DirectoryCleanupCategorySummary, len(categories))
	result.CategorySummaries = make([]DirectoryCleanupCategorySummary, len(categories))
	for i, cat := range categories {
		result.CategorySummaries[i] = DirectoryCleanupCategorySummary{Category: cat}
		summaryByCategory[cat] = &result.CategorySummaries[i]
	}
	for i := range result.JunkItems {
		item := &result.JunkItems[i]
		result.JunkTotalCount++
		result.JunkTotalSize += item.Size
		if summary := summaryByCategory[item.Category]; summary != nil {
			summary.Count++
			summary.TotalSize += item.Size
		}
	}

	if wantDuplicates {
		groups := buildDuplicateGroups(duplicateCandidates, knownDirCodes)
		result.DuplicateGroups = groups
		result.DuplicateGroupCount = len(groups)
		for i := range groups {
			result.DuplicateFileCount += groups[i].Count
			result.DuplicateTotalSize += groups[i].TotalSize
			result.DuplicateReclaimableSize += groups[i].ReclaimableSize
		}
	}

	result.DurationMS = time.Since(startedAt).Milliseconds()
	return result, nil
}

// DeleteDirectoryCleanupFiles safely removes validated relative file paths inside directory,
// hides any corresponding deleted video locations in DB, and optionally prunes empty subdirectories.
func DeleteDirectoryCleanupFiles(
	ctx context.Context,
	directory models.Directory,
	relativePaths []string,
	removeEmptyDirs bool,
) (*DirectoryCleanupDeleteResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if directory.ID <= 0 || directory.IsDelete {
		return nil, errors.New("invalid directory")
	}
	if activeDirectoryProcessingStatus(directory.ID) != "" {
		return nil, ErrDirectoryWorkInProgress
	}

	root := filepath.Clean(strings.TrimSpace(directory.Path))
	if root == "" || !filepath.IsAbs(root) {
		return nil, errors.New("directory root must be absolute")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}

	reserveCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	release, err := CancelAndReserveDirectoryScan(reserveCtx, directory.ID)
	cancel()
	if err != nil {
		return nil, err
	}
	defer release()

	result := &DirectoryCleanupDeleteResult{
		DeletedPaths: make([]string, 0, len(relativePaths)),
		Failed:       make([]DirectoryCleanupDeleteFailure, 0),
	}

	seen := make(map[string]struct{}, len(relativePaths))
	emptyDirCandidates := make([]string, 0)

	for _, raw := range relativePaths {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return result, ctxErr
		}
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		normalizedRel := filepath.ToSlash(cleanRelativePath(filepath.FromSlash(trimmed)))
		if normalizedRel == "" {
			result.Failed = append(result.Failed, DirectoryCleanupDeleteFailure{
				RelativePath: trimmed,
				ReasonZH:     "文件路径无效",
				ReasonEN:     "Invalid file path",
			})
			continue
		}
		if _, dup := seen[normalizedRel]; dup {
			continue
		}
		seen[normalizedRel] = struct{}{}

		target, info, reasonZH, reasonEN, ok := resolveSafeCleanupFilePath(root, realRoot, normalizedRel)
		if !ok {
			result.Failed = append(result.Failed, DirectoryCleanupDeleteFailure{
				RelativePath: normalizedRel,
				ReasonZH:     reasonZH,
				ReasonEN:     reasonEN,
			})
			continue
		}

		if err := util.MoveFileToTrash(target); err != nil {
			logging.Error("cleanup delete file failed id=%d path=%s err=%v", directory.ID, target, err)
			rZH, rEN := localizedCleanupFileError(err)
			result.Failed = append(result.Failed, DirectoryCleanupDeleteFailure{
				RelativePath: normalizedRel,
				ReasonZH:     rZH,
				ReasonEN:     rEN,
			})
			continue
		}

		result.DeletedCount++
		result.DeletedSize += info.Size()
		result.DeletedPaths = append(result.DeletedPaths, normalizedRel)
		if removeEmptyDirs {
			emptyDirCandidates = append(emptyDirCandidates, filepath.Dir(target))
		}
	}

	if len(result.DeletedPaths) > 0 {
		hidden, hideErr := db.HideDirectoryVideoLocationsByPaths(ctx, directory.ID, result.DeletedPaths)
		if hideErr != nil {
			logging.Error("hide deleted cleanup video locations failed id=%d err=%v", directory.ID, hideErr)
		} else {
			result.HiddenVideoLocations = hidden
		}
	}

	if removeEmptyDirs && len(emptyDirCandidates) > 0 {
		summary := &DirectoryProcessSummary{emptyDirectoryCandidates: emptyDirCandidates}
		cleanupEmptySourceDirectories(root, summary)
		result.EmptyDirectoriesMoved = summary.EmptyDirectoriesRemoved
	}

	return result, nil
}

// ResolveDirectoryCleanupPreviewPath validates and resolves a previewable image inside directory.
func ResolveDirectoryCleanupPreviewPath(directory models.Directory, relativePath string) (string, string, error) {
	root := filepath.Clean(strings.TrimSpace(directory.Path))
	if root == "" || !filepath.IsAbs(root) {
		return "", "", errors.New("directory root must be absolute")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", err
	}
	normalizedRel := filepath.ToSlash(cleanRelativePath(filepath.FromSlash(strings.TrimSpace(relativePath))))
	if normalizedRel == "" {
		return "", "", ErrCleanupFileUnavailable
	}
	if !isSupportedCleanupImageExt(filepath.Ext(normalizedRel)) {
		return "", "", ErrCleanupFileUnsupported
	}
	target, _, _, _, ok := resolveSafeCleanupFilePath(root, realRoot, normalizedRel)
	if !ok {
		return "", "", ErrCleanupFileUnavailable
	}
	return target, cleanupImageContentType(filepath.Ext(target)), nil
}

func resolveSafeCleanupFilePath(
	root string,
	realRoot string,
	relativePath string,
) (string, os.FileInfo, string, string, bool) {
	if strings.ContainsRune(relativePath, 0) {
		return "", nil, "文件路径包含非法字符", "File path contains invalid characters", false
	}
	target, err := safeDirectoryFilePath(root, relativePath)
	if err != nil {
		return "", nil, "文件路径超出所选目录范围", "File path is outside the selected directory", false
	}
	info, err := os.Lstat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil, "文件不存在或已被删除", "File does not exist or has already been removed", false
		}
		rZH, rEN := localizedCleanupFileError(err)
		return "", nil, rZH, rEN, false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", nil, "为安全起见，不支持操作符号链接文件", "Symlinks are not supported for safety", false
	}
	if info.IsDir() || !info.Mode().IsRegular() {
		return "", nil, "目标路径不是普通文件", "Target path is not a regular file", false
	}

	parentDir := filepath.Dir(target)
	realParent, err := filepath.EvalSymlinks(parentDir)
	if err != nil {
		rZH, rEN := localizedCleanupFileError(err)
		return "", nil, rZH, rEN, false
	}
	relParent, err := filepath.Rel(root, parentDir)
	if err != nil {
		return "", nil, "文件路径无效", "Invalid file path", false
	}
	expectedRealParent := filepath.Join(realRoot, relParent)
	if !pathWithinDirectory(realRoot, realParent) ||
		filepath.Clean(realParent) != filepath.Clean(expectedRealParent) {
		return "", nil, "为避免越过符号链接，未操作此文件", "Refused to operate across a symbolic link", false
	}
	return target, info, "", "", true
}

func localizedCleanupFileError(err error) (string, string) {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "文件不存在或已被删除", "File does not exist or has already been removed"
	case errors.Is(err, os.ErrPermission):
		return "没有足够的文件访问权限", "Permission denied"
	default:
		return "操作文件失败", "Failed to process file"
	}
}

func shouldSkipSystemProtectedDir(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "system volume information", "$recycle.bin", "lost+found":
		return true
	default:
		return false
	}
}

func junkCategoryOrder(category string) int {
	for i, candidate := range allJunkCategories {
		if candidate == category {
			return i
		}
	}
	return len(allJunkCategories)
}

func hasKey(set map[string]struct{}, key string) bool {
	_, ok := set[key]
	return ok
}

// --- Junk classification ---

var systemJunkExactNames = map[string][2]string{
	".ds_store":                           {"macOS 目录元数据文件 (.DS_Store)", "macOS folder metadata (.DS_Store)"},
	"thumbs.db":                           {"Windows 缩略图缓存 (Thumbs.db)", "Windows thumbnail cache (Thumbs.db)"},
	"ehthumbs.db":                         {"Windows 缩略图缓存 (ehthumbs.db)", "Windows thumbnail cache (ehthumbs.db)"},
	"ehthumbs_vista.db":                   {"Windows 缩略图缓存 (ehthumbs_vista.db)", "Windows thumbnail cache (ehthumbs_vista.db)"},
	"desktop.ini":                         {"Windows 目录配置文件 (desktop.ini)", "Windows folder config (desktop.ini)"},
	".directory":                          {"Linux 桌面目录元数据 (.directory)", "Linux folder metadata (.directory)"},
	".lsoverride":                         {"macOS 元数据文件 (.LSOverride)", "macOS metadata (.LSOverride)"},
	".localized":                          {"macOS 本地化标记文件 (.localized)", "macOS localization marker (.localized)"},
	".apdisk":                             {"macOS 共享目录元数据 (.apdisk)", "macOS share metadata (.apdisk)"},
	".volumeicon.icns":                    {"macOS 卷图标文件 (.VolumeIcon.icns)", "macOS volume icon (.VolumeIcon.icns)"},
	".com.apple.timemachine.donotpresent": {"macOS Time Machine 标记文件", "macOS Time Machine marker"},
	"icon\r":                              {"macOS 自定义图标文件", "macOS custom folder icon"},
}

var systemJunkDirSegments = map[string][2]string{
	"@eadir":          {"群晖 NAS 缩略图元数据 (@eaDir)", "Synology NAS metadata (@eaDir)"},
	".@__thumb":       {"威联通 NAS 缩略图缓存 (.@__thumb)", "QNAP NAS thumbnail cache (.@__thumb)"},
	".appledouble":    {"macOS 资源分支目录 (.AppleDouble)", "macOS resource fork directory (.AppleDouble)"},
	"__macosx":        {"macOS 压缩包元数据 (__MACOSX)", "macOS archive metadata (__MACOSX)"},
	".spotlight-v100": {"macOS Spotlight 索引文件", "macOS Spotlight index file"},
	".trashes":        {"macOS 废纸篓残留文件", "macOS trash leftover"},
	".fseventsd":      {"macOS 文件事件日志", "macOS file event log"},
	".temporaryitems": {"macOS 系统临时文件", "macOS system temporary item"},
}

var downloadLeftoverCompoundSuffixes = []string{
	".td.cfg",
	".xltd.cfg",
	".baiduyun.p.downloading",
	".baiduyun.downloading",
	".115.downloading",
}

var downloadLeftoverExtensions = map[string]struct{}{
	".!qb":           {},
	".!ut":           {},
	".!bt":           {},
	".bc!":           {},
	".uc!":           {},
	".part":          {},
	".partial":       {},
	".filepart":      {},
	".crdownload":    {},
	".download":      {},
	".opdownload":    {},
	".fdmdownload":   {},
	".rdownload":     {},
	".115download":   {},
	".qkdownloading": {},
	".downloading":   {},
	".unfinished":    {},
	".xltd":          {},
	".td":            {},
	".aria2":         {},
	".dltemp":        {},
	".dltmp":         {},
	".tmp":           {},
	".temp":          {},
	".fastresume":    {},
	".idm":           {},
	".idmpart":       {},
	".parts":         {},
	".padding":       {},
}

var torrentExtensions = map[string]struct{}{
	".torrent":  {},
	".magnet":   {},
	".metalink": {},
	".meta4":    {},
}

var windowsShortcutExtensions = map[string]struct{}{
	".lnk":     {},
	".url":     {},
	".website": {},
	".pif":     {},
	".scf":     {},
}

var textWebExtensions = map[string][2]string{
	".txt":        {"文本文档 (.txt)", "Text file (.txt)"},
	".rtf":        {"富文本文档 (.rtf)", "Rich text file (.rtf)"},
	".md":         {"Markdown 文本 (.md)", "Markdown document (.md)"},
	".diz":        {"发布说明文本 (.diz)", "Release info text (.diz)"},
	".log":        {"日志文件 (.log)", "Log file (.log)"},
	".htm":        {"网页文件 (.htm)", "Web page (.htm)"},
	".html":       {"网页文件 (.html)", "Web page (.html)"},
	".xhtml":      {"网页文件 (.xhtml)", "Web page (.xhtml)"},
	".mht":        {"网页存档 (.mht)", "Web archive (.mht)"},
	".mhtml":      {"网页存档 (.mhtml)", "Web archive (.mhtml)"},
	".webarchive": {"网页存档 (.webarchive)", "Web archive (.webarchive)"},
	".maff":       {"网页存档 (.maff)", "Web archive (.maff)"},
	".chm":        {"编译网页帮助文件 (.chm)", "Compiled HTML help (.chm)"},
	".webloc":     {"网页快捷存档 (.webloc)", "Web location file (.webloc)"},
}

func classifyNonImageJunk(relSlash, filename string) (string, string, string, bool) {
	lowerName := strings.ToLower(filename)
	lowerExt := strings.ToLower(filepath.Ext(filename))

	// 1. System junk (exact names, AppleDouble ._* files, OS/NAS metadata dirs).
	if reason, ok := systemJunkExactNames[lowerName]; ok {
		return JunkCategorySystemJunk, reason[0], reason[1], true
	}
	if strings.HasPrefix(filename, "._") && len(filename) > 2 {
		return JunkCategorySystemJunk, "macOS AppleDouble 资源分支文件 (._*)", "macOS AppleDouble resource fork (._*)", true
	}
	segments := strings.Split(relSlash, "/")
	for _, seg := range segments[:max(0, len(segments)-1)] {
		if reason, ok := systemJunkDirSegments[strings.ToLower(seg)]; ok {
			return JunkCategorySystemJunk, reason[0], reason[1], true
		}
	}

	// 2. Download leftovers (unwanted folder, compound suffixes, partial extensions, padding files).
	for _, seg := range segments[:max(0, len(segments)-1)] {
		if strings.ToLower(seg) == ".unwanted" {
			return JunkCategoryDownloadLeftover, "BT 客户端未勾选残留目录 (.unwanted)", "Torrent client .unwanted leftover", true
		}
	}
	for _, suffix := range downloadLeftoverCompoundSuffixes {
		if strings.HasSuffix(lowerName, suffix) {
			return JunkCategoryDownloadLeftover, fmt.Sprintf("下载器临时残留 (%s)", suffix), fmt.Sprintf("Downloader leftover (%s)", suffix), true
		}
	}
	if strings.Contains(lowerName, "____padding_file") {
		return JunkCategoryDownloadLeftover, "BitComet 比特彗星填充文件", "BitComet padding file", true
	}
	if strings.HasPrefix(lowerName, "~utorrentpartfile_") || lowerName == ".parts" {
		return JunkCategoryDownloadLeftover, "BT 客户端分片残留文件", "Torrent client partfile leftover", true
	}
	if _, ok := downloadLeftoverExtensions[lowerExt]; ok {
		return JunkCategoryDownloadLeftover, fmt.Sprintf("未完成下载/临时文件 (%s)", lowerExt), fmt.Sprintf("Incomplete download/temp file (%s)", lowerExt), true
	}

	// 3. Torrent files.
	if _, ok := torrentExtensions[lowerExt]; ok {
		return JunkCategoryTorrent, fmt.Sprintf("种子/下载描述文件 (%s)", lowerExt), fmt.Sprintf("Torrent/download descriptor (%s)", lowerExt), true
	}

	// 4. Windows shortcuts.
	if _, ok := windowsShortcutExtensions[lowerExt]; ok {
		return JunkCategoryWindowsShortcut, fmt.Sprintf("Windows 快捷方式 (%s)", lowerExt), fmt.Sprintf("Windows shortcut (%s)", lowerExt), true
	}

	// 5. Text and web archives (explicitly excludes .nfo and subtitle formats).
	if reason, ok := textWebExtensions[lowerExt]; ok {
		return JunkCategoryTextWeb, reason[0], reason[1], true
	}

	return "", "", "", false
}

// --- Suspected ad image classification ---

var supportedCleanupImageExts = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
	".jfif": "image/jpeg",
	".tif":  "image/tiff",
	".tiff": "image/tiff",
}

func isSupportedCleanupImageExt(ext string) bool {
	_, ok := supportedCleanupImageExts[strings.ToLower(strings.TrimSpace(ext))]
	return ok
}

func cleanupImageContentType(ext string) string {
	if ct, ok := supportedCleanupImageExts[strings.ToLower(strings.TrimSpace(ext))]; ok {
		return ct
	}
	return "application/octet-stream"
}

var protectedArtworkDirs = map[string]struct{}{
	"extrafanart": {},
	"extrathumbs": {},
	".actors":     {},
	"actors":      {},
}

var protectedArtworkStems = map[string]struct{}{
	"poster":    {},
	"fanart":    {},
	"folder":    {},
	"cover":     {},
	"thumb":     {},
	"landscape": {},
	"backdrop":  {},
	"clearlogo": {},
	"disc":      {},
	"discart":   {},
	"banner":    {},
	"movie":     {},
	"logo":      {},
	"keyart":    {},
}

var artworkSuffixRe = regexp.MustCompile(`^(?:poster|fanart|thumb|cover|landscape|backdrop|clearlogo|banner|disc|discart|logo|keyart|extrafanart|extrathumb)\d*$`)

var adImageChineseKeywords = []string{
	"广告", "宣传", "推广", "扫码", "二维码", "公众号", "福利", "网址", "最新地址", "永久地址",
	"备用地址", "防走丢", "防失联", "防屏蔽", "域名", "导航", "论坛", "社区", "免费看", "免费观看",
	"官网", "入口", "澳门", "赌场", "博彩", "棋牌", "彩票", "首存", "充送", "招租", "客服",
	"微信", "电报", "飞机群", "核工厂", "草榴", "色花堂", "杏吧", "98堂", "第一会所", "裸聊",
	"同城约", "约炮", "看片", "直播", "破解版", "app下载", "客户端下载", "收藏本站", "赞助",
}

var adImageEnglishTokens = map[string]struct{}{
	"ad":            {},
	"ads":           {},
	"advert":        {},
	"advertisement": {},
	"promo":         {},
	"promotion":     {},
	"qrcode":        {},
	"qr":            {},
	"casino":        {},
	"gambling":      {},
	"betting":       {},
	"telegram":      {},
	"wechat":        {},
	"whatsapp":      {},
	"bbs":           {},
	"forum":         {},
	"sponsor":       {},
	"apk":           {},
	"xunlei":        {},
	"bt4g":          {},
	"sehuatang":     {},
	"t66y":          {},
	"sis001":        {},
	"javdb":         {},
}

var adImageDomainRe = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:www\.[a-z0-9-]+|[a-z0-9-]{2,}\.(?:com|net|org|xyz|cc|me|vip|tv|top|la|app|club|icu|info|live|co|io|cn|tw|hk|jp|uk|pro|site|online|win|bid|pw|ws|gg|su|ru|tk|ml|ga|cf))(?:$|[^a-z0-9])`)

func isProtectedMediaArtwork(candidate cleanupImageCandidate, videos *cleanupDirVideoInfo) bool {
	ext := strings.ToLower(filepath.Ext(candidate.filename))
	stem := strings.ToLower(strings.TrimSuffix(candidate.filename, filepath.Ext(candidate.filename)))

	// Never treat GIF files as protected folder artwork unless they literally share a video's exact stem.
	if ext != ".gif" {
		segments := strings.Split(candidate.relPath, "/")
		for _, seg := range segments[:max(0, len(segments)-1)] {
			if _, ok := protectedArtworkDirs[strings.ToLower(seg)]; ok {
				return true
			}
		}
		if _, ok := protectedArtworkStems[stem]; ok {
			return true
		}
	}

	if videos == nil {
		return false
	}
	if _, ok := videos.stems[stem]; ok {
		return true
	}
	if ext == ".gif" {
		return false
	}
	for videoStem := range videos.stems {
		if matchesArtworkCompanion(stem, videoStem) {
			return true
		}
	}
	for code := range videos.codes {
		if stem == code || matchesArtworkCompanion(stem, code) {
			return true
		}
	}
	return false
}

func matchesArtworkCompanion(imageStem, base string) bool {
	if base == "" || len(imageStem) <= len(base)+1 {
		return false
	}
	if !strings.HasPrefix(imageStem, base) {
		return false
	}
	sep := imageStem[len(base)]
	if sep != '-' && sep != '.' && sep != '_' {
		return false
	}
	suffix := imageStem[len(base)+1:]
	return artworkSuffixRe.MatchString(suffix)
}

func classifySuspectedAdImage(
	candidate cleanupImageCandidate,
	videos *cleanupDirVideoInfo,
) (DirectoryCleanupJunkItem, bool) {
	if isProtectedMediaArtwork(candidate, videos) {
		return DirectoryCleanupJunkItem{}, false
	}

	ext := strings.ToLower(filepath.Ext(candidate.filename))
	stem := strings.TrimSuffix(candidate.filename, filepath.Ext(candidate.filename))
	lowerStem := strings.ToLower(stem)

	var reasonZH, reasonEN string
	matched := false

	switch {
	case ext == ".gif":
		matched = true
		reasonZH = "GIF 动图/疑似广告图"
		reasonEN = "GIF image / suspected ad"
	case hasAdImageKeyword(lowerStem):
		matched = true
		reasonZH = "文件名含广告/推广关键词"
		reasonEN = "Filename contains ad/promo keywords"
	case adImageDomainRe.MatchString(lowerStem):
		matched = true
		reasonZH = "文件名含网址域名"
		reasonEN = "Filename contains a website domain"
	}

	w, h := readImageDimensions(candidate.absPath, ext)
	if !matched {
		switch {
		case candidate.size == 0:
			matched = true
			reasonZH = "空图片文件"
			reasonEN = "Empty image file"
		case w > 0 && h > 0 && isExtremeAdAspectRatio(w, h):
			matched = true
			reasonZH = fmt.Sprintf("疑似横幅/长条广告图 (%dx%d)", w, h)
			reasonEN = fmt.Sprintf("Suspected banner/strip ad (%dx%d)", w, h)
		case w > 0 && h > 0 && w <= 240 && h <= 240 && candidate.size <= 30*1024:
			matched = true
			reasonZH = fmt.Sprintf("疑似小尺寸图标/二维码广告 (%dx%d)", w, h)
			reasonEN = fmt.Sprintf("Suspected small icon/QR ad (%dx%d)", w, h)
		}
	}

	if !matched {
		return DirectoryCleanupJunkItem{}, false
	}

	return DirectoryCleanupJunkItem{
		RelativePath:     candidate.relPath,
		Filename:         candidate.filename,
		Category:         JunkCategorySuspectedAdImage,
		Size:             candidate.size,
		ModifiedAtUnixMS: candidate.modifiedAt.UnixMilli(),
		ReasonZH:         reasonZH,
		ReasonEN:         reasonEN,
		Width:            w,
		Height:           h,
		Previewable:      true,
	}, true
}

func hasAdImageKeyword(lowerStem string) bool {
	for _, kw := range adImageChineseKeywords {
		if strings.Contains(lowerStem, kw) {
			return true
		}
	}
	for _, token := range splitASCIITokens(lowerStem) {
		if _, ok := adImageEnglishTokens[token]; ok {
			return true
		}
	}
	return false
}

func splitASCIITokens(value string) []string {
	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return tokens
}

func isExtremeAdAspectRatio(width, height int) bool {
	if width <= 0 || height <= 0 {
		return false
	}
	ratio := float64(width) / float64(height)
	return ratio >= 3.0 || ratio <= 1.0/3.0
}

func readImageDimensions(path, ext string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	switch ext {
	case ".webp":
		return readWebPDimensions(f)
	case ".bmp":
		return readBMPDimensions(f)
	default:
		cfg, _, err := image.DecodeConfig(f)
		if err != nil {
			return 0, 0
		}
		return cfg.Width, cfg.Height
	}
}

func readWebPDimensions(r io.Reader) (int, int) {
	var header [30]byte
	n, err := io.ReadFull(r, header[:])
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return 0, 0
	}
	buf := header[:n]
	if len(buf) < 25 || string(buf[0:4]) != "RIFF" || string(buf[8:12]) != "WEBP" {
		return 0, 0
	}
	switch string(buf[12:16]) {
	case "VP8 ":
		if len(buf) < 30 || buf[23] != 0x9d || buf[24] != 0x01 || buf[25] != 0x2a {
			return 0, 0
		}
		w := int(binary.LittleEndian.Uint16(buf[26:28]) & 0x3fff)
		h := int(binary.LittleEndian.Uint16(buf[28:30]) & 0x3fff)
		return w, h
	case "VP8L":
		if len(buf) < 25 || buf[20] != 0x2f {
			return 0, 0
		}
		bits := binary.LittleEndian.Uint32(buf[21:25])
		w := int(bits&0x3fff) + 1
		h := int((bits>>14)&0x3fff) + 1
		return w, h
	case "VP8X":
		if len(buf) < 30 {
			return 0, 0
		}
		w := (int(buf[24]) | int(buf[25])<<8 | int(buf[26])<<16) + 1
		h := (int(buf[27]) | int(buf[28])<<8 | int(buf[29])<<16) + 1
		return w, h
	default:
		return 0, 0
	}
}

func readBMPDimensions(r io.Reader) (int, int) {
	var header [26]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, 0
	}
	if header[0] != 'B' || header[1] != 'M' {
		return 0, 0
	}
	w := int(int32(binary.LittleEndian.Uint32(header[18:22])))
	h := int(int32(binary.LittleEndian.Uint32(header[22:26])))
	if h < 0 {
		h = -h
	}
	if w <= 0 || h <= 0 {
		return 0, 0
	}
	return w, h
}

// --- Duplicate detection by JAV code ---

var (
	cleanupDomainPrefixRe = regexp.MustCompile(`(?i)(?:[\[\(【（]\s*)?(?:https?://)?(?:www\.)?[a-z0-9-]+(?:\.[a-z0-9-]+)*\.(?:com|net|org|xyz|cc|me|vip|tv|top|la|app|club|icu|info|live|co|io|cn|tw|hk|jp|uk|pro|site|online|win|bid|pw|ws|gg|su|ru)(?:\s*[\]\)】）])?[@\s_-]*`)
	cleanupVideoNoiseRe   = regexp.MustCompile(`(?i)(^|[^a-z0-9])(480p|720p|1080p|1440p|2160p|4k|8k|h\.?264|h\.?265|x264|x265|hevc|av1|xvid|divx|fhd|uhd)([^a-z0-9]|$)`)
	cleanupHyphenSpaceRe  = regexp.MustCompile(`\s*([-_])\s*`)
	cleanupPureNumericRe  = regexp.MustCompile(`(^|[^0-9])(\d{6})[-_](\d{2,4})([^0-9]|$)`)
	cleanupExplicitPartRe = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(cd|part|disc|disk|pt|ep)[-_ ]?0*([1-9]\d?)(?:[^a-z0-9]|$)`)
	cleanupTrailingNumRe  = regexp.MustCompile(`(?i)[-_ ]0*([1-9])$`)
	cleanupTrailingABRe   = regexp.MustCompile(`(?i)[-_ ]([ab])$`)
)

var duplicateNoiseCodes = map[string]struct{}{
	"H264":  {},
	"H265":  {},
	"X264":  {},
	"X265":  {},
	"MP4":   {},
	"MKV":   {},
	"AVI":   {},
	"WMV":   {},
	"HD720": {},
	"HD108": {},
}

func normalizeDuplicateCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return ""
	}
	if cleanupPureNumericRe.MatchString(code) {
		code = strings.ReplaceAll(code, "_", "-")
	}
	return code
}

func extractPrimaryDuplicateCode(filename string, knownCodes map[string]string) string {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	cleaned := cleanupDomainPrefixRe.ReplaceAllString(base, " ")
	for {
		next := cleanupVideoNoiseRe.ReplaceAllString(cleaned, "$1 $3")
		if next == cleaned {
			break
		}
		cleaned = next
	}
	cleaned = cleanupHyphenSpaceRe.ReplaceAllString(cleaned, "$1")
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		cleaned = base
	}

	candidates := util.ExtractCodeFromName(cleaned)
	if len(candidates) == 0 && cleaned != base {
		candidates = util.ExtractCodeFromName(base)
	}

	// Prefer any candidate matching a known scraped JAV code in this directory.
	for _, candidate := range candidates {
		norm := normalizeDuplicateCode(candidate)
		if canonical, ok := knownCodes[norm]; ok {
			return canonical
		}
	}

	// Prefer explicit 6-digit uncensored numeric codes (e.g. 051626-001, 031419_001)
	// before generic patterns that might grab studio words.
	if m := cleanupPureNumericRe.FindStringSubmatch(cleaned); len(m) >= 4 {
		return fmt.Sprintf("%s-%s", m[2], m[3])
	}

	// Filter out obvious noise tokens and return the primary normalized code.
	for _, candidate := range candidates {
		norm := normalizeDuplicateCode(candidate)
		if norm == "" {
			continue
		}
		if _, isNoise := duplicateNoiseCodes[norm]; isNoise {
			continue
		}
		if isPartTokenCode(norm) {
			continue
		}
		return norm
	}
	return ""
}

func isPartTokenCode(code string) bool {
	switch {
	case strings.HasPrefix(code, "CD-"), strings.HasPrefix(code, "CD"):
		rest := strings.TrimPrefix(strings.TrimPrefix(code, "CD-"), "CD")
		return isAllDigits(rest)
	case strings.HasPrefix(code, "PART-"), strings.HasPrefix(code, "PART"):
		rest := strings.TrimPrefix(strings.TrimPrefix(code, "PART-"), "PART")
		return isAllDigits(rest)
	case strings.HasPrefix(code, "DISC-"), strings.HasPrefix(code, "DISC"):
		rest := strings.TrimPrefix(strings.TrimPrefix(code, "DISC-"), "DISC")
		return isAllDigits(rest)
	default:
		return false
	}
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func detectVideoPartLabel(filename, code string) (string, string) {
	stem := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	if m := cleanupExplicitPartRe.FindStringSubmatch(stem); len(m) >= 3 {
		kind := strings.ToUpper(m[1])
		num := m[2]
		return kind + num, "explicit:" + kind
	}
	// Strip the detected code prefix from stem to inspect trailing part markers like "-1", "_2", "-A", "-B".
	lowerStem := strings.ToLower(stem)
	lowerCode := strings.ToLower(code)
	remainder := stem
	if idx := strings.Index(lowerStem, lowerCode); idx >= 0 {
		remainder = stem[idx+len(lowerCode):]
	} else {
		compactCode := strings.ReplaceAll(lowerCode, "-", "")
		if idx := strings.Index(lowerStem, compactCode); idx >= 0 {
			remainder = stem[idx+len(compactCode):]
		}
	}
	remainder = strings.TrimSpace(remainder)
	if m := cleanupTrailingNumRe.FindStringSubmatch(remainder); len(m) >= 2 {
		return "#" + m[1], "trailing_num"
	}
	if m := cleanupTrailingABRe.FindStringSubmatch(remainder); len(m) >= 2 {
		return strings.ToUpper(m[1]), "trailing_ab"
	}
	return "", ""
}

func parseResolutionFromFingerprint(fp string) string {
	fp = strings.TrimSpace(fp)
	if fp == "" {
		return ""
	}
	parts := strings.Split(fp, "|")
	if len(parts) < 6 {
		return ""
	}
	dims := strings.Split(parts[0], "x")
	if len(dims) != 2 {
		return ""
	}
	w, errW := strconv.Atoi(dims[0])
	h, errH := strconv.Atoi(dims[1])
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return ""
	}
	return fmt.Sprintf("%dx%d", w, h)
}

func buildDuplicateGroups(
	candidates []cleanupDuplicateCandidate,
	knownCodes map[string]string,
) []DirectoryDuplicateGroup {
	type groupAccumulator struct {
		code       string
		title      string
		files      []DirectoryDuplicateFile
		partKinds  map[string]map[string]struct{}
		explicitPs map[string]struct{}
	}

	grouped := make(map[string]*groupAccumulator)
	for _, candidate := range candidates {
		code := ""
		codeSource := "filename"
		title := ""
		var locationID, videoID, durationSec int64
		var resolution, fingerprint string

		if candidate.meta != nil {
			locationID = candidate.meta.LocationID
			videoID = candidate.meta.VideoID
			durationSec = candidate.meta.DurationSec
			fingerprint = candidate.meta.Fingerprint
			resolution = parseResolutionFromFingerprint(fingerprint)
			title = firstNonEmptyString(candidate.meta.JavZhTitle, candidate.meta.JavTitle)

			if c := normalizeDuplicateCode(candidate.meta.JavCode); c != "" {
				code = c
				codeSource = "jav"
			} else if forced := forcedJavScrapeCode(candidate.meta.JavScrapeOverride); forced != "" {
				code = normalizeDuplicateCode(forced)
				codeSource = "override"
			}
		}

		if code == "" {
			code = extractPrimaryDuplicateCode(candidate.filename, knownCodes)
			codeSource = "filename"
		}
		if code == "" {
			continue
		}

		acc := grouped[code]
		if acc == nil {
			acc = &groupAccumulator{
				code:       code,
				partKinds:  make(map[string]map[string]struct{}),
				explicitPs: make(map[string]struct{}),
			}
			grouped[code] = acc
		}
		if acc.title == "" && title != "" {
			acc.title = title
		}

		partLabel, partKind := detectVideoPartLabel(candidate.filename, code)
		if partLabel != "" {
			if acc.partKinds[partKind] == nil {
				acc.partKinds[partKind] = make(map[string]struct{})
			}
			acc.partKinds[partKind][partLabel] = struct{}{}
			if strings.HasPrefix(partKind, "explicit:") {
				acc.explicitPs[partLabel] = struct{}{}
			}
		}

		acc.files = append(acc.files, DirectoryDuplicateFile{
			RelativePath:     candidate.relPath,
			Filename:         candidate.filename,
			Size:             candidate.size,
			ModifiedAtUnixMS: candidate.modifiedAt.UnixMilli(),
			Code:             code,
			CodeSource:       codeSource,
			PartLabel:        partLabel,
			LocationID:       locationID,
			VideoID:          videoID,
			DurationSec:      durationSec,
			Resolution:       resolution,
			Fingerprint:      fingerprint,
		})
	}

	groups := make([]DirectoryDuplicateGroup, 0)
	for _, acc := range grouped {
		if len(acc.files) < 2 {
			continue
		}

		hasMultiPart := len(acc.explicitPs) >= 2
		if !hasMultiPart {
			for _, labels := range acc.partKinds {
				if len(labels) >= 2 {
					hasMultiPart = true
					break
				}
			}
		}

		// Clear speculative trailing part labels if the group does not actually have distinct parts
		// (for example, two copies both ending in "-1" or a single file with "-a" and one without).
		if !hasMultiPart {
			for i := range acc.files {
				if !strings.HasPrefix(acc.files[i].PartLabel, "CD") &&
					!strings.HasPrefix(acc.files[i].PartLabel, "PART") &&
					!strings.HasPrefix(acc.files[i].PartLabel, "DISC") &&
					!strings.HasPrefix(acc.files[i].PartLabel, "DISK") &&
					!strings.HasPrefix(acc.files[i].PartLabel, "PT") &&
					!strings.HasPrefix(acc.files[i].PartLabel, "EP") {
					acc.files[i].PartLabel = ""
				}
			}
		}

		sort.Slice(acc.files, func(i, j int) bool {
			if acc.files[i].Size != acc.files[j].Size {
				return acc.files[i].Size > acc.files[j].Size
			}
			if acc.files[i].ModifiedAtUnixMS != acc.files[j].ModifiedAtUnixMS {
				return acc.files[i].ModifiedAtUnixMS > acc.files[j].ModifiedAtUnixMS
			}
			return acc.files[i].RelativePath < acc.files[j].RelativePath
		})

		var totalSize, maxSize int64
		for i := range acc.files {
			totalSize += acc.files[i].Size
			if acc.files[i].Size > maxSize {
				maxSize = acc.files[i].Size
			}
		}

		groups = append(groups, DirectoryDuplicateGroup{
			Code:            acc.code,
			Title:           acc.title,
			Count:           len(acc.files),
			TotalSize:       totalSize,
			ReclaimableSize: totalSize - maxSize,
			HasMultiPart:    hasMultiPart,
			Files:           acc.files,
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Count != groups[j].Count {
			return groups[i].Count > groups[j].Count
		}
		if groups[i].ReclaimableSize != groups[j].ReclaimableSize {
			return groups[i].ReclaimableSize > groups[j].ReclaimableSize
		}
		return groups[i].Code < groups[j].Code
	})

	return groups
}
