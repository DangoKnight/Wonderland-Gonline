package store

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"net/netip"
	"time"
	"wonderland-gonline/internal/game"
)

type IPBan struct {
	IP     string `json:"ip" gorm:"primaryKey"`
	Reason string `json:"reason"`
	At     string `json:"at"`
}

func (IPBan) TableName() string { return "banned_ips" }
func (s *Store) IPBans(ctx context.Context) ([]IPBan, error) {
	rows := []IPBan{}
	err := s.orm.WithContext(ctx).Order("ip").Find(&rows).Error
	return rows, err
}
func (s *Store) SetIPBan(ctx context.Context, ip, reason string, banned bool) error {
	address, err := netip.ParseAddr(ip)
	if err != nil {
		return errors.New("invalid IP address")
	}
	ip = address.Unmap().String()
	if len(reason) > maxSettingBytes {
		return errors.New("reason too long")
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Where(map[string]any{"ip": ip}).Delete(&IPBan{}).Error; err != nil {
			return err
		}
		if banned {
			if err := tx.Create(&IPBan{IP: ip, Reason: reason, At: time.Now().UTC().Format(time.RFC3339)}).Error; err != nil {
				return err
			}
		}
		return adminAudit(tx, "IP ban change", ip)
	})
}

const maxGuildNameBytes = 64

type AdminGuild struct {
	ID       uint32   `json:"id" gorm:"primaryKey"`
	Name     string   `json:"name"`
	Notice   string   `json:"notice"`
	LeaderID uint32   `json:"leader_id"`
	Icon     uint32   `json:"icon"`
	Members  []uint32 `json:"members" gorm:"-"`
}

func (AdminGuild) TableName() string { return "guilds" }

type guildMember struct {
	GuildID     uint32 `gorm:"primaryKey;autoIncrement:false"`
	CharacterID uint32 `gorm:"primaryKey;autoIncrement:false"`
	Rank        byte
}

func (guildMember) TableName() string { return "guild_members" }
func (s *Store) AdminGuilds(ctx context.Context) ([]AdminGuild, error) {
	guilds := []AdminGuild{}
	err := s.orm.WithContext(ctx).Order("id").Find(&guilds).Error
	if err != nil {
		return nil, err
	}
	for i := range guilds {
		var rows []guildMember
		if err = s.orm.WithContext(ctx).Where(map[string]any{"guild_id": guilds[i].ID}).Order("character_id").Find(&rows).Error; err != nil {
			return nil, err
		}
		guilds[i].Members = []uint32{}
		for _, r := range rows {
			guilds[i].Members = append(guilds[i].Members, r.CharacterID)
		}
	}
	return guilds, nil
}
func (s *Store) SaveAdminGuild(ctx context.Context, guild AdminGuild) error {
	if guild.Name == "" || len(guild.Name) > maxGuildNameBytes || len(guild.Notice) > GuildNoticeMaxBytes || len(guild.Members) > GuildMemberLimit || guild.ID > game.MaxNativeGuildID {
		return errors.New("invalid guild name or announcement")
	}
	leader := false
	seen := map[uint32]bool{}
	for _, id := range guild.Members {
		if id == 0 || seen[id] {
			return errors.New("invalid or duplicate guild member")
		}
		seen[id] = true
		leader = leader || id == guild.LeaderID
	}
	if !leader {
		return errors.New("guild leader must be a member")
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		for _, id := range guild.Members {
			var c characterRow
			if err := tx.Select("id").Where(map[string]any{"id": id}).Take(&c).Error; err != nil {
				return err
			}
			var count int64
			if err := tx.Model(&guildMember{}).Where("character_id = ? AND guild_id <> ?", id, guild.ID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return errors.New("character already belongs to another guild")
			}
		}
		var oldMembers []guildMember
		if err := tx.Where("guild_id = ?", guild.ID).Find(&oldMembers).Error; err != nil {
			return err
		}
		ranks := map[uint32]byte{}
		for _, member := range oldMembers {
			ranks[member.CharacterID] = member.Rank
		}
		if guild.Icon == 0 {
			var old AdminGuild
			if guild.ID != 0 {
				if err := tx.First(&old, guild.ID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
			}
			guild.Icon = old.Icon
			if guild.Icon == 0 {
				guild.Icon = DefaultGuildIcon
			}
		}
		if err := tx.Save(&guild).Error; err != nil {
			return err
		}
		if guild.ID > game.MaxNativeGuildID {
			return errors.New("guild ID limit reached")
		}
		if err := tx.Where(map[string]any{"guild_id": guild.ID}).Delete(&guildMember{}).Error; err != nil {
			return err
		}
		for _, id := range guild.Members {
			if err := tx.Create(&guildMember{GuildID: guild.ID, CharacterID: id, Rank: ranks[id]}).Error; err != nil {
				return err
			}
		}
		return adminAudit(tx, "guild edit", guild.ID)
	})
}
func (s *Store) DeleteAdminGuild(ctx context.Context, id uint32) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Where(map[string]any{"guild_id": id}).Delete(&guildMember{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&AdminGuild{}, id).Error; err != nil {
			return err
		}
		return adminAudit(tx, "guild disband", id)
	})
}

type AdminMarriage struct {
	ID         uint32 `json:"id" gorm:"primaryKey"`
	Character1 uint32 `json:"character1"`
	Character2 uint32 `json:"character2"`
	At         string `json:"at"`
}

func (AdminMarriage) TableName() string { return "marriages" }
func (s *Store) AdminMarriages(ctx context.Context) ([]AdminMarriage, error) {
	rows := []AdminMarriage{}
	err := s.orm.WithContext(ctx).Order("id").Find(&rows).Error
	return rows, err
}
func (s *Store) DeleteAdminMarriage(ctx context.Context, id uint32) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Delete(&AdminMarriage{}, id).Error; err != nil {
			return err
		}
		return adminAudit(tx, "marriage annulment", id)
	})
}

// Deletion cascades remove memberships. Elect the remaining lowest-ID member,
// or disband an empty guild, within the same transaction as the deletion.
func repairGuildLeaders(tx *gorm.DB) error {
	var guilds []AdminGuild
	if err := tx.Find(&guilds).Error; err != nil {
		return err
	}
	for _, guild := range guilds {
		var members []guildMember
		if err := tx.Where(map[string]any{"guild_id": guild.ID}).Order("character_id").Find(&members).Error; err != nil {
			return err
		}
		valid := false
		for _, m := range members {
			valid = valid || m.CharacterID == guild.LeaderID
		}
		if valid {
			continue
		}
		if len(members) == 0 {
			if err := tx.Delete(&guild).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&guild).Update("leader_id", members[0].CharacterID).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
