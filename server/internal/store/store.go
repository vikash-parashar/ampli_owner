// Package store is the Postgres data layer for the Ampli Owner server.
package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vikash-parashar/ampli_owner/server/migrations"
)

var ErrNotFound = errors.New("not found")

type Store struct{ DB *pgxpool.Pool }

func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{DB: pool}, nil
}

func (s *Store) Close() { s.DB.Close() }

// Migrate applies every embedded *.sql file not yet recorded in schema_migrations, in name order.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.DB.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	// Two cold starts can race here; a session lock makes the second wait for the first.
	conn, err := s.DB.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(7420017)`); err != nil {
		return err
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(7420017)`) //nolint:errcheck
	names, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		var done bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = $1)`, name).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		sql, err := fs.ReadFile(migrations.FS, name)
		if err != nil {
			return err
		}
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, name)
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
	}
	return nil
}

// ---- tickets ----

type Ticket struct {
	ID              int64     `json:"id"`
	Ref             string    `json:"ref"`
	ProductID       string    `json:"product"`
	Kind            string    `json:"kind"`
	Title           string    `json:"title"`
	Body            string    `json:"body"`
	Status          string    `json:"status"`
	Priority        int16     `json:"priority"`
	Source          string    `json:"source"`
	InstallID       string    `json:"-"`
	ReporterName    string    `json:"reporter_name"`
	ReporterContact string    `json:"reporter_contact"`
	AppVersion      string    `json:"app_version"`
	Device          string    `json:"device"`
	OSVersion       string    `json:"os"`
	Locale          string    `json:"locale"`
	IsPro           bool      `json:"pro"`
	OrderID         string    `json:"order_id"`
	Votes           int       `json:"votes"`
	GitHubIssueURL  string    `json:"github_issue_url"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func ref(id int64) string { return fmt.Sprintf("T-%04d", id) }

const ticketCols = `id, product_id, kind, title, body, status, priority, source, install_id, reporter_name,
	reporter_contact, app_version, device, os_version, locale, is_pro, order_id, votes, github_issue_url, created_at, updated_at`

func scanTicket(row pgx.Row) (Ticket, error) {
	var t Ticket
	err := row.Scan(&t.ID, &t.ProductID, &t.Kind, &t.Title, &t.Body, &t.Status, &t.Priority, &t.Source, &t.InstallID,
		&t.ReporterName, &t.ReporterContact, &t.AppVersion, &t.Device, &t.OSVersion, &t.Locale, &t.IsPro, &t.OrderID,
		&t.Votes, &t.GitHubIssueURL, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	t.Ref = ref(t.ID)
	return t, err
}

func (s *Store) CreateTicket(ctx context.Context, t Ticket) (Ticket, error) {
	row := s.DB.QueryRow(ctx, `INSERT INTO tickets (product_id, kind, title, body, status, priority, source, install_id,
		reporter_name, reporter_contact, app_version, device, os_version, locale, is_pro, order_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING `+ticketCols,
		t.ProductID, t.Kind, t.Title, t.Body, t.Status, t.Priority, t.Source, t.InstallID, t.ReporterName,
		t.ReporterContact, t.AppVersion, t.Device, t.OSVersion, t.Locale, t.IsPro, t.OrderID)
	return scanTicket(row)
}

func (s *Store) GetTicket(ctx context.Context, id int64) (Ticket, error) {
	return scanTicket(s.DB.QueryRow(ctx, `SELECT `+ticketCols+` FROM tickets WHERE id = $1`, id))
}

type TicketFilter struct {
	Product, Kind, Status, InstallID string
	Limit                            int
}

func (s *Store) ListTickets(ctx context.Context, f TicketFilter) ([]Ticket, error) {
	var where []string
	var args []any
	add := func(col, v string) {
		if v != "" {
			args = append(args, v)
			where = append(where, fmt.Sprintf("%s = $%d", col, len(args)))
		}
	}
	add("product_id", f.Product)
	add("kind", f.Kind)
	add("status", f.Status)
	add("install_id", f.InstallID)
	q := `SELECT ` + ticketCols + ` FROM tickets`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	order := " ORDER BY created_at DESC"
	if f.Kind == "feature" {
		order = " ORDER BY votes DESC, created_at DESC"
	}
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	args = append(args, f.Limit)
	q += order + fmt.Sprintf(" LIMIT $%d", len(args))
	rows, err := s.DB.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Ticket{}
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TicketPatch holds the owner-editable fields; nil means unchanged.
type TicketPatch struct {
	Status         *string `json:"status"`
	Priority       *int16  `json:"priority"`
	Title          *string `json:"title"`
	GitHubIssueURL *string `json:"github_issue_url"`
}

func (s *Store) PatchTicket(ctx context.Context, id int64, p TicketPatch) (Ticket, error) {
	return scanTicket(s.DB.QueryRow(ctx, `UPDATE tickets SET
		status = COALESCE($2, status), priority = COALESCE($3, priority), title = COALESCE($4, title),
		github_issue_url = COALESCE($5, github_issue_url), updated_at = now()
		WHERE id = $1 RETURNING `+ticketCols, id, p.Status, p.Priority, p.Title, p.GitHubIssueURL))
}

// ReportsSince counts tickets one install filed since t (for rate limiting).
func (s *Store) ReportsSince(ctx context.Context, installID string, t time.Time) (int, error) {
	var n int
	err := s.DB.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE install_id = $1 AND created_at >= $2`, installID, t).Scan(&n)
	return n, err
}

type Message struct {
	ID        int64     `json:"id"`
	TicketID  int64     `json:"ticket_id"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// AddMessage stores a reply and moves the ticket to the side that now owes an answer.
func (s *Store) AddMessage(ctx context.Context, ticketID int64, author, body string) (Message, error) {
	m := Message{TicketID: ticketID, Author: author, Body: body}
	status := "waiting_on_user"
	if author == "user" {
		status = "waiting_on_owner"
	}
	err := pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE tickets SET updated_at = now(),
			status = CASE WHEN status IN ('fixed','closed','planned','in_progress') THEN status ELSE $2 END WHERE id = $1`, ticketID, status)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return tx.QueryRow(ctx, `INSERT INTO ticket_messages (ticket_id, author, body) VALUES ($1,$2,$3) RETURNING id, created_at`,
			ticketID, author, body).Scan(&m.ID, &m.CreatedAt)
	})
	return m, err
}

func (s *Store) Messages(ctx context.Context, ticketID int64) ([]Message, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, ticket_id, author, body, created_at FROM ticket_messages WHERE ticket_id = $1 ORDER BY id`, ticketID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Message, error) {
		var m Message
		return m, r.Scan(&m.ID, &m.TicketID, &m.Author, &m.Body, &m.CreatedAt)
	})
}

// Vote records one vote per install on a feature request; it returns the new total.
func (s *Store) Vote(ctx context.Context, ticketID int64, installID string) (int, error) {
	var votes int
	err := pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		var kind string
		if err := tx.QueryRow(ctx, `SELECT kind FROM tickets WHERE id = $1 FOR UPDATE`, ticketID).Scan(&kind); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if kind != "feature" {
			return ErrNotFound
		}
		tag, err := tx.Exec(ctx, `INSERT INTO feature_votes (ticket_id, install_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, ticketID, installID)
		if err != nil {
			return err
		}
		return tx.QueryRow(ctx, `UPDATE tickets SET votes = votes + $2 WHERE id = $1 RETURNING votes`, ticketID, tag.RowsAffected()).Scan(&votes)
	})
	return votes, err
}
