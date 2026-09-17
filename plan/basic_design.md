# zoomcut: Basic Design & System Architecture

A semi-automated command-line and TUI tool written in Go to trim Zoom talk recordings, enrich metadata via AI, and publish them directly to YouTube with cut-adjusted chapter markers.

---

## 1. Vision & Core Goals

Zoom recordings of academic seminars, tech talks, and webinars consistently suffer from predictable friction:
- **Fluff & dead air**: 2–5 minutes of pre-talk banter, microphone checks, "can you see my screen", technical pauses, and rambling post-talk Q&A or administrative chatter.
- **Manual editing overhead**: Opening heavy video editing software (Premiere, DaVinci, iMovie) just to make 2 or 3 slice cuts is slow and tedious.
- **Loss of chapters & metadata**: Manual upload to YouTube requires re-entering titles, speaker bios, abstracts, and recalculating chapter timestamps by hand.

**`zoomcut`** automates this entire pipeline into a fast, terminal-native workflow:
1. **Ingest**: Consumes Zoom MP4 and WebVTT transcript (plus an optional talk announcement URL).
2. **AI Analysis**:
   - Identifies candidate cuts (intro preamble, dead air, trailing Q&A).
   - Scrapes the announcement URL to extract talk title, speaker, affiliation, and abstract.
3. **Interactive TUI**: Review and fine-tune cuts in a true-color terminal interface with transcript navigation, segment toggling, and lightweight `ffplay` preview.
4. **Export & Publish**:
   - Executes precise cuts via FFmpeg (lossless keyframe copy or fast smart re-encode).
   - Automatically recalculates chapter timestamps adjusted for excised segments.
   - Uploads directly to YouTube via resumable OAuth2 API with formatted description and chapters.

---

## 2. User Workflows

### Standard End-to-End Workflow

```bash
zoomcut \
  --video recording.mp4 \
  --transcript recording.vtt \
  --url "https://seminar-series.org/talks/2026-spring-talk"
```

1. **Phase 1: Ingestion & AI Pre-processing**
   - Parses the WebVTT file into time-indexed speaker segments.
   - Concurrently fetches the URL and queries the LLM to extract metadata (Speaker, Title, Abstract, Suggested Tags).
   - Sends the transcript with timestamp anchors to the LLM to mark proposed cut ranges (Preamble, Dead Air, Post-talk Q&A).

2. **Phase 2: Interactive TUI (Bubble Tea)**
   - **Screen 1: Cut Review**:
     - Visual transcript view with color-coded segments:
       - 🟢 **Kept** (default content)
       - 🔴 **Cut** (marked for deletion)
       - 🟡 **Review** (AI suggestions needing confirmation)
     - Hotkeys: `Space` (toggle cut/keep), `[` / `]` (adjust boundary), `p` (instant `ffplay` preview from cursor timestamp), `Tab` (jump between cut regions).
   - **Screen 2: Metadata & Chapter Review**:
     - Edit extracted Title, Description, Tags, and YouTube Visibility (`unlisted`, `public`, `private`).
     - Live preview of auto-adjusted chapter timestamps (recalculated based on active cuts).
   - **Screen 3: Render & Upload**:
     - Real-time FFmpeg slicing progress bar.
     - YouTube chunked resumable upload progress bar (bytes uploaded, speed, ETA).
     - Live YouTube URL output on completion.

### Local-Only Workflow (No YouTube Upload)

```bash
zoomcut --video recording.mp4 --transcript recording.vtt --output clean.mp4
```
- Performs cut review and renders `clean.mp4` locally without prompting for YouTube OAuth.
- Dumps `clean_chapters.txt` with formatted timestamps for manual copy-pasting.

---

## 3. System Architecture & Components

```
zoomcut/
├── cmd/
│   └── zoomcut/
│       └── main.go          # CLI entry point, flag parsing, orchestration
├── internal/
│   ├── ai/                  # AI client (Gemini / Anthropic / OpenAI / Ollama)
│   │   ├── client.go        # Unified LLM provider interface
│   │   ├── cuts.go          # Preamble, dead air & Q&A detection prompt
│   │   └── metadata.go      # Webpage + transcript metadata extraction prompt
│   ├── config/              # User preferences, API keys, OAuth tokens
│   │   └── config.go        # ~/.config/zoomcut/config.json loader & store
│   ├── cutter/              # Video processing engine
│   │   ├── chapters.go      # Chapter timestamp recalculation post-cut
│   │   ├── ffmpeg.go        # FFmpeg process wrapper & progress parser
│   │   └── probe.go         # ffprobe keyframe & stream duration analyzer
│   ├── metadata/            # URL scraper & text sanitizer
│   │   └── fetcher.go       # HTTP client, readability/HTML text extraction
│   ├── model/               # Core domain types
│   │   ├── cut.go           # CutAction, CutSegment, CutList
│   │   ├── state.go         # AppState and session state
│   │   ├── transcript.go    # SubtitleCue, Transcript
│   │   └── video.go         # VideoMetadata, YouTubeOptions
│   ├── ui/                  # Bubble Tea TUI
│   │   ├── app.go           # Root Tea model & state machine transitions
│   │   ├── keys.go          # Keybindings & help definitions
│   │   ├── styles.go        # Lip Gloss true-color themes & palettes
│   │   ├── view_cuts.go     # Transcript cut reviewer screen
│   │   ├── view_meta.go     # Metadata & chapter editor screen
│   │   └── view_prog.go     # Render & upload progress screen
│   ├── vtt/                 # WebVTT / SRT parser
│   │   └── parser.go        # Streaming VTT parser & serializer
│   └── youtube/             # YouTube Data API v3 integration
│       ├── auth.go          # OAuth2 web loop & token cache
│       └── uploader.go      # Resumable video upload with progress callback
├── plan/
│   └── basic_design.md      # This document
├── go.mod
└── go.sum
```

---

## 4. Detailed Component Design

### 4.1. Domain Models (`internal/model`)

```go
type CutAction int

const (
    ActionKeep CutAction = iota
    ActionCut
    ActionReview
)

type SubtitleCue struct {
    Index     int
    Start     time.Duration
    End       time.Duration
    Speaker   string
    Text      string
    Action    CutAction
    CutReason string // e.g., "Preamble banter", "Screen sharing setup", "Q&A"
}

type CutInterval struct {
    Start time.Duration
    End   time.Duration
}

type VideoMetadata struct {
    Title       string
    Speaker     string
    Affiliation string
    Abstract    string
    Tags        []string
    Privacy     string // "unlisted", "public", "private"
    Chapters    []ChapterMarker
}

type ChapterMarker struct {
    OriginalTime time.Duration
    AdjustedTime time.Duration
    Title        string
}
```

### 4.2. VTT Parser (`internal/vtt`)
- WebVTT cues follow the standard format:
  ```text
  00:01:23.450 --> 00:01:28.100
  <v Speaker Name>Good morning everyone, let's wait two minutes.</v>
  ```
- Parser streams cues sequentially, normalizes timestamps into `time.Duration`, strips voice formatting tags, and retains speaker names.

### 4.3. AI Intelligence Layer (`internal/ai`)
- **Configurable backends**: Supports Google Gemini (via `google.golang.org/genai` or standard REST), Anthropic, OpenAI, or local Ollama.
- **Structured Output**: Uses JSON schema enforcement to guarantee parsing reliability:
  1. **Cut Proposal**: Returns an array of intervals to excise with confidence scores and reasoning.
  2. **Talk Metadata Extraction**: Takes sanitized HTML text from the talk announcement page + first 10 minutes of transcript -> returns structured title, speaker, abstract, and topics.

### 4.4. Video Processing Engine (`internal/cutter`)
Cutting video without re-encoding requires handling keyframe (I-frame) boundaries:
- **Mode A: Lossless Stream Copy (Default)**:
  - Invokes `ffprobe` with `-show_frames -select_streams v -show_entries frame=pkt_pts_time,key_frame` to detect keyframe timestamps around cut points.
  - Snaps cut boundaries to the nearest I-frame to prevent frozen video or PTS/DTS sync errors.
  - Slices segments with `-c copy -avoid_negative_ts make_zero`.
  - Joins remaining segments via the FFmpeg `concat` demuxer.
- **Mode B: Accurate Frame Cut (Re-encode)**:
  - When sub-second precision is required or keyframes are too sparse:
  - Runs filter graph `[0:v]trim=...[v0]; [0:a]atrim=...[a0]; ... concat=n=K:v=1:a=1` with hardware acceleration or `libx264 -crf 20 -preset fast`.
- **Chapter Offset Adjustment**:
  - Whenever a segment $[S_i, E_i]$ is cut, any chapter $T$ occurring after $E_i$ shifts left by $\sum (E_i - S_i)$.
  - Formats valid YouTube description timestamps:
    ```text
    00:00 Introduction
    02:14 Background
    ...
    ```

### 4.5. Interactive TUI (`internal/ui`)
Built with **Bubble Tea** and **Lip Gloss** with 24-bit True Color support:
- **Responsive Layout**: Adapts to terminal dimensions (`tea.WindowSizeMsg`).
- **Main Viewport**: High-performance scrolling transcript view with line-level action highlights.
- **Sidebar**:
  - Duration counter: `Original: 58m 12s | Clean: 49m 04s (-9m 08s)`.
  - Cut segments list with jump-to-segment navigation.
  - Mini-timeline representation.
- **Preview Integration**: Pressing `p` spawns `ffplay -ss <timestamp> -autoexit -nodisp` (or with video window) so the user can verify audio/video before cutting.

### 4.6. YouTube API & OAuth2 (`internal/youtube`)
- Uses official `google.golang.org/api/youtube/v3`.
- **Client Credentials**: Looks for `~/.config/zoomcut/client_secrets.json`.
- **One-time Browser Auth**: Spins up a local loopback listener (`http://localhost:8085/oauth2callback`), prompts user in browser, exchanges auth code for token, and writes encrypted/restricted token to `~/.config/zoomcut/youtube_token.json`.
- **Resumable Upload**: Implements chunked upload using `googleapi.MediaOption` with byte tracking callbacks to feed Bubble Tea progress bars.

---

## 5. Engineering Standards & Quality Gates

In accordance with `/home/sariel/prog/standards/go/GUIDELINES.md`:
- **Nesting Depth**: Maximum 4 levels. Guard clauses and early returns on all error paths.
- **Decision Complexity**: Functions capped at 15 decision branches.
- **Function Sizing**:
  - Domain logic: 20–60 lines (max 110).
  - TUI Declarative views: 40–100 lines (max 160).
  - Key event dispatchers: 50–120 lines (max 200).
- **Tooling Checks**:
  - `go-audit` for sizing and cognitive complexity.
  - `go-static-analysis` (`gocritic`, `shadow`, `revive`) before commits.
  - Standard formatting with `gofmt -s` and `staticcheck`.

---

## 6. Implementation Milestones

1. **Milestone 1: Foundations & Models**
   - Core domain models (`model/`).
   - WebVTT parser and unit tests (`vtt/`).
2. **Milestone 2: FFmpeg Cutting & Chapter Math**
   - Probe, lossless slice & concat pipeline (`cutter/`).
   - Chapter timestamp offset recalculation (`cutter/chapters.go`).
3. **Milestone 3: AI Cut & Metadata Extractor**
   - LLM client with structured JSON parsing (`ai/`).
   - URL scraper and sanitizer (`metadata/`).
4. **Milestone 4: Interactive TUI**
   - Bubble Tea multi-screen model, Lip Gloss styling (`ui/`).
   - Transcript viewer, keybindings, cut toggling, `ffplay` preview.
5. **Milestone 5: YouTube Upload & Integration**
   - OAuth2 loopback authentication flow.
   - Resumable upload with progress bar.
   - End-to-end orchestration CLI.
