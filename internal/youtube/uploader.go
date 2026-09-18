// Package youtube implements YouTube OAuth2 authorization, resumable video upload, and caption synchronization.
package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"talk_cut/internal/model"
)

var (
	videosEndpoint        = "https://www.googleapis.com/upload/youtube/v3/videos?uploadType=resumable&part=snippet,status"
	captionsEndpoint      = "https://www.googleapis.com/upload/youtube/v3/captions?uploadType=multipart&part=snippet"
	videoStatusEndpoint   = "https://www.googleapis.com/youtube/v3/videos?part=snippet,status,processingDetails,contentDetails"
	videoUpdateEndpoint   = "https://www.googleapis.com/youtube/v3/videos?part=snippet,status"
	channelsEndpoint      = "https://www.googleapis.com/youtube/v3/channels?part=contentDetails&mine=true"
	playlistItemsEndpoint = "https://www.googleapis.com/youtube/v3/playlistItems?part=snippet"
)

// VideoVerification contains the publication, processing status, and short link for a YouTube video.
type VideoVerification struct {
	VideoID          string        `json:"id"`
	Title            string        `json:"title"`
	UploadStatus     string        `json:"upload_status"`     // "uploaded", "processed", "rejected", "failed"
	PrivacyStatus    string        `json:"privacy_status"`    // "public", "unlisted", "private"
	ProcessingStatus string        `json:"processing_status"` // "processing", "succeeded", "failed", "terminated"
	ShortURL         string        `json:"short_url"`         // "https://youtu.be/<id>"
	WatchURL         string        `json:"watch_url"`         // "https://www.youtube.com/watch?v=<id>"
	Duration         time.Duration `json:"duration,omitempty"`
	DurationRaw      string        `json:"duration_raw,omitempty"`
}

// UploadOptions specifies video and metadata parameters for YouTube upload.
type UploadOptions struct {
	VideoPath  string
	Metadata   model.TalkMetadata
	OnProgress func(bytesSent, totalBytes int64, percent float64)
}

// UploadResult contains the published YouTube video identifiers.
type UploadResult struct {
	VideoID  string `json:"id"`
	VideoURL string `json:"video_url"`
}

type countingReader struct {
	reader     io.Reader
	totalBytes int64
	bytesRead  int64
	onProgress func(bytesSent, totalBytes int64, percent float64)
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.reader.Read(p)
	if n > 0 {
		cr.bytesRead += int64(n)
		if cr.onProgress != nil && cr.totalBytes > 0 {
			pct := (float64(cr.bytesRead) / float64(cr.totalBytes)) * 100.0
			cr.onProgress(cr.bytesRead, cr.totalBytes, pct)
		}
	}
	return n, err
}

// UploadVideo executes a resumable chunked upload of the video file to YouTube.
func UploadVideo(ctx context.Context, client *http.Client, opts UploadOptions) (*UploadResult, error) {
	stat, err := os.Stat(opts.VideoPath)
	if err != nil {
		return nil, fmt.Errorf("stat video file %q: %w", opts.VideoPath, err)
	}
	totalSize := stat.Size()

	uploadURL, err := initiateResumableUpload(ctx, client, opts.Metadata, totalSize)
	if err != nil {
		return nil, fmt.Errorf("initiating upload session: %w", err)
	}

	return uploadVideoPayload(ctx, client, uploadURL, opts.VideoPath, totalSize, opts.OnProgress)
}

// initiateResumableUpload sends initial metadata to obtain the unique resumable upload location.
func initiateResumableUpload(ctx context.Context, client *http.Client, meta model.TalkMetadata, size int64) (string, error) {
	privacy := strings.ToLower(meta.Privacy)
	if privacy != "unlisted" && privacy != "private" {
		privacy = "public"
	}

	payload := map[string]any{
		"snippet": map[string]any{
			"title":       sanitizeYouTubeText(meta.Title),
			"description": sanitizeYouTubeText(meta.BuildYouTubeDescription()),
			"tags":        meta.Tags,
			"categoryId":  "27", // Education
		},
		"status": map[string]any{
			"privacyStatus":           privacy,
			"selfDeclaredMadeForKids": false,
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshaling metadata payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, videosEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("creating initiation request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Length", fmt.Sprintf("%d", size))
	req.Header.Set("X-Upload-Content-Type", "video/mp4")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending upload initiation request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("initiation failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	uploadURL := resp.Header.Get("Location")
	if uploadURL == "" {
		return "", fmt.Errorf("response missing 'Location' upload header")
	}

	return uploadURL, nil
}

// uploadVideoPayload streams the raw video bytes to the resumable location URL.
func uploadVideoPayload(
	ctx context.Context,
	client *http.Client,
	uploadURL, videoPath string,
	totalSize int64,
	onProgress func(bytesSent, totalBytes int64, percent float64),
) (*UploadResult, error) {
	file, err := os.Open(videoPath)
	if err != nil {
		return nil, fmt.Errorf("opening video file: %w", err)
	}
	defer file.Close()

	reader := &countingReader{
		reader:     file,
		totalBytes: totalSize,
		onProgress: onProgress,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, reader)
	if err != nil {
		return nil, fmt.Errorf("creating payload request: %w", err)
	}
	req.ContentLength = totalSize
	req.Header.Set("Content-Type", "video/mp4")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("streaming video bytes: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("upload rejected (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parsing uploaded video id: %w", err)
	}

	return &UploadResult{
		VideoID:  result.ID,
		VideoURL: fmt.Sprintf("https://youtu.be/%s", result.ID),
	}, nil
}

// UploadCaption inserts a WebVTT caption track into an existing YouTube video.
func UploadCaption(ctx context.Context, client *http.Client, videoID, vttPath, language, trackName string) error {
	vttData, err := os.ReadFile(vttPath)
	if err != nil {
		return fmt.Errorf("reading caption file %q: %w", vttPath, err)
	}

	if language == "" {
		language = "en"
	}
	if trackName == "" {
		trackName = "English"
	}

	metadata := map[string]any{
		"snippet": map[string]any{
			"videoId":  videoID,
			"language": language,
			"name":     trackName,
			"isDraft":  false,
		},
	}
	metaBytes, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshaling caption metadata: %w", err)
	}

	return performMultipartCaptionUpload(ctx, client, metaBytes, vttData)
}

// performMultipartCaptionUpload sends the metadata and vtt caption file in a multipart request.
func performMultipartCaptionUpload(ctx context.Context, client *http.Client, metaBytes, vttData []byte) error {
	boundary := "-------talk_cut_caption_boundary"
	var body bytes.Buffer

	body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	body.WriteString("Content-Type: application/json; charset=UTF-8\r\n\r\n")
	body.Write(metaBytes)
	body.WriteString("\r\n")

	body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	body.WriteString("Content-Type: text/vtt\r\n\r\n")
	body.Write(vttData)
	body.WriteString(fmt.Sprintf("\r\n--%s--\r\n", boundary))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, captionsEndpoint, &body)
	if err != nil {
		return fmt.Errorf("creating caption request: %w", err)
	}

	req.Header.Set("Content-Type", fmt.Sprintf("multipart/related; boundary=%s", boundary))
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("sending caption request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("caption upload failed (status %d): %s", resp.StatusCode, string(respBytes))
	}

	return nil
}

// UpdateVideoMetadata updates the title, description, tags, and privacy of an existing YouTube video.
func UpdateVideoMetadata(ctx context.Context, client *http.Client, videoID string, meta model.TalkMetadata) (*VideoVerification, error) {
	if videoID == "" {
		return nil, fmt.Errorf("video ID cannot be empty")
	}

	privacy := strings.ToLower(meta.Privacy)
	if privacy != "unlisted" && privacy != "private" {
		privacy = "public"
	}

	payload := map[string]any{
		"id": videoID,
		"snippet": map[string]any{
			"title":       sanitizeYouTubeText(meta.Title),
			"description": sanitizeYouTubeText(meta.BuildYouTubeDescription()),
			"tags":        meta.Tags,
			"categoryId":  "27", // Education
		},
		"status": map[string]any{
			"privacyStatus": privacy,
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling update payload: %w", err)
	}

	reqURL := videoUpdateEndpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating update request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending update request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("update rejected (status %d): %s", resp.StatusCode, string(respBody))
	}

	return VerifyVideo(ctx, client, videoID)
}

// VerifyVideo queries YouTube Data API to confirm video registration and processing status.
func VerifyVideo(ctx context.Context, client *http.Client, videoID string) (*VideoVerification, error) {
	if videoID == "" {
		return nil, fmt.Errorf("video ID cannot be empty")
	}

	reqURL := fmt.Sprintf("%s&id=%s", videoStatusEndpoint, videoID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating verification request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying video status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("verification request rejected (status %d): %s", resp.StatusCode, string(body))
	}

	var data struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
			Status struct {
				UploadStatus  string `json:"uploadStatus"`
				PrivacyStatus string `json:"privacyStatus"`
			} `json:"status"`
			ProcessingDetails struct {
				ProcessingStatus string `json:"processingStatus"`
			} `json:"processingDetails"`
			ContentDetails struct {
				Duration string `json:"duration"`
			} `json:"contentDetails"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("parsing verification response: %w", err)
	}

	if len(data.Items) == 0 {
		return nil, fmt.Errorf("video %q not found on YouTube", videoID)
	}

	shortURL := fmt.Sprintf("https://youtu.be/%s", videoID)
	watchURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)

	item := data.Items[0]
	procStatus := item.ProcessingDetails.ProcessingStatus
	if procStatus == "" {
		procStatus = item.Status.UploadStatus
	}

	durRaw := item.ContentDetails.Duration
	var dur time.Duration
	if durRaw != "" {
		dur, _ = ParseISO8601Duration(durRaw)
	}

	return &VideoVerification{
		VideoID:          item.ID,
		Title:            item.Snippet.Title,
		UploadStatus:     item.Status.UploadStatus,
		PrivacyStatus:    item.Status.PrivacyStatus,
		ProcessingStatus: procStatus,
		ShortURL:         shortURL,
		WatchURL:         watchURL,
		Duration:         dur,
		DurationRaw:      durRaw,
	}, nil
}

// FindChannelVideoForTalk searches the authenticated channel's recent uploads for a matching video.
func FindChannelVideoForTalk(ctx context.Context, client *http.Client, title, dirName string) (*VideoVerification, error) {
	playlistID, err := getUploadsPlaylistID(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("retrieving uploads playlist: %w", err)
	}

	reqURL := fmt.Sprintf("%s&playlistId=%s&maxResults=50", playlistItemsEndpoint, playlistID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating playlist request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching playlist items: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("playlist request failed (status %d): %s", resp.StatusCode, string(body))
	}

	var data struct {
		Items []struct {
			Snippet struct {
				Title      string `json:"title"`
				ResourceID struct {
					VideoID string `json:"videoId"`
				} `json:"resourceId"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decoding playlist items: %w", err)
	}

	for _, item := range data.Items {
		vid := item.Snippet.ResourceID.VideoID
		vTitle := item.Snippet.Title
		if vid != "" && matchVideoTitle(vTitle, title, dirName) {
			if ver, err := VerifyVideo(ctx, client, vid); err == nil {
				return ver, nil
			}
			return &VideoVerification{
				VideoID:      vid,
				Title:        vTitle,
				UploadStatus: "uploaded",
				ShortURL:     fmt.Sprintf("https://youtu.be/%s", vid),
				WatchURL:     fmt.Sprintf("https://www.youtube.com/watch?v=%s", vid),
			}, nil
		}
	}

	return nil, nil
}

// getUploadsPlaylistID queries channels API for the authenticated channel's upload playlist ID.
func getUploadsPlaylistID(ctx context.Context, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, channelsEndpoint, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("channel request failed (status %d): %s", resp.StatusCode, string(body))
	}

	var data struct {
		Items []struct {
			ContentDetails struct {
				RelatedPlaylists struct {
					Uploads string `json:"uploads"`
				} `json:"relatedPlaylists"`
			} `json:"contentDetails"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}
	if len(data.Items) == 0 || data.Items[0].ContentDetails.RelatedPlaylists.Uploads == "" {
		return "", fmt.Errorf("no upload playlist found for channel")
	}

	return data.Items[0].ContentDetails.RelatedPlaylists.Uploads, nil
}

// matchVideoTitle compares an uploaded video's title against the candidate talk title and directory name.
func matchVideoTitle(itemTitle, title, dirName string) bool {
	itemNorm := strings.ToLower(strings.TrimSpace(itemTitle))
	titleNorm := strings.ToLower(strings.TrimSpace(title))
	dirNorm := strings.ToLower(strings.TrimSpace(dirName))

	if itemNorm == "" {
		return false
	}
	if titleNorm != "" && itemNorm == titleNorm {
		return true
	}
	if dirNorm != "" && itemNorm == dirNorm {
		return true
	}
	if len(titleNorm) >= 8 && (strings.Contains(itemNorm, titleNorm) || strings.Contains(titleNorm, itemNorm)) {
		return true
	}
	return false
}

// sanitizeYouTubeText replaces angle brackets forbidden by YouTube API with unicode fullwidth lookalikes.
func sanitizeYouTubeText(s string) string {
	r := strings.NewReplacer("<", "＜", ">", "＞")
	return r.Replace(s)
}
