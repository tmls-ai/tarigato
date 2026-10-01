package session

// Valid reports whether a session can be used at the given Unix timestamp.
func Valid(expiresAt, now int64) bool {
	return expiresAt >= now
}
