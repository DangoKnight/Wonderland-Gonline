# Bahamut quest and mission reference research

Research date: 2026-11-04. Starting source: [Wonderland Online Rhodes Island portal](https://acg.gamer.com.tw/wikimenu.php?s=113640).

The portal links into the shared `9045:` WLO wiki namespace while identifying the
Rhodes portal as `ss=113640`. These are supplementary community references, with
some older/classic content. Their claims must be reconciled with native IDs and
scripts before changing gameplay. This research does not change server settings,
quest rewards or the installed databases.

## Coverage and saved evidence

Cached **72 public pages**: 26 quest/support roots and 46 directly linked context
pages. Of these, 35 have article content, 36 have no parsed article body, and one
is a placeholder (`任務技能`, filled with template text). HTTP success alone is
not evidence that a usable article exists.

The index contains **266 sections with explicit factual fields**, covering general,
companion, battle, item, EXP-potion, star, chest-key, friendship, Heart of Ocean,
life-skill, battle-skill and job-related material. This is a section count, not
266 distinct additional quests: entries can repeat between categories, and some
headings describe a region or a stage. The existing [345-entry notepad inventory](QUEST_LIST.md)
remains separate.

Tracked factual references are under [gamer-quests](../tools/data_export/reference/gamer-quests):

| File | Contents |
| --- | --- |
| `pages.csv` | 72 URLs, article/placeholder/empty status, sizes and section counts |
| `quest_sections.csv` | 266 source-linked sections with explicit location, NPC, prerequisites, required items and rewards where stated |
| `article_links.csv` | 151 distinct article-to-article link records; site navigation is excluded |
| `quest_bindings.csv` | 13 reviewed Chinese-title/native-mark bindings with expected native captions |
| `native_comparisons.csv` | Nine checked reward/cost details with exact map/event/branch and skill/item IDs |
| `companion_revive_costs.csv` | 20 companion entries with source resurrection-star claims and caveats; not imported into runtime |
| `sources.json`, `comparison_sources.json` | Cache hashes, coverage and native-catalog comparison provenance |

Full HTML and extracted section text stay in ignored `var/quest-research/gamer`.
The tracked tables preserve identifiers and factual fields, not copied walkthrough
prose. Empty fields mean the extractor found no explicit matching field; they do
not mean the quest has no requirement or reward. Narrative-only requirements must
still be read in context.

## Details checked against native scripts

| Source topic | Native identity | Checked detail |
| --- | --- | --- |
| [Soul Dodge / 魂閃](https://wiki2.gamer.com.tw/wiki.php?n=9045%3A%E9%AD%82%E9%96%83&ss=113640) | Skill 15026; event 12055:4, branch 16 | Three Stars (item 30025) consumed at confirmation |
| [Arrow Rain / 箭雨](https://wiki2.gamer.com.tw/wiki.php?n=9045%3A%E7%AE%AD%E9%9B%A8&ss=113640) | Skill 11041; event 12300:10, branch 21 | Two Stars consumed at confirmation |
| [Primary Alchemy](https://wiki2.gamer.com.tw/wiki.php?n=9045%3A%E7%85%89%E9%87%91%E8%A1%93&ss=113640) | Skill 15997; event 12053:3, branch 5 | Fairy Axe and Psychic Dagger consumed, one each |
| Junior Alchemy | Skill 15998; event 10071:6, branch 6 | Five Dragon Teeth, two Chameleon Tongues and one Ant Antennae |
| Superior Alchemy | Skill 15999; event 10071:2 | Three Magic Powder, five Clear Devil's Spar and two Bluestone at branch 7; five Stars at branch 15; Junior Alchemy grade 10 gate |
| [The Lost Dog](https://wiki2.gamer.com.tw/wiki.php?n=9045%3A%E4%B8%80%E8%88%AC%E4%BB%BB%E5%8B%99&ss=113640#迷失的小狗) | Mark 13046; event 12000:38, branch 7 | Two Coconuts, item 41066 |
| Pig Taking A Walk | Mark 12020; event 12000:12, branch 9 | One Fine Stick, item 18064 |
| Squirrel Mission | Mark 12012; event 12050:37, branch 15 | Twelve Brown Nuts, item 41141 |

The alchemy reference describes both the material hand-in and the later star
confirmation. Looking only at the final Superior Alchemy branch would incorrectly
suggest its material costs are absent. The native program handles them in an
earlier stage. These checks establish the listed details, not complete quest
acceptance.

**Collection is skill 15983**, the vacuum-gathering skill, awarded by Busy Old
Farmer (13108/13109) for five Barley (41007). **Soul Dodge is the SP-stealing skill
15026.** The earlier installed reward-bundle fixture tested Collection correctly
but called its subtest `soul_dodge`; that label and the earlier research prose
were corrected without changing gameplay.

## Additional information for the pending quest audit

### Marriage ceremony

The [marriage-system article](https://wiki2.gamer.com.tw/wiki.php?n=9045%3A%E7%B5%90%E5%A9%9A%E7%B3%BB%E7%B5%B1&ss=113640)
provides a useful checklist beyond the notepad's Fall in Love ring quest:

- Both characters level 30 or higher.
- Cupid's arrow/ring journey through Hanging Garden before registration.
- A two-person party and a fee of 60,000 gold per person.
- Obtain wedding clothes from the changing-room chests; ceremony equipment restrictions.
- A furniture-door reward with access limited to the couple.

These remain reference claims for comparison with registration, ownership,
equipment and ceremony scripts. The article does not establish a complete
reservation/invitation/deadline protocol. Existing chat marriage commands are not
evidence that these native flows are implemented.

### Goddess and job-related chains

The [goddess-skill page](https://wiki2.gamer.com.tw/wiki.php?n=9045%3A%E5%A5%B3%E7%A5%9E%E6%8A%80%E8%83%BD&ss=113640)
links the Goddess Revelation, opening-heart, Erina and blessing stages. It lists
five Stars for the first goddess skill and seven for the true goddess skill,
with seven elemental glass items awarded for the latter.

The [true-goddess page](https://wiki2.gamer.com.tw/wiki.php?n=9045%3A%E7%9C%9F%E5%A5%B3%E7%A5%9E%E6%8A%80&ss=113640)
adds a level-50 qualification, the Sky Jade step, a distinction between the battle
and the following scripted strikes, and one matching elemental glass consumed
per cast. Its author explicitly leaves the below-50 retry behavior uncertain;
do not turn that uncertainty into a gameplay rule.

The [job-advancement guide](https://wiki2.gamer.com.tw/wiki.php?n=9045%3A%E8%BD%89%E8%81%B7%E4%BB%BB%E5%8B%99&ss=113640)
describes level 100+, three sacred artifacts, successive boss battles and
reinforcements. The [job-skill page](https://wiki2.gamer.com.tw/wiki.php?n=9045%3A%E8%81%B7%E6%A5%AD%E6%8A%80%E8%83%BD&ss=113640)
states a separate five-Star skill fee. These supply stage/order and cost checks
for the existing native job scripts, rather than a second replacement quest chain.

### Companion deaths and resurrection

The [companion page](https://wiki2.gamer.com.tw/wiki.php?n=9045%3A%E5%AF%B5%E7%89%A9NPC&ss=113640)
lists resurrection costs alongside companion story information. For example,
it gives Roca/Niss five Stars, Xaolan four, Clive six, Sasha eight and Fred ten.
Some entries say the companion has not actually died. Those exceptions and the
source's original labels are retained in `companion_revive_costs.csv`; an absent
numeric cost is not converted to a free resurrection. Native IDs and the
resurrection transaction still need comparison.

## Reproduction and next steps

From the repository root:

```powershell
python tools/data_export/fetch_gamer_quests.py --follow-context
python tools/data_export/fetch_gamer_quests.py --follow-context --offline
python -m unittest discover -s tools/data_export -p test_fetch_gamer_quests.py
```

The fetcher paces public requests, reuses cached HTML and follows only one hop of
context links from the selected roots. It does not crawl the whole Bahamut site.
The four parser tests cover navigation exclusion, namespace checks, heading
anchors and factual-field extraction. The renamed installed Collection/Xaolan
reward-bundle fixture also passes.

Next work is to bind the remaining Chinese sections to native IDs, compare their
requirements and rewards, and audit the marriage, goddess and job stages using
these source checklists. Keep source conflicts and uncertain statements explicit.
No downloaded claim is automatically promoted into SQL gameplay definitions.
