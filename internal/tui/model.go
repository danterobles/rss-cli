package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/danterobles/rss-cli/internal/rss"
	"github.com/danterobles/rss-cli/internal/storage"
)

type focusArea int
type screenState int
type pendingDelete int

const (
	focusFeeds focusArea = iota
	focusArticles
)

const (
	stateBrowser screenState = iota
	stateReading
	stateAddingFeed
	stateConfirmDelete
)

const (
	deleteNothing pendingDelete = iota
	deleteFeed
	deleteArticle
)

type feedItem struct {
	feed storage.Feed
}

func (i feedItem) Title() string       { return i.feed.Title }
func (i feedItem) Description() string { return i.feed.URL }
func (i feedItem) FilterValue() string { return i.feed.Title + " " + i.feed.URL }

type articleItem struct {
	article storage.Article
}

func (i articleItem) Title() string { return i.article.Title }
func (i articleItem) Description() string {
	date := "No date"
	if !i.article.PublishedAt.IsZero() {
		date = i.article.PublishedAt.Local().Format("2006-01-02 15:04")
	}
	if i.article.IsRead {
		return "Read • " + date
	}
	return "Unread • " + date
}
func (i articleItem) FilterValue() string { return i.article.Title + " " + i.article.Link }

type feedsLoadedMsg struct {
	feeds []storage.Feed
	err   error
}

type articlesLoadedMsg struct {
	feedID   int64
	articles []storage.Article
	err      error
}

type articleRenderedMsg struct {
	article storage.Article
	body    string
	err     error
}

type syncFinishedMsg struct {
	feedID int64
	added  int
	err    error
}

type addFeedFinishedMsg struct {
	added int
	err   error
}

type deleteFeedFinishedMsg struct {
	err error
}

type deleteArticleFinishedMsg struct {
	feedID int64
	err    error
}

type model struct {
	ctx          context.Context
	repo         *storage.Repository
	service      *rss.Service
	ready        bool
	width        int
	height       int
	state        screenState
	focus        focusArea
	pending      pendingDelete
	status       string
	err          error
	errorMessage string
	feeds        []storage.Feed
	articles     []storage.Article
	feedList     list.Model
	articleList  list.Model
	viewport     viewport.Model
	input        textinput.Model
	renderer     *glamour.TermRenderer
}

func NewModel(ctx context.Context, repo *storage.Repository, service *rss.Service) (tea.Model, error) {
	feedList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	feedList.Title = "Feeds"
	feedList.SetShowHelp(false)
	feedList.SetFilteringEnabled(false)
	feedList.Styles.Title = lipgloss.NewStyle().Bold(true)

	articleList := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	articleList.Title = "Articles"
	articleList.SetShowHelp(false)
	articleList.SetFilteringEnabled(false)
	articleList.Styles.Title = lipgloss.NewStyle().Bold(true)

	input := textinput.New()
	input.Placeholder = "https://example.com/feed.xml"
	input.CharLimit = 500

	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(100),
	)
	if err != nil {
		return nil, fmt.Errorf("create renderer: %w", err)
	}

	return model{
		ctx:         ctx,
		repo:        repo,
		service:     service,
		state:       stateBrowser,
		focus:       focusFeeds,
		status:      "Loading feeds...",
		feedList:    feedList,
		articleList: articleList,
		input:       input,
		renderer:    renderer,
	}, nil
}

func (m model) Init() tea.Cmd {
	return m.loadFeedsCmd()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.resize()
		return m, nil
	case feedsLoadedMsg:
		if msg.err != nil {
			m.setError(msg.err)
			return m, nil
		}
		m.feeds = msg.feeds
		m.setFeedItems()
		if len(m.feeds) == 0 {
			m.articles = nil
			m.articleList.SetItems(nil)
			m.setStatus("No feeds yet. Press 'a' to add one.")
			return m, nil
		}
		selected := m.selectedFeed()
		if selected.ID == 0 {
			m.feedList.Select(0)
			selected = m.selectedFeed()
		}
		m.setStatus(fmt.Sprintf("%d feed(s) loaded", len(m.feeds)))
		return m, m.loadArticlesCmd(selected.ID)
	case articlesLoadedMsg:
		if msg.err != nil {
			m.setError(msg.err)
			return m, nil
		}
		if current := m.selectedFeed(); current.ID != msg.feedID {
			return m, nil
		}
		m.articles = msg.articles
		m.setArticleItems()
		if len(msg.articles) == 0 {
			m.setStatus("No articles yet. Press 'r' to sync.")
		} else {
			m.setStatus(fmt.Sprintf("%d article(s)", len(msg.articles)))
		}
		return m, nil
	case articleRenderedMsg:
		if msg.err != nil {
			m.setError(msg.err)
			return m, nil
		}
		m.viewport.SetContent(msg.body)
		m.viewport.GotoTop()
		m.state = stateReading
		m.setStatus(msg.article.Title)
		return m, m.markReadCmd(msg.article.ID)
	case syncFinishedMsg:
		if msg.err != nil {
			m.setError(msg.err)
			return m, nil
		}
		m.setStatus(fmt.Sprintf("Synced %d new article(s)", msg.added))
		return m, tea.Batch(m.loadFeedsCmd(), m.loadArticlesCmd(msg.feedID))
	case addFeedFinishedMsg:
		m.state = stateBrowser
		m.input.Blur()
		m.input.SetValue("")
		if msg.err != nil {
			m.setError(msg.err)
			return m, nil
		}
		m.setStatus(fmt.Sprintf("Feed added with %d new article(s)", msg.added))
		return m, m.loadFeedsCmd()
	case deleteFeedFinishedMsg:
		m.state = stateBrowser
		m.pending = deleteNothing
		if msg.err != nil {
			m.setError(msg.err)
			return m, nil
		}
		m.setStatus("Feed deleted")
		return m, m.loadFeedsCmd()
	case deleteArticleFinishedMsg:
		m.state = stateBrowser
		m.pending = deleteNothing
		if msg.err != nil {
			m.setError(msg.err)
			return m, nil
		}
		m.setStatus("Article deleted")
		return m, m.loadArticlesCmd(msg.feedID)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	switch m.state {
	case stateReading:
		m.viewport, cmd = m.viewport.Update(msg)
	case stateAddingFeed:
		m.input, cmd = m.input.Update(msg)
	default:
		if m.focus == focusFeeds {
			m.feedList, cmd = m.feedList.Update(msg)
		} else {
			m.articleList, cmd = m.articleList.Update(msg)
		}
	}
	return m, cmd
}

func (m model) View() string {
	if !m.ready {
		return "Loading..."
	}

	switch m.state {
	case stateReading:
		return m.readingView()
	case stateAddingFeed:
		return m.withOverlay(m.browserView(), m.modal("Add feed", m.input.View()+"\n\nEnter to save • Esc to cancel"))
	case stateConfirmDelete:
		return m.withOverlay(m.browserView(), m.deleteModal())
	default:
		return m.withOverlay(m.browserView(), m.errorModal())
	}
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.errorMessage != "" {
		switch msg.String() {
		case "enter", "esc":
			m.clearError()
			return m, nil
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	}

	switch m.state {
	case stateReading:
		switch msg.String() {
		case "esc":
			m.state = stateBrowser
			return m, nil
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		m.viewport, _ = m.viewport.Update(msg)
		return m, nil
	case stateAddingFeed:
		switch msg.String() {
		case "esc":
			m.state = stateBrowser
			m.input.Blur()
			m.input.SetValue("")
			return m, nil
		case "enter":
			value := strings.TrimSpace(m.input.Value())
			if value == "" {
				m.setStatus("Feed URL is required")
				return m, nil
			}
			m.setStatus("Adding feed...")
			return m, m.addFeedCmd(value)
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	case stateConfirmDelete:
		switch msg.String() {
		case "y":
			switch m.pending {
			case deleteFeed:
				feed := m.selectedFeed()
				if feed.ID != 0 {
					m.setStatus("Deleting feed...")
					return m, m.deleteFeedCmd(feed.ID)
				}
			case deleteArticle:
				article := m.selectedArticle()
				if article.ID != 0 {
					m.setStatus("Deleting article...")
					return m, m.deleteArticleCmd(article.ID, article.FeedID)
				}
			}
			m.state = stateBrowser
			m.pending = deleteNothing
			return m, nil
		case "n", "esc":
			m.state = stateBrowser
			m.pending = deleteNothing
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab":
		if m.focus == focusFeeds {
			m.focus = focusArticles
		} else {
			m.focus = focusFeeds
		}
		return m, nil
	case "a":
		m.state = stateAddingFeed
		m.input.Focus()
		m.clearError()
		return m, nil
	case "d":
		if m.focus == focusArticles {
			if m.selectedArticle().ID != 0 {
				m.state = stateConfirmDelete
				m.pending = deleteArticle
			}
			return m, nil
		}
		if m.selectedFeed().ID != 0 {
			m.state = stateConfirmDelete
			m.pending = deleteFeed
		}
		return m, nil
	case "r":
		feed := m.selectedFeed()
		if feed.ID == 0 {
			return m, nil
		}
		m.setStatus("Syncing feed...")
		return m, m.syncFeedCmd(feed)
	case "enter":
		if m.focus == focusFeeds {
			feed := m.selectedFeed()
			if feed.ID != 0 {
				return m, m.loadArticlesCmd(feed.ID)
			}
			return m, nil
		}
		article := m.selectedArticle()
		if article.ID == 0 {
			return m, nil
		}
		return m, m.renderArticleCmd(article)
	}

	var cmd tea.Cmd
	if m.focus == focusFeeds {
		prev := m.selectedFeed().ID
		m.feedList, cmd = m.feedList.Update(msg)
		if next := m.selectedFeed().ID; next != 0 && next != prev {
			return m, tea.Batch(cmd, m.loadArticlesCmd(next))
		}
		return m, cmd
	}

	m.articleList, cmd = m.articleList.Update(msg)
	return m, cmd
}

func (m model) browserView() string {
	leftStyle := panelStyle(m.focus == focusFeeds, m.width/3)
	rightStyle := panelStyle(m.focus == focusArticles, m.width-m.width/3)

	header := lipgloss.NewStyle().Bold(true).Render("rss-cli")
	status := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(m.statusLine())

	left := leftStyle.Render(m.feedList.View())
	right := rightStyle.Render(m.articleList.View())
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	help := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("Tab switch • Enter open • a add • d delete selected • r sync • q quit")
	return lipgloss.JoinVertical(lipgloss.Left, header, body, status, help)
}

func (m model) readingView() string {
	frame := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height - 2).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("63")).
		Render(m.viewport.View())

	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("j/k or arrows scroll • Esc back • q quit")
	return lipgloss.JoinVertical(lipgloss.Left, frame, footer)
}

func (m model) modal(title, body string) string {
	style := lipgloss.NewStyle().
		Width(min(70, m.width-4)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(1, 2)
	return style.Render(lipgloss.NewStyle().Bold(true).Render(title) + "\n\n" + body)
}

func (m model) deleteModal() string {
	switch m.pending {
	case deleteArticle:
		article := m.selectedArticle()
		title := article.Title
		if title == "" {
			title = "selected article"
		}
		return m.modal("Delete article", fmt.Sprintf("Delete \"%s\"?\n\nPress y to confirm or n to cancel.", title))
	default:
		feed := m.selectedFeed()
		title := feed.Title
		if title == "" {
			title = "selected feed"
		}
		return m.modal("Delete feed", fmt.Sprintf("Delete \"%s\" and its stored articles?\n\nPress y to confirm or n to cancel.", title))
	}
}

func (m model) errorModal() string {
	if m.errorMessage == "" {
		return ""
	}
	return m.modal("Error", m.errorMessage+"\n\nPress Esc or Enter to dismiss.")
}

func (m model) withOverlay(base, overlay string) string {
	if overlay == "" {
		return base
	}
	return base + "\n" + overlay
}

func (m *model) resize() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	leftWidth := max(30, m.width/3)
	rightWidth := max(40, m.width-leftWidth-2)
	listHeight := max(10, m.height-6)

	m.feedList.SetSize(leftWidth-4, listHeight)
	m.articleList.SetSize(rightWidth-4, listHeight)

	m.viewport = viewport.New(max(20, m.width-4), max(10, m.height-4))
}

func (m *model) setFeedItems() {
	items := make([]list.Item, 0, len(m.feeds))
	for _, feed := range m.feeds {
		items = append(items, feedItem{feed: feed})
	}
	m.feedList.SetItems(items)
}

func (m *model) setArticleItems() {
	items := make([]list.Item, 0, len(m.articles))
	for _, article := range m.articles {
		items = append(items, articleItem{article: article})
	}
	m.articleList.SetItems(items)
}

func (m model) selectedFeed() storage.Feed {
	item, ok := m.feedList.SelectedItem().(feedItem)
	if !ok {
		return storage.Feed{}
	}
	return item.feed
}

func (m model) selectedArticle() storage.Article {
	item, ok := m.articleList.SelectedItem().(articleItem)
	if !ok {
		return storage.Article{}
	}
	return item.article
}

func (m *model) setError(err error) {
	m.err = err
	m.errorMessage = friendlyError(err)
	m.status = "Operation failed"
}

func (m *model) clearError() {
	m.err = nil
	m.errorMessage = ""
}

func (m *model) setStatus(status string) {
	m.clearError()
	m.status = status
}

func (m model) statusLine() string {
	if m.errorMessage != "" {
		return "Error: " + m.errorMessage
	}
	return m.status
}

func panelStyle(active bool, width int) lipgloss.Style {
	borderColor := lipgloss.Color("240")
	if active {
		borderColor = lipgloss.Color("42")
	}
	return lipgloss.NewStyle().
		Width(width).
		Border(lipgloss.NormalBorder()).
		BorderForeground(borderColor)
}

func (m model) loadFeedsCmd() tea.Cmd {
	return func() tea.Msg {
		feeds, err := m.repo.ListFeeds(m.ctx)
		return feedsLoadedMsg{feeds: feeds, err: err}
	}
}

func (m model) loadArticlesCmd(feedID int64) tea.Cmd {
	return func() tea.Msg {
		articles, err := m.repo.ListArticlesByFeed(m.ctx, feedID)
		return articlesLoadedMsg{feedID: feedID, articles: articles, err: err}
	}
}

func (m model) renderArticleCmd(article storage.Article) tea.Cmd {
	return func() tea.Msg {
		body := article.Content
		if strings.TrimSpace(body) == "" {
			body = fmt.Sprintf("# %s\n\n[%s](%s)", article.Title, article.Link, article.Link)
		}
		rendered, err := m.renderer.Render(body)
		if err != nil {
			return articleRenderedMsg{err: err}
		}
		header := lipgloss.NewStyle().Bold(true).Render(article.Title)
		meta := article.Link
		if !article.PublishedAt.IsZero() {
			meta = article.PublishedAt.Local().Format(time.RFC1123) + "\n" + article.Link
		}
		return articleRenderedMsg{
			article: article,
			body:    header + "\n\n" + meta + "\n\n" + rendered,
		}
	}
}

func (m model) markReadCmd(articleID int64) tea.Cmd {
	return func() tea.Msg {
		_ = m.repo.MarkArticleRead(m.ctx, articleID)
		return nil
	}
}

func (m model) syncFeedCmd(feed storage.Feed) tea.Cmd {
	return func() tea.Msg {
		added, err := m.service.SyncFeed(m.ctx, feed)
		return syncFinishedMsg{feedID: feed.ID, added: added, err: err}
	}
}

func (m model) addFeedCmd(url string) tea.Cmd {
	return func() tea.Msg {
		_, added, err := m.service.AddFeed(m.ctx, url)
		return addFeedFinishedMsg{added: added, err: err}
	}
}

func (m model) deleteFeedCmd(feedID int64) tea.Cmd {
	return func() tea.Msg {
		err := m.repo.DeleteFeedByID(m.ctx, feedID)
		return deleteFeedFinishedMsg{err: err}
	}
}

func (m model) deleteArticleCmd(articleID, feedID int64) tea.Cmd {
	return func() tea.Msg {
		err := m.repo.DeleteArticleByID(m.ctx, articleID)
		return deleteArticleFinishedMsg{feedID: feedID, err: err}
	}
}

func friendlyError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, storage.ErrNotFound) {
		return "The selected item no longer exists."
	}

	message := err.Error()
	switch {
	case strings.Contains(message, "parse feed"):
		return "Could not parse the feed URL. Verify that it is a valid RSS or Atom source."
	case strings.Contains(message, "UNIQUE constraint failed: feeds.url"):
		return "That feed is already registered."
	case strings.Contains(message, "sync feed"):
		return "The feed could not be refreshed right now."
	default:
		return message
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
