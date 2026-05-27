# Gator
 
A multi-user RSS feed aggregator CLI. Add feeds, follow them, and browse the latest posts — all from your terminal.
 
## Prerequisites
 
- [Go](https://go.dev/doc/install) 1.26 or later
- [PostgreSQL](https://www.postgresql.org/download/)
- [lib/pq](https://github.com/lib/pq) — the PostgreSQL driver for Go:
```bash
go get github.com/lib/pq
```
 
Make sure Go and PostgreSQL are installed and available on your `PATH` before continuing.
 
## Installation
 
```bash
go install github.com/Fonthom/gator@latest
```
 
This installs the `gator` binary to your `$GOPATH/bin`. Make sure that directory is on your `PATH`.
 
## Configuration
 
Gator reads its config from `~/.gatorconfig.json`. Create that file and add your PostgreSQL connection string:
 
```json
{
  "db_url": "postgres://username:password@localhost:5432/gator?sslmode=disable"
}
```
 
Replace `username`, `password`, and `gator` with your actual Postgres credentials and database name. If you haven't created the database yet:
 
```bash
psql -c "CREATE DATABASE gator;"
```
 
### Running Migrations
 
Install [Goose](https://github.com/pressly/goose):
 
```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
```
 
Then run the migrations from the `sql/schema` directory:
 
```bash
cd sql/schema
goose postgres "postgres://username:password@localhost:5432/gator" up
```
 
## Usage
 
### User management
 
```bash
# Register a new user and log in as them
gator register <name>
 
# Log in as an existing user
gator login <name>
 
# List all users (* marks the current user)
gator users
 
# Reset the database (deletes all users and their data)
gator reset
```
 
### Feed management
 
```bash
# Add a new feed and automatically follow it
gator addfeed "Hacker News" "https://news.ycombinator.com/rss"
 
# List all feeds in the database
gator feeds
 
# Follow an existing feed by URL
gator follow "https://news.ycombinator.com/rss"
 
# Unfollow a feed by URL
gator unfollow "https://news.ycombinator.com/rss"
 
# List all feeds the current user is following
gator following
```
 
### Aggregation and browsing
 
```bash
# Start the feed aggregator (runs continuously, Ctrl+C to stop)
# Pass a duration string for how often to fetch feeds
# Don't do a DOS, pass either a fairly long duration string or keep run time at a minimum.
gator agg 1m
 
# Browse the latest posts from feeds you follow
gator browse        # defaults to 2 posts
gator browse --limit=10     # show 10 posts
gator browse --feed="Hacker News"             # 2 newest from Hacker News
gator browse --feed="Hacker News" --limit=5   # 5 newest from Hacker News
gator browse --feed="Hacker News" --sort=oldest --limit=5
```
 
### Example workflow
 
```bash
gator register fonthom
gator addfeed "Boot.dev Blog" "https://www.boot.dev/blog/index.xml"
gator addfeed "Hacker News" "https://news.ycombinator.com/rss"
gator agg 30s       
gator browse --limit=5 
```