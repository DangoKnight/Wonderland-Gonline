// live-test prepares isolated database fixtures for manual legacy-client testing.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"wonderland-gonline/internal/assetsql"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/livetest"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	path := flag.String("config", "config.local.json", "existing server config; assets and listener settings only")
	assetsPath := flag.String("assets-database", "", "optional current SQL asset database copy; source database stays read-only")
	scenario := flag.String("scenario", "", "scenario ID; use -list")
	output := flag.String("output", "", "new run directory, preferably under ignored var/; parent must exist")
	list := flag.Bool("list", false, "list available legacy-client scenarios")
	flag.Parse()
	if *list {
		for _, s := range livetest.Scenarios() {
			fmt.Printf("%-22s %s\n", s.ID, s.Description)
		}
		return nil
	}
	if *output == "" || *scenario == "" {
		return fmt.Errorf("provide -scenario and -output, or use -list")
	}
	if _, err := livetest.Find(*scenario); err != nil {
		return err
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	if *assetsPath != "" {
		cfg.AssetsDatabase = *assetsPath
	}
	a, err := assetsql.LoadDatabase(cfg.AssetsDatabase)
	if err != nil {
		return err
	}
	report, err := livetest.Prepare(context.Background(), cfg, a, *scenario, *output)
	if err != nil {
		return err
	}
	fmt.Printf("Scenario prepared. Credentials, startup command and checklist: %s\nNo server was started; acceptance status is NOT RUN.\n", report)
	return nil
}
