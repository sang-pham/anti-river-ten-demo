package db

import (
	"context"
	"fmt"

	"gorm.io/gorm/clause"
)

// SeedDefaultRoles upserts the default roles into DEMO.ROLE.
func (d *DB) SeedDefaultRoles(ctx context.Context) error {
	roles := []Role{
		{
			Code:        "USER",
			Name:        "User",
			Description: "Standard user role",
		},
		{
			Code:        "ADMIN",
			Name:        "Administrator",
			Description: "Administrator role",
		},
		{
			Code:        "ANALYZER",
			Name:        "Analyzer",
			Description: "Data analyzer role",
		},
		{
			Code:        "MONITOR",
			Name:        "Monitor",
			Description: "System monitor role",
		},
		{
			Code:        "TEAM_LEADER",
			Name:        "Team Leader",
			Description: "Team leader role",
		},
	}

	for _, r := range roles {
		role := r // copy to avoid loop variable capture
		if err := d.Gorm.WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "code"}},
				DoNothing: true, // do not overwrite existing roles
			}).
			Create(&role).Error; err != nil {
			return fmt.Errorf("seed role %s: %w", role.Code, err)
		}
	}
	return nil
}
