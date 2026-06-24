package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Task struct {
	ID          int64     `json:"id"`
	No          int       `json:"no"`
	PageSection string    `json:"pageSection"`
	Component   string    `json:"component"`
	Issue       string    `json:"issue"`
	Status      string    `json:"status"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type DashboardStats struct {
	Total       int               `json:"total"`
	Done        int               `json:"done"`
	InProgress  int               `json:"inProgress"`
	Todo        int               `json:"todo"`
	Blocked     int               `json:"blocked"`
	BySection   []SectionSummary  `json:"bySection"`
	RecentTasks []Task            `json:"recentTasks"`
}

type SectionSummary struct {
	Name  string `json:"name"`
	Total int    `json:"total"`
	Done  int    `json:"done"`
}

type workbookTask struct {
	No          int
	PageSection string
	Component   string
	Issue       string
	Status      string
}

type app struct {
	db *sql.DB
}

func main() {
	ctx := context.Background()
	db := mustOpenDB()
	defer db.Close()

	mustInitSchema(ctx, db)

	xlsxPath := env("XLSX_PATH", filepath.Join("..", "Monitoring Pekerjaan.xlsx"))
	if err := seedFromWorkbook(ctx, db, xlsxPath); err != nil {
		log.Printf("seed warning: %v", err)
	}

	srv := &app{db: db}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", srv.handleHealth)
	mux.HandleFunc("/api/stats", srv.handleStats)
	mux.HandleFunc("/api/tasks", srv.handleTasks)
	mux.HandleFunc("/api/tasks/", srv.handleTaskByID)

	handler := withCORS(mux)
	port := env("PORT", "8090")
	addr := ":" + port
	log.Printf("backend listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, handler))
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func mustOpenDB() *sql.DB {
	candidates := []string{}
	if envDSN := strings.TrimSpace(os.Getenv("DATABASE_URL")); envDSN != "" {
		candidates = append(candidates, envDSN)
	}
	candidates = append(candidates,
		"postgres://cbqa:cbqa@127.0.0.1:5439/cbqa?sslmode=disable",
		"postgres://cbqa:cbqa@127.0.0.1:5432/cbqa?sslmode=disable",
	)

	for _, dsn := range candidates {
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			log.Printf("open db %s: %v", dsn, err)
			continue
		}
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(30 * time.Minute)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = db.PingContext(ctx)
		cancel()
		if err == nil {
			log.Printf("connected to postgres: %s", dsn)
			return db
		}
		_ = db.Close()
		log.Printf("ping db %s: %v", dsn, err)
	}

	log.Fatal("no reachable postgres instance found")
	return nil
}

func mustInitSchema(ctx context.Context, db *sql.DB) {
	const schema = `
CREATE TABLE IF NOT EXISTS tasks (
	id BIGSERIAL PRIMARY KEY,
	no INTEGER NOT NULL UNIQUE,
	page_section TEXT NOT NULL,
	component TEXT NOT NULL,
	issue TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'todo' CHECK (status IN ('todo','in_progress','blocked','done')),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`
	if _, err := db.ExecContext(ctx, schema); err != nil {
		log.Fatalf("init schema: %v", err)
	}
}

func seedFromWorkbook(ctx context.Context, db *sql.DB, xlsxPath string) error {
	tasks, err := loadWorkbookTasks(xlsxPath)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	const upsert = `
INSERT INTO tasks (no, page_section, component, issue, status, updated_at)
VALUES ($1, $2, $3, $4, $5, now())
ON CONFLICT (no) DO UPDATE
SET page_section = EXCLUDED.page_section,
    component = EXCLUDED.component,
    issue = EXCLUDED.issue,
    updated_at = now();
`
	for _, task := range tasks {
		if _, err := tx.ExecContext(ctx, upsert, task.No, task.PageSection, task.Component, task.Issue, task.Status); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (a *app) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *app) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	type countRow struct {
		Total      int
		Done       int
		InProgress int
		Todo       int
		Blocked    int
	}
	var c countRow
	if err := a.db.QueryRowContext(ctx, `
SELECT
  COUNT(*) AS total,
  COUNT(*) FILTER (WHERE status = 'done') AS done,
  COUNT(*) FILTER (WHERE status = 'in_progress') AS in_progress,
  COUNT(*) FILTER (WHERE status = 'todo') AS todo,
  COUNT(*) FILTER (WHERE status = 'blocked') AS blocked
FROM tasks;`).Scan(&c.Total, &c.Done, &c.InProgress, &c.Todo, &c.Blocked); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rows, err := a.db.QueryContext(ctx, `
SELECT page_section,
       COUNT(*) AS total,
       COUNT(*) FILTER (WHERE status = 'done') AS done
FROM tasks
GROUP BY page_section
ORDER BY total DESC, page_section ASC;`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	sections := make([]SectionSummary, 0)
	for rows.Next() {
		var s SectionSummary
		if err := rows.Scan(&s.Name, &s.Total, &s.Done); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		sections = append(sections, s)
	}

	recent, err := a.fetchTasks(ctx, taskFilter{Limit: 8, OrderBy: "updated_at DESC, no DESC"})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, DashboardStats{
		Total:       c.Total,
		Done:        c.Done,
		InProgress:  c.InProgress,
		Todo:        c.Todo,
		Blocked:     c.Blocked,
		BySection:   sections,
		RecentTasks: recent,
	})
}

type taskFilter struct {
	Query   string
	Section string
	Status  string
	Limit   int
	Offset  int
	OrderBy string
}

func (a *app) fetchTasks(ctx context.Context, f taskFilter) ([]Task, error) {
	args := make([]any, 0, 4)
	clauses := make([]string, 0, 4)

	if f.Query != "" {
		args = append(args, "%"+f.Query+"%")
		idx := len(args)
		clauses = append(clauses, "(page_section ILIKE $"+strconv.Itoa(idx)+" OR component ILIKE $"+strconv.Itoa(idx)+" OR issue ILIKE $"+strconv.Itoa(idx)+")")
	}
	if f.Section != "" {
		args = append(args, f.Section)
		clauses = append(clauses, "page_section = $"+strconv.Itoa(len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		clauses = append(clauses, "status = $"+strconv.Itoa(len(args)))
	}

	query := "SELECT id, no, page_section, component, issue, status, updated_at FROM tasks"
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	if f.OrderBy == "" {
		f.OrderBy = "no ASC"
	}
	query += " ORDER BY " + f.OrderBy
	if f.Limit > 0 {
		args = append(args, f.Limit)
		query += " LIMIT $" + strconv.Itoa(len(args))
	}
	if f.Offset > 0 {
		args = append(args, f.Offset)
		query += " OFFSET $" + strconv.Itoa(len(args))
	}

	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]Task, 0)
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.No, &t.PageSection, &t.Component, &t.Issue, &t.Status, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (a *app) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		f := taskFilter{
			Query:   strings.TrimSpace(r.URL.Query().Get("query")),
			Section: strings.TrimSpace(r.URL.Query().Get("section")),
			Status:  strings.TrimSpace(r.URL.Query().Get("status")),
		}
		if limit := strings.TrimSpace(r.URL.Query().Get("limit")); limit != "" {
			if v, err := strconv.Atoi(limit); err == nil && v > 0 {
				f.Limit = v
			}
		}
		if offset := strings.TrimSpace(r.URL.Query().Get("offset")); offset != "" {
			if v, err := strconv.Atoi(offset); err == nil && v >= 0 {
				f.Offset = v
			}
		}
		if f.Limit == 0 {
			f.Limit = 1000
		}
		tasks, err := a.fetchTasks(r.Context(), f)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, tasks)
	case http.MethodPost:
		var payload struct {
			PageSection string `json:"pageSection"`
			Component   string `json:"component"`
			Issue       string `json:"issue"`
			Status      string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		payload.PageSection = strings.TrimSpace(payload.PageSection)
		payload.Component = strings.TrimSpace(payload.Component)
		payload.Issue = strings.TrimSpace(payload.Issue)
		payload.Status = strings.TrimSpace(payload.Status)
		if payload.PageSection == "" || payload.Component == "" || payload.Issue == "" {
			http.Error(w, "pageSection, component, and issue are required", http.StatusBadRequest)
			return
		}
		if payload.Status == "" {
			payload.Status = "todo"
		}
		if !validStatus(payload.Status) {
			http.Error(w, "invalid status", http.StatusBadRequest)
			return
		}

		var task Task
		tx, err := a.db.BeginTx(r.Context(), nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer func() { _ = tx.Rollback() }()

		if err := tx.QueryRowContext(r.Context(), `
SELECT COALESCE(MAX(no), 0) + 1
FROM tasks;`).Scan(&task.No); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := tx.QueryRowContext(r.Context(), `
INSERT INTO tasks (no, page_section, component, issue, status, updated_at)
VALUES ($1, $2, $3, $4, $5, now())
RETURNING id, no, page_section, component, issue, status, updated_at;`,
			task.No, payload.PageSection, payload.Component, payload.Issue, payload.Status,
		).Scan(&task.ID, &task.No, &task.PageSection, &task.Component, &task.Issue, &task.Status, &task.UpdatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, task)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *app) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/api/tasks/") {
		http.NotFound(w, r)
		return
	}
	idPart := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	idPart = strings.Trim(idPart, "/")
	if idPart == "" {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(idPart, 10, 64)
	if err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPatch:
		var payload struct {
			PageSection *string `json:"pageSection"`
			Component   *string `json:"component"`
			Issue       *string `json:"issue"`
			Status      *string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		updateFields := map[string]any{}
		if payload.PageSection != nil {
			value := strings.TrimSpace(*payload.PageSection)
			if value == "" {
				http.Error(w, "pageSection cannot be empty", http.StatusBadRequest)
				return
			}
			updateFields["page_section"] = value
		}
		if payload.Component != nil {
			value := strings.TrimSpace(*payload.Component)
			if value == "" {
				http.Error(w, "component cannot be empty", http.StatusBadRequest)
				return
			}
			updateFields["component"] = value
		}
		if payload.Issue != nil {
			value := strings.TrimSpace(*payload.Issue)
			if value == "" {
				http.Error(w, "issue cannot be empty", http.StatusBadRequest)
				return
			}
			updateFields["issue"] = value
		}
		if payload.Status != nil {
			value := strings.TrimSpace(*payload.Status)
			if !validStatus(value) {
				http.Error(w, "invalid status", http.StatusBadRequest)
				return
			}
			updateFields["status"] = value
		}
		if len(updateFields) == 0 {
			http.Error(w, "no fields to update", http.StatusBadRequest)
			return
		}
		var task Task
		setParts := []string{}
		args := []any{}
		i := 1
		for _, key := range []string{"page_section", "component", "issue", "status"} {
			if value, ok := updateFields[key]; ok {
				setParts = append(setParts, key+" = $"+strconv.Itoa(i))
				args = append(args, value)
				i++
			}
		}
		args = append(args, id)
		query := `
UPDATE tasks
SET ` + strings.Join(setParts, ", ") + `,
    updated_at = now()
WHERE id = $` + strconv.Itoa(i) + `
RETURNING id, no, page_section, component, issue, status, updated_at;`
		if err := a.db.QueryRowContext(r.Context(), query, args...).Scan(&task.ID, &task.No, &task.PageSection, &task.Component, &task.Issue, &task.Status, &task.UpdatedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, task)
	case http.MethodDelete:
		res, err := a.db.ExecContext(r.Context(), `DELETE FROM tasks WHERE id = $1`, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		affected, err := res.RowsAffected()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if affected == 0 {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func validStatus(status string) bool {
	switch status {
	case "todo", "in_progress", "blocked", "done":
		return true
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
