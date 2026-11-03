package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"talk_cut/internal/youtube"
)

// recentVideoLimit is how many of the channel's latest uploads the playlist panel offers.
const recentVideoLimit = 20

// playlistPrivacies is the cycle order for a new playlist's privacy setting.
var playlistPrivacies = []string{"public", "unlisted", "private"}

// plPanelMode is the sub-view shown by the playlists panel.
type plPanelMode int

const (
	// plModeList shows the channel's playlists.
	plModeList plPanelMode = iota
	// plModeVideos shows recent uploads with membership toggles for one playlist.
	plModeVideos
	// plModeCreate shows the new-playlist form.
	plModeCreate
)

// PlaylistsModel manages the playlist browser and membership editor (Tab 6).
type PlaylistsModel struct {
	theme      Theme
	mode       plPanelMode
	channel    string
	loaded     bool
	loading    bool
	playlists  []youtube.Playlist
	plIdx      int
	videos     []youtube.ChannelVideo
	vidIdx     int
	members    map[string]bool
	loadingMem bool
	memberSeq  int
	pending    map[string]bool
	titleInput textinput.Model
	privacyIdx int
	creating   bool
	feedback   string
	isError    bool
	width      int
	height     int
}

// NewPlaylistsModel creates an empty playlists panel; data loads on first visit.
func NewPlaylistsModel() PlaylistsModel {
	ti := textinput.New()
	ti.Placeholder = "New playlist title"
	ti.CharLimit = 150
	ti.Width = 60
	return PlaylistsModel{
		theme:      DefaultTheme(),
		titleInput: ti,
		pending:    make(map[string]bool),
		width:      100,
		height:     30,
	}
}

// SetDimensions updates the viewport dimensions.
func (m *PlaylistsModel) SetDimensions(w, h int) {
	m.width = w
	m.height = h
}

// NeedsLoad reports whether the panel must fetch data for channel: it never has, or it holds another channel's data.
func (m PlaylistsModel) NeedsLoad(channel string) bool {
	return m.channel != channel || (!m.loaded && !m.loading)
}

// IsEditing reports whether the title input has focus.
func (m PlaylistsModel) IsEditing() bool {
	return m.mode == plModeCreate
}

// Mode returns the current sub-view.
func (m PlaylistsModel) Mode() plPanelMode {
	return m.mode
}

// SetLoading marks a full refresh for channel as in flight, discarding another channel's data.
func (m *PlaylistsModel) SetLoading(channel string) {
	if channel != m.channel {
		m.reset(channel)
	}
	m.loading = true
	m.setFeedback("Fetching playlists and recent uploads...", false)
}

// reset clears all channel data and returns to the playlist list.
func (m *PlaylistsModel) reset(channel string) {
	m.channel = channel
	m.loaded = false
	m.mode = plModeList
	m.playlists, m.videos, m.members = nil, nil, nil
	m.plIdx, m.vidIdx = 0, 0
	m.loadingMem, m.creating = false, false
	m.pending = make(map[string]bool)
	m.titleInput.Blur()
}

// SetLoaded stores freshly fetched playlists and uploads; responses for another channel are dropped.
func (m *PlaylistsModel) SetLoaded(channel string, pls []youtube.Playlist, videos []youtube.ChannelVideo, err error) {
	if channel != m.channel {
		return
	}
	m.loading = false
	if err != nil {
		m.setFeedback(fmt.Sprintf("Failed to load: %v", err), true)
		return
	}
	m.loaded = true
	m.playlists = pls
	m.videos = videos
	m.plIdx = clampIndex(m.plIdx, len(pls))
	m.vidIdx = clampIndex(m.vidIdx, len(videos))
	m.setFeedback("", false)
}

// SelectedPlaylist returns the playlist under the cursor, or nil.
func (m PlaylistsModel) SelectedPlaylist() *youtube.Playlist {
	if m.plIdx < 0 || m.plIdx >= len(m.playlists) {
		return nil
	}
	return &m.playlists[m.plIdx]
}

// SelectedVideo returns the video under the cursor, or nil.
func (m PlaylistsModel) SelectedVideo() *youtube.ChannelVideo {
	if m.vidIdx < 0 || m.vidIdx >= len(m.videos) {
		return nil
	}
	return &m.videos[m.vidIdx]
}

// MoveCursor shifts the cursor of the active list by delta, wrapping around.
func (m *PlaylistsModel) MoveCursor(delta int) {
	switch m.mode {
	case plModeList:
		m.plIdx = wrapIndex(m.plIdx+delta, len(m.playlists))
	case plModeVideos:
		m.vidIdx = wrapIndex(m.vidIdx+delta, len(m.videos))
	}
}

// OpenPlaylist switches to the membership view, marks membership as loading,
// and returns the sequence number the matching SetMembers call must carry.
func (m *PlaylistsModel) OpenPlaylist() int {
	m.memberSeq++
	m.mode = plModeVideos
	m.members = nil
	m.loadingMem = true
	m.vidIdx = 0
	m.setFeedback("", false)
	return m.memberSeq
}

// SetMembers stores the video IDs of the open playlist; responses from superseded fetches are dropped.
func (m *PlaylistsModel) SetMembers(seq int, playlistID string, ids map[string]bool, err error) {
	pl := m.SelectedPlaylist()
	if seq != m.memberSeq || pl == nil || pl.ID != playlistID {
		return
	}
	m.loadingMem = false
	if err != nil {
		m.setFeedback(fmt.Sprintf("Failed to read playlist: %v", err), true)
		return
	}
	m.members = ids
}

// MembershipKnown reports whether toggles can be applied to the open playlist.
func (m PlaylistsModel) MembershipKnown() bool {
	return m.mode == plModeVideos && !m.loadingMem && m.members != nil
}

// IsMember reports whether the video belongs to the open playlist.
func (m PlaylistsModel) IsMember(videoID string) bool {
	return m.members[videoID]
}

// IsPending reports whether a toggle for the video is in flight.
func (m PlaylistsModel) IsPending(videoID string) bool {
	return m.pending[videoID]
}

// SetPending marks a toggle as in flight.
func (m *PlaylistsModel) SetPending(videoID string, title string, adding bool) {
	m.pending[videoID] = true
	verb := "Removing"
	if adding {
		verb = "Adding"
	}
	m.setFeedback(fmt.Sprintf("%s %q...", verb, title), false)
}

// ApplyToggle records the result of an add or remove call.
func (m *PlaylistsModel) ApplyToggle(playlistID, videoID string, added bool, err error) {
	delete(m.pending, videoID)
	if err != nil {
		m.setFeedback(fmt.Sprintf("Playlist update failed: %v", err), true)
		return
	}
	delta := int64(-1)
	if added {
		delta = 1
	}
	for i := range m.playlists {
		if m.playlists[i].ID == playlistID {
			m.playlists[i].ItemCount = max(m.playlists[i].ItemCount+delta, 0)
		}
	}
	pl := m.SelectedPlaylist()
	if pl == nil || pl.ID != playlistID || m.members == nil {
		return
	}
	if added {
		m.members[videoID] = true
		m.setFeedback(fmt.Sprintf("✔ Added to %q", pl.Title), false)
	} else {
		delete(m.members, videoID)
		m.setFeedback(fmt.Sprintf("✔ Removed from %q", pl.Title), false)
	}
}

// BeginCreate opens the new-playlist form.
func (m *PlaylistsModel) BeginCreate() tea.Cmd {
	m.mode = plModeCreate
	m.privacyIdx = 0
	m.titleInput.SetValue("")
	m.setFeedback("", false)
	return m.titleInput.Focus()
}

// CancelCreate leaves the new-playlist form.
func (m *PlaylistsModel) CancelCreate() {
	m.titleInput.Blur()
	m.mode = plModeList
}

// CyclePrivacy advances the new playlist's privacy setting.
func (m *PlaylistsModel) CyclePrivacy() {
	m.privacyIdx = (m.privacyIdx + 1) % len(playlistPrivacies)
}

// CreateRequest returns the trimmed title and privacy for the form, and whether it can be submitted.
func (m PlaylistsModel) CreateRequest() (title, privacy string, ok bool) {
	title = strings.TrimSpace(m.titleInput.Value())
	return title, playlistPrivacies[m.privacyIdx], title != "" && !m.creating
}

// SetCreating marks a create call as in flight.
func (m *PlaylistsModel) SetCreating(title string) {
	m.creating = true
	m.setFeedback(fmt.Sprintf("Creating playlist %q...", title), false)
}

// ApplyCreated adds a newly created playlist and opens it; responses for another channel are dropped.
func (m *PlaylistsModel) ApplyCreated(channel string, pl *youtube.Playlist, err error) {
	if channel != m.channel {
		return
	}
	m.creating = false
	if err != nil {
		m.setFeedback(fmt.Sprintf("Failed to create playlist: %v", err), true)
		return
	}
	if !m.hasPlaylist(pl.ID) {
		m.playlists = append([]youtube.Playlist{*pl}, m.playlists...)
		if m.mode != plModeCreate {
			m.plIdx++ // keep the cursor on the playlist it was on
		}
	}
	msg := fmt.Sprintf("✔ Created playlist %q (%s)", pl.Title, pl.PrivacyStatus)
	if m.mode != plModeCreate {
		m.setFeedback(msg, false) // the user moved on; do not navigate away
		return
	}
	m.titleInput.Blur()
	m.plIdx = m.indexOf(pl.ID)
	m.OpenPlaylist()
	m.loadingMem = false
	m.members = make(map[string]bool)
	m.setFeedback(msg, false)
}

func (m PlaylistsModel) hasPlaylist(id string) bool {
	return m.indexOf(id) >= 0
}

func (m PlaylistsModel) indexOf(id string) int {
	for i := range m.playlists {
		if m.playlists[i].ID == id {
			return i
		}
	}
	return -1
}

// BackToList returns from the membership view to the playlist list.
func (m *PlaylistsModel) BackToList() {
	m.mode = plModeList
	m.setFeedback("", false)
}

// Update forwards messages to the title input while the create form is open.
func (m PlaylistsModel) Update(msg tea.Msg) (PlaylistsModel, tea.Cmd) {
	if m.mode != plModeCreate {
		return m, nil
	}
	var cmd tea.Cmd
	m.titleInput, cmd = m.titleInput.Update(msg)
	return m, cmd
}

func (m *PlaylistsModel) setFeedback(msg string, isError bool) {
	m.feedback = msg
	m.isError = isError
}

// View renders the playlists tab.
func (m PlaylistsModel) View() string {
	var body string
	switch m.mode {
	case plModeVideos:
		body = m.renderVideos()
	case plModeCreate:
		body = m.renderCreate()
	default:
		body = m.renderList()
	}
	header := RenderTabBar(int(ScreenPlaylists), m.width, m.theme)
	footer := lipgloss.NewStyle().Width(m.width).MarginTop(1).Render(m.theme.HelpDesc.Render(" " + m.footerHint()))
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m PlaylistsModel) footerHint() string {
	switch m.mode {
	case plModeVideos:
		return "↑/↓: move | Space/Enter: toggle membership | o: open playlist | r: refresh | Esc: playlists"
	case plModeCreate:
		return "Enter: create | Tab: cycle privacy | Esc: cancel"
	}
	return "↑/↓: move | Enter: open | n: new playlist | o: open in browser | r: refresh | ←/→: tabs"
}

func (m PlaylistsModel) renderList() string {
	lines := []string{m.theme.TitleStyle.Render(" YOUTUBE PLAYLISTS: " + m.channel + " "), ""}
	switch {
	case m.loading && len(m.playlists) == 0:
	case len(m.playlists) == 0:
		lines = append(lines, "  No playlists on this channel. Press [n] to create one.")
	default:
		start, end := visibleWindow(m.plIdx, len(m.playlists), m.listRows())
		for i := start; i < end; i++ {
			pl := m.playlists[i]
			line := fmt.Sprintf("%s%s  (%d videos, %s)", cursorPrefix(i == m.plIdx), pl.Title, pl.ItemCount, pl.PrivacyStatus)
			lines = append(lines, m.highlight(line, i == m.plIdx))
		}
	}
	return m.box(m.withFeedback(lines))
}

func (m PlaylistsModel) renderVideos() string {
	pl := m.SelectedPlaylist()
	if pl == nil {
		return m.box([]string{"  No playlist selected."})
	}
	lines := []string{
		m.theme.TitleStyle.Render(" PLAYLIST: " + pl.Title + " "),
		m.theme.HelpDesc.Render(fmt.Sprintf("  %d videos, %s — showing the %d most recent uploads", pl.ItemCount, pl.PrivacyStatus, recentVideoLimit)),
		"",
	}
	if m.loadingMem {
		lines = append(lines, "  Reading playlist contents...")
		return m.box(m.withFeedback(lines))
	}
	if len(m.videos) == 0 {
		lines = append(lines, "  No uploads found on this channel.")
		return m.box(m.withFeedback(lines))
	}
	start, end := visibleWindow(m.vidIdx, len(m.videos), m.listRows()-1)
	for i := start; i < end; i++ {
		lines = append(lines, m.highlight(m.videoLine(i), i == m.vidIdx))
	}
	return m.box(m.withFeedback(lines))
}

func (m PlaylistsModel) videoLine(i int) string {
	v := m.videos[i]
	marker := "[ ]"
	switch {
	case m.pending[v.ID]:
		marker = "[…]"
	case m.members[v.ID]:
		marker = "[✔]"
	}
	date := v.PublishedAt
	if len(date) >= 10 {
		date = date[:10]
	}
	return fmt.Sprintf("%s%s %s  %s", cursorPrefix(i == m.vidIdx), marker, date, v.Title)
}

func (m PlaylistsModel) renderCreate() string {
	var privacy []string
	for i, p := range playlistPrivacies {
		if i == m.privacyIdx {
			privacy = append(privacy, m.theme.PrimaryText.Bold(true).Render("["+p+"]"))
		} else {
			privacy = append(privacy, " "+p+" ")
		}
	}
	lines := []string{
		m.theme.TitleStyle.Render(" NEW YOUTUBE PLAYLIST "),
		"",
		"Title:    " + m.titleInput.View(),
		"Privacy:  " + strings.Join(privacy, " "),
	}
	return m.box(m.withFeedback(lines))
}

func (m PlaylistsModel) withFeedback(lines []string) []string {
	if m.feedback == "" {
		return lines
	}
	style := m.theme.SuccessText
	if m.isError {
		style = m.theme.DangerText
	}
	return append(lines, "", "  "+style.Render(m.feedback))
}

func (m PlaylistsModel) highlight(line string, selected bool) string {
	if !selected {
		return line
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#38BDF8")).Render(line)
}

func (m PlaylistsModel) box(lines []string) string {
	w := max(m.width-4, 20)
	box := m.theme.SidebarBox.Width(w).Render(strings.Join(lines, "\n"))
	return lipgloss.NewStyle().MarginLeft(2).Render(box)
}

// listRows is how many list rows fit between the header, box chrome, feedback, and footer.
func (m PlaylistsModel) listRows() int {
	return max(m.height-12, 3)
}

func cursorPrefix(selected bool) string {
	if selected {
		return " > "
	}
	return "   "
}

// visibleWindow returns the [start, end) slice of n rows that keeps cursor visible within rows.
func visibleWindow(cursor, n, rows int) (int, int) {
	if n <= rows {
		return 0, n
	}
	start := min(max(cursor-rows/2, 0), n-rows)
	return start, start + rows
}

func wrapIndex(i, n int) int {
	if n == 0 {
		return 0
	}
	return ((i % n) + n) % n
}

func clampIndex(i, n int) int {
	if n == 0 {
		return 0
	}
	return min(max(i, 0), n-1)
}
