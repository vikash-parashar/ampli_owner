// Package secret encrypts ad-platform tokens at rest and signs OAuth state values.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Box holds two subkeys derived from TOKEN_KEY: one for encryption, one for signing OAuth state.
type Box struct{ key, macKey []byte }

// New decodes a base64 32-byte key (generate one with `openssl rand -base64 32`).
func New(b64 string) (*Box, error) {
	k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil || len(k) != 32 {
		return nil, errors.New("TOKEN_KEY must be 32 random bytes, base64 encoded")
	}
	enc, err := hkdf.Key(sha256.New, k, nil, "ampli-owner token encryption", 32)
	if err != nil {
		return nil, err
	}
	mac, err := hkdf.Key(sha256.New, k, nil, "ampli-owner oauth state", 32)
	if err != nil {
		return nil, err
	}
	return &Box{key: enc, macKey: mac}, nil
}

func (b *Box) aead() cipher.AEAD {
	blk, _ := aes.NewCipher(b.key) // key length checked in New
	g, _ := cipher.NewGCM(blk)
	return g
}

// Seal encrypts plain; the nonce is prepended.
func (b *Box) Seal(plain string) []byte {
	g := b.aead()
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic(err)
	}
	return g.Seal(nonce, nonce, []byte(plain), nil)
}

func (b *Box) Open(sealed []byte) (string, error) {
	g := b.aead()
	if len(sealed) < g.NonceSize() {
		return "", errors.New("sealed value too short")
	}
	out, err := g.Open(nil, sealed[:g.NonceSize()], sealed[g.NonceSize():], nil)
	return string(out), err
}

func (b *Box) mac(msg string) string {
	m := hmac.New(sha256.New, b.macKey)
	m.Write([]byte("state:" + msg))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// State returns a signed OAuth state that names the platform and expires after ttl.
func (b *Box) State(platform string, now time.Time, ttl time.Duration) string {
	msg := platform + "." + strconv.FormatInt(now.Add(ttl).Unix(), 10)
	return msg + "." + b.mac(msg)
}

// CheckState verifies a value from State and returns its platform.
func (b *Box) CheckState(state string, now time.Time) (string, error) {
	parts := strings.Split(state, ".")
	if len(parts) != 3 {
		return "", errors.New("bad state")
	}
	msg := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(b.mac(msg)), []byte(parts[2])) {
		return "", errors.New("bad state signature")
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || now.Unix() > exp {
		return "", fmt.Errorf("state expired")
	}
	return parts[0], nil
}

// HashKey returns a short keyed hash (for IP prefixes and purchase tokens) so raw values are never stored.
func (b *Box) HashKey(purpose, v string) string {
	m := hmac.New(sha256.New, b.macKey)
	m.Write([]byte(purpose + ":" + v))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil)[:18])
}
