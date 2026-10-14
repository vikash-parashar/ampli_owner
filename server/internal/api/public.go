package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/vikash-parashar/ampli_owner/server/internal/store"
)

// Report is what the product apps send; see in-app-hook-spec.md.
type Report struct {
	Product    string `json:"product"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	Contact    string `json:"contact"`
	InstallID  string `json:"install_id"`
	AppVersion string `json:"app_version"`
	Device     string `json:"device"`
	OS         string `json:"os"`
	Locale     string `json:"locale"`
	Pro        bool   `json:"pro"`
	OrderID    string `json:"order_id"`
}

func (rp Report) validate() string {
	switch {
	case productID(rp.Product) == "":
		return "product must be mp3 or carhub"
	case !oneOf(rp.Kind, "bug", "feature", "support", "feedback"):
		return "kind must be bug, feature, support or feedback"
	case !lenBetween(rp.Title, 3, 120):
		return "title must be 3 to 120 characters"
	case !lenBetween(rp.Body, 0, 4000):
		return "body must be at most 4000 characters"
	case !lenBetween(rp.InstallID, 8, 64):
		return "install_id is required"
	case len(rp.Contact) > 200, len(rp.AppVersion) > 64, len(rp.Device) > 120, len(rp.OS) > 64, len(rp.Locale) > 16, len(rp.OrderID) > 64:
		return "a field is too long"
	}
	return ""
}

func (s *Server) createReport(w http.ResponseWriter, r *http.Request) {
	var rp Report
	if !decode(w, r, &rp) {
		return
	}
	if msg := rp.validate(); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	n, err := s.Store.ReportsSince(r.Context(), rp.InstallID, s.Now().Add(-24*time.Hour))
	if err != nil {
		fail(w, r, err)
		return
	}
	if n >= ReportsPerDay {
		writeErr(w, http.StatusTooManyRequests, "daily report limit reached, try again tomorrow")
		return
	}
	priority := int16(2)
	if rp.Kind == "support" && rp.Pro {
		priority = 1 // paying users asking for help go to the top of the inbox
	}
	t, err := s.Store.CreateTicket(r.Context(), store.Ticket{
		ProductID: rp.Product, Kind: rp.Kind, Title: strings.TrimSpace(rp.Title), Body: strings.TrimSpace(rp.Body),
		Status: "new", Priority: priority, Source: "app", InstallID: rp.InstallID, ReporterContact: rp.Contact,
		AppVersion: rp.AppVersion, Device: rp.Device, OSVersion: rp.OS, Locale: rp.Locale, IsPro: rp.Pro, OrderID: rp.OrderID,
	})
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": t.ID, "ref": t.Ref})
}

type publicTicket struct {
	Ref       string          `json:"ref"`
	Kind      string          `json:"kind"`
	Title     string          `json:"title"`
	Status    string          `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
	Replies   []store.Message `json:"replies"`
}

// myReports lists one install's own tickets with the conversation, so the app can show "My requests".
func (s *Server) myReports(w http.ResponseWriter, r *http.Request) {
	install := r.URL.Query().Get("install_id")
	if !lenBetween(install, 8, 64) {
		writeErr(w, http.StatusBadRequest, "install_id is required")
		return
	}
	ts, err := s.Store.ListTickets(r.Context(), store.TicketFilter{InstallID: install, Limit: 50})
	if err != nil {
		fail(w, r, err)
		return
	}
	out := make([]publicTicket, 0, len(ts))
	for _, t := range ts {
		msgs, err := s.Store.Messages(r.Context(), t.ID)
		if err != nil {
			fail(w, r, err)
			return
		}
		out = append(out, publicTicket{Ref: t.Ref, Kind: t.Kind, Title: t.Title, Status: t.Status, CreatedAt: t.CreatedAt, Replies: msgs})
	}
	writeJSON(w, http.StatusOK, out)
}

type publicFeature struct {
	ID     int64  `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Votes  int    `json:"votes"`
}

func (s *Server) publicFeatures(w http.ResponseWriter, r *http.Request) {
	p := productID(r.URL.Query().Get("product"))
	if p == "" {
		writeErr(w, http.StatusBadRequest, "product must be mp3 or carhub")
		return
	}
	ts, err := s.Store.ListTickets(r.Context(), store.TicketFilter{Product: p, Kind: "feature", Limit: 50})
	if err != nil {
		fail(w, r, err)
		return
	}
	out := []publicFeature{}
	for _, t := range ts {
		if t.Status == "closed" {
			continue
		}
		out = append(out, publicFeature{ID: t.ID, Title: t.Title, Status: t.Status, Votes: t.Votes})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) vote(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		InstallID string `json:"install_id"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !lenBetween(body.InstallID, 8, 64) {
		writeErr(w, http.StatusBadRequest, "install_id is required")
		return
	}
	votes, err := s.Store.Vote(r.Context(), id, body.InstallID)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"votes": votes})
}
