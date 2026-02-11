package util

import (
	"database/sql"
	"time"
)

// DefaultBool coerces a bool, *bool, or sql.NullBool to a plain bool, returning
// fallback for nil pointers and invalid null types.
func DefaultBool(i any, fallback bool) bool {
	switch t := i.(type) {
	case sql.NullBool:
		if t.Valid {
			return t.Bool
		}
	case bool:
		return t
	case *bool:
		if t != nil {
			return *t
		}
	}
	return fallback
}

// DefaultString coerces a string, *string, or sql.NullString to a plain string.
func DefaultString(i any, fallback string) string {
	switch t := i.(type) {
	case sql.NullString:
		if t.Valid {
			return t.String
		}
	case string:
		return t
	case *string:
		if t != nil {
			return *t
		}
	}
	return fallback
}

// DefaultInt coerces an int64, *int64, or sql.NullInt64 to a plain int64.
func DefaultInt(i any, fallback int64) int64 {
	switch t := i.(type) {
	case sql.NullInt64:
		if t.Valid {
			return t.Int64
		}
	case int64:
		return t
	case *int64:
		if t != nil {
			return *t
		}
	}
	return fallback
}

// DefaultTime coerces a time.Time, *time.Time, or sql.NullTime to a plain time.
func DefaultTime(i any, fallback time.Time) time.Time {
	switch t := i.(type) {
	case sql.NullTime:
		if t.Valid {
			return t.Time
		}
	case time.Time:
		return t
	case *time.Time:
		if t != nil {
			return *t
		}
	}
	return fallback
}
