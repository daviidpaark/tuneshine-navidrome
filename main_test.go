package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/HugoSmits86/nativewebp"
	"github.com/navidrome/navidrome/plugins/pdk/go/host"
	"github.com/navidrome/navidrome/plugins/pdk/go/pdk"
	"github.com/navidrome/navidrome/plugins/pdk/go/scrobbler"
	"github.com/stretchr/testify/mock"
)

func createTestPNG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestConvertToWebP(t *testing.T) {
	pngData := createTestPNG(120, 120)
	webpData, err := convertToWebP(pngData)
	if err != nil {
		t.Fatalf("convertToWebP failed: %v", err)
	}

	if len(webpData) == 0 {
		t.Fatal("expected non-empty WebP data")
	}

	// Decode back to check dimensions
	decoded, err := nativewebp.Decode(bytes.NewReader(webpData))
	if err != nil {
		t.Fatalf("failed to decode generated WebP: %v", err)
	}

	bounds := decoded.Bounds()
	if bounds.Dx() != tuneshineSize || bounds.Dy() != tuneshineSize {
		t.Errorf("expected dimensions %dx%d, got %dx%d", tuneshineSize, tuneshineSize, bounds.Dx(), bounds.Dy())
	}
}

func TestImageHash(t *testing.T) {
	img1 := createTestPNG(64, 64)
	img2 := createTestPNG(64, 64)
	img3 := createTestPNG(32, 32)

	h1 := imageHash(img1)
	h2 := imageHash(img2)
	h3 := imageHash(img3)

	if h1 != h2 {
		t.Errorf("expected identical hashes for identical images, got %s vs %s", h1, h2)
	}
	if h1 == h3 {
		t.Errorf("expected different hashes for different images, both got %s", h1)
	}
}

func TestIsAuthorized(t *testing.T) {
	p := &tuneshine{}

	// Case 1: No user filter configured (allow all)
	pdk.ResetMock()
	pdk.PDKMock.On("GetConfig", userKey).Return("", false)
	auth, err := p.IsAuthorized(scrobbler.IsAuthorizedRequest{Username: "alice"})
	if err != nil || !auth {
		t.Errorf("expected authorized=true, got %v (err: %v)", auth, err)
	}

	// Case 2: Whitelist configured, user matches
	pdk.ResetMock()
	pdk.PDKMock.On("GetConfig", userKey).Return("alice, bob", true)
	auth, err = p.IsAuthorized(scrobbler.IsAuthorizedRequest{Username: "bob"})
	if err != nil || !auth {
		t.Errorf("expected bob to be authorized, got %v", auth)
	}

	// Case 3: Whitelist configured, user not in list
	pdk.ResetMock()
	pdk.PDKMock.On("GetConfig", userKey).Return("alice, bob", true)
	auth, err = p.IsAuthorized(scrobbler.IsAuthorizedRequest{Username: "charlie"})
	if err != nil || auth {
		t.Errorf("expected charlie to be rejected, got %v", auth)
	}
}

func TestNowPlaying(t *testing.T) {
	p := &tuneshine{}

	pdk.ResetMock()
	host.CacheMock.ExpectedCalls = nil
	host.CacheMock.Calls = nil
	host.SchedulerMock.ExpectedCalls = nil
	host.SchedulerMock.Calls = nil
	host.HTTPMock.ExpectedCalls = nil
	host.HTTPMock.Calls = nil
	host.SubsonicAPIMock.ExpectedCalls = nil
	host.SubsonicAPIMock.Calls = nil

	pdk.PDKMock.On("GetConfig", hostKey).Return("192.168.1.100", true)
	pdk.PDKMock.On("GetConfig", modeKey).Return("hub", true)
	pdk.PDKMock.On("GetConfig", serviceNameKey).Return("Navidrome", true)
	pdk.PDKMock.On("Log", pdk.LogInfo, "[Tuneshine] Sent track 'Organon' by 'Men I Trust' to 192.168.1.100 (hub mode)").Return()

	host.SchedulerMock.On("CancelSchedule", pauseClearScheduleID).Return(nil)
	host.CacheMock.On("GetString", cacheKeyLastTrack).Return("", false, nil)
	host.CacheMock.On("SetString", cacheKeyLastTrack, "track-123", int64(cacheTTLSeconds)).Return(nil)

	testImg := createTestPNG(64, 64)
	host.SubsonicAPIMock.On("CallRaw", "/getCoverArt?u=test+user%26name&id=track-123&size=300").
		Return("image/png", testImg, nil)

	host.HTTPMock.On("Send", mock.MatchedBy(func(req host.HTTPRequest) bool {
		return req.Method == "POST" && req.URL == "http://192.168.1.100/image"
	})).Return(&host.HTTPResponse{StatusCode: 200}, nil)

	req := scrobbler.NowPlayingRequest{
		Username: "test user&name",
		Track: scrobbler.TrackInfo{
			ID:     "track-123",
			Title:  "Organon",
			Artist: "Men I Trust",
			Album:  "Untourable Album",
		},
		Position: 0,
	}

	err := p.NowPlaying(req)
	if err != nil {
		t.Fatalf("NowPlaying failed: %v", err)
	}

	pdk.PDKMock.AssertExpectations(t)
	host.HTTPMock.AssertExpectations(t)
}

func TestPlaybackReport_ExpiredDifferentTrackIgnored(t *testing.T) {
	p := &tuneshine{}

	pdk.ResetMock()
	host.CacheMock.ExpectedCalls = nil
	host.CacheMock.Calls = nil
	host.SchedulerMock.ExpectedCalls = nil
	host.SchedulerMock.Calls = nil

	pdk.PDKMock.On("GetConfig", hostKey).Return("192.168.1.100", true)
	pdk.PDKMock.On("GetConfig", modeKey).Return("hub", true)
	pdk.PDKMock.On("GetConfig", serviceNameKey).Return("Navidrome", true)

	// Display is currently showing track-active
	host.CacheMock.On("GetString", cacheKeyLastTrack).Return("track-active", true, nil)

	// An expired event arrives for an OLD session playing track-old
	req := scrobbler.PlaybackReportRequest{
		Username: "user",
		Track: scrobbler.TrackInfo{
			ID: "track-old",
		},
		State: "expired",
	}

	err := p.PlaybackReport(req)
	if err != nil {
		t.Fatalf("PlaybackReport failed: %v", err)
	}

	// Delayed clear should NOT have been scheduled
	host.SchedulerMock.AssertNotCalled(t, "ScheduleOneTime", mock.Anything, mock.Anything, mock.Anything)
}

func TestPlaybackReport_ExpiredEmptyTrackIgnored(t *testing.T) {
	p := &tuneshine{}

	pdk.ResetMock()
	host.CacheMock.ExpectedCalls = nil
	host.CacheMock.Calls = nil
	host.SchedulerMock.ExpectedCalls = nil
	host.SchedulerMock.Calls = nil

	pdk.PDKMock.On("GetConfig", hostKey).Return("192.168.1.100", true)
	pdk.PDKMock.On("GetConfig", modeKey).Return("hub", true)
	pdk.PDKMock.On("GetConfig", serviceNameKey).Return("Navidrome", true)

	// Display is currently showing track-active
	host.CacheMock.On("GetString", cacheKeyLastTrack).Return("track-active", true, nil)

	// A generic disconnect event arrives with NO track ID
	req := scrobbler.PlaybackReportRequest{
		Username: "user",
		Track:    scrobbler.TrackInfo{},
		State:    "expired",
	}

	err := p.PlaybackReport(req)
	if err != nil {
		t.Fatalf("PlaybackReport failed: %v", err)
	}

	// Delayed clear should NOT have been scheduled
	host.SchedulerMock.AssertNotCalled(t, "ScheduleOneTime", mock.Anything, mock.Anything, mock.Anything)
}

func TestPlaybackReport_PausedCurrentTrackSchedulesClear(t *testing.T) {
	p := &tuneshine{}

	pdk.ResetMock()
	host.CacheMock.ExpectedCalls = nil
	host.CacheMock.Calls = nil
	host.SchedulerMock.ExpectedCalls = nil
	host.SchedulerMock.Calls = nil

	pdk.PDKMock.On("GetConfig", hostKey).Return("192.168.1.100", true)
	pdk.PDKMock.On("GetConfig", modeKey).Return("hub", true)
	pdk.PDKMock.On("GetConfig", serviceNameKey).Return("Navidrome", true)

	// Display is currently showing track-active
	host.CacheMock.On("GetString", cacheKeyLastTrack).Return("track-active", true, nil)

	host.SchedulerMock.On("CancelSchedule", pauseClearScheduleID).Return(nil)
	host.SchedulerMock.On("ScheduleOneTime", int32(5), "", pauseClearScheduleID).Return("job-1", nil)

	// User pauses the CURRENT track
	req := scrobbler.PlaybackReportRequest{
		Username: "user",
		Track: scrobbler.TrackInfo{
			ID: "track-active",
		},
		State: "paused",
	}

	err := p.PlaybackReport(req)
	if err != nil {
		t.Fatalf("PlaybackReport failed: %v", err)
	}

	host.SchedulerMock.AssertExpectations(t)
}

func TestNowPlaying_Deduplication(t *testing.T) {
	p := &tuneshine{}

	pdk.ResetMock()
	host.CacheMock.ExpectedCalls = nil
	host.CacheMock.Calls = nil
	host.SchedulerMock.ExpectedCalls = nil
	host.SchedulerMock.Calls = nil
	host.HTTPMock.ExpectedCalls = nil
	host.HTTPMock.Calls = nil
	host.SubsonicAPIMock.ExpectedCalls = nil
	host.SubsonicAPIMock.Calls = nil

	pdk.PDKMock.On("GetConfig", hostKey).Return("192.168.1.100", true)
	pdk.PDKMock.On("GetConfig", modeKey).Return("hub", true)
	pdk.PDKMock.On("GetConfig", serviceNameKey).Return("Navidrome", true)

	host.SchedulerMock.On("CancelSchedule", pauseClearScheduleID).Return(nil)
	// Track is ALREADY in cache
	host.CacheMock.On("GetString", cacheKeyLastTrack).Return("track-123", true, nil)

	req := scrobbler.NowPlayingRequest{
		Username: "user",
		Track: scrobbler.TrackInfo{
			ID:     "track-123",
			Title:  "Organon",
			Artist: "Men I Trust",
			Album:  "Untourable Album",
		},
		Position: 10,
	}

	err := p.NowPlaying(req)
	if err != nil {
		t.Fatalf("NowPlaying failed: %v", err)
	}

	// Should not have called SubsonicAPI or HTTP Send
	host.SubsonicAPIMock.AssertNotCalled(t, "CallRaw", mock.Anything)
	host.HTTPMock.AssertNotCalled(t, "Send", mock.Anything)
}

