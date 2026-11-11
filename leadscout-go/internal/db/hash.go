package db

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashKey returns a short stable hash for a key string (used by merge tests).
func HashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:32]
}
