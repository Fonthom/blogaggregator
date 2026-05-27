package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

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

func handlerAgg(s *state, cmd command) error {
	feed, err := fetchFeed(context.Background(), "https://www.wagslane.dev/index.xml")
	if err != nil {
		return fmt.Errorf("error fetching feed: %w", err)
	}
	fmt.Printf("%+v\n", feed)
	return nil
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