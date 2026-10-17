// Package play reads and answers Google Play reviews with a service account
// (Play Console > Users and permissions > invite the service account with "Reply to reviews").
package play

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	email    string
	key      *rsa.PrivateKey
	tokenURL string
	Base     string
	Report   string // https://playdeveloperreporting.googleapis.com/v1beta1
	Storage  string // https://storage.googleapis.com
	h        *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
}

// New parses the service account JSON key file contents.
func New(serviceAccountJSON string) (*Client, error) {
	var sa struct {
		Email      string `json:"client_email"`
		PrivateKey string `json:"private_key"`
		TokenURI   string `json:"token_uri"`
	}
	if err := json.Unmarshal([]byte(serviceAccountJSON), &sa); err != nil {
		return nil, fmt.Errorf("PLAY_SERVICE_ACCOUNT_JSON: %w", err)
	}
	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return nil, errors.New("PLAY_SERVICE_ACCOUNT_JSON: no private key")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("PLAY_SERVICE_ACCOUNT_JSON: key is not RSA")
	}
	if sa.TokenURI == "" {
		sa.TokenURI = "https://oauth2.googleapis.com/token"
	}
	return &Client{email: sa.Email, key: rk, tokenURL: sa.TokenURI,
		Base: "https://androidpublisher.googleapis.com/androidpublisher/v3", Report: "https://playdeveloperreporting.googleapis.com/v1beta1",
		Storage: "https://storage.googleapis.com", h: &http.Client{Timeout: 30 * time.Second}}, nil
}

// One token covers reviews and purchases, vitals, and the bulk-report bucket.
const scopes = "https://www.googleapis.com/auth/androidpublisher https://www.googleapis.com/auth/playdeveloperreporting " +
	"https://www.googleapis.com/auth/devstorage.read_only"

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Until(c.expires) > time.Minute {
		return c.token, nil
	}
	now := time.Now()
	head, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{"iss": c.email, "scope": scopes,
		"aud": c.tokenURL, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()})
	unsigned := b64(head) + "." + b64(claims)
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {unsigned + "." + b64(sig)}}
	var tk struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := c.do(ctx, "POST", c.tokenURL, "", form, &tk); err != nil {
		return "", err
	}
	c.token, c.expires = tk.AccessToken, now.Add(time.Duration(tk.ExpiresIn)*time.Second)
	return c.token, nil
}

func (c *Client) do(ctx context.Context, method, u, bearer string, body any, out any) error {
	var rd io.Reader
	ctype := ""
	switch b := body.(type) {
	case nil:
	case url.Values:
		rd, ctype = strings.NewReader(b.Encode()), "application/x-www-form-urlencoded"
	default:
		buf, _ := json.Marshal(b)
		rd, ctype = bytes.NewReader(buf), "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.h.Do(req)
	if err != nil {
		return fmt.Errorf("Google Play: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		return &HTTPError{Status: resp.StatusCode, Msg: e.Error.Message}
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// HTTPError is a non-2xx answer from Google. Status 4xx means the request (e.g. a purchase token) was refused;
// 5xx means try again later.
type HTTPError struct {
	Status int
	Msg    string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("Google Play: %s (HTTP %d)", e.Msg, e.Status) }

// Review is one Play review (Play only returns those from the last week).
type Review struct {
	ID         string
	Author     string
	Rating     int16
	Text       string
	Reply      string
	AppVersion string
	At         time.Time
}

func (c *Client) Reviews(ctx context.Context, pkg string) ([]Review, error) {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	var out []Review
	next := ""
	for page := 0; page < 20; page++ {
		q := url.Values{"maxResults": {"100"}}
		if next != "" {
			q.Set("token", next)
		}
		var res struct {
			Reviews []struct {
				ID       string `json:"reviewId"`
				Author   string `json:"authorName"`
				Comments []struct {
					User *struct {
						Text    string `json:"text"`
						Stars   int16  `json:"starRating"`
						Version string `json:"appVersionName"`
						Mod     struct {
							Seconds string `json:"seconds"`
						} `json:"lastModified"`
					} `json:"userComment"`
					Dev *struct {
						Text string `json:"text"`
					} `json:"developerComment"`
				} `json:"comments"`
			} `json:"reviews"`
			Page struct {
				Next string `json:"nextPageToken"`
			} `json:"tokenPagination"`
		}
		if err := c.do(ctx, "GET", c.Base+"/applications/"+url.PathEscape(pkg)+"/reviews?"+q.Encode(), tok, nil, &res); err != nil {
			return nil, err
		}
		for _, r := range res.Reviews {
			v := Review{ID: r.ID, Author: r.Author}
			for _, cm := range r.Comments {
				if cm.User != nil {
					v.Text, v.Rating, v.AppVersion = strings.TrimSpace(cm.User.Text), cm.User.Stars, cm.User.Version
					if s, err := strconv.ParseInt(cm.User.Mod.Seconds, 10, 64); err == nil {
						v.At = time.Unix(s, 0)
					}
				}
				if cm.Dev != nil {
					v.Reply = cm.Dev.Text
				}
			}
			if v.Rating >= 1 && v.Rating <= 5 {
				out = append(out, v)
			}
		}
		if res.Page.Next == "" {
			break
		}
		next = res.Page.Next
	}
	return out, nil
}

func (c *Client) Reply(ctx context.Context, pkg, reviewID, text string) error {
	tok, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	return c.do(ctx, "POST", c.Base+"/applications/"+url.PathEscape(pkg)+"/reviews/"+url.PathEscape(reviewID)+":reply", tok,
		map[string]string{"replyText": text}, nil)
}
