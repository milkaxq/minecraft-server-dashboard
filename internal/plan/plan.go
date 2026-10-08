package plan

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUnavailable = errors.New("plan unavailable")

type Plan struct {
	ID                   uuid.UUID `json:"id"`
	Name                 string    `json:"name"`
	Description          string    `json:"description"`
	PriceCents           int64     `json:"price_cents"`
	Currency             string    `json:"currency"`
	BillingPeriod        string    `json:"billing_period"`
	RAMMB                int       `json:"ram_mb"`
	DiskMB               int       `json:"disk_mb"`
	CPULimit             int       `json:"cpu_limit"`
	BackupLimit          int       `json:"backup_limit"`
	DatabaseLimit        int       `json:"database_limit"`
	PlayerRecommendation int       `json:"player_recommendation"`
	Enabled              bool      `json:"enabled"`
}

const Columns = "id,name,description,price_cents,currency,billing_period,ram_mb,disk_mb,cpu_limit,backup_limit,database_limit,player_recommendation,enabled"

func Scan(row pgx.Row) (Plan, error) {
	var p Plan
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.PriceCents, &p.Currency, &p.BillingPeriod, &p.RAMMB, &p.DiskMB, &p.CPULimit, &p.BackupLimit, &p.DatabaseLimit, &p.PlayerRecommendation, &p.Enabled)
	return p, err
}

type Service struct{ DB *pgxpool.Pool }

func (s Service) List(ctx context.Context) ([]Plan, error) {
	rows, err := s.DB.Query(ctx, "SELECT "+Columns+" FROM plans WHERE enabled ORDER BY price_cents,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Plan{}
	for rows.Next() {
		p, err := Scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
