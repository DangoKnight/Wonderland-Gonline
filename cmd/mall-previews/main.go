// mall-previews exports client-only bag contents; it never modifies gacha odds.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"sort"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/clientassets"
)

func main() {
	source := flag.String("source", "", "client installation containing data/Lbd.dat")
	legacy := flag.String("legacy", "data/server/gacha_packs.json", "legacy authored pack import")
	output := flag.String("output", "internal/clientassets/mall_contents.json", "preview asset")
	overrides := flag.String("overrides", "internal/clientassets/mall_contents_overrides.json", "authored screenshot reward lists")
	fittingSource := flag.String("fitting-source", "internal/assetsql/boxed_items_defaults.json", "authored equipment-box definitions")
	fittingOutput := flag.String("fitting-output", "internal/clientassets/mall_fitting.json", "client fitting metadata")
	voucherSource := flag.String("voucher-source", "data/server/pet_vouchers.json", "authored companion voucher import")
	voucherOutput := flag.String("voucher-output", "internal/clientassets/mall_vouchers.json", "client companion preview metadata")
	forgeSource := flag.String("forge-source", "data/server/mall_forging.json", "authored forge families")
	forgeOutput := flag.String("forge-output", "internal/clientassets/mall_forging.json", "client forge preview metadata")
	flag.Parse()
	if *source == "" {
		log.Fatal("-source is required")
	}
	raw, err := os.ReadFile(filepath.Join(*source, "data", "Lbd.dat"))
	must(err)
	packs, err := clientassets.ParseMallContents(raw)
	must(err)
	merged := map[uint16]clientassets.MallContents{}
	raw, err = os.ReadFile(*legacy)
	must(err)
	var wrapped struct {
		Text string `json:"text"`
	}
	must(json.Unmarshal(raw, &wrapped))
	var previous []assets.GachaPack
	must(json.Unmarshal([]byte(wrapped.Text), &previous))
	for _, p := range previous {
		entry := clientassets.MallContents{ItemID: p.ID}
		for _, r := range p.Rewards {
			if r.Quantity < 1 || r.Quantity > 255 {
				log.Fatal("invalid legacy reward quantity")
			}
			entry.Rewards = append(entry.Rewards, clientassets.MallReward{ItemID: r.ID, Quantity: byte(r.Quantity)})
		}
		merged[p.ID] = entry
	}
	// Native current-client definitions take priority over old executable tables.
	// Lbd is a bounded cache populated by viewing bags. Preserve earlier exported
	// observations if the installation has since evicted a group from that cache.
	if existing, readErr := os.ReadFile(*output); readErr == nil {
		var observed []clientassets.MallContents
		must(json.Unmarshal(existing, &observed))
		for _, p := range observed {
			merged[p.ItemID] = p
		}
	} else if !os.IsNotExist(readErr) {
		must(readErr)
	}
	for _, p := range packs {
		merged[p.ItemID] = p
	}
	raw, err = os.ReadFile(*overrides)
	must(err)
	var authored []clientassets.MallContents
	must(json.Unmarshal(raw, &authored))
	for _, p := range authored {
		if len(p.Rewards) == 0 {
			p.Rewards = merged[p.ItemID].Rewards
		}
		merged[p.ItemID] = p
	}
	packs = nil
	for _, p := range merged {
		packs = append(packs, p)
	}
	sort.Slice(packs, func(i, j int) bool { return packs[i].ItemID < packs[j].ItemID })
	raw, err = json.MarshalIndent(packs, "", "  ")
	must(err)
	must(os.WriteFile(*output, append(raw, '\n'), 0644))
	log.Printf("exported %d bag previews", len(packs))
	raw, err = os.ReadFile(*fittingSource)
	must(err)
	var boxes []struct {
		BoxID  uint16 `json:"box_id"`
		ItemID uint16 `json:"item_id"`
	}
	must(json.Unmarshal(raw, &boxes))
	raw, err = json.MarshalIndent(boxes, "", "  ")
	must(err)
	must(os.WriteFile(*fittingOutput, append(raw, '\n'), 0644))
	raw, err = os.ReadFile(*voucherSource)
	must(err)
	must(json.Unmarshal(raw, &wrapped))
	vouchers, err := assets.ParsePetVouchers([]byte(wrapped.Text))
	must(err)
	raw, err = json.MarshalIndent(vouchers, "", "  ")
	must(err)
	must(os.WriteFile(*voucherOutput, append(raw, '\n'), 0644))
	raw, err = os.ReadFile(*forgeSource)
	must(err)
	must(json.Unmarshal(raw, &wrapped))
	var config struct {
		Families [][]uint16 `json:"families"`
	}
	must(json.Unmarshal([]byte(wrapped.Text), &config))
	upgrades := map[uint16]assets.ForgeUpgrade{}
	for _, family := range config.Families {
		for i, id := range family {
			u := assets.ForgeUpgrade{Scrolls: byte(i + 1)}
			if i+1 < len(family) {
				u.Next = family[i+1]
			}
			upgrades[id] = u
		}
	}
	raw, err = json.MarshalIndent(upgrades, "", "  ")
	must(err)
	must(os.WriteFile(*forgeOutput, append(raw, '\n'), 0644))
}
func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
