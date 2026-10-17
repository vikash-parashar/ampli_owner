package secret

import (
	"encoding/base64"
	"testing"
	"time"
)

func TestSealAndState(t *testing.T) {
	b, err := New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.Open(b.Seal("hello"))
	if err != nil || got != "hello" {
		t.Fatalf("open = %q, %v", got, err)
	}
	now := time.Unix(1_800_000_000, 0)
	st := b.State("meta", now, time.Minute)
	if p, err := b.CheckState(st, now); err != nil || p != "meta" {
		t.Fatalf("check = %q, %v", p, err)
	}
	if _, err := b.CheckState(st, now.Add(2*time.Minute)); err == nil {
		t.Fatal("expired state accepted")
	}
	if _, err := b.CheckState("tiktok"+st[4:], now); err == nil {
		t.Fatal("tampered state accepted")
	}
	if _, err := New("short"); err == nil {
		t.Fatal("bad key accepted")
	}
}
