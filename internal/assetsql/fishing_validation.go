package assetsql

import (
	"fmt"
	"wonderland-gonline/internal/assets"
)

func validateFishingCatalog(c *assets.Catalog) error {
	if err := assets.ValidateFishing(c.Fishing, c.Items); err != nil {
		return err
	}
	if !c.Fishing.Enabled {
		return nil
	}
	for _, id := range c.Fishing.Skills {
		if _, ok := c.Skills[id]; !ok {
			return fmt.Errorf("missing fishing skill %d", id)
		}
	}
	for _, id := range c.Fishing.Maps {
		if _, ok := c.Maps[id]; !ok {
			return fmt.Errorf("missing fishing map %d", id)
		}
	}
	return nil
}
