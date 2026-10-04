// Command data-rebuild recreates the SQL asset catalog and optionally gameplay DB.
package main

import (
	"flag"
	"fmt"
	"os"

	"wonderland-go/internal/assetdb"
	"wonderland-go/internal/assetsql"
	"wonderland-go/internal/config"
)

func main() {
	data := flag.String("data", "data", "directory containing asset_manifest.json and exports")
	assetsDB := flag.String("assets-db", "", "SQL asset database to recreate; existing file is backed up")
	configPath := flag.String("config", "", "server config selecting assets and gameplay databases")
	resetGameplay := flag.Bool("reset-gameplay", false, "also erase accounts, characters, settings and audit; existing DB is backed up; stop the server first")
	flag.Parse()
	settings, err := config.Load(*configPath)
	if err != nil {
		fail(err)
	}
	if *assetsDB == "" {
		*assetsDB = settings.AssetsDatabase
	}
	options := assetdb.RebuildOptions{DataDirectory: *data, AssetsDB: *assetsDB, PrepareAssets: assetsql.MigrateDatabase, Progress: func(asset string) { fmt.Println("Imported", asset) }}
	if *resetGameplay {
		options.GameplayDB = settings.Database
	}
	result, err := assetdb.Rebuild(options)
	if err != nil {
		fail(err)
	}
	fmt.Printf("Rebuilt %s: %d assets, %d SQL records; audio payloads remain on disk\n", result.AssetsDB, result.Assets, result.Records)
	if result.GameplayDB != "" {
		fmt.Println("Reset gameplay database:", result.GameplayDB)
	}
	for _, backup := range result.Backups {
		fmt.Println("Backup:", backup)
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
