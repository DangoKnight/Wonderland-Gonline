// Command database-migrate creates verified, upgraded copies of both databases.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"wonderland-go/internal/config"
	"wonderland-go/internal/dbmigration"
)

func main() {
	configPath := flag.String("config", "config.local.json", "startup configuration selecting source databases")
	output := flag.String("output-dir", "", "new directory for upgraded copies; stop the server first")
	flag.Parse()
	if err := run(*configPath, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(configPath, output string) error {
	if output == "" {
		return fmt.Errorf("-output-dir is required; source databases are never replaced")
	}
	c, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if err = os.Mkdir(output, 0700); err != nil {
		return err
	}
	success := false
	defer func() {
		if !success {
			os.RemoveAll(output)
		}
	}()
	targets := []struct {
		source, name string
		assets       bool
	}{{c.AssetsDatabase, "assets.db", true}, {c.Database, "wonderland.db", false}}
	for _, target := range targets {
		path := filepath.Join(output, target.name)
		fmt.Println("Preparing", path)
		if err = dbmigration.Copy(target.source, path, target.assets); err != nil {
			return err
		}
		fmt.Println("Verified", path)
	}
	success = true
	fmt.Println("Sources preserved. Set assets_database and database in the startup configuration to these paths before starting the upgraded server.")
	return nil
}
