package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"minecraft-hosting/internal/audit"
)

const MaxMembers = 20

var ErrMemberConflict = errors.New("member operation conflicts with existing access")
var ErrMemberUnavailable = errors.New("verified member account unavailable")

type Member struct {
	UserID uuid.UUID `json:"user_id"`
	Name   string    `json:"name"`
	Email  string    `json:"email"`
	Role   Role      `json:"role"`
}
type MemberChange struct {
	Email        string `json:"email,omitempty"`
	Role         Role   `json:"role,omitempty"`
	Confirmation string `json:"confirmation,omitempty"`
}

func (s Service) ListMembers(ctx context.Context, actor, id uuid.UUID) ([]Member, error) {
	if err := s.Authorize(ctx, actor, id, Members); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `SELECT u.id,u.name,u.email,'OWNER' FROM servers s JOIN users u ON u.id=s.owner_id WHERE s.id=$1 AND s.owner_id=$2 UNION ALL SELECT u.id,u.name,u.email,m.role FROM server_members m JOIN users u ON u.id=m.user_id JOIN servers s ON s.id=m.server_id WHERE s.id=$1 AND s.owner_id=$2 ORDER BY 4 DESC,3`, id, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Member{}
	for rows.Next() {
		var m Member
		if err = rows.Scan(&m.UserID, &m.Name, &m.Email, &m.Role); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// Membership, the replay receipt, and its audit entry commit atomically. The
// shared server lock prevents a power/file operation from using stale membership
// during its remote mutation. Console streams recheck access periodically.
func (s Service) ChangeMember(ctx context.Context, actor, id, key, target uuid.UUID, action string, in MemberChange, rid string) (any, error) {
	if key == uuid.Nil {
		return nil, ErrInvalid
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	var owner uuid.UUID
	err = tx.QueryRow(ctx, "SELECT owner_id FROM servers WHERE id=$1", id).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && owner != actor {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var held bool
	if err = tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(hashtextextended($1,0))", "server:"+id.String()).Scan(&held); err != nil {
		return nil, err
	}
	if !held {
		return nil, ErrMemberConflict
	}
	// Recheck ownership after acquiring the lock and hold the server row too.
	if err = tx.QueryRow(ctx, "SELECT owner_id FROM servers WHERE id=$1 FOR UPDATE", id).Scan(&owner); err != nil {
		return nil, err
	}
	if owner != actor {
		return nil, ErrNotFound
	}
	switch action {
	case "add":
		in.Email = strings.ToLower(strings.TrimSpace(in.Email))
		address, e := mail.ParseAddress(in.Email)
		if e != nil || address.Address != in.Email || len(in.Email) > 254 || !strings.Contains(in.Email, "@") || in.Confirmation != "" || target != uuid.Nil {
			return nil, ErrInvalid
		}
		if in.Role != Admin && in.Role != Moderator {
			return nil, ErrInvalid
		}
	case "update":
		if target == uuid.Nil || in.Email != "" || in.Confirmation != "" || (in.Role != Admin && in.Role != Moderator) {
			return nil, ErrInvalid
		}
	case "remove":
		if target == uuid.Nil || in.Confirmation != target.String() || in.Role != "" || in.Email != "" {
			return nil, ErrInvalid
		}
	default:
		return nil, ErrInvalid
	}
	if target == owner {
		return nil, ErrInvalid
	}
	payload, _ := json.Marshal(struct {
		Action string
		Target uuid.UUID
		Input  MemberChange
	}{action, target, in})
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(payload))
	var previous string
	var data []byte
	err = tx.QueryRow(ctx, "SELECT fingerprint,result FROM member_operations WHERE server_id=$1 AND actor_id=$2 AND idempotency_key=$3", id, actor, key).Scan(&previous, &data)
	if err == nil {
		if previous != fingerprint {
			return nil, ErrMemberConflict
		}
		var result any
		if err = json.Unmarshal(data, &result); err != nil {
			return nil, err
		}
		return result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	var result any
	if action == "add" {
		var m Member
		err = tx.QueryRow(ctx, "SELECT id,name,email FROM users WHERE email=$1 AND verified_at IS NOT NULL FOR SHARE", in.Email).Scan(&m.UserID, &m.Name, &m.Email)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMemberUnavailable
		}
		if err != nil {
			return nil, err
		}
		if m.UserID == owner {
			return nil, ErrInvalid
		}
		var count int
		var exists bool
		if err = tx.QueryRow(ctx, "SELECT count(*),COALESCE(bool_or(user_id=$2),false) FROM server_members WHERE server_id=$1", id, m.UserID).Scan(&count, &exists); err != nil {
			return nil, err
		}
		if exists || count >= MaxMembers {
			return nil, ErrMemberConflict
		}
		if _, err = tx.Exec(ctx, "INSERT INTO server_members(server_id,user_id,role) VALUES($1,$2,$3)", id, m.UserID, in.Role); err != nil {
			return nil, err
		}
		m.Role = in.Role
		target = m.UserID
		result = m
	} else if action == "update" {
		var m Member
		err = tx.QueryRow(ctx, "UPDATE server_members m SET role=$3 FROM users u WHERE m.server_id=$1 AND m.user_id=$2 AND u.id=m.user_id RETURNING u.id,u.name,u.email,m.role", id, target, in.Role).Scan(&m.UserID, &m.Name, &m.Email, &m.Role)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		result = m
	} else {
		tag, e := tx.Exec(ctx, "DELETE FROM server_members WHERE server_id=$1 AND user_id=$2", id, target)
		if e != nil {
			return nil, e
		}
		if tag.RowsAffected() != 1 {
			return nil, ErrNotFound
		}
		result = map[string]string{"status": "removed"}
	}
	data, err = json.Marshal(result)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO member_operations(server_id,actor_id,idempotency_key,fingerprint,result) VALUES($1,$2,$3,$4,$5)", id, actor, key, fingerprint, data); err != nil {
		return nil, err
	}
	if err = audit.Record(ctx, tx, actor, id, "member."+action, rid, map[string]any{"member_id": target, "role": in.Role}); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}
