// Package feedback keeps tester reports private and operator responses audited.
package feedback

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"minecraft-hosting/internal/audit"
	"minecraft-hosting/internal/auth"
	"minecraft-hosting/internal/jobs"
	"minecraft-hosting/internal/server"
)

var ErrConflict = errors.New("report changed or submission key reused")

type Service struct{ DB *pgxpool.Pool }
type Report struct {
	RequestReference *uuid.UUID `json:"request_reference"`
	SourcePage       string     `json:"source_page"`
	ID               uuid.UUID  `json:"id"`
	AuthorID         uuid.UUID  `json:"author_id"`
	AuthorName       string     `json:"author_name"`
	AuthorEmail      string     `json:"author_email"`
	ServerID         *uuid.UUID `json:"server_id"`
	Category         string     `json:"category"`
	Title            string     `json:"title"`
	Message          string     `json:"message"`
	Status           string     `json:"status"`
	Response         string     `json:"response"`
	Revision         int        `json:"revision"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}
type Input struct {
	RequestReference *uuid.UUID `json:"request_reference"`
	SourcePage       string     `json:"source_page"`
	ServerID         *uuid.UUID `json:"server_id"`
	Category         string     `json:"category"`
	Title            string     `json:"title"`
	Message          string     `json:"message"`
}
type Update struct {
	Status   string `json:"status"`
	Response string `json:"response"`
	Revision int    `json:"revision"`
}

const columns = "f.id,f.author_id,u.name,u.email,f.server_id,f.category,f.title,f.message,f.status,f.response,f.revision,f.created_at,f.updated_at,f.request_reference,f.source_page"

func scan(row pgx.Row) (Report, error) {
	var v Report
	err := row.Scan(&v.ID, &v.AuthorID, &v.AuthorName, &v.AuthorEmail, &v.ServerID, &v.Category, &v.Title, &v.Message, &v.Status, &v.Response, &v.Revision, &v.CreatedAt, &v.UpdatedAt, &v.RequestReference, &v.SourcePage)
	return v, err
}
func validText(s string, min, max int, multiline bool) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) < min || utf8.RuneCountInString(s) > max {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\r' || r == '\t')) {
			return false
		}
	}
	return true
}
func requireUser(ctx context.Context, tx pgx.Tx, actor uuid.UUID, admin bool, lock string) error {
	var allowed bool
	err := tx.QueryRow(ctx, "SELECT is_admin FROM users WHERE id=$1 AND verified_at IS NOT NULL "+lock, actor).Scan(&allowed)
	if errors.Is(err, pgx.ErrNoRows) {
		return jobs.ErrForbidden
	}
	if err != nil {
		return err
	}
	if admin && !allowed {
		return jobs.ErrForbidden
	}
	return nil
}
func (s Service) Create(ctx context.Context, actor, key uuid.UUID, in Input, rid string) (Report, error) {
	in.SourcePage = strings.TrimSpace(in.SourcePage)
	if !ValidSourcePage(in.SourcePage) || (in.RequestReference != nil && *in.RequestReference == uuid.Nil) {
		return Report{}, server.ErrInvalid
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Message = strings.TrimSpace(in.Message)
	if key == uuid.Nil || !validText(in.Title, 3, 120, false) || !validText(in.Message, 10, 4000, true) || (in.Category != "BUG" && in.Category != "QUESTION" && in.Category != "IDEA") || (in.ServerID != nil && *in.ServerID == uuid.Nil) {
		return Report{}, server.ErrInvalid
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Report{}, err
	}
	defer tx.Rollback(context.Background())
	// Serialize submissions per author, including retries and the daily quota.
	if err = requireUser(ctx, tx, actor, false, "FOR UPDATE"); err != nil {
		return Report{}, err
	}
	existing, err := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM beta_feedback f JOIN users u ON u.id=f.author_id WHERE f.author_id=$1 AND f.submission_key=$2", actor, key))
	if err == nil {
		sameServer := (existing.ServerID == nil && in.ServerID == nil) || (existing.ServerID != nil && in.ServerID != nil && *existing.ServerID == *in.ServerID)
		sameReference := (existing.RequestReference == nil && in.RequestReference == nil) || (existing.RequestReference != nil && in.RequestReference != nil && *existing.RequestReference == *in.RequestReference)
		if !sameReference || existing.SourcePage != in.SourcePage || !sameServer || existing.Category != in.Category || existing.Title != in.Title || existing.Message != in.Message {
			return Report{}, ErrConflict
		}
		return existing, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Report{}, err
	}
	if in.ServerID != nil {
		var accessible bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM servers s WHERE s.id=$2 AND (s.owner_id=$1 OR EXISTS(SELECT 1 FROM server_members m WHERE m.server_id=s.id AND m.user_id=$1)))", actor, *in.ServerID).Scan(&accessible); err != nil {
			return Report{}, err
		}
		if !accessible {
			return Report{}, server.ErrNotFound
		}
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM beta_feedback WHERE author_id=$1 AND created_at>now()-interval '24 hours'", actor).Scan(&count); err != nil {
		return Report{}, err
	}
	if count >= 20 {
		return Report{}, auth.ErrRateLimited
	}
	id := uuid.New()
	if _, err = tx.Exec(ctx, "INSERT INTO beta_feedback(id,author_id,server_id,submission_key,category,title,message,request_reference,source_page) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)", id, actor, in.ServerID, key, in.Category, in.Title, in.Message, in.RequestReference, in.SourcePage); err != nil {
		return Report{}, err
	}
	if err = audit.Record(ctx, tx, actor, uuid.Nil, "beta.feedback_created", rid, map[string]string{"report_id": id.String(), "category": in.Category}); err != nil {
		return Report{}, err
	}
	out, err := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM beta_feedback f JOIN users u ON u.id=f.author_id WHERE f.id=$1", id))
	if err != nil {
		return Report{}, err
	}
	return out, tx.Commit(ctx)
}
func (s Service) List(ctx context.Context, actor uuid.UUID, admin bool, offset int) ([]Report, error) {
	if offset < 0 || offset > 100000 {
		return nil, server.ErrInvalid
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	if err = requireUser(ctx, tx, actor, admin, "FOR SHARE"); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, "SELECT "+columns+" FROM beta_feedback f JOIN users u ON u.id=f.author_id WHERE ($2::boolean OR f.author_id=$1) ORDER BY f.created_at DESC,f.id LIMIT 50 OFFSET $3", actor, admin, offset)
	if err != nil {
		return nil, err
	}
	out := []Report{}
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			rows.Close()
			return nil, e
		}
		out = append(out, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	return out, tx.Commit(ctx)
}
func (s Service) Update(ctx context.Context, actor, id uuid.UUID, in Update, rid string) (Report, error) {
	in.Response = strings.TrimSpace(in.Response)
	if in.Revision < 1 || !validText(in.Response, 0, 2000, true) || (in.Status != "NEW" && in.Status != "REVIEWED" && in.Status != "RESOLVED") {
		return Report{}, server.ErrInvalid
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Report{}, err
	}
	defer tx.Rollback(context.Background())
	if err = requireUser(ctx, tx, actor, true, "FOR SHARE"); err != nil {
		return Report{}, err
	}
	var revision int
	if err = tx.QueryRow(ctx, "SELECT revision FROM beta_feedback WHERE id=$1 FOR UPDATE", id).Scan(&revision); errors.Is(err, pgx.ErrNoRows) {
		return Report{}, server.ErrNotFound
	} else if err != nil {
		return Report{}, err
	}
	if revision != in.Revision {
		return Report{}, ErrConflict
	}
	if _, err = tx.Exec(ctx, "UPDATE beta_feedback SET status=$2,response=$3,revision=revision+1,updated_at=now() WHERE id=$1", id, in.Status, in.Response); err != nil {
		return Report{}, err
	}
	if err = audit.Record(ctx, tx, actor, uuid.Nil, "beta.feedback_updated", rid, map[string]string{"report_id": id.String(), "status": in.Status}); err != nil {
		return Report{}, err
	}
	out, err := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM beta_feedback f JOIN users u ON u.id=f.author_id WHERE f.id=$1", id))
	if err != nil {
		return Report{}, err
	}
	return out, tx.Commit(ctx)
}

var serverPage = regexp.MustCompile(`^/servers/([0-9a-fA-F-]{36})(?:/(?:plugins|mods|files|console|backups|settings|worlds|members|players|addons/simple-voice-chat))?$`)

// Only app routes are retained; URLs, query strings and arbitrary paths may contain secrets.
func ValidSourcePage(page string) bool {
	switch page {
	case "", "/", "/dashboard", "/servers", "/servers/new", "/billing", "/account", "/guide", "/admin", "/feedback", "/login", "/register":
		return true
	}
	match := serverPage.FindStringSubmatch(page)
	if len(match) != 2 {
		return false
	}
	_, err := uuid.Parse(match[1])
	return err == nil
}
