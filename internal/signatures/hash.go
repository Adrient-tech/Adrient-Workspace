package signatures

import (
	"crypto/sha256"
	"encoding/hex"
)

func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
