package inventory

import "wonderland-gonline/internal/localization"

import (
	"strconv"
	"strings"
	"wonderland-gonline/internal/assets"
)

// Authored native presentation table: FUN_00485d44, WLRI ca19ee087b60.
var alchemyBaseNames = map[uint16]string{
	1:   localization.Text("ui.inventory.flower_77dc2a65"),
	2:   localization.Text("ui.inventory.grass_84573c57"),
	3:   localization.Text("ui.inventory.leaf_b83382a9"),
	4:   localization.Text("ui.inventory.wood_73114028"),
	5:   localization.Text("ui.inventory.rice_1b9bc7b7"),
	6:   localization.Text("ui.inventory.whea_b86b9a8e"),
	7:   localization.Text("ui.inventory.veggie_385e4748"),
	8:   localization.Text("ui.inventory.fruit_779089b3"),
	9:   localization.Text("ui.inventory.meat_ba4c9e95"),
	10:  localization.Text("ui.inventory.egg_37c50c93"),
	11:  localization.Text("ui.inventory.bean_1d086001"),
	12:  localization.Text("ui.inventory.milk_8189b840"),
	13:  localization.Text("ui.inventory.seafood_72028719"),
	14:  localization.Text("ui.inventory.medicine_c4d3d42e"),
	15:  localization.Text("ui.inventory.gold_6249df43"),
	16:  localization.Text("ui.inventory.silv_68a81a4c"),
	17:  localization.Text("ui.inventory.copp_77303dab"),
	18:  localization.Text("ui.inventory.iron_bd223c81"),
	19:  localization.Text("ui.inventory.hematite_94a19425"),
	20:  localization.Text("ui.inventory.tin_6377a611"),
	21:  localization.Text("ui.inventory.lead_64597828"),
	22:  localization.Text("ui.inventory.alum_e1075959"),
	23:  localization.Text("ui.inventory.sili_dbb437e5"),
	24:  localization.Text("ui.inventory.sulf_f50dacbe"),
	25:  localization.Text("ui.inventory.salt_150fe551"),
	26:  localization.Text("ui.inventory.coal_4cc1d6e9"),
	27:  localization.Text("ui.inventory.stee_14ee47ca"),
	28:  localization.Text("ui.inventory.jade_9fbbe982"),
	29:  localization.Text("ui.inventory.gem_5108a0f0"),
	30:  localization.Text("ui.inventory.diamond_b639c243"),
	31:  localization.Text("ui.inventory.crystal_b8c18c6a"),
	32:  localization.Text("ui.inventory.mercury_6a7367ef"),
	33:  localization.Text("ui.inventory.silver_a0b43df0"),
	34:  localization.Text("ui.inventory.stone_6ffa6b8b"),
	35:  localization.Text("ui.inventory.magnet_12d5da11"),
	36:  localization.Text("ui.inventory.merc_66c3aece"),
	37:  localization.Text("ui.inventory.tita_32c5b61d"),
	42:  localization.Text("ui.inventory.clay_05947b7b"),
	43:  localization.Text("ui.inventory.red_clay_27676c29"),
	44:  localization.Text("ui.inventory.clay_05947b7b"),
	45:  localization.Text("ui.inventory.black_clay_fc8e40e4"),
	46:  localization.Text("ui.inventory.white_clay_bba1c19f"),
	47:  localization.Text("ui.inventory.grey_clay_b4794bae"),
	48:  localization.Text("ui.inventory.dry_clay_39aad265"),
	49:  localization.Text("ui.inventory.fur_db0e2c9d"),
	50:  localization.Text("ui.inventory.feather_b6898405"),
	51:  localization.Text("ui.inventory.carapace_6af2365c"),
	52:  localization.Text("ui.inventory.bone_f950109f"),
	53:  localization.Text("ui.inventory.nest_e5716be7"),
	54:  localization.Text("ui.inventory.innards_27da5c99"),
	55:  localization.Text("ui.inventory.feces_7e614d78"),
	56:  localization.Text("ui.inventory.secrets_d8707d41"),
	57:  localization.Text("ui.inventory.alcohol_670a802e"),
	58:  localization.Text("ui.inventory.nylon_76e26ff9"),
	59:  localization.Text("ui.inventory.crude_2df76452"),
	60:  localization.Text("ui.inventory.ref_oil_c36cdbff"),
	61:  localization.Text("ui.inventory.fuel_a80f942f"),
	62:  localization.Text("ui.inventory.seed_367ef06b"),
	63:  "WolfMeat",
	64:  localization.Text("ui.inventory.plastic_b8370014"),
	65:  localization.Text("ui.inventory.healthy_body_77d45118"),
	66:  localization.Text("ui.inventory.crystali_46a4727e"),
	67:  localization.Text("ui.inventory.magical_item_d61be33a"),
	68:  localization.Text("ui.inventory.poison_d5a78b99"),
	69:  localization.Text("ui.inventory.fur_db0e2c9d"),
	71:  localization.Text("ui.inventory.chemical_3e5ffd9b"),
	72:  "夥伴能力道具",
	74:  "玩家能力道具",
	101: localization.Text("ui.inventory.drinkable_12be111a"),
	102: localization.Text("ui.inventory.can_t_drink_49a5cfb1"),
}

func alchemyBaseLine(item assets.NativeItem) string {
	names := []string{}
	for _, base := range item.AlchemyItem().Bases {
		if base == 0 {
			continue
		}
		name := alchemyBaseNames[base]
		if name == "" {
			name = localization.Text("ui.inventory.base_4f1271d8") + strconv.Itoa(int(base))
		}
		names = append(names, name)
	}
	return localization.Text("ui.inventory.alch_mat_44848ff4") + strings.Join(names, " / ")
}

func init() {
	localization.Register(func() {
		alchemyBaseNames = map[uint16]string{
			1:   localization.Text("ui.inventory.flower_77dc2a65"),
			2:   localization.Text("ui.inventory.grass_84573c57"),
			3:   localization.Text("ui.inventory.leaf_b83382a9"),
			4:   localization.Text("ui.inventory.wood_73114028"),
			5:   localization.Text("ui.inventory.rice_1b9bc7b7"),
			6:   localization.Text("ui.inventory.whea_b86b9a8e"),
			7:   localization.Text("ui.inventory.veggie_385e4748"),
			8:   localization.Text("ui.inventory.fruit_779089b3"),
			9:   localization.Text("ui.inventory.meat_ba4c9e95"),
			10:  localization.Text("ui.inventory.egg_37c50c93"),
			11:  localization.Text("ui.inventory.bean_1d086001"),
			12:  localization.Text("ui.inventory.milk_8189b840"),
			13:  localization.Text("ui.inventory.seafood_72028719"),
			14:  localization.Text("ui.inventory.medicine_c4d3d42e"),
			15:  localization.Text("ui.inventory.gold_6249df43"),
			16:  localization.Text("ui.inventory.silv_68a81a4c"),
			17:  localization.Text("ui.inventory.copp_77303dab"),
			18:  localization.Text("ui.inventory.iron_bd223c81"),
			19:  localization.Text("ui.inventory.hematite_94a19425"),
			20:  localization.Text("ui.inventory.tin_6377a611"),
			21:  localization.Text("ui.inventory.lead_64597828"),
			22:  localization.Text("ui.inventory.alum_e1075959"),
			23:  localization.Text("ui.inventory.sili_dbb437e5"),
			24:  localization.Text("ui.inventory.sulf_f50dacbe"),
			25:  localization.Text("ui.inventory.salt_150fe551"),
			26:  localization.Text("ui.inventory.coal_4cc1d6e9"),
			27:  localization.Text("ui.inventory.stee_14ee47ca"),
			28:  localization.Text("ui.inventory.jade_9fbbe982"),
			29:  localization.Text("ui.inventory.gem_5108a0f0"),
			30:  localization.Text("ui.inventory.diamond_b639c243"),
			31:  localization.Text("ui.inventory.crystal_b8c18c6a"),
			32:  localization.Text("ui.inventory.mercury_6a7367ef"),
			33:  localization.Text("ui.inventory.silver_a0b43df0"),
			34:  localization.Text("ui.inventory.stone_6ffa6b8b"),
			35:  localization.Text("ui.inventory.magnet_12d5da11"),
			36:  localization.Text("ui.inventory.merc_66c3aece"),
			37:  localization.Text("ui.inventory.tita_32c5b61d"),
			42:  localization.Text("ui.inventory.clay_05947b7b"),
			43:  localization.Text("ui.inventory.red_clay_27676c29"),
			44:  localization.Text("ui.inventory.clay_05947b7b"),
			45:  localization.Text("ui.inventory.black_clay_fc8e40e4"),
			46:  localization.Text("ui.inventory.white_clay_bba1c19f"),
			47:  localization.Text("ui.inventory.grey_clay_b4794bae"),
			48:  localization.Text("ui.inventory.dry_clay_39aad265"),
			49:  localization.Text("ui.inventory.fur_db0e2c9d"),
			50:  localization.Text("ui.inventory.feather_b6898405"),
			51:  localization.Text("ui.inventory.carapace_6af2365c"),
			52:  localization.Text("ui.inventory.bone_f950109f"),
			53:  localization.Text("ui.inventory.nest_e5716be7"),
			54:  localization.Text("ui.inventory.innards_27da5c99"),
			55:  localization.Text("ui.inventory.feces_7e614d78"),
			56:  localization.Text("ui.inventory.secrets_d8707d41"),
			57:  localization.Text("ui.inventory.alcohol_670a802e"),
			58:  localization.Text("ui.inventory.nylon_76e26ff9"),
			59:  localization.Text("ui.inventory.crude_2df76452"),
			60:  localization.Text("ui.inventory.ref_oil_c36cdbff"),
			61:  localization.Text("ui.inventory.fuel_a80f942f"),
			62:  localization.Text("ui.inventory.seed_367ef06b"),
			63:  "WolfMeat",
			64:  localization.Text("ui.inventory.plastic_b8370014"),
			65:  localization.Text("ui.inventory.healthy_body_77d45118"),
			66:  localization.Text("ui.inventory.crystali_46a4727e"),
			67:  localization.Text("ui.inventory.magical_item_d61be33a"),
			68:  localization.Text("ui.inventory.poison_d5a78b99"),
			69:  localization.Text("ui.inventory.fur_db0e2c9d"),
			71:  localization.Text("ui.inventory.chemical_3e5ffd9b"),
			72:  "夥伴能力道具",
			74:  "玩家能力道具",
			101: localization.Text("ui.inventory.drinkable_12be111a"),
			102: localization.Text("ui.inventory.can_t_drink_49a5cfb1"),
		}
	})
}
