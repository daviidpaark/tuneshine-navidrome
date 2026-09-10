# Tuneshine Plugin for Navidrome

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Navidrome](https://img.shields.io/badge/Navidrome-Plugin-blue.svg)](https://www.navidrome.org/)
[![Go 1.25](https://img.shields.io/badge/Go-1.25-blue.svg)](https://golang.org/)

A [Navidrome](https://www.navidrome.org/) plugin that sends album art and track metadata to a [Tuneshine](https://www.tuneshine.rocks/) LED display (or a [Tuneshine Hub](https://github.com/daviidpaark/tuneshine-hub) proxy) on your local network in real time as you listen to music.

---

## The Tuneshine Ecosystem

* **[tuneshine-navidrome](https://github.com/daviidpaark/tuneshine-navidrome)** *(This repository)*: Official Navidrome plugin. Streams live playback and cover art from your Navidrome music server to Tuneshine Hub (or directly to a physical Tuneshine device).
* **[tuneshine-windows](https://github.com/daviidpaark/tuneshine-windows)**: Standalone Windows System Tray desktop companion. Hooks into Windows Media Controls (SMTC) to capture and stream real-time playback from Spotify, Apple Music, YouTube, Tidal, and local players to Tuneshine Hub (or directly to a physical Tuneshine device).
* **[tuneshine-hub](https://github.com/daviidpaark/tuneshine-hub)**: Central Docker hub service. Manages 24/7 background Spotify tracking, converts raw artwork to 64×64 WebP, arbitrates multi-source priority, and drives your physical Tuneshine device.

---

## Features

- **Two Operation Modes:**
  - **Direct to Device:** Navidrome handles 64x64 lossless WebP conversion and debouncing, then sends updates directly to a physical Tuneshine device.
  - **Tuneshine Hub:** Navidrome forwards raw cover art and metadata to a `tuneshine-hub` container, which handles image processing, debouncing, and multi-source coordination.
- Displays 64x64 album art on the Tuneshine device when a track starts playing
- Sends artist and album name as metadata
- Clears the display when paused, stopped, or expired — with debounced handling to prevent screen flickering during seeks and track transitions
- Deduplicates requests for the same track or identical cover art
- Works for all Navidrome users, with an optional allowlist to restrict which users update the display

---

## Operation Modes

| Mode | Target | Description |
| :--- | :--- | :--- |
| **`Direct to Device`** *(Default)* | Physical Tuneshine device (e.g. `192.168.1.100` or `tuneshine.local`) | Converts cover art to 64x64 lossless WebP in WASM and sends it directly to the device. Uses Navidrome scheduler to debounce screen clearing during seeks. |
| **`Tuneshine Hub`** | Tuneshine Hub (e.g. `tuneshine-hub:8585` or `<hub-ip>:8585`) | Forwards raw cover art and playback events to Hub. Hub handles image processing, debounce timers, Spotify fallback, and multi-source coordination. |

---

## Requirements

- Navidrome **v0.62.0 or later** — the `PlaybackReport` scrobbler event used by this plugin was introduced in that release.
- Tuneshine device with Firmware 2.3.0+ (or Firmware 2.7.0+ for standalone "API Mode") on the same LAN, or a `tuneshine-hub` proxy.

---

## Installation

1. Download `tuneshine.ndp` from the [Releases](https://github.com/daviidpaark/tuneshine-navidrome/releases)
2. Place it in your Navidrome plugins directory (e.g. `/data/plugins/`)
3. Go to **Settings → Plugins** and click **Rescan** to detect the plugin
4. Go to **Settings → Plugins → Tuneshine** and configure:
  - **Operation Mode** — Choose `Direct to Device` or `Tuneshine Hub`
   - **Target Host** — IP address or hostname of your physical Tuneshine (e.g. `192.168.1.100` or `tuneshine.local`) or Tuneshine Hub (e.g. `tuneshine-hub:8585` or `<hub-ip>:8585`)
   - **Service Name** — Label shown on the Tuneshine display (default: `Navidrome`)
   - **Restrict to User(s)** — Optional. Only show playback from these usernames (e.g. `user1` or `user1,user2`). Leave blank for all users.
5. Enable the plugin

---

## Building from Source

Requires Go 1.25+ with WASM support. [TinyGo](https://tinygo.org/) is also supported and produces a smaller binary.

The `Makefile` handles both — it prefers TinyGo if it is on your `PATH`, otherwise falls back to standard Go:

```sh
make package
```

Or manually:

```sh
# Standard Go
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o plugin.wasm .

# TinyGo
tinygo build -target wasip1 -buildmode=c-shared -o plugin.wasm -scheduler=none .

zip tuneshine.ndp plugin.wasm manifest.json
```

---

## Configuration

| Setting | Required | Default | Description |
|---------|----------|---------|-------------|
| `mode` | No | `direct` | Operation mode (`direct` for physical Tuneshine, `hub` for Tuneshine Hub) |
| `host` | Yes | — | Hostname or IP of the physical Tuneshine device or Tuneshine Hub |
| `servicename` | No | `Navidrome` | Music source name shown on the display |
| `user` | No | _(all users)_ | Comma-separated list of usernames allowed to update the display |

---

## Dependencies

| Package | Purpose |
|---------|--------|
| [navidrome/navidrome/plugins/pdk/go](https://github.com/navidrome/navidrome) | Navidrome Plugin Development Kit |
| [HugoSmits86/nativewebp](https://github.com/HugoSmits86/nativewebp) | Pure Go lossless WebP encoder (VP8L) |
| [golang.org/x/image](https://pkg.go.dev/golang.org/x/image) | Bilinear image scaling |

---

## AI Disclosure & Personal Project Note

> [!NOTE]
> This project was developed as a personal home lab tool with the assistance of **GitHub Copilot (Claude Sonnet / Opus)** and **Google Antigravity (Gemini Flash / Pro)** AI pair programming. It is shared publicly for the benefit of the community and other Tuneshine owners. Contributions, feedback, and issue reports are always welcome!

---

## License

MIT License. See [LICENSE](LICENSE) for details.
