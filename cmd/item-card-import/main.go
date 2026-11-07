package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/assetsql"
	"wonderland-gonline/internal/game"
)

func main() {
	assetsPath := flag.String("assets", "", "structured assets database (required)")
	english := flag.String("english", "data/runtime/locales/en.json", "compiled English catalog")
	chinese := flag.String("chinese", "data/runtime/locales/zh-TW.json", "compiled Chinese catalog")
	overrides := flag.String("overrides", "internal/assetsql/item_cards_overrides.json", "authored reward and gender rules")
	output := flag.String("output", "internal/assetsql/item_cards_defaults.json", "output seed JSON")
	sources := flag.String("sources", "internal/assetsql/item_cards_sources.json", "output source hashes")
	flag.Parse()
	if *assetsPath == "" {
		panic("-assets is required")
	}
	db, e := assetdb.OpenReadOnly(*assetsPath)
	if e != nil {
		panic(e)
	}
	defer assetdb.Close(db)
	var items []assetsql.NativeItemsRow
	if e := db.Order("native_items_key").Find(&items).Error; e != nil {
		panic(e)
	}
	langs := []map[string]string{}
	hashes := map[string]string{}
	nativeRaw, _ := json.Marshal(items)
	hashes["native_item_definitions"] = fmt.Sprintf("%x", sha256.Sum256(nativeRaw))
	for _, lang := range []string{*english, *chinese} {
		b, e := os.ReadFile(lang)
		if e != nil {
			panic(e)
		}
		hashes[lang] = fmt.Sprintf("%x", sha256.Sum256(b))
		var c struct {
			Strings map[string]string `json:"strings"`
		}
		if e := json.Unmarshal(b, &c); e != nil {
			panic(e)
		}
		langs = append(langs, c.Strings)
	}
	counts := []map[string]int{}
	for _, lang := range langs {
		count := map[string]int{}
		for _, it := range items {
			count[strings.ToLower(lang[fmt.Sprintf("item.%d.name", it.NativeItemsKey)])]++
		}
		counts = append(counts, count)
	}
	type row struct {
		ID uint16 `json:"card_id"`
		assets.ItemCard
		Evidence string `json:"evidence"`
	}
	manualRaw, e := os.ReadFile(*overrides)
	if e != nil {
		panic(e)
	}
	hashes[*overrides] = fmt.Sprintf("%x", sha256.Sum256(manualRaw))
	var manual []row
	if e := json.Unmarshal(manualRaw, &manual); e != nil {
		panic(e)
	}
	byID := map[uint16]row{}
	for _, r := range manual {
		byID[r.ID] = r
	}
	out := []row{}
	split := regexp.MustCompile(`[,，、。]| or | and |或|以及|等`)
	paren := regexp.MustCompile(`\([^)]*\)|（[^）]*）`)
	for _, it := range items {
		if it.ValueDefinitionType != game.ItemTypeRedemptionCard {
			continue
		}
		id := it.NativeItemsKey
		matches := map[uint16]bool{}
		for l, lang := range langs {
			localMatches := map[uint16]bool{}
			desc := strings.ToLower(lang[fmt.Sprintf("item.%d.description", id)])
			start := -1
			for _, word := range []string{"redeem", "自行選擇", "兌換"} {
				if n := strings.Index(desc, word); n >= 0 {
					start = n + len(word)
					break
				}
			}
			if start < 0 {
				continue
			}
			desc = strings.TrimPrefix(desc[start:], "兌換")
			tokens := split.Split(paren.ReplaceAllString(desc, ""), -1)
			for _, target := range items {
				if target.ValueDefinitionType == 40 {
					continue
				}
				name := strings.ToLower(lang[fmt.Sprintf("item.%d.name", target.NativeItemsKey)])
				if name == "" {
					continue
				}
				if counts[l][name] != 1 {
					continue
				}
				for _, token := range tokens {
					token = strings.TrimSpace(paren.ReplaceAllString(token, ""))
					if l == 0 {
						token = strings.Trim(token, ". ")
					}
					if token == name {
						localMatches[target.NativeItemsKey] = true
					}
				}
			}
			if len(localMatches) > 0 {
				matches = localMatches
			}
		}
		r := row{ID: id, Evidence: "Exact named reward phrases in compiled English/Chinese item descriptions; unspecified premium props are not invented"}
		ids := []int{}
		for id := range matches {
			ids = append(ids, int(id))
		}
		sort.Ints(ids)
		for _, id := range ids {
			r.Rewards = append(r.Rewards, assets.CardReward{ItemID: uint16(id), Quantity: 1})
		}
		if known, ok := byID[id]; ok {
			r = known
		}
		if len(r.Rewards) > 0 {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	b, _ := json.MarshalIndent(out, "", "  ")
	definitions := make(map[uint16]game.ItemDefinition, len(items))
	for _, item := range items {
		definitions[item.NativeItemsKey] = game.ItemDefinition{ID: item.NativeItemsKey}
	}
	if _, err := assets.ParseItemCards(b, definitions); err != nil {
		panic(err)
	}
	if e := os.WriteFile(*output, append(b, '\n'), 0644); e != nil {
		panic(e)
	}
	raw, _ := json.MarshalIndent(hashes, "", "  ")
	if e := os.WriteFile(*sources, append(raw, '\n'), 0644); e != nil {
		panic(e)
	}
	fmt.Println("cards", len(out))
}
