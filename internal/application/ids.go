package application

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"
)

type IDGenerator struct{ counter atomic.Uint64 }

func (g *IDGenerator) New(prefix string, now time.Time) string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		n := g.counter.Add(1)
		return fmt.Sprintf("%s_%x_%x", prefix, now.UnixNano(), n)
	}
	return prefix + "_" + hex.EncodeToString(b)
}

func (g *IDGenerator) CredentialNumber(now time.Time) string {
	return fmt.Sprintf("SRC-%s-%06d", now.UTC().Format("20060102"), g.counter.Add(1)%1000000)
}
