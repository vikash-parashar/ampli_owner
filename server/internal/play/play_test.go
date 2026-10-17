package play

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestParseUTF16CSV(t *testing.T) {
	text := "Date,Package Name,Daily User Installs,Daily User Uninstalls,Active Device Installs\n2026-10-01,com.vikash.ampli,120,7,3400\n"
	u := utf16.Encode([]rune(text))
	b := []byte{0xFF, 0xFE}
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	rows, err := parseCSV(b)
	if err != nil || len(rows) != 1 || firstInt(rows[0], "Daily User Installs") != 120 || rows[0]["Date"] != "2026-10-01" {
		t.Fatalf("rows = %v, %v", rows, err)
	}
}

func TestEarningsNet(t *testing.T) {
	// Indian buyer, no Tax row: GST (18%) comes out of the gross, as the developer pays it.
	in := EarningOrder{Gross: 199, Fee: -25.31, Country: "IN", HasCharge: true}
	if got := in.NetPaise(); got != 14333 {
		t.Fatalf("IN net = %d, want 14333", got)
	}
	// Google withheld the tax itself: rows add up as they are.
	us := EarningOrder{Gross: 250, Fee: -37.5, Tax: -20, Country: "US", HasCharge: true, HasTax: true}
	if got := us.NetPaise(); got != 19250 {
		t.Fatalf("US net = %d", got)
	}
	// Indian buyer with TDS (194-O) withheld: TDS is a credit, GST still comes out once.
	tds := EarningOrder{Gross: 199, Fee: -25.31, TaxCredit: -1.99, Country: "IN", HasCharge: true}
	if got := tds.NetPaise(); got != 14333 {
		t.Fatalf("IN with TDS net = %d, want 14333", got)
	}
	// Indian buyer where Google did withhold the GST: the Tax row is subtracted as it is, not twice.
	held := EarningOrder{Gross: 199, Fee: -25.31, Tax: -30.36, Country: "IN", HasCharge: true, HasTax: true}
	if got := held.NetPaise(); got != 14333 {
		t.Fatalf("IN with GST withheld net = %d, want 14333", got)
	}
	// Indian buyer with some other small Tax row (not the GST share): GST still comes out, the row is ignored.
	other := EarningOrder{Gross: 199, Fee: -25.31, Tax: -4.56, Country: "IN", HasCharge: true, HasTax: true}
	if got := other.NetPaise(); got != 14333 {
		t.Fatalf("IN with other tax row net = %d, want 14333", got)
	}
	if !isTaxCredit("tax", "TDS u/s 194-O") || !isTaxCredit("tax on google fee", "") || isTaxCredit("tax", "GST") {
		t.Fatal("isTaxCredit")
	}
	// Charged in one month, half refunded the next: the merge keeps charge minus refund.
	m := MergeEarnings(map[string]EarningOrder{"GPA.1": {Gross: 199, Fee: -25.31, Country: "IN", HasCharge: true}},
		map[string]EarningOrder{"GPA.1": {Gross: -99.5, Fee: 12.66, Country: "IN"}})
	if e := m["GPA.1"]; !e.HasCharge || e.NetPaise() != 7167 {
		t.Fatalf("merged = %+v net %d", e, e.NetPaise())
	}
}

type failTransport struct{}

func (failTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("connection reset by peer")
}

// A transport error must not carry the request URL: purchase URLs hold the buyer's purchase token.
func TestTransportErrorHidesToken(t *testing.T) {
	c := &Client{h: &http.Client{Transport: failTransport{}}}
	err := c.do(context.Background(), "GET", "https://androidpublisher.googleapis.com/x/purchases/products/ampli_pro/tokens/SECRET-TOKEN-123", "t", nil, nil)
	if err == nil || strings.Contains(err.Error(), "SECRET-TOKEN-123") || !strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("err = %v", err)
	}
}
