package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Fonthom/gator/internal/database"
	"github.com/google/uuid"
)

func scrapeFeeds(s *state) {
	feed, err := s.db.GetNextFeedToFetch(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting next feed to fetch: %v\n", err)
		return
	}

	if err := s.db.MarkFeedFetched(context.Background(), feed.ID); err != nil {
		fmt.Fprintf(os.Stderr, "error marking feed as fetched: %v\n", err)
		return
	}

	rssFeed, err := fetchFeed(context.Background(), feed.Url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching feed %s: %v\n", feed.Url, err)
		return
	}

	for _, item := range rssFeed.Channel.Item {
		_, err := s.db.CreatePost(context.Background(), database.CreatePostParams{
			ID:          uuid.New(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Title:       item.Title,
			Url:         item.Link,
			Description: sql.NullString{String: item.Description, Valid: item.Description != ""},
			PublishedAt: parsePublishedAt(item.PubDate),
			FeedID:      feed.ID,
		})
		if err != nil && !strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			fmt.Fprintf(os.Stderr, "error saving post %s: %v\n", item.Link, err)
		}
	}
	fmt.Printf("Fetched %d posts from %s\n", len(rssFeed.Channel.Item), feed.Name)
}

func parsePublishedAt(s string) sql.NullTime {
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		"2006-01-02T15:04:05Z",
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 MST",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return sql.NullTime{Time: t, Valid: true}
		}
	}
	return sql.NullTime{Valid: false}
}

func handlerAgg(s *state, cmd command) error {
	if len(cmd.args) < 1 {
		return fmt.Errorf("agg requires a time_between_reqs argument (e.g. 1s, 1m, 1h)")
	}

	timeBetweenRequests, err := time.ParseDuration(cmd.args[0])
	if err != nil {
		return fmt.Errorf("invalid duration: %w", err)
	}

	fmt.Printf("Collecting feeds every %s\n", timeBetweenRequests)

	ticker := time.NewTicker(timeBetweenRequests)
	for ; ; <-ticker.C {
		scrapeFeeds(s)
	}
}

func handlerBrowse(s *state, cmd command, user database.User) error {
	flags := flag.NewFlagSet("browse", flag.ContinueOnError)
	limit := flags.Int("limit", 2, "number of posts to show")
	sort := flags.String("sort", "newest", "sort order: newest or oldest")
	feed := flags.String("feed", "", "filter by feed name")
	if err := flags.Parse(cmd.args); err != nil {
		return fmt.Errorf("error parsing flags: %w", err)
	}

	posts, err := fetchPosts(s, user, *limit, *sort, *feed)
	if err != nil {
		return fmt.Errorf("error getting posts: %w", err)
	}

	for _, post := range posts {
		fmt.Printf("--- %s ---\n", post.Title)
		fmt.Printf("  URL:  %s\n", post.Url)
		if post.Description.Valid {
			fmt.Printf("  Desc: %s\n", post.Description.String)
		}
		if post.PublishedAt.Valid {
			fmt.Printf("  Published: %s\n", post.PublishedAt.Time.Format(time.RFC822))
		}
		fmt.Println()
	}
	return nil
}

func fetchPosts(s *state, user database.User, limit int, sort, feed string) ([]database.Post, error) {
	ctx := context.Background()
	switch {
	case feed != "" && sort == "oldest":
		return s.db.GetPostsForUserFilteredOldest(ctx, database.GetPostsForUserFilteredOldestParams{
			UserID: user.ID,
			Name:   feed,
			Limit:  int32(limit),
		})
	case feed != "":
		return s.db.GetPostsForUserFiltered(ctx, database.GetPostsForUserFilteredParams{
			UserID: user.ID,
			Name:   feed,
			Limit:  int32(limit),
		})
	case sort == "oldest":
		return s.db.GetPostsForUserOldest(ctx, database.GetPostsForUserOldestParams{
			UserID: user.ID,
			Limit:  int32(limit),
		})
	default:
		return s.db.GetPostsForUser(ctx, database.GetPostsForUserParams{
			UserID: user.ID,
			Limit:  int32(limit),
		})
	}
}