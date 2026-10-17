package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/vikash-parashar/ampli_owner/server/internal/store"
)

func (s *Server) reviews(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("product")
	if p == "all" {
		p = ""
	}
	if p != "" && productID(p) == "" {
		writeErr(w, http.StatusBadRequest, "product must be all, mp3, carhub or naukri")
		return
	}
	ctx := r.Context()
	recent, err := s.Store.Reviews(ctx, p, 500)
	if err != nil {
		fail(w, r, err)
		return
	}
	stars, err := s.Store.Stars(ctx, p)
	if err != nil {
		fail(w, r, err)
		return
	}
	sent := store.Sentiment(stars)
	praise, complaints := store.Themes(recent)
	if len(recent) > 100 {
		recent = recent[:100]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"summary": map[string]any{"total": stars[0] + stars[1] + stars[2] + stars[3] + stars[4], "positive": sent["positive"],
			"mixed": sent["mixed"], "negative": sent["negative"], "stars": stars},
		"themes":  map[string]any{"praise": praise, "complaints": complaints},
		"reviews": recent,
	})
}

func (s *Server) addReview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Product string `json:"product"`
		Author  string `json:"author"`
		Rating  int16  `json:"rating"`
		Body    string `json:"body"`
	}
	if !decode(w, r, &in) {
		return
	}
	if productID(in.Product) == "" || in.Rating < 1 || in.Rating > 5 || !lenBetween(in.Body, 0, 4000) || !lenBetween(in.Author, 0, 120) {
		writeErr(w, http.StatusBadRequest, "product, rating 1-5 and body (up to 4000 characters) are required")
		return
	}
	v, err := s.Store.SaveReview(r.Context(), store.Review{ProductID: in.Product, Source: "manual", Author: in.Author,
		Rating: in.Rating, Body: strings.TrimSpace(in.Body), CreatedAt: s.Now()})
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) replyReview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Text string `json:"text"`
	}
	if !decode(w, r, &in) {
		return
	}
	// Google Play allows replies of up to 350 characters.
	if !lenBetween(in.Text, 1, 350) {
		writeErr(w, http.StatusBadRequest, "reply must be 1 to 350 characters")
		return
	}
	ctx := r.Context()
	v, err := s.Store.Review(ctx, id)
	if err != nil {
		fail(w, r, err)
		return
	}
	if v.Source == "play" && v.ExternalID != nil {
		if s.Play == nil {
			writeErr(w, http.StatusConflict, "set PLAY_SERVICE_ACCOUNT_JSON on the server to reply on Google Play")
			return
		}
		pkg, err := s.packageOf(ctx, v.ProductID)
		if err != nil {
			fail(w, r, err)
			return
		}
		if err := s.Play.Reply(ctx, pkg, *v.ExternalID, strings.TrimSpace(in.Text)); err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
	}
	v, err = s.Store.SetReviewReply(ctx, id, strings.TrimSpace(in.Text))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) packageOf(ctx context.Context, product string) (string, error) {
	prods, err := s.Store.Products(ctx)
	if err != nil {
		return "", err
	}
	for _, p := range prods {
		if p.ID == product {
			return p.PackageID, nil
		}
	}
	return "", store.ErrNotFound
}

// SyncReviews copies the last week of Google Play reviews for every product.
func (s *Server) SyncReviews(ctx context.Context) (int, []string) {
	if s.Play == nil {
		return 0, []string{"PLAY_SERVICE_ACCOUNT_JSON is not set"}
	}
	prods, err := s.Store.Products(ctx)
	if err != nil {
		return 0, []string{err.Error()}
	}
	n, problems := 0, []string{}
	for _, p := range prods {
		rs, err := s.Play.Reviews(ctx, p.PackageID)
		if err != nil {
			problems = append(problems, p.Name+": "+err.Error())
			continue
		}
		for _, pr := range rs {
			ext := pr.ID
			at := pr.At
			if at.IsZero() {
				at = time.Now()
			}
			if _, err := s.Store.SaveReview(ctx, store.Review{ProductID: p.ID, ExternalID: &ext, Source: "play", Author: pr.Author,
				Rating: pr.Rating, Body: pr.Text, Reply: pr.Reply, AppVersion: pr.AppVersion, CreatedAt: at}); err != nil {
				problems = append(problems, err.Error())
				continue
			}
			n++
		}
	}
	return n, problems
}

func (s *Server) syncReviews(w http.ResponseWriter, r *http.Request) {
	n, problems := s.SyncReviews(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"synced": n, "errors": problems})
}
