package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"
	"strconv"
	"strings"
	"flag"

	"github.com/Fonthom/gator/internal/config"
	"github.com/Fonthom/gator/internal/database"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

type state struct {
	db  *database.Queries
	cfg *config.Config
}

type command struct {
	name string
	args []string
}

type commands struct {
	handlers map[string]func(*state, command) error
}

func (c *commands) run(s *state, cmd command) error {
	handler, ok := c.handlers[cmd.name]
	if !ok {
		return fmt.Errorf("unknown command: %s", cmd.name)
	}
	return handler(s, cmd)
}

func (c *commands) register(name string, f func(*state, command) error) {
	c.handlers[name] = f
}

func handlerLogin(s *state, cmd command) error {
	if len(cmd.args) == 0 {
		return fmt.Errorf("login requires a username argument")
	}
	username := cmd.args[0]

	_, err := s.db.GetUser(context.Background(), username)
	if err != nil {
		fmt.Fprintln(os.Stderr, "user does not exist")
		os.Exit(1)
	}

	if err := s.cfg.SetUser(username); err != nil {
		return fmt.Errorf("error setting user: %w", err)
	}
	fmt.Printf("User has been set to: %s\n", username)
	return nil
}

func handlerRegister(s *state, cmd command) error {
	if len(cmd.args) == 0 {
		return fmt.Errorf("register requires a name argument")
	}
	name := cmd.args[0]

	user, err := s.db.CreateUser(context.Background(), database.CreateUserParams{
		ID:        uuid.New(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Name:      name,
	})
	if err != nil {
    	fmt.Fprintf(os.Stderr, "error creating user: %v\n", err)
    	os.Exit(1)
	}	

	if err := s.cfg.SetUser(name); err != nil {
		return fmt.Errorf("error setting current user: %w", err)
	}

	fmt.Printf("User created successfully: %s\n", name)
	fmt.Printf("  ID:         %v\n", user.ID)
	fmt.Printf("  Name:       %v\n", user.Name)
	fmt.Printf("  CreatedAt:  %v\n", user.CreatedAt)
	fmt.Printf("  UpdatedAt:  %v\n", user.UpdatedAt)
	return nil
}

func handlerReset(s *state, cmd command) error {
	if err := s.db.DeleteAllUsers(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "error resetting database: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Database reset successfully")
	return nil
}

func middlewareLoggedIn(handler func(s *state, cmd command, user database.User) error) func(*state, command) error {
	return func(s *state, cmd command) error {
		user, err := s.db.GetUser(context.Background(), s.cfg.CurrentUserName)
		if err != nil {
			return fmt.Errorf("error getting current user: %w", err)
		}
		return handler(s, cmd, user)
	}
}

func handlerGetUsers(s *state, cmd command) error {
	users, err := s.db.GetUsers(context.Background())
	if err != nil {
		return fmt.Errorf("error getting users: %w", err)
	}
	for _, user := range users {
		if user.Name == s.cfg.CurrentUserName {
			fmt.Printf("* %s (current)\n", user.Name)
		} else {
			fmt.Printf("* %s\n", user.Name)
		}
	}
	return nil
}

func scrapeFeeds(s *state) {
	feed, err := s.db.GetNextFeedToFetch(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting next feed to fetch: %v\n", err)
		return
	}

	err = s.db.MarkFeedFetched(context.Background(), feed.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marking feed as fetched: %v\n", err)
		return
	}

	rssFeed, err := fetchFeed(context.Background(), feed.Url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching feed %s: %v\n", feed.Url, err)
		return
	}

	for _, item := range rssFeed.Channel.Item {
		publishedAt := parsePublishedAt(item.PubDate)

		_, err := s.db.CreatePost(context.Background(), database.CreatePostParams{
			ID:          uuid.New(),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Title:       item.Title,
			Url:         item.Link,
			Description: sql.NullString{String: item.Description, Valid: item.Description != ""},
			PublishedAt: publishedAt,
			FeedID:      feed.ID,
		})
		if err != nil {
			// ignore duplicate URL errors, log everything else
			if !strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
				fmt.Fprintf(os.Stderr, "error saving post %s: %v\n", item.Link, err)
			}
		}
	}
	fmt.Printf("Fetched %d posts from %s\n", len(rssFeed.Channel.Item), feed.Name)
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

func handlerAddFeed(s *state, cmd command, user database.User) error {
	if len(cmd.args) < 2 {
		return fmt.Errorf("addfeed requires a name and url argument")
	}
	name := cmd.args[0]
	url := cmd.args[1]

	feed, err := s.db.CreateFeed(context.Background(), database.CreateFeedParams{
		ID:        uuid.New(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Name:      name,
		Url:       url,
		UserID:    user.ID,
	})
	if err != nil {
		return fmt.Errorf("error creating feed: %w", err)
	}

	fmt.Printf("Feed created successfully:\n")
	fmt.Printf("  ID:        %v\n", feed.ID)
	fmt.Printf("  Name:      %v\n", feed.Name)
	fmt.Printf("  URL:       %v\n", feed.Url)
	fmt.Printf("  UserID:    %v\n", feed.UserID)
	fmt.Printf("  CreatedAt: %v\n", feed.CreatedAt)
	fmt.Printf("  UpdatedAt: %v\n", feed.UpdatedAt)

	feedFollow, err := s.db.CreateFeedFollow(context.Background(), database.CreateFeedFollowParams{
    	ID:        uuid.New(),
    	CreatedAt: time.Now(),
    	UpdatedAt: time.Now(),
    	UserID:    user.ID,
    	FeedID:    feed.ID,
	})
	if err != nil {
    	return fmt.Errorf("error creating feed follow: %w", err)
	}
	fmt.Printf("Now following feed: %s\n", feedFollow.FeedName)

	return nil
}

func handlerGetFeeds(s *state, cmd command) error {
	feeds, err := s.db.GetFeeds(context.Background())
	if err != nil {
		return fmt.Errorf("error getting feeds: %w", err)
	}
	for _, feed := range feeds {
		fmt.Printf("* %s\n", feed.Name)
		fmt.Printf("  URL:  %s\n", feed.Url)
		fmt.Printf("  User: %s\n", feed.UserName)
	}
	return nil
}

func handlerFollow(s *state, cmd command, user database.User) error {
	if len(cmd.args) < 1 {
		return fmt.Errorf("follow requires a url argument")
	}
	url := cmd.args[0]

	feed, err := s.db.GetFeedByURL(context.Background(), url)
	if err != nil {
		return fmt.Errorf("error getting feed: %w", err)
	}

	feedFollow, err := s.db.CreateFeedFollow(context.Background(), database.CreateFeedFollowParams{
		ID:        uuid.New(),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		UserID:    user.ID,
		FeedID:    feed.ID,
	})
	if err != nil {
		return fmt.Errorf("error creating feed follow: %w", err)
	}

	fmt.Printf("Following feed:\n")
	fmt.Printf("  Feed: %s\n", feedFollow.FeedName)
	fmt.Printf("  User: %s\n", feedFollow.UserName)
	return nil
}

func handlerFollowing(s *state, cmd command, user database.User) error {
	feedFollows, err := s.db.GetFeedFollowsForUser(context.Background(), user.ID)
	if err != nil {
		return fmt.Errorf("error getting feed follows: %w", err)
	}

	fmt.Printf("Feeds followed by %s:\n", user.Name)
	for _, ff := range feedFollows {
		fmt.Printf("  * %s\n", ff.FeedName)
	}
	return nil
}

func handlerUnfollow(s *state, cmd command, user database.User) error {
	if len(cmd.args) < 1 {
		return fmt.Errorf("unfollow requires a url argument")
	}
	url := cmd.args[0]

	feed, err := s.db.GetFeedByURL(context.Background(), url)
	if err != nil {
		return fmt.Errorf("error getting feed: %w", err)
	}

	err = s.db.DeleteFeedFollow(context.Background(), database.DeleteFeedFollowParams{
		UserID: user.ID,
		FeedID: feed.ID,
	})
	if err != nil {
		return fmt.Errorf("error unfollowing feed: %w", err)
	}

	fmt.Printf("Unfollowed feed: %s\n", feed.Name)
	return nil
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
		t, err := time.Parse(format, s)
		if err == nil {
			return sql.NullTime{Time: t, Valid: true}
		}
	}
	return sql.NullTime{Valid: false}
}

func handlerBrowse(s *state, cmd command, user database.User) error {
	flags := flag.NewFlagSet("browse", flag.ContinueOnError)
	limit := flags.Int("limit", 2, "number of posts to show")
	sort := flags.String("sort", "newest", "sort order: newest or oldest")
	feed := flags.String("feed", "", "filter by feed name")
	if err := flags.Parse(cmd.args); err != nil {
		return fmt.Errorf("error parsing flags: %w", err)
	}

	var posts []database.Post
	var err error
	ctx := context.Background()

	switch {
	case *feed != "" && *sort == "oldest":
		posts, err = s.db.GetPostsForUserFilteredOldest(ctx, database.GetPostsForUserFilteredOldestParams{
			UserID: user.ID,
			Name:   *feed,
			Limit:  int32(*limit),
		})
	case *feed != "":
		posts, err = s.db.GetPostsForUserFiltered(ctx, database.GetPostsForUserFilteredParams{
			UserID: user.ID,
			Name:   *feed,
			Limit:  int32(*limit),
		})
	case *sort == "oldest":
		posts, err = s.db.GetPostsForUserOldest(ctx, database.GetPostsForUserOldestParams{
			UserID: user.ID,
			Limit:  int32(*limit),
		})
	default:
		posts, err = s.db.GetPostsForUser(ctx, database.GetPostsForUserParams{
			UserID: user.ID,
			Limit:  int32(*limit),
		})
	}
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

func main() {
	cfg, err := config.Read()
	if err != nil {
		log.Fatalf("error reading config: %v", err)
	}

	db, err := sql.Open("postgres", cfg.DbURL)
	if err != nil {
		log.Fatalf("error opening database: %v", err)
	}

	dbQueries := database.New(db)

	s := &state{
		db:  dbQueries,
		cfg: &cfg,
	}

	cmds := &commands{
		handlers: make(map[string]func(*state, command) error),
	}

	cmds.register("login", handlerLogin)
	cmds.register("register", handlerRegister)
	cmds.register("reset", handlerReset)
	cmds.register("users", handlerGetUsers)
	cmds.register("agg", handlerAgg)
	cmds.register("addfeed", middlewareLoggedIn(handlerAddFeed))
	cmds.register("feeds", handlerGetFeeds)
	cmds.register("follow", middlewareLoggedIn(handlerFollow))
	cmds.register("following", middlewareLoggedIn(handlerFollowing))
	cmds.register("unfollow", middlewareLoggedIn(handlerUnfollow))
	cmds.register("browse", middlewareLoggedIn(handlerBrowse))

	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "error: not enough arguments, a command is required")
		os.Exit(1)
	}

	cmdName := os.Args[1]
	cmdArgs := os.Args[2:]

	if err := cmds.run(s, command{name: cmdName, args: cmdArgs}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}