package store

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"strings"
	"wonderland-gonline/internal/game"
)

const (
	GuildRankMember     byte   = 0
	GuildRankViceLeader byte   = 1
	GuildRankLeader     byte   = 2
	DefaultGuildIcon    uint32 = 3402
	GuildMemberLimit           = 255
	GuildNameMaxBytes          = 20
	GuildNoticeMaxBytes        = 255
)

var ErrGuildPermission = errors.New("guild permission denied")

type GuildMemberInfo struct {
	Character game.Character
	Rank      byte
}
type GuildInfo struct {
	AdminGuild
	Roster []GuildMemberInfo
}

func guildForCharacter(tx *gorm.DB, id uint32) (AdminGuild, error) {
	var member guildMember
	if err := tx.Where("character_id = ?", id).Take(&member).Error; err != nil {
		return AdminGuild{}, err
	}
	var guild AdminGuild
	err := tx.First(&guild, member.GuildID).Error
	return guild, err
}
func (s *Store) GuildForCharacter(ctx context.Context, id uint32) (*GuildInfo, error) {
	var result *GuildInfo
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		guild, err := guildForCharacter(tx, id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		result = &GuildInfo{AdminGuild: guild}
		var members []guildMember
		if err = tx.Where("guild_id = ?", guild.ID).Order("character_id").Find(&members).Error; err != nil {
			return err
		}
		ids := make([]uint32, 0, len(members))
		for _, member := range members {
			ids = append(ids, member.CharacterID)
		}
		var identities []characterRow
		var states []characterStateRow
		if len(ids) > 0 {
			if err = tx.Select("id, name").Where("id IN ?", ids).Find(&identities).Error; err != nil {
				return err
			}
			if err = tx.Select("character_id, level, element").Where("character_id IN ?", ids).Find(&states).Error; err != nil {
				return err
			}
		}
		chars := map[uint32]game.Character{}
		for _, row := range identities {
			chars[row.ID] = game.Character{ID: row.ID, Name: row.Name}
		}
		for _, row := range states {
			char := chars[row.CharacterID]
			char.Level, char.Element = row.Level, row.Element
			chars[row.CharacterID] = char
		}
		for _, member := range members {
			character, ok := chars[member.CharacterID]
			if !ok || character.Level == 0 {
				return gorm.ErrRecordNotFound
			}
			rank := member.Rank
			if rank == GuildRankLeader {
				rank = GuildRankMember
			}
			if member.CharacterID == guild.LeaderID {
				rank = GuildRankLeader
			}
			result.Roster = append(result.Roster, GuildMemberInfo{Character: character, Rank: rank})
			result.Members = append(result.Members, member.CharacterID)
		}

		return nil
	})
	return result, err
}
func (s *Store) CreateGuild(ctx context.Context, leader uint32, name string) error {
	name = strings.TrimSpace(name)
	if !validSocialText(name, GuildNameMaxBytes) {
		return errors.New("guild name must contain 1–20 bytes")
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&guildMember{}).Where("character_id = ?", leader).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("already in a guild")
		}
		if err := tx.Model(&AdminGuild{}).Where("lower(name) = lower(?)", name).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrConflict
		}
		guild := AdminGuild{Name: name, LeaderID: leader, Icon: DefaultGuildIcon}
		if err := tx.Create(&guild).Error; err != nil {
			return err
		}
		// Native guild identity is a uint16. Never publish a truncated ID.
		if guild.ID > game.MaxNativeGuildID {
			return errors.New("guild ID limit reached")
		}
		return tx.Create(&guildMember{GuildID: guild.ID, CharacterID: leader, Rank: GuildRankLeader}).Error
	})
}
func (s *Store) JoinGuild(ctx context.Context, inviter, target uint32) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		guild, err := guildForCharacter(tx, inviter)
		if err != nil {
			return err
		}
		var member guildMember
		if err = tx.Where("character_id = ?", inviter).Take(&member).Error; err != nil {
			return err
		}
		if guild.LeaderID != inviter && member.Rank != GuildRankViceLeader {
			return ErrGuildPermission
		}
		var count int64
		if err = tx.Model(&guildMember{}).Where("guild_id = ?", guild.ID).Count(&count).Error; err != nil {
			return err
		}
		if count >= GuildMemberLimit {
			return errors.New("guild is full")
		}
		return tx.Create(&guildMember{GuildID: guild.ID, CharacterID: target, Rank: GuildRankMember}).Error
	})
}
func (s *Store) LeaveGuild(ctx context.Context, actor, target uint32) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		guild, err := guildForCharacter(tx, actor)
		if err != nil {
			return err
		}
		if actor != target && guild.LeaderID != actor {
			return ErrGuildPermission
		}
		if target == guild.LeaderID && actor != target {
			return ErrGuildPermission
		}
		if err = tx.Where("guild_id = ? AND character_id = ?", guild.ID, target).Delete(&guildMember{}).Error; err != nil {
			return err
		}
		return repairGuildLeaders(tx)
	})
}
func (s *Store) EditGuild(ctx context.Context, actor uint32, notice *string, icon *uint32, target uint32, rank *byte) error {
	if notice != nil && len(*notice) > GuildNoticeMaxBytes {
		return errors.New("guild notice exceeds native string limit")
	}
	if rank != nil && *rank > GuildRankViceLeader {
		return ErrGuildPermission
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		guild, err := guildForCharacter(tx, actor)
		if err != nil {
			return err
		}
		if guild.LeaderID != actor {
			return ErrGuildPermission
		}
		if notice != nil {
			if err = tx.Model(&guild).Update("notice", *notice).Error; err != nil {
				return err
			}
		}
		if icon != nil {
			if err = tx.Model(&guild).Update("icon", *icon).Error; err != nil {
				return err
			}
		}
		if rank != nil {
			if target == guild.LeaderID {
				return ErrGuildPermission
			}
			result := tx.Model(&guildMember{}).Where("guild_id = ? AND character_id = ?", guild.ID, target).Update("rank", *rank)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
		}
		return nil
	})
}
func validSocialText(text string, limit int) bool {
	if strings.TrimSpace(text) == "" || len(text) > limit {
		return false
	}
	for _, b := range []byte(text) {
		if b < 32 || b == 127 {
			return false
		}
	}
	return true
}
