package migrate

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func sortRows(rows []StatusRow) {
	sort.Slice(rows, func(i, j int) bool { return rows[i].Version < rows[j].Version })
}

// Main is the entry point of a project's migration runner:
//
//	runner [-uri URI] [-db NAME] up | down [n] | status
//
// The connection defaults to the MONGO_URI and MONGO_DB environment variables.
func Main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	uri := fs.String("uri", os.Getenv("MONGO_URI"), "MongoDB connection string (default $MONGO_URI)")
	dbName := fs.String("db", os.Getenv("MONGO_DB"), "database name (default $MONGO_DB)")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: migrate [-uri URI] [-db NAME] up | down [n] | status")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	cmd, rest := "up", fs.Args()
	if len(rest) > 0 {
		cmd, rest = rest[0], rest[1:]
	}
	steps := 1
	if cmd == "down" && len(rest) > 0 {
		n, err := strconv.Atoi(rest[0])
		if err != nil || n < 1 {
			return fmt.Errorf("down: expected a positive step count, got %q", rest[0])
		}
		steps = n
	}
	switch cmd {
	case "up", "down", "status":
	default:
		fs.Usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
	if *uri == "" {
		return fmt.Errorf("no connection string: set MONGO_URI or pass -uri")
	}
	if *dbName == "" {
		return fmt.Errorf("no database: set MONGO_DB or pass -db")
	}

	ctx := context.Background()
	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(connectCtx, options.Client().ApplyURI(*uri))
	if err != nil {
		return err
	}
	defer client.Disconnect(ctx)

	r := &Runner{DB: client.Database(*dbName), Log: os.Stdout}
	switch cmd {
	case "up":
		n, err := r.Up(ctx)
		if err == nil && n == 0 {
			fmt.Println("nothing to migrate")
		}
		return err
	case "down":
		n, err := r.Down(ctx, steps)
		if err == nil && n == 0 {
			fmt.Println("nothing to roll back")
		}
		return err
	default:
		rows, err := r.Status(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("database: %s\n\n %-6s %-16s %s\n", *dbName, "Status", "Version", "Name")
		for _, row := range rows {
			state := "down"
			if row.Applied {
				state = "up"
			}
			name := row.Name
			if row.Up == nil {
				name += " (NO FILE)"
			}
			fmt.Printf(" %-6s %-16d %s\n", state, row.Version, name)
		}
		return nil
	}
}
