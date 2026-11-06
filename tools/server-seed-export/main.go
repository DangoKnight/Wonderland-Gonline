// Command server-seed-export refreshes the repository's server-only SQL seed.
package main

import (
	"flag"
	"log"
	"wonderland-gonline/internal/assetsql"
)

func main() {
	database := flag.String("database", "", "offline current-schema assets database; gameplay databases are rejected")
	output := flag.String("output", "internal/assetsql/server_defaults/catalog.db.gz", "tracked compressed server-only catalog")
	flag.Parse()
	if *database == "" {
		log.Fatal("-database is required")
	}
	if err := assetsql.ExportRepositorySeed(*database, *output); err != nil {
		log.Fatal(err)
	}
}
