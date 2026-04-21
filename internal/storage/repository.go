package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrNotFound = errors.New("resource not found")

type Feed struct {
	ID        int64
	Title     string
	URL       string
	CreatedAt time.Time
}

type Article struct {
	ID          int64
	FeedID      int64
	FeedTitle   string
	Title       string
	Link        string
	Content     string
	PublishedAt time.Time
	IsRead      bool
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Init(ctx context.Context) error {
	schema := `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS feeds (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT NOT NULL,
	url TEXT UNIQUE NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS articles (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	feed_id INTEGER NOT NULL,
	title TEXT NOT NULL,
	link TEXT UNIQUE,
	content TEXT,
	published_at DATETIME,
	is_read BOOLEAN DEFAULT 0,
	FOREIGN KEY(feed_id) REFERENCES feeds(id) ON DELETE CASCADE
);
`
	_, err := r.db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("init schema: %w", err)
	}
	return nil
}

func (r *Repository) CreateFeed(ctx context.Context, title, url string) (Feed, error) {
	res, err := r.db.ExecContext(ctx, `INSERT INTO feeds (title, url) VALUES (?, ?)`, strings.TrimSpace(title), strings.TrimSpace(url))
	if err != nil {
		return Feed{}, fmt.Errorf("insert feed: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Feed{}, fmt.Errorf("feed id: %w", err)
	}
	return r.GetFeedByID(ctx, id)
}

func (r *Repository) UpdateFeedTitle(ctx context.Context, id int64, title string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE feeds SET title = ? WHERE id = ?`, strings.TrimSpace(title), id)
	if err != nil {
		return fmt.Errorf("update feed title: %w", err)
	}
	return nil
}

func (r *Repository) GetFeedByID(ctx context.Context, id int64) (Feed, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, title, url, created_at FROM feeds WHERE id = ?`, id)
	return scanFeed(row)
}

func (r *Repository) GetFeedByURL(ctx context.Context, url string) (Feed, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, title, url, created_at FROM feeds WHERE url = ?`, strings.TrimSpace(url))
	return scanFeed(row)
}

func (r *Repository) ListFeeds(ctx context.Context) ([]Feed, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, title, url, created_at FROM feeds ORDER BY title COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("list feeds: %w", err)
	}
	defer rows.Close()

	var feeds []Feed
	for rows.Next() {
		var feed Feed
		if err := rows.Scan(&feed.ID, &feed.Title, &feed.URL, &feed.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan feed: %w", err)
		}
		feeds = append(feeds, feed)
	}
	return feeds, rows.Err()
}

func (r *Repository) DeleteFeedByURL(ctx context.Context, url string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM feeds WHERE url = ?`, strings.TrimSpace(url))
	if err != nil {
		return fmt.Errorf("delete feed: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete feed affected: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) DeleteFeedByID(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM feeds WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete feed: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete feed affected: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) UpsertArticle(ctx context.Context, article Article) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO articles (feed_id, title, link, content, published_at, is_read)
VALUES (?, ?, ?, ?, ?, 0)
ON CONFLICT(link) DO UPDATE SET
	title = excluded.title,
	content = excluded.content,
	published_at = excluded.published_at
`, article.FeedID, article.Title, article.Link, article.Content, nullableTime(article.PublishedAt))
	if err != nil {
		return fmt.Errorf("upsert article: %w", err)
	}
	return nil
}

func (r *Repository) ListArticlesByFeed(ctx context.Context, feedID int64) ([]Article, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT a.id, a.feed_id, f.title, a.title, a.link, a.content, COALESCE(a.published_at, ''), a.is_read
FROM articles a
JOIN feeds f ON f.id = a.feed_id
WHERE a.feed_id = ?
ORDER BY COALESCE(a.published_at, a.id) DESC, a.id DESC
`, feedID)
	if err != nil {
		return nil, fmt.Errorf("list articles: %w", err)
	}
	defer rows.Close()

	var articles []Article
	for rows.Next() {
		article, err := scanArticle(rows)
		if err != nil {
			return nil, err
		}
		articles = append(articles, article)
	}
	return articles, rows.Err()
}

func (r *Repository) GetArticleByID(ctx context.Context, id int64) (Article, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT a.id, a.feed_id, f.title, a.title, a.link, a.content, COALESCE(a.published_at, ''), a.is_read
FROM articles a
JOIN feeds f ON f.id = a.feed_id
WHERE a.id = ?
`, id)
	return scanArticle(row)
}

func (r *Repository) MarkArticleRead(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE articles SET is_read = 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("mark article read: %w", err)
	}
	return nil
}

func scanFeed(scanner interface{ Scan(dest ...any) error }) (Feed, error) {
	var feed Feed
	if err := scanner.Scan(&feed.ID, &feed.Title, &feed.URL, &feed.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Feed{}, ErrNotFound
		}
		return Feed{}, fmt.Errorf("scan feed: %w", err)
	}
	return feed, nil
}

func scanArticle(scanner interface{ Scan(dest ...any) error }) (Article, error) {
	var article Article
	var published string
	if err := scanner.Scan(
		&article.ID,
		&article.FeedID,
		&article.FeedTitle,
		&article.Title,
		&article.Link,
		&article.Content,
		&published,
		&article.IsRead,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Article{}, ErrNotFound
		}
		return Article{}, fmt.Errorf("scan article: %w", err)
	}
	if published != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, published); err == nil {
			article.PublishedAt = parsed
		} else if parsed, err := time.Parse("2006-01-02 15:04:05", published); err == nil {
			article.PublishedAt = parsed
		}
	}
	return article, nil
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}
