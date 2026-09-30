// Tuneshine Plugin for Navidrome
//
// Sends album art and track metadata to a physical Tuneshine device (Direct mode)
// or offloads processing to a Tuneshine Hub container (Hub mode).
//
// Capabilities: Scrobbler, SchedulerCallback
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"strings"

	"github.com/HugoSmits86/nativewebp"
	"github.com/navidrome/navidrome/plugins/pdk/go/host"
	"github.com/navidrome/navidrome/plugins/pdk/go/pdk"
	"github.com/navidrome/navidrome/plugins/pdk/go/scheduler"
	"github.com/navidrome/navidrome/plugins/pdk/go/scrobbler"
	"golang.org/x/image/draw"
)

const (
	modeKey        = "mode" // "direct" or "hub"
	hostKey        = "host"
	serviceNameKey = "servicename"
	userKey        = "user"

	cacheKeyLastTrack = "last-track-id"
	cacheTTLSeconds   = 3600 // 1 hour

	tuneshineSize = 64 // Tuneshine display is 64x64

	// Scheduler constants for debounced pause-clear (Direct mode)
	pauseClearScheduleID = "tuneshine-pause-clear"
	pauseClearDelay      = 5 // seconds to wait before clearing on pause
)

type pluginConfig struct {
	Mode        string // "direct" or "hub"
	DeviceHost  string
	ServiceName string
}

// tuneshine implements the scrobbler and scheduler callback interfaces.
type tuneshine struct{}

func init() {
	t := &tuneshine{}
	scrobbler.Register(t)
	scheduler.Register(t)
}

// getConfig loads the plugin configuration.
func getConfig() (pluginConfig, error) {
	deviceHost, ok := pdk.GetConfig(hostKey)
	if !ok || strings.TrimSpace(deviceHost) == "" {
		return pluginConfig{}, fmt.Errorf("target host not configured")
	}

	mode, ok := pdk.GetConfig(modeKey)
	if !ok || strings.TrimSpace(mode) == "" {
		mode = "direct"
	} else {
		mode = strings.ToLower(strings.TrimSpace(mode))
	}

	serviceName, ok := pdk.GetConfig(serviceNameKey)
	if !ok || strings.TrimSpace(serviceName) == "" {
		serviceName = "Navidrome"
	}

	return pluginConfig{
		Mode:        mode,
		DeviceHost:  strings.TrimSpace(deviceHost),
		ServiceName: strings.TrimSpace(serviceName),
	}, nil
}

// trackMetadata is the JSON metadata sent alongside multipart image uploads.
type trackMetadata struct {
	TrackName   string `json:"trackName,omitempty"`
	ArtistName  string `json:"artistName,omitempty"`
	AlbumName   string `json:"albumName,omitempty"`
	ServiceName string `json:"serviceName,omitempty"`
	ItemID      string `json:"itemId,omitempty"`
}

// convertToWebP decodes an image (JPEG/PNG), center-crops and resizes it to 64x64, and encodes as lossless WebP.
func convertToWebP(imageData []byte) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	dst := image.NewRGBA(image.Rect(0, 0, tuneshineSize, tuneshineSize))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, centerSquare(src.Bounds()), draw.Over, nil)

	var buf bytes.Buffer
	if err := nativewebp.Encode(&buf, dst, nil); err != nil {
		return nil, fmt.Errorf("encode webp: %w", err)
	}

	return buf.Bytes(), nil
}

// centerSquare returns the largest centered square inside r, so non-square art is cropped instead of stretched.
func centerSquare(r image.Rectangle) image.Rectangle {
	w, h := r.Dx(), r.Dy()
	if w > h {
		x := r.Min.X + (w-h)/2
		return image.Rect(x, r.Min.Y, x+h, r.Max.Y)
	}
	y := r.Min.Y + (h-w)/2
	return image.Rect(r.Min.X, y, r.Max.X, y+w)
}

// detectImageType returns the MIME type and file extension of JPEG, PNG, or WebP data, defaulting to JPEG.
func detectImageType(data []byte) (string, string) {
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png", "png"
	case len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return "image/webp", "webp"
	default:
		return "image/jpeg", "jpg"
	}
}

// imageHash computes a fast 64-bit FNV-1a hash of image data, returned as a hex string.
func imageHash(data []byte) string {
	h := fnv.New64a()
	h.Write(data)
	return fmt.Sprintf("%016x", h.Sum64())
}

// postImage sends image bytes and metadata to the destination host (Tuneshine or Hub).
func postImage(imageData []byte, contentType, filename string, meta trackMetadata, deviceHost string) error {
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	body, formContentType, err := buildMultipartBody(imageData, contentType, filename, metaJSON)
	if err != nil {
		return fmt.Errorf("failed to build multipart body: %w", err)
	}

	url := fmt.Sprintf("http://%s/image", deviceHost)
	resp, err := host.HTTPSend(host.HTTPRequest{
		Method:    "POST",
		URL:       url,
		Headers:   map[string]string{"Content-Type": formContentType},
		Body:      body,
		TimeoutMs: 15000,
	})
	if err != nil {
		return fmt.Errorf("HTTP error posting image: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("POST /image returned %d: %s", resp.StatusCode, string(resp.Body))
	}
	return nil
}

// buildMultipartBody encodes the image and metadata fields with a random boundary.
func buildMultipartBody(imageData []byte, contentType, filename string, metaJSON []byte) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	imageHeader := make(textproto.MIMEHeader)
	imageHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="%s"`, filename))
	imageHeader.Set("Content-Type", contentType)
	part, err := w.CreatePart(imageHeader)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(imageData); err != nil {
		return nil, "", err
	}

	metaHeader := make(textproto.MIMEHeader)
	metaHeader.Set("Content-Disposition", `form-data; name="metadata"`)
	metaHeader.Set("Content-Type", "application/json")
	part, err = w.CreatePart(metaHeader)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(metaJSON); err != nil {
		return nil, "", err
	}

	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

// clearDisplay sends DELETE /image to the destination host to revert to the idle screen.
func clearDisplay(deviceHost string) error {
	url := fmt.Sprintf("http://%s/image", deviceHost)
	resp, err := host.HTTPSend(host.HTTPRequest{
		Method:    "DELETE",
		URL:       url,
		TimeoutMs: 10000,
	})
	if err != nil {
		pdk.Log(pdk.LogWarn, fmt.Sprintf("[Tuneshine] HTTP error deleting image: %v", err))
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		pdk.Log(pdk.LogWarn, fmt.Sprintf("[Tuneshine] DELETE /image returned %d: %s", resp.StatusCode, string(resp.Body)))
		return nil
	}

	pdk.Log(pdk.LogInfo, "[Tuneshine] Cleared display")
	_ = host.CacheRemove(cacheKeyLastTrack)
	return nil
}

// ============================================================================
// Scrobbler Implementation
// ============================================================================

// IsAuthorized allows all users, or only the configured user(s) if "user" is set.
func (t *tuneshine) IsAuthorized(input scrobbler.IsAuthorizedRequest) (bool, error) {
	allowed, ok := pdk.GetConfig(userKey)
	if !ok || strings.TrimSpace(allowed) == "" {
		return true, nil
	}
	for _, name := range strings.Split(allowed, ",") {
		if strings.TrimSpace(name) == input.Username {
			return true, nil
		}
	}
	return false, nil
}

// NowPlaying handles now-playing events from Subsonic clients (e.g. Feishin, Symfonium, DSub).
func (t *tuneshine) NowPlaying(input scrobbler.NowPlayingRequest) error {
	cfg, err := getConfig()
	if err != nil {
		pdk.Log(pdk.LogWarn, fmt.Sprintf("[Tuneshine] Config error: %v", err))
		return nil
	}

	cancelPendingClear()
	return t.displayTrack(input.Username, input.Track, cfg)
}

// PlaybackReport handles playback state changes from Navidrome Web UI and OpenSubsonic clients.
func (t *tuneshine) PlaybackReport(input scrobbler.PlaybackReportRequest) error {
	cfg, err := getConfig()
	if err != nil {
		pdk.Log(pdk.LogWarn, fmt.Sprintf("[Tuneshine] Config error: %v", err))
		return nil
	}

	switch input.State {
	case "playing", "starting":
		cancelPendingClear()
		return t.displayTrack(input.Username, input.Track, cfg)
	case "paused", "stopped", "expired":
		// Only clear if the paused/stopped/expired report explicitly identifies the currently displayed track.
		// If the event has no track ID (e.g. generic session disconnect) or refers to a different track, ignore it.
		lastID, found, _ := host.CacheGetString(cacheKeyLastTrack)
		if !found || input.Track.ID == "" || lastID != input.Track.ID {
			return nil
		}
		// Debounce clear in both Direct and Hub modes to avoid flicker during seeks,
		// track transitions, and temporary player reconnects.
		scheduleDelayedClear()
		return nil
	default:
		return nil
	}
}

// scheduleDelayedClear schedules a one-time delayed clear using the Navidrome scheduler.
func scheduleDelayedClear() {
	_ = host.SchedulerCancelSchedule(pauseClearScheduleID)
	_, err := host.SchedulerScheduleOneTime(pauseClearDelay, "", pauseClearScheduleID)
	if err != nil {
		pdk.Log(pdk.LogWarn, fmt.Sprintf("[Tuneshine] Failed to schedule delayed clear: %v", err))
	}
}

// cancelPendingClear cancels any pending pause-clear timer.
func cancelPendingClear() {
	_ = host.SchedulerCancelSchedule(pauseClearScheduleID)
}

// ============================================================================
// Scheduler Callback Implementation
// ============================================================================

// OnCallback is called by Navidrome when a scheduled timer fires.
func (t *tuneshine) OnCallback(req scheduler.SchedulerCallbackRequest) error {
	if req.ScheduleID == pauseClearScheduleID {
		cfg, err := getConfig()
		if err != nil {
			return nil
		}
		return clearDisplay(cfg.DeviceHost)
	}
	return nil
}

// ============================================================================
// Display Logic
// ============================================================================

// displayTrack uploads track artwork and metadata to the destination host.
func (t *tuneshine) displayTrack(username string, track scrobbler.TrackInfo, cfg pluginConfig) error {
	if track.ID == "" {
		return nil
	}

	// Check if the track changed — skip upload if already displaying this track.
	lastID, found, _ := host.CacheGetString(cacheKeyLastTrack)
	if found && lastID == track.ID {
		return nil
	}

	if err := uploadTrackImage(username, track, cfg); err != nil {
		pdk.Log(pdk.LogWarn, fmt.Sprintf("[Tuneshine] %v", err))
		return nil
	}
	_ = host.CacheSetString(cacheKeyLastTrack, track.ID, cacheTTLSeconds)
	return nil
}

// uploadTrackImage fetches artwork via SubsonicAPI and POSTs to Tuneshine (or Hub).
func uploadTrackImage(username string, track scrobbler.TrackInfo, cfg pluginConfig) error {
	// Size hint: request 64px for Direct mode, 300px for Hub mode
	sizeHint := tuneshineSize
	if cfg.Mode == "hub" {
		sizeHint = 300
	}

	_, imageData, err := host.SubsonicAPICallRaw(
		fmt.Sprintf("/getCoverArt?u=%s&id=%s&size=%d", url.QueryEscape(username), track.ID, sizeHint),
	)
	if err != nil {
		return fmt.Errorf("failed to fetch artwork: %w", err)
	}

	meta := trackMetadata{
		TrackName:   track.Title,
		ArtistName:  track.Artist,
		AlbumName:   track.Album,
		ServiceName: cfg.ServiceName,
		ItemID:      track.ID,
	}

	var uploadBytes []byte
	var contentType string
	var filename string

	if cfg.Mode == "hub" {
		// Hub mode: pass raw image bytes directly (Hub offloads 64x64 WebP conversion)
		uploadBytes = imageData
		var ext string
		contentType, ext = detectImageType(imageData)
		filename = "cover." + ext
	} else {
		// Direct mode: perform 64x64 lossless WebP conversion in plugin for physical device
		webpData, err := convertToWebP(imageData)
		if err != nil {
			return fmt.Errorf("failed to convert to WebP: %w", err)
		}
		uploadBytes = webpData
		contentType = "image/webp"
		filename = "cover.webp"
	}

	if err := postImage(uploadBytes, contentType, filename, meta, cfg.DeviceHost); err != nil {
		return err
	}

	pdk.Log(pdk.LogInfo, fmt.Sprintf("[Tuneshine] Sent track '%s' by '%s' to %s (%s mode)",
		track.Title, track.Artist, cfg.DeviceHost, cfg.Mode))
	return nil
}

// Scrobble is a no-op for Tuneshine.
func (t *tuneshine) Scrobble(_ scrobbler.ScrobbleRequest) error {
	return nil
}

func main() {}
