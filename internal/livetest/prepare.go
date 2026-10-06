package livetest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/store"
)

const (
	privateDirectoryMode  = 0700
	privateFileMode       = 0600
	credentialRandomBytes = 5 // Ten hex characters fit the original client's UI limit.
	adminTokenRandomBytes = 24
)

func secret(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// Prepare exclusively creates a new directory/database; it never edits the
// configured gameplay database. Credentials stay in the private instructions.
func Prepare(ctx context.Context, cfg config.Config, a *assets.Catalog, scenario, output string) (report string, err error) {
	plan, err := Find(scenario)
	if err != nil {
		return "", err
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return "", err
	}
	cfg.AssetsDatabase, err = filepath.Abs(cfg.AssetsDatabase)
	if err != nil {
		return "", err
	}
	cfg.Database = filepath.Join(output, "wonderland.db")
	if err = config.Validate(cfg); err != nil {
		return "", err
	}
	// Validate prerequisites before creating any account or output artifacts.
	if _, err = Character(a, scenario, 10001, "LiveTester", false); err != nil {
		return "", err
	}
	if err = os.Mkdir(output, privateDirectoryMode); err != nil {
		return "", fmt.Errorf("create new run directory (parent must exist): %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(output)
		}
	}()
	db, err := store.Open(cfg.Database)
	if err != nil {
		return "", err
	}
	defer db.Close()
	var credentials strings.Builder
	for _, observer := range []bool{false, true} {
		user, e := secret(credentialRandomBytes)
		if e != nil {
			return "", e
		}
		password, e := secret(credentialRandomBytes)
		if e != nil {
			return "", e
		}
		account, e := db.Register(ctx, user, password, "")
		if e != nil {
			return "", e
		}
		name, role := "LiveTester", "Tester"
		if observer {
			name, role = "LiveObserver", "Observer"
		}
		c, e := Character(a, scenario, account.CharacterID(1), name, observer)
		if e != nil {
			return "", e
		}
		if e = db.CreateCharacterWithCode(ctx, account, c, password); e != nil {
			return "", e
		}
		fmt.Fprintf(&credentials, "## %s\n\nUsername: `%s`  \nPassword: `%s`  \nDeletion code: same as password  \nCharacter: `%s` (slot 1, ID %d)  \nMap: %d, position: %d, %d\n\n", role, user, password, name, c.ID, c.Map, c.X, c.Y)
		if !observer && strings.HasPrefix(scenario, "compound-") {
			describeCompounding(&credentials, a, c)
		}
		if !observer && strings.HasPrefix(scenario, "fishing") {
			for _, rod := range a.Fishing.Rods {
				fmt.Fprintf(&credentials, "Configured rod: %d (%s), maximum grade %d.  \n", rod.ItemID, a.Items[rod.ItemID].Name, rod.MaxGrade)
			}
			fmt.Fprintf(&credentials, "Per-grade catch requirements: %v.  \n", a.Fishing.CatchRequirements)
			for _, skill := range c.Skills {
				for _, id := range a.Fishing.Skills {
					if skill.ID == id {
						fmt.Fprintf(&credentials, "Prepared fishing skill: %d, grade %d, per-grade EXP %d.  \n", skill.ID, skill.Grade, skill.EXP)
					}
				}
			}
			credentials.WriteString("\n")
		}
	}
	token, err := secret(adminTokenRandomBytes)
	if err != nil {
		return "", err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	configPath := filepath.Join(output, "config.json")
	if err = os.WriteFile(configPath, append(raw, '\n'), privateFileMode); err != nil {
		return "", err
	}
	report = filepath.Join(output, "instructions.md")
	var text strings.Builder
	fmt.Fprintf(&text, "# Live test: %s\n\n%s\n\nStatus: NOT RUN. Preparation is not a passing acceptance test.\n\n", plan.ID, plan.Description)
	text.WriteString(credentials.String())
	fmt.Fprintf(&text, "## Start the test server\n\nStop the regular server first: this configuration preserves its listener addresses. From the project root run:\n\n```sh\nexport WONDERLAND_ADMIN_TOKEN=%s\ngo run ./cmd/wonderland -config %s -debug -log-file %s\n```\n\n", token, shellQuote(configPath), shellQuote(filepath.Join(output, "server-diagnostics.jsonl")))
	fmt.Fprintf(&text, "Point the legacy launcher/aLogin at this test server using your usual local-server setup. Login listener: `%s`; world: `%s`; launcher status: `%s`. Credentials above are for this isolated test database only. Assets remain read-only at `%s`.\n\n", cfg.Login, cfg.World, cfg.Status, cfg.AssetsDatabase)
	fmt.Fprintf(&text, "Fishing interval from this catalog: %d seconds.\n\n## Checklist\n\n", a.Fishing.IntervalSeconds)
	for _, step := range plan.Steps {
		fmt.Fprintf(&text, "- [ ] %s\n", step)
	}
	text.WriteString("\n## Evidence and result\n\nFor each step record PASS, FAIL or NOT TESTED, the observed result, local timestamp and relevant packet/log lines. Save screenshots alongside this file. Keep credentials/admin token out of shared evidence. Do not count unrelated rejected packets as proof of a feature failure without matching the action.\n\nAfter the test, log out both clients and stop the server gracefully. Reconnect checks must use this same config/database. Preserve this directory while investigating a failure; create a new run directory for a clean retry. Installed player data was never copied or modified.\n")
	if err = os.WriteFile(report, []byte(text.String()), privateFileMode); err != nil {
		return "", err
	}
	return report, nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
