package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Fonthom/gator/internal/database"
)

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	styleTitleBar = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	styleSelected = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212"))

	styleNormal = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	styleDim = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	styleDetail = lipgloss.NewStyle().
			Padding(1, 2)

	styleHeading = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212")).
			MarginBottom(1)

	styleURL = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Underline(true)

	styleHelp = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			MarginTop(1)

	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)
)

type viewMode int

const (
	modeList   viewMode = iota
	modeDetail
)

type tuiModel struct {
	posts    []database.Post
	cursor   int
	offset   int // for scrolling
	mode     viewMode
	width    int
	height   int
	message  string // transient status message
}

func newTUIModel(posts []database.Post) tuiModel {
	return tuiModel{posts: posts}
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		m.message = ""
		switch msg.String() {

		case "q", "ctrl+c":
			return m, tea.Quit

		case "up", "k":
			if m.mode == modeList && m.cursor > 0 {
				m.cursor--
				if m.cursor < m.offset {
					m.offset--
				}
			}

		case "down", "j":
			if m.mode == modeList && m.cursor < len(m.posts)-1 {
				m.cursor++
				visibleRows := m.listHeight()
				if m.cursor >= m.offset+visibleRows {
					m.offset++
				}
			}

		case "enter":
			if m.mode == modeList && len(m.posts) > 0 {
				m.mode = modeDetail
			}

		case "esc", "b":
			if m.mode == modeDetail {
				m.mode = modeList
			}

		case "o":
			if len(m.posts) > 0 {
				url := m.posts[m.cursor].Url
				if err := openBrowser(url); err != nil {
					m.message = fmt.Sprintf("could not open browser: %v", err)
				} else {
					m.message = "opened in browser"
				}
			}
		}
	}
	return m, nil
}


func (m tuiModel) View() string {
	if m.width == 0 {
		return "loading…"
	}
	if len(m.posts) == 0 {
		return "No posts found. Run `gator agg` to fetch some feeds first.\n"
	}
	switch m.mode {
	case modeDetail:
		return m.detailView()
	default:
		return m.listView()
	}
}

func (m tuiModel) listView() string {
	var b strings.Builder

	title := styleTitleBar.Render(fmt.Sprintf(" Gator — %d posts ", len(m.posts)))
	b.WriteString(title + "\n\n")

	visibleRows := m.listHeight()
	end := m.offset + visibleRows
	if end > len(m.posts) {
		end = len(m.posts)
	}

	for i := m.offset; i < end; i++ {
		post := m.posts[i]
		label := truncate(post.Title, m.width-6)
		date := ""
		if post.PublishedAt.Valid {
			date = styleDim.Render(" · " + post.PublishedAt.Time.Format("02 Jan 2006"))
		}
		if i == m.cursor {
			b.WriteString(styleSelected.Render("▶ "+label) + date + "\n")
		} else {
			b.WriteString(styleNormal.Render("  "+label) + date + "\n")
		}
	}

	if len(m.posts) > visibleRows {
		scrollInfo := styleDim.Render(fmt.Sprintf("\n  %d–%d of %d", m.offset+1, end, len(m.posts)))
		b.WriteString(scrollInfo)
	}

	help := styleHelp.Render("↑/↓ navigate · enter view · o open in browser · q quit")
	b.WriteString("\n" + help)

	if m.message != "" {
		b.WriteString("\n" + styleDim.Render(m.message))
	}
	return b.String()
}

func (m tuiModel) detailView() string {
	post := m.posts[m.cursor]
	maxW := m.width - 6
	if maxW < 20 {
		maxW = 20
	}

	var b strings.Builder
	b.WriteString(styleHeading.Render(wordWrap(post.Title, maxW)) + "\n")

	if post.PublishedAt.Valid {
		b.WriteString(styleDim.Render("Published: "+post.PublishedAt.Time.Format(time.RFC1123)) + "\n")
	}
	b.WriteString(styleURL.Render(post.Url) + "\n\n")

	if post.Description.Valid && post.Description.String != "" {
		desc := stripHTML(post.Description.String)
		b.WriteString(wordWrap(desc, maxW) + "\n")
	} else {
		b.WriteString(styleDim.Render("No description available.") + "\n")
	}

	help := styleHelp.Render("esc/b back · o open in browser · q quit")
	b.WriteString("\n" + help)

	if m.message != "" {
		b.WriteString("\n" + styleDim.Render(m.message))
	}
	return styleDetail.Render(b.String())
}

func (m tuiModel) listHeight() int {
	reserved := 5
	h := m.height - reserved
	if h < 1 {
		h = 1
	}
	return h
}


func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

func wordWrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	words := strings.Fields(s)
	var lines []string
	line := ""
	for _, w := range words {
		if line == "" {
			line = w
		} else if len(line)+1+len(w) <= width {
			line += " " + w
		} else {
			lines = append(lines, line)
			line = w
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	result := strings.ReplaceAll(b.String(), "\n\n\n", "\n\n")
	return strings.TrimSpace(result)
}

func openBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}


func handlerTUI(s *state, cmd command, user database.User) error {
	posts, err := s.db.GetPostsForUser(context.Background(), database.GetPostsForUserParams{
		UserID: user.ID,
		Limit:  100,
	})
	if err != nil {
		return fmt.Errorf("error fetching posts: %w", err)
	}

	p := tea.NewProgram(
		newTUIModel(posts),
		tea.WithAltScreen(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}
	return nil
}