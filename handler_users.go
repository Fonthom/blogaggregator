package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Fonthom/gator/internal/database"
	"github.com/google/uuid"
)

func handlerLogin(s *state, cmd command) error {
	if len(cmd.args) == 0 {
		return fmt.Errorf("login requires a username argument")
	}
	username := cmd.args[0]

	if _, err := s.db.GetUser(context.Background(), username); err != nil {
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

	fmt.Printf("User created successfully:\n")
	fmt.Printf("  ID:        %v\n", user.ID)
	fmt.Printf("  Name:      %v\n", user.Name)
	fmt.Printf("  CreatedAt: %v\n", user.CreatedAt)
	fmt.Printf("  UpdatedAt: %v\n", user.UpdatedAt)
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