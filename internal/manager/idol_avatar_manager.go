package manager

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sync/singleflight"

	"javboss/internal/common/logging"
	"javboss/internal/jav/gfriends"
	"javboss/internal/jav/javdb"
	"javboss/internal/util"
)

const minValidIdolAvatarSizeBytes int64 = 1024

// maxIdolAvatarUploadBytes caps the size of a user-uploaded avatar file.
const maxIdolAvatarUploadBytes int64 = 20 << 20

// idolAvatarUploadScheme marks avatar URLs that point to a locally uploaded file
// instead of a remote image. The file name is carried as the URL opaque part.
const idolAvatarUploadScheme = "upload"

// ErrIdolAvatarNotFound indicates that no avatar is available for the idol.
var ErrIdolAvatarNotFound = errors.New("idol avatar not found")

// Errors returned while saving a user-uploaded avatar file.
var (
	ErrIdolAvatarUploadTooLarge = errors.New("idol avatar upload is too large")
	ErrIdolAvatarUploadType     = errors.New("idol avatar upload type is invalid")
	ErrIdolAvatarUploadInvalid  = errors.New("idol avatar upload is invalid")
)

// IdolAvatarManager downloads and caches idol avatars from the Gfriends repository.
// Every candidate image is stored as "<idolID>-<candidateKey>.<ext>" so an idol
// can keep several selectable avatars at once.
type IdolAvatarManager struct {
	avatarDir string
	client    *gfriends.Client
	group     singleflight.Group
}

// NewIdolAvatarManager creates a manager for the given avatar directory.
// It returns nil when avatarDir is empty.
func NewIdolAvatarManager(avatarDir string, client *gfriends.Client) *IdolAvatarManager {
	avatarDir = strings.TrimSpace(avatarDir)
	if avatarDir == "" {
		return nil
	}
	if client == nil {
		client = gfriends.New(gfriends.WithCachePath(filepath.Join(avatarDir, "Filetree.json")))
	}
	return &IdolAvatarManager{avatarDir: avatarDir, client: client}
}

// Start prepares the avatar directory. Safe to call on a nil manager.
func (m *IdolAvatarManager) Start(context.Context) {
	if m == nil {
		return
	}
	if err := os.MkdirAll(m.avatarDir, 0o755); err != nil {
		logging.Error("idol avatar: create dir: %v", err)
	}
}

// Dir returns the configured avatar directory.
func (m *IdolAvatarManager) Dir() string {
	if m == nil {
		return ""
	}
	return m.avatarDir
}

// Candidates returns the ordered avatar candidates for the given idol names.
func (m *IdolAvatarManager) Candidates(ctx context.Context, names ...string) []gfriends.Image {
	if m == nil {
		return nil
	}
	return m.client.Lookup(ctx, names...)
}

// CandidateKey returns the stable file/selection key for one candidate URL.
func CandidateKey(imageURL string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(imageURL)))
	return hex.EncodeToString(sum[:])[:16]
}

// LocalPath returns the cached file path for a specific candidate, if present.
func (m *IdolAvatarManager) LocalPath(idolID int64, imageURL string) (string, bool) {
	if m == nil || idolID <= 0 {
		return "", false
	}
	imageURL = strings.TrimSpace(imageURL)
	if imageURL == "" {
		return "", false
	}
	if name, ok := uploadAvatarName(imageURL); ok {
		target := filepath.Join(m.avatarDir, name)
		return target, isValidIdolAvatarFile(target)
	}
	target := filepath.Join(m.avatarDir, idolAvatarFileName(idolID, imageURL))
	return target, isValidIdolAvatarFile(target)
}

// Exists reports whether any avatar file exists for the idol.
func (m *IdolAvatarManager) Exists(idolID int64) bool {
	if m == nil {
		return false
	}
	return len(listIdolAvatarFiles(m.avatarDir, idolID)) > 0
}

// Ensure downloads and returns the cached path for a specific candidate URL.
func (m *IdolAvatarManager) Ensure(ctx context.Context, idolID int64, imageURL string) (string, error) {
	if m == nil {
		return "", ErrIdolAvatarNotFound
	}
	if idolID <= 0 {
		return "", errors.New("idol id must be positive")
	}
	imageURL = strings.TrimSpace(imageURL)
	if imageURL == "" {
		return "", ErrIdolAvatarNotFound
	}
	if target, ok := m.LocalPath(idolID, imageURL); ok {
		return target, nil
	}
	if IsUploadAvatarURL(imageURL) {
		return "", ErrIdolAvatarNotFound
	}
	return m.download(ctx, idolID, imageURL)
}

// EnsureAuto downloads the first candidate that succeeds and returns its path
// together with the chosen URL. The candidate list is ordered deterministically
// with AI-optimized copies first.
func (m *IdolAvatarManager) EnsureAuto(ctx context.Context, idolID int64, names []string) (string, string, error) {
	if m == nil {
		return "", "", ErrIdolAvatarNotFound
	}
	images := m.Candidates(ctx, names...)
	if len(images) == 0 {
		return "", "", ErrIdolAvatarNotFound
	}
	var lastErr error
	for _, image := range images {
		avatarPath, err := m.Ensure(ctx, idolID, image.URL)
		if err == nil {
			return avatarPath, image.URL, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return "", "", fmt.Errorf("download idol avatar: %w", lastErr)
	}
	return "", "", ErrIdolAvatarNotFound
}

// FindCandidate returns the candidate matching the given key.
func (m *IdolAvatarManager) FindCandidate(ctx context.Context, key string, names ...string) (gfriends.Image, bool) {
	key = strings.TrimSpace(key)
	if m == nil || key == "" {
		return gfriends.Image{}, false
	}
	for _, image := range m.Candidates(ctx, names...) {
		if CandidateKey(image.URL) == key {
			return image, true
		}
	}
	return gfriends.Image{}, false
}

// SaveUpload stores a user-uploaded avatar file and returns the synthetic
// "upload:" URL that selects it. The file name embeds the idol ID so uploads
// can be pruned when the idol switches to another avatar.
func (m *IdolAvatarManager) SaveUpload(idolID int64, ext string, input io.Reader) (string, error) {
	if m == nil {
		return "", ErrIdolAvatarNotFound
	}
	if idolID <= 0 {
		return "", errors.New("idol id must be positive")
	}
	ext = normalizeIdolAvatarUploadExt(ext)
	if ext == "" {
		return "", ErrIdolAvatarUploadType
	}
	if err := os.MkdirAll(m.avatarDir, 0o755); err != nil {
		return "", fmt.Errorf("ensure idol avatar dir: %w", err)
	}

	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate idol avatar name: %w", err)
	}
	// Upload file names are independent of the idol ID; ownership lives in the
	// jav_idol_avatar table so merging idols needs no file renames.
	name := idolAvatarUploadScheme + "-" + hex.EncodeToString(raw[:]) + ext
	target := filepath.Join(m.avatarDir, name)
	tmp := target + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return "", fmt.Errorf("create idol avatar temp: %w", err)
	}
	written, err := io.Copy(out, io.LimitReader(input, maxIdolAvatarUploadBytes+1))
	if err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return "", fmt.Errorf("write idol avatar upload: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("close idol avatar upload: %w", err)
	}
	if written > maxIdolAvatarUploadBytes {
		_ = os.Remove(tmp)
		return "", ErrIdolAvatarUploadTooLarge
	}
	if written < minValidIdolAvatarSizeBytes || !isDecodableCoverFile(tmp) {
		_ = os.Remove(tmp)
		return "", ErrIdolAvatarUploadInvalid
	}
	_ = os.Remove(target)
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("finalize idol avatar upload: %w", err)
	}
	return idolAvatarUploadScheme + ":" + name, nil
}

// DeleteUpload removes a stored upload file. Ownership is tracked by the
// jav_idol_avatar table, so the file itself carries no idol identifier.
func (m *IdolAvatarManager) DeleteUpload(imageURL string) {
	if m == nil {
		return
	}
	name, ok := uploadAvatarName(imageURL)
	if !ok {
		return
	}
	_ = os.Remove(filepath.Join(m.avatarDir, name))
}

// IsUploadAvatarURL reports whether the avatar URL points to a local upload.
func IsUploadAvatarURL(imageURL string) bool {
	_, ok := uploadAvatarName(imageURL)
	return ok
}

// Remove deletes all cached avatars (candidates and legacy files) for the idols.
func (m *IdolAvatarManager) Remove(idolIDs ...int64) {
	if m == nil {
		return
	}
	for _, idolID := range idolIDs {
		if idolID <= 0 {
			continue
		}
		for _, file := range listIdolAvatarFiles(m.avatarDir, idolID) {
			_ = os.Remove(file)
		}
	}
}

// RemoveRemote deletes downloaded candidate caches for the idol but keeps any
// locally uploaded avatar files, which cannot be re-downloaded.
func (m *IdolAvatarManager) RemoveRemote(idolID int64) {
	if m == nil || idolID <= 0 {
		return
	}
	uploadPrefix := strconv.FormatInt(idolID, 10) + "-upload-"
	for _, file := range listIdolAvatarFiles(m.avatarDir, idolID) {
		if strings.HasPrefix(filepath.Base(file), uploadPrefix) {
			continue
		}
		_ = os.Remove(file)
	}
}

func (m *IdolAvatarManager) download(ctx context.Context, idolID int64, imageURL string) (string, error) {
	groupKey := strconv.FormatInt(idolID, 10) + "-" + CandidateKey(imageURL)
	value, err, _ := m.group.Do(groupKey, func() (any, error) {
		if target, ok := m.LocalPath(idolID, imageURL); ok {
			return target, nil
		}
		return m.downloadOne(ctx, idolID, imageURL)
	})
	if err != nil {
		return "", err
	}
	target, _ := value.(string)
	if target == "" {
		return "", ErrIdolAvatarNotFound
	}
	return target, nil
}

func (m *IdolAvatarManager) downloadOne(ctx context.Context, idolID int64, imageURL string) (string, error) {
	if err := validateIdolAvatarURL(imageURL); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", fmt.Errorf("build idol avatar request: %w", err)
	}
	util.SetJavImageRequestHeaders(req)

	resp, err := util.DefaultHTTPClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("request idol avatar: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("idol avatar status %s", resp.Status)
	}

	if err := os.MkdirAll(m.avatarDir, 0o755); err != nil {
		return "", fmt.Errorf("ensure idol avatar dir: %w", err)
	}
	target := filepath.Join(m.avatarDir, idolAvatarFileName(idolID, imageURL))
	tmp := target + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return "", fmt.Errorf("create idol avatar temp: %w", err)
	}
	body, _ := javdb.DecodeImageBody(resp.Body)
	written, err := io.Copy(out, body)
	if err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return "", fmt.Errorf("write idol avatar: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("close idol avatar: %w", err)
	}
	if written < minValidIdolAvatarSizeBytes || !isDecodableCoverFile(tmp) {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("invalid idol avatar image (%d bytes)", written)
	}
	_ = os.Remove(target)
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("finalize idol avatar: %w", err)
	}
	return target, nil
}

// idolAvatarFileName builds the on-disk name for a candidate. It also drops the
// legacy "<id>.<ext>" auto file so old caches are replaced by keyed candidates.
func idolAvatarFileName(idolID int64, imageURL string) string {
	return strconv.FormatInt(idolID, 10) + "-" + CandidateKey(imageURL) + idolAvatarExt(imageURL)
}

// uploadAvatarName extracts the safe base file name from an upload avatar URL.
func uploadAvatarName(imageURL string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(imageURL))
	if err != nil || parsed == nil || parsed.Scheme != idolAvatarUploadScheme {
		return "", false
	}
	name := strings.TrimSpace(parsed.Opaque)
	if name == "" || name == "." || name == ".." || name != filepath.Base(name) {
		return "", false
	}
	return name, true
}

func normalizeIdolAvatarUploadExt(ext string) string {
	switch strings.ToLower(strings.TrimSpace(ext)) {
	case ".jpg", ".jpeg":
		return ".jpg"
	case ".png":
		return ".png"
	default:
		return ""
	}
}

func idolAvatarExt(imageURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(imageURL))
	if err == nil && parsed != nil {
		ext := strings.ToLower(path.Ext(parsed.Path))
		if ext != "" && len(ext) <= 5 {
			return ext
		}
	}
	return ".jpg"
}

func listIdolAvatarFiles(dir string, idolID int64) []string {
	dir = strings.TrimSpace(dir)
	if dir == "" || idolID <= 0 {
		return nil
	}
	prefix := strconv.FormatInt(idolID, 10) + "-"
	legacyPrefix := strconv.FormatInt(idolID, 10) + "."
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) && !strings.HasPrefix(name, legacyPrefix) {
			continue
		}
		if strings.HasSuffix(name, ".tmp") {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	return files
}

func isValidIdolAvatarFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return info.Size() >= minValidIdolAvatarSizeBytes
}

// validateIdolAvatarURL ensures the URL is safe to fetch.
func validateIdolAvatarURL(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil || parsed.Hostname() == "" {
		return errors.New("invalid idol avatar url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("idol avatar url must be http or https")
	}
	return nil
}
