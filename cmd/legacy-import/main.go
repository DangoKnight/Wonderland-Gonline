// Command legacy-import inspects/imports a read-only C# SQLite snapshot offline.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"wonderland-gonline/internal/assetsql"
	"wonderland-gonline/internal/legacyimport"
	"wonderland-gonline/internal/store"
)

func main() {
	source := flag.String("source", "", "Private Server SQLite snapshot, opened read-only")
	assets := flag.String("assets-db", "", "upgraded Go assets.db for definition validation")
	output := flag.String("output-dir", "", "new directory for wonderland.db; omitted inspects only")
	skip := flag.String("skip-tables", "", "explicit comma-separated unmapped tables to exclude")
	flag.Parse()
	if err := run(*source, *assets, *output, *skip); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(source, assetsPath, output, skip string) error {
	if source == "" || assetsPath == "" {
		return fmt.Errorf("-source and -assets-db are required")
	}
	catalog, err := assetsql.LoadDatabase(assetsPath)
	if err != nil {
		return err
	}
	ctx := context.Background()
	plan, err := legacyimport.Read(ctx, source, catalog)
	if err != nil {
		return err
	}
	return apply(plan, output, skip)
}

func apply(plan legacyimport.Plan, output, skip string) error {
	ctx := context.Background()
	r := plan.Report
	fmt.Printf("Validated %d accounts, %d characters, %d items, %d pets, %d skills, %d quests\n", r.Accounts, r.Characters, r.Items, r.Pets, r.Skills, r.Quests)
	omitted := map[string]bool{}
	for _, name := range strings.Split(skip, ",") {
		if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
			omitted[name] = true
		}
	}
	unresolved := false
	names := make([]string, 0, len(r.Unmapped))
	for name := range r.Unmapped {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		count := r.Unmapped[name]
		fmt.Printf("Unmapped table %s: %d rows\n", name, count)
		if !omitted[strings.ToLower(name)] {
			unresolved = true
		}
	}
	if output == "" {
		fmt.Println("Inspection only. Source and destination databases unchanged.")
		return nil
	}
	if unresolved {
		return fmt.Errorf("unmapped tables require review; use -skip-tables only for tables intentionally excluded")
	}
	if err := os.Mkdir(output, 0700); err != nil {
		return err
	}
	success := false
	defer func() {
		if !success {
			os.RemoveAll(output)
		}
	}()
	target := filepath.Join(output, "wonderland.db")
	db, err := store.Open(target)
	if err != nil {
		return err
	}
	if err = db.ImportLegacy(ctx, plan.Accounts, plan.Friendships...); err != nil {
		db.Close()
		return err
	}
	if err = db.Close(); err != nil {
		return err
	}
	db, err = store.Open(target)
	if err != nil {
		return err
	}
	defer db.Close()
	for _, account := range plan.Accounts {
		chars, e := db.Characters(ctx, account.Account.ID)
		if e != nil {
			return e
		}
		if len(chars) != len(account.Characters) {
			return fmt.Errorf("import verification count mismatch")
		}
		byID := map[uint32]int{}
		for i, c := range chars {
			byID[c.ID] = i
		}
		for _, c := range account.Characters {
			at, ok := byID[c.ID]
			if !ok || !reflect.DeepEqual(c, chars[at]) {
				return fmt.Errorf("import verification mismatch for character %d", c.ID)
			}
		}
	}
	for _, f := range plan.Friendships {
		friends, e := db.Friends(ctx, f.Character1)
		if e != nil {
			return e
		}
		found := false
		for _, c := range friends {
			found = found || c.ID == f.Character2
		}
		if !found {
			return fmt.Errorf("import verification friendship mismatch")
		}
	}
	success = true
	fmt.Println("Verified", target, "— source preserved; set database in startup configuration to activate.")
	return nil
}
