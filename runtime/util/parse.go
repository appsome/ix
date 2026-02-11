package util

import "strconv"

// ParseBool parses s as a boolean, returning fallback on error.
func ParseBool(s string, fallback bool) bool {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return fallback
	}
	return v
}

// ParseInt32 parses s as a base-10 int32, returning fallback on error.
func ParseInt32(s string, fallback int32) int32 {
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return fallback
	}
	return int32(v)
}
