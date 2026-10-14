package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/vikash-parashar/ampli-owner/server/internal/store"
)

var ticketStatuses = []string{"new", "waiting_on_owner", "waiting_on_user", "planned", "in_progress", "fixed", "closed"}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	o, err := s.Store.Overview(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

// ---- tickets ----

func (s *Server) listTickets(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	ts, err := s.Store.ListTickets(r.Context(), store.TicketFilter{Product: q.Get("product"), Kind: q.Get("kind"), Status: q.Get("status"), Limit: limit})
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ts)
}

// createTicket is for tickets the owner logs by hand (an email, a Play review, an Instagram DM).
func (s *Server) createTicket(w http.ResponseWriter, r *http.Request) {
	var t store.Ticket
	if !decode(w, r, &t) {
		return
	}
	if productID(t.ProductID) == "" || !oneOf(t.Kind, "bug", "feature", "support", "review", "feedback") || !lenBetween(t.Title, 1, 200) {
		writeErr(w, http.StatusBadRequest, "product, kind and title are required")
		return
	}
	if t.Source == "" {
		t.Source = "manual"
	}
	if !oneOf(t.Source, "app", "play", "email", "manual") {
		writeErr(w, http.StatusBadRequest, "source must be app, play, email or manual")
		return
	}
	if t.Status == "" {
		t.Status = "new"
	}
	if !oneOf(t.Status, ticketStatuses...) {
		writeErr(w, http.StatusBadRequest, "unknown status")
		return
	}
	if t.Priority == 0 {
		t.Priority = 2
	}
	t, err := s.Store.CreateTicket(r.Context(), t)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) getTicket(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := s.Store.GetTicket(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return
	}
	msgs, err := s.Store.Messages(r.Context(), id)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": t, "messages": msgs})
}

func (s *Server) patchTicket(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var p store.TicketPatch
	if !decode(w, r, &p) {
		return
	}
	if p.Status != nil && !oneOf(*p.Status, ticketStatuses...) {
		writeErr(w, http.StatusBadRequest, "unknown status")
		return
	}
	if p.Priority != nil && (*p.Priority < 1 || *p.Priority > 3) {
		writeErr(w, http.StatusBadRequest, "priority must be 1, 2 or 3")
		return
	}
	t, err := s.Store.PatchTicket(r.Context(), id, p)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) ownerReply(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Body string `json:"body"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !lenBetween(body.Body, 1, 4000) {
		writeErr(w, http.StatusBadRequest, "reply must be 1 to 4000 characters")
		return
	}
	m, err := s.Store.AddMessage(r.Context(), id, "owner", strings.TrimSpace(body.Body))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// ---- checklist ----

func (s *Server) checklist(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.Checklist(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// importChecklist upserts the whole list; the body is the checklist.json array.
func (s *Server) importChecklist(w http.ResponseWriter, r *http.Request) {
	var items []store.ChecklistItem
	if !decode(w, r, &items) {
		return
	}
	for _, it := range items {
		if it.ID == "" || it.Title == "" || it.Phase == "" {
			writeErr(w, http.StatusBadRequest, "every item needs id, phase and title")
			return
		}
		if it.Status != "" && !oneOf(it.Status, "todo", "in-progress", "done") {
			writeErr(w, http.StatusBadRequest, "status must be todo, in-progress or done")
			return
		}
	}
	if err := s.Store.UpsertChecklist(r.Context(), items, false); err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"imported": len(items)})
}

func (s *Server) patchChecklist(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Status string `json:"status"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !oneOf(body.Status, "todo", "in-progress", "done") {
		writeErr(w, http.StatusBadRequest, "status must be todo, in-progress or done")
		return
	}
	it, err := s.Store.SetChecklistStatus(r.Context(), r.PathValue("id"), body.Status)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, it)
}

// ---- todos ----

func validDate(p *string) bool {
	if p == nil || *p == "" {
		return true
	}
	_, err := time.Parse(time.DateOnly, *p)
	return err == nil
}

func (s *Server) todos(w http.ResponseWriter, r *http.Request) {
	ts, err := s.Store.Todos(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ts)
}

func (s *Server) createTodo(w http.ResponseWriter, r *http.Request) {
	var t store.Todo
	if !decode(w, r, &t) {
		return
	}
	if !lenBetween(t.Title, 1, 200) || !validDate(t.Due) || (t.ProductID != nil && productID(*t.ProductID) == "") {
		writeErr(w, http.StatusBadRequest, "title is required; due is YYYY-MM-DD; product is mp3 or carhub")
		return
	}
	if t.Due != nil && *t.Due == "" {
		t.Due = nil
	}
	t, err := s.Store.CreateTodo(r.Context(), t)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) patchTodo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var p store.TodoPatch
	if !decode(w, r, &p) {
		return
	}
	if !validDate(p.Due) {
		writeErr(w, http.StatusBadRequest, "due is YYYY-MM-DD")
		return
	}
	t, err := s.Store.PatchTodo(r.Context(), id, p)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) deleteTodo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteTodo(r.Context(), id); err != nil {
		fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- features ----

var featureStates = []string{"built", "left", "now", "next", "later"}

func (s *Server) features(w http.ResponseWriter, r *http.Request) {
	fs, err := s.Store.Features(r.Context(), r.URL.Query().Get("product"))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, fs)
}

func (s *Server) createFeature(w http.ResponseWriter, r *http.Request) {
	var f store.Feature
	if !decode(w, r, &f) {
		return
	}
	if productID(f.ProductID) == "" || !lenBetween(f.Title, 1, 200) || !oneOf(f.State, featureStates...) {
		writeErr(w, http.StatusBadRequest, "product, title and state (built, left, now, next, later) are required")
		return
	}
	f, err := s.Store.CreateFeature(r.Context(), f)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (s *Server) patchFeature(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var p store.FeaturePatch
	if !decode(w, r, &p) {
		return
	}
	if p.State != nil && !oneOf(*p.State, featureStates...) {
		writeErr(w, http.StatusBadRequest, "state must be built, left, now, next or later")
		return
	}
	f, err := s.Store.PatchFeature(r.Context(), id, p)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// ---- campaigns ----

func (s *Server) campaigns(w http.ResponseWriter, r *http.Request) {
	cs, err := s.Store.Campaigns(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *Server) saveCampaign(w http.ResponseWriter, r *http.Request) {
	var c store.Campaign
	if !decode(w, r, &c) {
		return
	}
	c.ID = 0
	if r.PathValue("id") != "" {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		c.ID = id
	}
	if !oneOf(c.Platform, "instagram", "facebook", "youtube", "google", "other") || !lenBetween(c.Name, 1, 200) ||
		(c.Status != "" && !oneOf(c.Status, "draft", "scheduled", "live", "ended")) ||
		(c.ProductID != nil && productID(*c.ProductID) == "") || !validDate(c.StartsOn) || !validDate(c.EndsOn) {
		writeErr(w, http.StatusBadRequest, "platform, name are required; status draft|scheduled|live|ended; dates YYYY-MM-DD")
		return
	}
	code := http.StatusOK
	if c.ID == 0 {
		code = http.StatusCreated
	}
	c, err := s.Store.SaveCampaign(r.Context(), c)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, code, c)
}

// ---- stats ----

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	to := q.Get("to")
	if to == "" {
		to = s.Now().Format(time.DateOnly)
	}
	from := q.Get("from")
	if from == "" {
		from = s.Now().AddDate(0, 0, -29).Format(time.DateOnly)
	}
	if !validDate(&from) || !validDate(&to) {
		writeErr(w, http.StatusBadRequest, "from and to are YYYY-MM-DD")
		return
	}
	st, err := s.Store.Stats(r.Context(), q.Get("product"), from, to)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// putStats upserts day rows: typed by hand now, sent by the Play/AdMob sync job later.
func (s *Server) putStats(w http.ResponseWriter, r *http.Request) {
	var days []store.DayStat
	if !decode(w, r, &days) {
		return
	}
	for _, d := range days {
		if productID(d.ProductID) == "" || d.Day == "" || !validDate(&d.Day) {
			writeErr(w, http.StatusBadRequest, "every row needs product and day (YYYY-MM-DD)")
			return
		}
		if err := s.Store.UpsertStat(r.Context(), d); err != nil {
			fail(w, r, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"saved": len(days)})
}

// ---- guides ----

func (s *Server) guides(w http.ResponseWriter, r *http.Request) {
	gs, err := s.Store.Guides(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, gs)
}

func (s *Server) saveGuide(w http.ResponseWriter, r *http.Request) {
	var g store.Guide
	if !decode(w, r, &g) {
		return
	}
	g.ID = r.PathValue("id")
	if !lenBetween(g.Title, 1, 200) {
		writeErr(w, http.StatusBadRequest, "title is required")
		return
	}
	if err := s.Store.SaveGuide(r.Context(), g); err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}
