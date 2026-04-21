package rss

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/danterobles/rss-cli/internal/storage"
	"github.com/mmcdole/gofeed"
)

type Service struct {
	parser *gofeed.Parser
	repo   *storage.Repository
}

func NewService(repo *storage.Repository) *Service {
	return &Service{
		parser: gofeed.NewParser(),
		repo:   repo,
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
			Content:     extractContent(item),
			PublishedAt: publishedAt(item),
		}
		if article.Title == "" {
			article.Title = article.Link
		}
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
