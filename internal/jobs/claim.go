package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Lease holds a PostgreSQL session advisory lock until completion. There is no
// time-based lease that can expire under an active network request. Process death
// releases the lock; RUNNING jobs are reclaimable after available_at.
// Use this same connection for job writes, so a lost locking session cannot commit.
type Lease struct {
	Conn *pgxpool.Conn
	Job  Job
	key  string
}

func (l *Lease) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var unlocked bool
	err := l.Conn.QueryRow(ctx, "SELECT pg_advisory_unlock(hashtextextended($1,0))", l.key).Scan(&unlocked)
	if err != nil || !unlocked {
		_ = l.Conn.Conn().Close(ctx)
	}
	l.Conn.Release()
}
func (s Service) Claim(ctx context.Context) (*Lease, error) {
	conn, err := s.DB.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx, "SELECT id,server_id FROM jobs WHERE status IN ('PENDING','RUNNING') AND available_at<=now() ORDER BY available_at,created_at LIMIT 32")
	if err != nil {
		conn.Release()
		return nil, err
	}
	type candidate struct{ id, server uuid.UUID }
	items := []candidate{}
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.id, &c.server); err != nil {
			rows.Close()
			conn.Release()
			return nil, err
		}
		items = append(items, c)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		conn.Release()
		return nil, err
	}
	for _, item := range items {
		key := "server:" + item.server.String()
		var locked bool
		if err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", key).Scan(&locked); err != nil {
			_ = conn.Conn().Close(context.Background())
			conn.Release()
			return nil, err
		}
		if !locked {
			continue
		}
		lease := &Lease{Conn: conn, key: key}
		job, err := Scan(conn.QueryRow(ctx, `UPDATE jobs SET status='RUNNING',attempts=attempts+1,updated_at=now() WHERE id=$1 AND status IN ('PENDING','RUNNING') AND available_at<=now() RETURNING `+columns, item.id))
		if err != nil {
			lease.Close()
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, nil
			}
			return nil, err
		}
		lease.Job = job
		return lease, nil
	}
	conn.Release()
	return nil, nil
}
