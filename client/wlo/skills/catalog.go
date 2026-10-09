// Package skills ports TRe_SkillForm's server-confirmed skill lists and tree.
package skills

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/clientfs"
)

const (
	BasicAttack                   = 10001
	Defense                       = 60021
	LifeLayer                     = 13
	nativeIconField               = 100
	nativeTargetField             = 99 // FUN_00481640: TSkillDat +0x67, after four-byte prefix.
	nativeMaximumGradeField       = 105
	nativeAnimationFixedTailBytes = 198 // FUN_00377060: 72+24+6+6+60+24+6.
	nativeWeaponFlagsFromEnd      = 95  // Record +0x131..135, within that fixed tail.
	WeaponKinds                   = 5
)

type Definition struct {
	assets.Skill
	Description  string
	Icon         uint16
	MaximumGrade byte
	Weapons      [WeaponKinds]bool
	NativeTarget byte
	TargetKnown  bool
}

// Target policies returned by FUN_00481640 and checked in FUN_0038972c.
const (
	TargetEnemy     = 0
	TargetAlly      = 1
	TargetEither    = 2
	TargetSelf      = 3
	TargetOtherAlly = 5
)

func (d Definition) AllowsTarget(sameSide, self bool) bool {
	switch d.NativeTarget {
	case TargetEnemy:
		return !sameSide
	case TargetAlly:
		return sameSide
	case TargetEither:
		return true
	case TargetSelf:
		return self
	case TargetOtherAlly:
		return sameSide && !self
	}
	return false
}

func (d Definition) IconName() string { return fmt.Sprintf("Icon_sk%ds", d.Icon) }

type Catalog struct {
	Definitions map[uint16]Definition
	Orders      map[uint16]uint16
}

// Load uses complete exported client records. Skill.dat's effect_layer is the
// native UI category; its type field is a separate domain, often the element.
func Load(a login.Assets) (*Catalog, error) {
	raw, err := clientfs.ReadFile(a.DataPath("skill_data.json"))
	if err != nil {
		return nil, err
	}
	c, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	raw, err = clientfs.ReadFile(a.DataPath("animation_data.json"))
	if err != nil {
		return nil, err
	}
	if err = c.ApplyWeapons(raw); err != nil {
		return nil, err
	}
	return c, nil
}
func Parse(raw []byte) (*Catalog, error) {
	var doc struct {
		Records []struct {
			Name struct {
				Text string `json:"text"`
			} `json:"name"`
			Description struct {
				Text string `json:"text"`
			} `json:"description"`
			Fields     assets.Skill `json:"fields"`
			DecodedHex string       `json:"decoded_hex"`
		} `json:"records"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	c := &Catalog{Definitions: map[uint16]Definition{}, Orders: map[uint16]uint16{}}
	for _, r := range doc.Records {
		b, err := hex.DecodeString(r.DecodedHex)
		if err != nil || len(b) <= nativeMaximumGradeField {
			return nil, fmt.Errorf("invalid skill record %d", r.Fields.ID)
		}
		if r.Fields.ID == 0 {
			continue
		}
		if _, exists := c.Definitions[r.Fields.ID]; exists {
			return nil, fmt.Errorf("duplicate skill %d", r.Fields.ID)
		}
		d := Definition{Skill: r.Fields, Description: r.Description.Text, Icon: uint16(b[nativeIconField]) | uint16(b[nativeIconField+1])<<8, MaximumGrade: b[nativeMaximumGradeField]}
		d.NativeTarget, d.TargetKnown = b[nativeTargetField], true
		d.Name = r.Name.Text
		c.Definitions[d.ID] = d
		if d.TableOrder != 0 {
			if _, exists := c.Orders[d.TableOrder]; exists {
				return nil, fmt.Errorf("duplicate skill order %d", d.TableOrder)
			}
			c.Orders[d.TableOrder] = d.ID
		}
	}
	return c, nil
}

// ApplyWeapons reads the five native weapon flags from the fixed tail of each
// MBTM record (FUN_00377060), independently of preceding variable animations.
func (c *Catalog) ApplyWeapons(raw []byte) error {
	var doc struct {
		Records []struct {
			ID         uint16 `json:"id"`
			DecodedHex string `json:"decoded_hex"`
		} `json:"records"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	updates := make(map[uint16][WeaponKinds]bool)
	for _, r := range doc.Records {
		b, err := hex.DecodeString(r.DecodedHex)
		if err != nil || len(b) < nativeAnimationFixedTailBytes {
			return fmt.Errorf("invalid skill animation %d", r.ID)
		}
		var flags [WeaponKinds]bool
		for i := range flags {
			v := b[len(b)-nativeWeaponFlagsFromEnd+i]
			if v > 1 {
				return fmt.Errorf("invalid weapon flag for %d", r.ID)
			}
			flags[i] = v == 1
		}
		updates[r.ID] = flags
	}
	for id, flags := range updates {
		if d, ok := c.Definitions[id]; ok {
			d.Weapons = flags
			c.Definitions[id] = d
		}
	}
	return nil
}
