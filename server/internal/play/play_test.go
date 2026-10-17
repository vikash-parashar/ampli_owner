package play

import (
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
	// Charged in one month, half refunded the next: the merge keeps charge minus refund.
	m := MergeEarnings(map[string]EarningOrder{"GPA.1": {Gross: 199, Fee: -25.31, Country: "IN", HasCharge: true}},
		map[string]EarningOrder{"GPA.1": {Gross: -99.5, Fee: 12.66, Country: "IN"}})
	if e := m["GPA.1"]; !e.HasCharge || e.NetPaise() != 7167 {
		t.Fatalf("merged = %+v net %d", e, e.NetPaise())
	}
}
