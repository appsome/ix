// Package util holds small, dependency-free helpers shared across ix-generated
// projects: environment lookups with fallbacks and database/sql null-type
// coercion. The logic is imported, not vendored.
package util

import "os"

// GetEnv returns the value of the environment variable named key, or fallback
// when the variable is unset.
func GetEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// GetEnvBool returns the boolean value of the environment variable named key,
// or fallback when unset or unparseable.
func GetEnvBool(key string, fallback bool) bool {
	value, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}
	return ParseBool(value, fallback)
}

// GetEnvInt32 returns the int32 value of the environment variable named key, or
// fallback when unset or unparseable.
func GetEnvInt32(key string, fallback int32) int32 {
	value, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}
	return ParseInt32(value, fallback)
}
