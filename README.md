# talk_cut

**talk_cut** is an interactive CLI and True-Color Bubble Tea terminal application for editing recorded talks and lectures (Zoom recording bundles), automatically detecting candidate cuts (intro banter, dead pauses, Q&A) via an LLM, previewing and adjusting cuts in a split-pane TUI, slicing/splicing with FFmpeg, recalculating YouTube chapters, and publishing to YouTube with synchronized captions.

---

## Key Features

- **Zoom Bundle Ingestion**: Pass a recording directory (e.g. `talk_cut examples/26_09_08/`). Automatically detects video feeds, WebVTT transcripts, and chat logs.
- **Smart Layout Selection**: Automatically prioritizes presentation video feeds (`--layout slides`, `clean`, `speaker`, or `gallery`).
- **AI Cut & Fluff Detection**: Uses OpenRouter (e.g. Gemini 2.5 Flash Lite) with token-optimized transcript sampling to pinpoint intro preamble, mic checks, and post-talk outro/Q&A.
- **True-Color Terminal Interface**:
  - **Cut Reviewer**: Scrolling transcript on the left, live statistics and cut interval summary on the right.
  - **Live Preview**: Press `p` to open an SDL `ffplay` video window starting at any subtitle cue.
  - **Metadata & Chapters**: Edit talk title, speaker, abstract, and tags while viewing live, recalculated YouTube chapters (guarantees `00:00` start).
  - **Progress Dashboard**: Live FFmpeg rendering bar and stage reporting.
- **Lossless FFmpeg Engine**: Slices kept intervals and splices them via FFmpeg's `concat` demuxer (`-c copy`) without quality loss or re-encoding.
- **Export Artifacts**: Produces trimmed MP4 video, retimed `.vtt` subtitles, and a YouTube-ready `_chapters.txt` file.
- **Multi-Channel YouTube Publishing**: Built-in OAuth2 desktop authorization supporting multiple distinct channels (`--channel <name>`), resumable video uploads, and caption track syncing.
- **Strict Security Model**: Zero raw API keys in configuration files. Secrets and tokens reside in `~/.config/auth/` with `0600` permissions.

---

## Requirements

- **Go**: 1.22 or newer
- **FFmpeg**: `ffmpeg`, `ffprobe`, and `ffplay` installed and available in `$PATH`
- **OpenRouter API Key** (optional, for AI cuts): Stored in `~/.config/auth/openrouter_api_key`

---

## Installation

Clone the repository and run:

```bash
make install
```

This validates all quality gates and copies the binary to `~/bin/talk_cut`. Ensure `~/bin` is in your `$PATH`.

---

## Quick Start

### 1. Dry-Run Inspection
Inspect a Zoom recording bundle, detect cuts, and print statistics without opening the TUI:

```bash
talk_cut --dry-run examples/26_09_08/
```

### 2. Interactive Cut Review
Launch the interactive terminal interface:

```bash
talk_cut examples/26_09_08/
```

With clean presentation slides (no speaker thumbnail):
```bash
talk_cut --layout clean examples/26_09_08/
```

With seminar announcement web page metadata scraping:
```bash
talk_cut -u "https://seminar.example.edu/talks/2026/linear-approximation" examples/26_09_08/
```

---

## TUI Keyboard Shortcuts

### Cut Review Screen
| Key | Action |
|---|---|
| <kbd>j</kbd> / <kbd>k</kbd> / <kbd>↓</kbd> / <kbd>↑</kbd> | Move cursor down / up across cues |
| <kbd>PgDn</kbd> / <kbd>PgUp</kbd> | Scroll page down / up |
| <kbd>g</kbd> / <kbd>G</kbd> | Jump to top / bottom of transcript |
| <kbd>Space</kbd> / <kbd>x</kbd> | Toggle cue between `KEEP` and `CUT` |
| <kbd>p</kbd> | Launch `ffplay` video preview starting at current cue |
| <kbd>n</kbd> / <kbd>N</kbd> | Jump to next / previous cut region |
| <kbd>Tab</kbd> / <kbd>Enter</kbd> | Advance to Metadata & Chapter editor |
| <kbd>?</kbd> | Toggle help overlay |
| <kbd>q</kbd> / <kbd>Ctrl+C</kbd> | Quit `talk_cut` |

### Metadata & Chapter Editor Screen
| Key | Action |
|---|---|
| <kbd>Tab</kbd> / <kbd>↓</kbd> | Focus next input field |
| <kbd>Shift+Tab</kbd> / <kbd>↑</kbd> | Focus previous input field |
| <kbd>Space</kbd> | Cycle privacy status (`unlisted` ↔ `public` ↔ `private`) |
| <kbd>Esc</kbd> | Return to Cut Review screen |
| <kbd>Ctrl+R</kbd> / <kbd>Enter</kbd> (on commit) | Start FFmpeg slicing and rendering |

### Progress & Completion Screen
| Key | Action |
|---|---|
| <kbd>p</kbd> | Play completed cut video in `ffplay` |
| <kbd>q</kbd> / <kbd>Esc</kbd> | Exit `talk_cut` |

---

## Multi-Channel YouTube Publishing

### 1. Authorize a YouTube Channel
To configure YouTube publishing for a channel (e.g. `seminar`):

```bash
talk_cut auth --channel seminar /path/to/client_secrets.json
```

1. Starts a local loopback server on `127.0.0.1`.
2. Opens your browser for Google OAuth consent.
3. Obtains the token and saves it to `~/.config/auth/youtube_seminar.json` (permissions `0600`).
4. Updates `~/.config/talk_cut/config.json` with channel pointers.

### 2. Upload Video and Captions
Publish directly to YouTube upon completion:

```bash
talk_cut --upload --channel seminar examples/26_09_08/
```

If `--channel` is omitted, the configured `default_channel` is used.

---

## Configuration

Configuration is located at `~/.config/talk_cut/config.json`:

```json
{
  "key_file": "~/.config/auth/openrouter_api_key",
  "model": "google/gemini-2.5-flash-lite",
  "base_url": "https://openrouter.ai/api/v1",
  "default_privacy": "unlisted",
  "preferred_layout": "slides",
  "default_channel": "seminar",
  "channels": {
    "seminar": "~/.config/auth/youtube_seminar.json"
  }
}
```

### Environment Variables
- `TALK_CUT_KEY_FILE`: Override OpenRouter API key file path.
- `TALK_CUT_MODEL`: Override OpenRouter AI model.
- `OPENROUTER_API_KEY`: Direct API key override.
- `TALK_CUT_YOUTUBE_SECRETS`: Override Google client secrets JSON path.
- `TALK_CUT_YOUTUBE_TOKEN`: Override OAuth token file path.

---

## Development & Verification

All scripts are written in Ruby (`tools/*.rb`). Commands must adhere to the Go sizing and cognitive complexity standards (`go-audit`, `go-static-analysis`):

```bash
make check     # Quality gate: formatting, vet, staticcheck, go-audit, tests, build
make review    # Deep static review: dupl, gocritic, shadow, revive, govulncheck
make test      # Run all unit tests
make wip       # Checkpoint WIP commit
make bump      # Bump patch version and install
make bump-minor# Bump minor version and install
```

---

## License

MIT License.
