package inventory

import (
	"strconv"
	"strings"
	"wonderland-gonline/internal/assets"
)

// Authored native presentation table: FUN_00485d44, WLRI ca19ee087b60.
var alchemyBaseNames = map[uint16]string{
	1:   "Flower",
	2:   "Grass",
	3:   "Leaf",
	4:   "Wood",
	5:   "Rice",
	6:   "Whea",
	7:   "Veggie",
	8:   "Fruit",
	9:   "Meat",
	10:  "Egg",
	11:  "Bean",
	12:  "Milk",
	13:  "Seafood",
	14:  "Medicine",
	15:  "Gold",
	16:  "Silv",
	17:  "Copp",
	18:  "Iron",
	19:  "Hematite",
	20:  "Tin",
	21:  "Lead",
	22:  "Alum",
	23:  "Sili",
	24:  "Sulf",
	25:  "Salt",
	26:  "Coal",
	27:  "Stee",
	28:  "Jade",
	29:  "Gem",
	30:  "Diamond",
	31:  "Crystal",
	32:  "Mercury",
	33:  "Silver",
	34:  "Stone",
	35:  "Magnet",
	36:  "Merc",
	37:  "Tita",
	42:  "Clay",
	43:  "Red Clay",
	44:  "Clay",
	45:  "Black Clay",
	46:  "White Clay",
	47:  "Grey Clay",
	48:  "Dry Clay",
	49:  "Fur",
	50:  "Feather",
	51:  "Carapace",
	52:  "Bone",
	53:  "Nest",
	54:  "Innards",
	55:  "Feces",
	56:  "Secrets",
	57:  "Alcohol",
	58:  "Nylon",
	59:  "Crude",
	60:  "Ref Oil",
	61:  "Fuel",
	62:  "Seed",
	63:  "WolfMeat",
	64:  "Plastic",
	65:  "Healthy Body",
	66:  "Crystali",
	67:  "Magical Item",
	68:  "Poison",
	69:  "Fur",
	71:  "Chemical",
	72:  "夥伴能力道具",
	74:  "玩家能力道具",
	101: "Drinkable",
	102: "Can't Drink",
}

func alchemyBaseLine(item assets.NativeItem) string {
	names := []string{}
	for _, base := range item.AlchemyItem().Bases {
		if base == 0 {
			continue
		}
		name := alchemyBaseNames[base]
		if name == "" {
			name = "Base " + strconv.Itoa(int(base))
		}
		names = append(names, name)
	}
	return "Alch Mat: " + strings.Join(names, " / ")
}
