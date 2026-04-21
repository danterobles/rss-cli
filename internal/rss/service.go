package rss

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/danterobles/rss-cli/internal/storage"
	"github.com/mmcdole/gofeed"
)

type Service struct {
	parser *gofeed.Parser
	repo   *storage.Repository
	client *http.Client
}

func NewService(repo *storage.Repository) *Service {
	return &Service{
		parser: gofeed.NewParser(),
		repo:   repo,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *Service) AddFeed(ctx context.Context, url string) (storage.Feed, int, error) {
	parsed, err := s.parser.ParseURLWithContext(strings.TrimSpace(url), ctx)
	if err != nil {
		return storage.Feed{}, 0, fmt.Errorf("parse feed: %w", err)
	}

	feed, err := s.repo.GetFeedByURL(ctx, url)
	if err == nil {
		if parsed.Title != "" && parsed.Title != feed.Title {
			if err := s.repo.UpdateFeedTitle(ctx, feed.ID, parsed.Title); err != nil {
				return storage.Feed{}, 0, err
			}
			feed.Title = parsed.Title
		}
	} else {
		feed, err = s.repo.CreateFeed(ctx, feedTitle(parsed), url)
		if err != nil {
			return storage.Feed{}, 0, err
		}
	}

	count, err := s.storeItems(ctx, feed.ID, parsed.Items)
	if err != nil {
		return storage.Feed{}, 0, err
	}

	return feed, count, nil
}

func (s *Service) SyncAll(ctx context.Context) (int, error) {
	feeds, err := s.repo.ListFeeds(ctx)
	if err != nil {
		return 0, err
	}

	total := 0
	for _, feed := range feeds {
		parsed, err := s.parser.ParseURLWithContext(feed.URL, ctx)
		if err != nil {
			return total, fmt.Errorf("sync %s: %w", feed.URL, err)
		}
		if parsed.Title != "" && parsed.Title != feed.Title {
			if err := s.repo.UpdateFeedTitle(ctx, feed.ID, parsed.Title); err != nil {
				return total, err
			}
		}
		inserted, err := s.storeItems(ctx, feed.ID, parsed.Items)
		if err != nil {
			return total, err
		}
		total += inserted
	}
	return total, nil
}

func (s *Service) SyncFeed(ctx context.Context, feed storage.Feed) (int, error) {
	parsed, err := s.parser.ParseURLWithContext(feed.URL, ctx)
	if err != nil {
		return 0, fmt.Errorf("sync feed: %w", err)
	}
	if parsed.Title != "" && parsed.Title != feed.Title {
		if err := s.repo.UpdateFeedTitle(ctx, feed.ID, parsed.Title); err != nil {
			return 0, err
		}
	}
	return s.storeItems(ctx, feed.ID, parsed.Items)
}

func (s *Service) storeItems(ctx context.Context, feedID int64, items []*gofeed.Item) (int, error) {
	inserted := 0
	for _, item := range items {
		if item == nil || strings.TrimSpace(item.Link) == "" {
			continue
		}
		article := storage.Article{
			FeedID:      feedID,
			Title:       strings.TrimSpace(item.Title),
			Link:        strings.TrimSpace(item.Link),
			PublishedAt: publishedAt(item),
		}
		if article.Title == "" {
			article.Title = article.Link
		}
		article.Content = previewContent(article.Title, article.Link, extractContent(item))

		created, err := s.repo.UpsertArticle(ctx, article)
		if err != nil {
			return inserted, err
		}
		if created {
			inserted++
		}
	}
	return inserted, nil
}

var whitespaceRE = regexp.MustCompile(`\s+`)

func (s *Service) HydrateArticle(ctx context.Context, article storage.Article) (storage.Article, error) {
	content := strings.TrimSpace(article.Content)
	if isFullArticleContent(content) {
		return article, nil
	}

	fullContent, err := s.fetchArticleContent(ctx, article.Link)
	if err != nil {
		if content != "" {
			return article, nil
		}
		return article, err
	}

	if err := s.repo.UpdateArticleContent(ctx, article.ID, fullContent); err != nil {
		return article, err
	}
	article.Content = fullContent
	return article, nil
}

func previewContent(title, link, fallback string) string {
	fallback = strings.TrimSpace(fallback)
	if fallback != "" {
		return fallback
	}
	return fmt.Sprintf("# %s\n\n[%s](%s)", title, link, link)
}

func isFullArticleContent(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return false
	}
	if strings.HasPrefix(content, "# ") {
		return true
	}
	return len(content) > 1500
}

func (s *Service) fetchArticleContent(ctx context.Context, articleURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, articleURL, nil)
	if err != nil {
		return "", fmt.Errorf("build article request: %w", err)
	}
	req.Header.Set("User-Agent", "rss-cli/0.1 (+https://github.com/danterobles/rss-cli)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	res, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch article: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("fetch article status: %s", res.Status)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return "", fmt.Errorf("parse article html: %w", err)
	}

	removeNoise(doc)

	root := pickReadableRoot(doc)
	if root.Length() == 0 {
		root = doc.Find("body").First()
	}
	if root.Length() == 0 {
		return "", fmt.Errorf("article body not found")
	}

	markdown := extractMarkdown(root, articleURL)
	if strings.TrimSpace(markdown) == "" {
		return "", fmt.Errorf("article content empty")
	}
	return markdown, nil
}

func removeNoise(doc *goquery.Document) {
	doc.Find("script, style, noscript, svg, canvas, iframe, form, header nav, footer, aside, .advertisement, .ads, .ad, .promo, .related, .sidebar, .share, .social").Each(func(_ int, s *goquery.Selection) {
		s.Remove()
	})
}

func pickReadableRoot(doc *goquery.Document) *goquery.Selection {
	candidates := []string{
		"article",
		"[itemprop='articleBody']",
		".article-content",
		".article__content",
		".post-content",
		".entry-content",
		".content__article-body",
		".story-body",
		".story-content",
		"main",
	}

	var best *goquery.Selection
	bestScore := -1
	for _, selector := range candidates {
		doc.Find(selector).Each(func(_ int, s *goquery.Selection) {
			score := readableScore(s)
			if score > bestScore {
				bestScore = score
				best = s
			}
		})
	}

	if best != nil {
		return best
	}
	return doc.Find("body").First()
}

func readableScore(s *goquery.Selection) int {
	text := normalizeSpace(s.Text())
	if text == "" {
		return 0
	}
	return len(text) + s.Find("p").Length()*120 + s.Find("h1,h2,h3").Length()*80
}

func extractMarkdown(root *goquery.Selection, articleURL string) string {
	var blocks []string
	baseURL, _ := url.Parse(articleURL)

	root.Find("h1, h2, h3, h4, h5, h6, p, li, blockquote, pre").Each(func(_ int, s *goquery.Selection) {
		tag := goquery.NodeName(s)
		text := renderNodeText(s, baseURL)
		if text == "" {
			return
		}

		switch tag {
		case "h1":
			blocks = append(blocks, "# "+text)
		case "h2":
			blocks = append(blocks, "## "+text)
		case "h3", "h4", "h5", "h6":
			blocks = append(blocks, "### "+text)
		case "li":
			blocks = append(blocks, "- "+text)
		case "blockquote":
			blocks = append(blocks, "> "+strings.ReplaceAll(text, "\n", "\n> "))
		case "pre":
			blocks = append(blocks, "```text\n"+text+"\n```")
		default:
			blocks = append(blocks, text)
		}
	})

	return dedupeBlocks(blocks)
}

func renderNodeText(s *goquery.Selection, baseURL *url.URL) string {
	clone := s.Clone()
	clone.Find("script, style, noscript").Remove()

	clone.Find("a").Each(func(_ int, a *goquery.Selection) {
		text := normalizeSpace(a.Text())
		href, _ := a.Attr("href")
		if href != "" && baseURL != nil {
			if parsed, err := baseURL.Parse(href); err == nil {
				href = parsed.String()
			}
		}

		switch {
		case text == "" && href != "":
			a.SetText(href)
		case text != "" && href != "" && strings.HasPrefix(goquery.NodeName(s), "p"):
			a.SetText(text + " (" + href + ")")
		default:
			a.SetText(text)
		}
	})

	return normalizeSpace(clone.Text())
}

func dedupeBlocks(blocks []string) string {
	seen := make(map[string]struct{}, len(blocks))
	filtered := make([]string, 0, len(blocks))
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if len(block) < 2 {
			continue
		}
		if _, ok := seen[block]; ok {
			continue
		}
		seen[block] = struct{}{}
		filtered = append(filtered, block)
	}
	return strings.Join(filtered, "\n\n")
}

func normalizeSpace(s string) string {
	s = html.UnescapeString(s)
	s = whitespaceRE.ReplaceAllString(strings.TrimSpace(s), " ")
	return s
}

func feedTitle(feed *gofeed.Feed) string {
	if feed == nil || strings.TrimSpace(feed.Title) == "" {
		return "Untitled feed"
	}
	return strings.TrimSpace(feed.Title)
}

func extractContent(item *gofeed.Item) string {
	if item == nil {
		return ""
	}
	switch {
	case strings.TrimSpace(item.Content) != "":
		return html.UnescapeString(item.Content)
	case strings.TrimSpace(item.Description) != "":
		return html.UnescapeString(item.Description)
	default:
		return ""
	}
}

func publishedAt(item *gofeed.Item) time.Time {
	if item == nil {
		return time.Time{}
	}
	if item.PublishedParsed != nil {
		return item.PublishedParsed.UTC()
	}
	if item.UpdatedParsed != nil {
		return item.UpdatedParsed.UTC()
	}
	return time.Time{}
}
