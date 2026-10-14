package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---- checklist ----

type ChecklistItem struct {
	ID       string     `json:"id"`
	Phase    string     `json:"phase"`
	Title    string     `json:"title"`
	Detail   string     `json:"detail"`
	Products []string   `json:"products"`
	Status   string     `json:"status"`
	Link     string     `json:"link"`
	Sort     int        `json:"order"`
	DoneAt   *time.Time `json:"done_at,omitempty"`
}

// UpsertChecklist imports items keyed by id (the ids from ampli-company/checklist.json).
// With keepStatus, rows that already exist keep the owner's ticks and only their text is refreshed.
func (s *Store) UpsertChecklist(ctx context.Context, items []ChecklistItem, keepStatus bool) error {
	return pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		for i, it := range items {
			if it.Status == "" {
				it.Status = "todo"
			}
			if it.Sort == 0 {
				it.Sort = (i + 1) * 10
			}
			if it.Products == nil {
				it.Products = []string{}
			}
			_, err := tx.Exec(ctx, `INSERT INTO checklist_items (id, phase, title, detail, products, status, link, sort, done_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8, CASE WHEN $6 = 'done' THEN now() END)
				ON CONFLICT (id) DO UPDATE SET phase = EXCLUDED.phase, title = EXCLUDED.title, detail = EXCLUDED.detail,
				products = EXCLUDED.products, link = EXCLUDED.link, sort = EXCLUDED.sort,
				status = CASE WHEN $9 THEN checklist_items.status ELSE EXCLUDED.status END,
				done_at = CASE WHEN (CASE WHEN $9 THEN checklist_items.status ELSE EXCLUDED.status END) = 'done'
					THEN COALESCE(checklist_items.done_at, now()) END`,
				it.ID, it.Phase, it.Title, it.Detail, it.Products, it.Status, it.Link, it.Sort, keepStatus)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) Checklist(ctx context.Context) ([]ChecklistItem, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, phase, title, detail, products, status, link, sort, done_at FROM checklist_items ORDER BY sort, id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (ChecklistItem, error) {
		var c ChecklistItem
		return c, r.Scan(&c.ID, &c.Phase, &c.Title, &c.Detail, &c.Products, &c.Status, &c.Link, &c.Sort, &c.DoneAt)
	})
}

func (s *Store) SetChecklistStatus(ctx context.Context, id, status string) (ChecklistItem, error) {
	var c ChecklistItem
	err := s.DB.QueryRow(ctx, `UPDATE checklist_items SET status = $2,
		done_at = CASE WHEN $2 = 'done' THEN COALESCE(done_at, now()) END WHERE id = $1
		RETURNING id, phase, title, detail, products, status, link, sort, done_at`, id, status).
		Scan(&c.ID, &c.Phase, &c.Title, &c.Detail, &c.Products, &c.Status, &c.Link, &c.Sort, &c.DoneAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// ---- todos ----

type Todo struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Due       *string   `json:"due"` // YYYY-MM-DD
	ProductID *string   `json:"product"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"created_at"`
}

const todoCols = `id, title, to_char(due, 'YYYY-MM-DD'), product_id, done, created_at`

func scanTodo(r pgx.Row) (Todo, error) {
	var t Todo
	err := r.Scan(&t.ID, &t.Title, &t.Due, &t.ProductID, &t.Done, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

func (s *Store) Todos(ctx context.Context) ([]Todo, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+todoCols+` FROM todos ORDER BY done, due NULLS LAST, id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Todo, error) { return scanTodo(r) })
}

func (s *Store) CreateTodo(ctx context.Context, t Todo) (Todo, error) {
	return scanTodo(s.DB.QueryRow(ctx, `INSERT INTO todos (title, due, product_id) VALUES ($1, $2::date, $3) RETURNING `+todoCols,
		t.Title, t.Due, t.ProductID))
}

type TodoPatch struct {
	Title *string `json:"title"`
	Due   *string `json:"due"`
	Done  *bool   `json:"done"`
}

func (s *Store) PatchTodo(ctx context.Context, id int64, p TodoPatch) (Todo, error) {
	return scanTodo(s.DB.QueryRow(ctx, `UPDATE todos SET title = COALESCE($2, title), due = COALESCE($3::date, due),
		done = COALESCE($4, done) WHERE id = $1 RETURNING `+todoCols, id, p.Title, p.Due, p.Done))
}

func (s *Store) DeleteTodo(ctx context.Context, id int64) error {
	tag, err := s.DB.Exec(ctx, `DELETE FROM todos WHERE id = $1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// ---- features (built vs left, roadmap) ----

type Feature struct {
	ID        int64  `json:"id"`
	ProductID string `json:"product"`
	Title     string `json:"title"`
	State     string `json:"state"`
	TicketID  *int64 `json:"ticket_id"`
	Build     string `json:"build"`
	Sort      int    `json:"order"`
}

const featureCols = `id, product_id, title, state, ticket_id, build, sort`

func scanFeature(r pgx.Row) (Feature, error) {
	var f Feature
	err := r.Scan(&f.ID, &f.ProductID, &f.Title, &f.State, &f.TicketID, &f.Build, &f.Sort)
	if errors.Is(err, pgx.ErrNoRows) {
		return f, ErrNotFound
	}
	return f, err
}

func (s *Store) Features(ctx context.Context, product string) ([]Feature, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+featureCols+` FROM features WHERE ($1 = '' OR product_id = $1) ORDER BY product_id, sort, id`, product)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Feature, error) { return scanFeature(r) })
}

func (s *Store) CreateFeature(ctx context.Context, f Feature) (Feature, error) {
	return scanFeature(s.DB.QueryRow(ctx, `INSERT INTO features (product_id, title, state, ticket_id, build, sort)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING `+featureCols, f.ProductID, f.Title, f.State, f.TicketID, f.Build, f.Sort))
}

type FeaturePatch struct {
	Title *string `json:"title"`
	State *string `json:"state"`
	Build *string `json:"build"`
	Sort  *int    `json:"order"`
}

func (s *Store) PatchFeature(ctx context.Context, id int64, p FeaturePatch) (Feature, error) {
	return scanFeature(s.DB.QueryRow(ctx, `UPDATE features SET title = COALESCE($2, title), state = COALESCE($3, state),
		build = COALESCE($4, build), sort = COALESCE($5, sort), updated_at = now() WHERE id = $1 RETURNING `+featureCols,
		id, p.Title, p.State, p.Build, p.Sort))
}

// SeedFeatures inserts features only when the table is empty and returns how many were added.
func (s *Store) SeedFeatures(ctx context.Context, fs []Feature) (int, error) {
	added := 0
	err := pgx.BeginFunc(ctx, s.DB, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM features`).Scan(&n); err != nil || n > 0 {
			return err
		}
		for _, f := range fs {
			if _, err := tx.Exec(ctx, `INSERT INTO features (product_id, title, state, build, sort) VALUES ($1,$2,$3,$4,$5)`,
				f.ProductID, f.Title, f.State, f.Build, f.Sort); err != nil {
				return err
			}
			added++
		}
		return nil
	})
	return added, err
}

// ---- campaigns ----

type Campaign struct {
	ID          int64   `json:"id"`
	Platform    string  `json:"platform"`
	ProductID   *string `json:"product"`
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	BudgetPaise int64   `json:"budget_paise"`
	SpendPaise  int64   `json:"spend_paise"`
	Impressions int64   `json:"impressions"`
	Clicks      int64   `json:"clicks"`
	Installs    int64   `json:"installs"`
	PostURL     string  `json:"post_url"`
	StartsOn    *string `json:"starts_on"`
	EndsOn      *string `json:"ends_on"`
}

const campaignCols = `id, platform, product_id, name, status, budget_paise, spend_paise, impressions, clicks, installs, post_url,
	to_char(starts_on, 'YYYY-MM-DD'), to_char(ends_on, 'YYYY-MM-DD')`

func scanCampaign(r pgx.Row) (Campaign, error) {
	var c Campaign
	err := r.Scan(&c.ID, &c.Platform, &c.ProductID, &c.Name, &c.Status, &c.BudgetPaise, &c.SpendPaise, &c.Impressions,
		&c.Clicks, &c.Installs, &c.PostURL, &c.StartsOn, &c.EndsOn)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

func (s *Store) Campaigns(ctx context.Context) ([]Campaign, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+campaignCols+` FROM campaigns ORDER BY starts_on DESC NULLS FIRST, id DESC`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Campaign, error) { return scanCampaign(r) })
}

// SaveCampaign inserts when ID is 0, otherwise replaces the row.
func (s *Store) SaveCampaign(ctx context.Context, c Campaign) (Campaign, error) {
	if c.Status == "" {
		c.Status = "draft"
	}
	if c.ID == 0 {
		return scanCampaign(s.DB.QueryRow(ctx, `INSERT INTO campaigns (platform, product_id, name, status, budget_paise, spend_paise,
			impressions, clicks, installs, post_url, starts_on, ends_on) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::date,$12::date)
			RETURNING `+campaignCols, c.Platform, c.ProductID, c.Name, c.Status, c.BudgetPaise, c.SpendPaise, c.Impressions,
			c.Clicks, c.Installs, c.PostURL, c.StartsOn, c.EndsOn))
	}
	return scanCampaign(s.DB.QueryRow(ctx, `UPDATE campaigns SET platform=$2, product_id=$3, name=$4, status=$5, budget_paise=$6,
		spend_paise=$7, impressions=$8, clicks=$9, installs=$10, post_url=$11, starts_on=$12::date, ends_on=$13::date
		WHERE id=$1 RETURNING `+campaignCols, c.ID, c.Platform, c.ProductID, c.Name, c.Status, c.BudgetPaise, c.SpendPaise,
		c.Impressions, c.Clicks, c.Installs, c.PostURL, c.StartsOn, c.EndsOn))
}

// ---- daily stats ----

type DayStat struct {
	ProductID       string   `json:"product"`
	Day             string   `json:"day"`
	Installs        int      `json:"installs"`
	Uninstalls      int      `json:"uninstalls"`
	ActiveUsers     int      `json:"active_users"`
	Rating          *float64 `json:"rating"`
	Reviews         int      `json:"reviews"`
	ProSales        int      `json:"pro_sales"`
	ProRevenuePaise int64    `json:"pro_revenue_paise"`
	AdRevenuePaise  int64    `json:"ad_revenue_paise"`
}

func (s *Store) UpsertStat(ctx context.Context, d DayStat) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO daily_stats (product_id, day, installs, uninstalls, active_users, rating, reviews,
		pro_sales, pro_revenue_paise, ad_revenue_paise) VALUES ($1,$2::date,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (product_id, day) DO UPDATE SET installs=EXCLUDED.installs, uninstalls=EXCLUDED.uninstalls,
		active_users=EXCLUDED.active_users, rating=EXCLUDED.rating, reviews=EXCLUDED.reviews, pro_sales=EXCLUDED.pro_sales,
		pro_revenue_paise=EXCLUDED.pro_revenue_paise, ad_revenue_paise=EXCLUDED.ad_revenue_paise`,
		d.ProductID, d.Day, d.Installs, d.Uninstalls, d.ActiveUsers, d.Rating, d.Reviews, d.ProSales, d.ProRevenuePaise, d.AdRevenuePaise)
	return err
}

func (s *Store) Stats(ctx context.Context, product, from, to string) ([]DayStat, error) {
	rows, err := s.DB.Query(ctx, `SELECT product_id, to_char(day, 'YYYY-MM-DD'), installs, uninstalls, active_users, rating::float8,
		reviews, pro_sales, pro_revenue_paise, ad_revenue_paise FROM daily_stats
		WHERE ($1 = '' OR product_id = $1) AND day BETWEEN $2::date AND $3::date ORDER BY day, product_id`, product, from, to)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (DayStat, error) {
		var d DayStat
		return d, r.Scan(&d.ProductID, &d.Day, &d.Installs, &d.Uninstalls, &d.ActiveUsers, &d.Rating, &d.Reviews,
			&d.ProSales, &d.ProRevenuePaise, &d.AdRevenuePaise)
	})
}

// ---- guides ----

type Guide struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	BodyMD string `json:"body_md"`
}

func (s *Store) Guides(ctx context.Context) ([]Guide, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, title, body_md FROM guides ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Guide])
}

func (s *Store) SaveGuide(ctx context.Context, g Guide) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO guides (id, title, body_md) VALUES ($1,$2,$3)
		ON CONFLICT (id) DO UPDATE SET title = EXCLUDED.title, body_md = EXCLUDED.body_md`, g.ID, g.Title, g.BodyMD)
	return err
}

// ---- overview ----

type Overview struct {
	OpenTickets    int            `json:"open_tickets"`
	WaitingOnOwner int            `json:"waiting_on_owner"`
	TicketsByKind  map[string]int `json:"tickets_by_kind"`
	ChecklistTotal int            `json:"checklist_total"`
	ChecklistDone  int            `json:"checklist_done"`
	ChecklistDoing int            `json:"checklist_in_progress"`
	NextChecklist  *ChecklistItem `json:"next_checklist_item"`
	OpenTodos      []Todo         `json:"open_todos"`
	Last7Days      DayStat        `json:"last_7_days"`
}

func (s *Store) Overview(ctx context.Context) (Overview, error) {
	o := Overview{TicketsByKind: map[string]int{}, OpenTodos: []Todo{}}
	rows, err := s.DB.Query(ctx, `SELECT kind, count(*), count(*) FILTER (WHERE status IN ('new','waiting_on_owner'))
		FROM tickets WHERE status NOT IN ('fixed','closed') GROUP BY kind`)
	if err != nil {
		return o, err
	}
	for rows.Next() {
		var k string
		var n, w int
		if err := rows.Scan(&k, &n, &w); err != nil {
			return o, err
		}
		o.TicketsByKind[k] = n
		o.OpenTickets += n
		o.WaitingOnOwner += w
	}
	if err := rows.Err(); err != nil {
		return o, err
	}
	if err := s.DB.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE status='done'), count(*) FILTER (WHERE status='in-progress')
		FROM checklist_items`).Scan(&o.ChecklistTotal, &o.ChecklistDone, &o.ChecklistDoing); err != nil {
		return o, err
	}
	var next ChecklistItem
	err = s.DB.QueryRow(ctx, `SELECT id, phase, title, detail, products, status, link, sort, done_at FROM checklist_items
		WHERE status <> 'done' ORDER BY (status = 'in-progress') DESC, sort LIMIT 1`).
		Scan(&next.ID, &next.Phase, &next.Title, &next.Detail, &next.Products, &next.Status, &next.Link, &next.Sort, &next.DoneAt)
	if err == nil {
		o.NextChecklist = &next
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return o, err
	}
	trows, err := s.DB.Query(ctx, `SELECT `+todoCols+` FROM todos WHERE NOT done ORDER BY due NULLS LAST, id LIMIT 5`)
	if err != nil {
		return o, err
	}
	if o.OpenTodos, err = pgx.CollectRows(trows, func(r pgx.CollectableRow) (Todo, error) { return scanTodo(r) }); err != nil {
		return o, err
	}
	err = s.DB.QueryRow(ctx, `SELECT COALESCE(sum(installs),0), COALESCE(sum(uninstalls),0), COALESCE(sum(reviews),0),
		COALESCE(sum(pro_sales),0), COALESCE(sum(pro_revenue_paise),0), COALESCE(sum(ad_revenue_paise),0)
		FROM daily_stats WHERE day > current_date - 7`).Scan(&o.Last7Days.Installs, &o.Last7Days.Uninstalls,
		&o.Last7Days.Reviews, &o.Last7Days.ProSales, &o.Last7Days.ProRevenuePaise, &o.Last7Days.AdRevenuePaise)
	return o, err
}
