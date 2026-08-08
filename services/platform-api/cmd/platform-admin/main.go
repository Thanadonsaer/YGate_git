package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"ygate/platform-api/internal/bootstrap"
	"ygate/platform-api/internal/config"
	"ygate/platform-api/internal/database"
	"ygate/platform-api/internal/envfile"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "bootstrap-user" && os.Args[1] != "bootstrap-middleware" && os.Args[1] != "migrate") {
		log.Fatal("usage: platform-admin bootstrap-user|bootstrap-middleware|migrate")
	}
	if err := envfile.LoadDefault(); err != nil {
		log.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// database.Open applies every embedded forward-only migration that is not
	// yet in schema_migrations before returning (see internal/database), under
	// a Postgres advisory lock so concurrent instances don't race. Running
	// this subcommand IS "migrate deploy": the target database must already
	// exist, and this only brings its schema up to date — no separate
	// migration runner needed.
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if os.Args[1] == "migrate" {
		fmt.Println("database schema is up to date")
		return
	}
	if os.Args[1] == "bootstrap-middleware" {
		input, inputErr := bootstrap.MiddlewareInputFromEnvironment()
		if inputErr != nil {
			log.Fatal(inputErr)
		}
		key, bootstrapErr := bootstrap.CreateMiddleware(ctx, pool, input)
		if bootstrapErr != nil {
			log.Fatal(bootstrapErr)
		}
		fmt.Printf("middleware API key (shown once): %s\n", key)
		return
	}
	input, err := bootstrap.UserInputFromEnvironment()
	if err != nil {
		log.Fatal(err)
	}
	if err = bootstrap.CreateUser(ctx, pool, input); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("bootstrap user created: %s (%s)\n", input.Email, input.OrganizationCode)
}
