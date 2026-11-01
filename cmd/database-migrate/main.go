// Command database-migrate upgrades both configured databases in place.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/dbmigration"
)

func main() {
	configPath := flag.String("config", "config.local.json", "startup configuration selecting the databases; stop the server first")
	flag.Parse()
	if err := run(*configPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run upgrades the configured databases where they are: verified copies
// replace them, and the originals move to a backups directory beside them.
func run(configPath string) error {
	c, err := config.Load(configPath)
	if err != nil {
		return err
	}
	backups, err := dbmigration.UpgradeInPlace([]dbmigration.Target{
		{Path: c.AssetsDatabase, Assets: true},
		{Path: c.Database},
	}, time.Now().Format("20060102-150405"))
	if err != nil {
		return err
	}
	fmt.Println("Upgraded", c.AssetsDatabase, "and", c.Database)
	for _, b := range backups {
		fmt.Println("Originals kept in", b)
	}
	return nil
}
