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
