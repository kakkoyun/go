// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package trace

import (
	"context"
	"strings"
	"sync/atomic"
	_ "unsafe"
)

// SQLErrorCode represents the type of SQL error that occurred.
type SQLErrorCode uint8

const (
	SQLErrorNone       SQLErrorCode = iota
	SQLErrorTimeout                 // Query timed out
	SQLErrorCanceled                // Query was canceled
	SQLErrorConnection              // Connection error
	SQLErrorSyntax                  // SQL syntax error
	SQLErrorConstraint              // Constraint violation
	SQLErrorOther                   // Other error
)

// String returns a human-readable representation of the error code.
func (c SQLErrorCode) String() string {
	switch c {
	case SQLErrorNone:
		return "none"
	case SQLErrorTimeout:
		return "timeout"
	case SQLErrorCanceled:
		return "canceled"
	case SQLErrorConnection:
		return "connection"
	case SQLErrorSyntax:
		return "syntax"
	case SQLErrorConstraint:
		return "constraint"
	case SQLErrorOther:
		return "other"
	default:
		return "unknown"
	}
}

// SQLSpan represents an in-progress SQL query span.
// Use End to mark the completion of the span.
type SQLSpan struct {
	spanID    uint64
	isNoop    bool
	errorCode SQLErrorCode
}

var noopSQLSpan = &SQLSpan{isNoop: true}

// SQL query sanitization configuration
var (
	sqlSanitizer atomic.Pointer[func(string) string]
)

// SetError sets the error code for a failed query.
// This should be called before End() when the query fails.
func (s *SQLSpan) SetError(code SQLErrorCode) {
	if !s.isNoop {
		s.errorCode = code
	}
}

// End completes the SQL span with the number of rows affected.
// Use rowsAffected=-1 if the count is unknown or not applicable.
func (s *SQLSpan) End(rowsAffected int64) {
	if s.isNoop {
		return
	}
	sqlQueryEnd(s.spanID, rowsAffected, s.errorCode)
}

// SQLQuery traces a database query execution.
// It emits a SQLQueryStart event with the given trace context and query.
//
// The query string is automatically sanitized to remove literal values
// by default. Use SetSQLSanitizer to customize sanitization.
//
// Returns a SQLSpan that must be ended with End(rowsAffected) when the
// query completes.
//
//	span := trace.SQLQuery(ctx, tc, "SELECT * FROM users WHERE id = ?")
//	rows, err := db.QueryContext(ctx, query, id)
//	if err != nil {
//	    span.SetError(trace.SQLErrorOther)
//	}
//	defer span.End(rowsAffected)
func SQLQuery(ctx context.Context, tc TraceContext, query string) *SQLSpan {
	if !IsEnabled() {
		return noopSQLSpan
	}

	// Apply sampling
	if !shouldSample() {
		return noopSQLSpan
	}

	// If no trace context provided, generate new one
	if tc.TraceIDHi == 0 && tc.TraceIDLo == 0 {
		tc = NewTraceContext()
	}

	// If no span ID, generate one
	if tc.SpanID == 0 {
		tc.SpanID = NewSpanID()
	}

	// Sanitize the query
	query = SanitizeSQL(query)

	// Derive 64-bit trace ID from 128-bit ID (XOR high and low parts)
	traceID := tc.TraceIDHi ^ tc.TraceIDLo
	sqlQueryStart(traceID, tc.SpanID, query)
	return &SQLSpan{spanID: tc.SpanID}
}

// SanitizeSQL removes sensitive information from SQL queries.
// By default, it replaces string literals and numeric literals with placeholders.
//
// Custom sanitization can be configured using SetSQLSanitizer.
//
// Example:
//
//	SanitizeSQL("SELECT * FROM users WHERE name = 'John' AND age = 30")
//	// Returns: "SELECT * FROM users WHERE name = ? AND age = ?"
func SanitizeSQL(query string) string {
	if fn := sqlSanitizer.Load(); fn != nil {
		return (*fn)(query)
	}
	return defaultSQLSanitizer(query)
}

// defaultSQLSanitizer replaces string and numeric literals with placeholders.
func defaultSQLSanitizer(query string) string {
	var result strings.Builder
	result.Grow(len(query))

	i := 0
	for i < len(query) {
		c := query[i]

		// Handle string literals (single quotes)
		if c == '\'' {
			result.WriteByte('?')
			i++
			// Skip until closing quote, handling escaped quotes
			for i < len(query) {
				if query[i] == '\'' {
					i++
					// Check for escaped quote ('')
					if i < len(query) && query[i] == '\'' {
						i++
						continue
					}
					break
				}
				i++
			}
			continue
		}

		// Handle double-quoted strings (some SQL dialects)
		if c == '"' {
			result.WriteByte('?')
			i++
			for i < len(query) {
				if query[i] == '"' {
					i++
					if i < len(query) && query[i] == '"' {
						i++
						continue
					}
					break
				}
				i++
			}
			continue
		}

		// Handle numeric literals
		if c >= '0' && c <= '9' {
			// Check if this is part of an identifier (preceded by letter/underscore)
			if i > 0 {
				prev := query[i-1]
				if (prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z') || prev == '_' {
					result.WriteByte(c)
					i++
					continue
				}
			}
			result.WriteByte('?')
			// Skip the rest of the number
			for i < len(query) {
				nc := query[i]
				if (nc >= '0' && nc <= '9') || nc == '.' || nc == 'e' || nc == 'E' || nc == '+' || nc == '-' {
					// Handle scientific notation carefully
					if (nc == '+' || nc == '-') && i > 0 && query[i-1] != 'e' && query[i-1] != 'E' {
						break
					}
					i++
				} else {
					break
				}
			}
			continue
		}

		result.WriteByte(c)
		i++
	}

	return result.String()
}

// SetSQLSanitizer configures a custom SQL sanitization function.
// The function receives the raw SQL query and should return the sanitized version.
//
// Set to nil to restore the default sanitizer (which replaces literals with ?).
//
// Example - disable sanitization:
//
//	trace.SetSQLSanitizer(func(query string) string {
//	    return query // No sanitization
//	})
func SetSQLSanitizer(fn func(string) string) {
	if fn == nil {
		sqlSanitizer.Store(nil)
	} else {
		sqlSanitizer.Store(&fn)
	}
}

//
// Function declarations - implementations provided by runtime via linkname
//

// emits SQLQueryStart event.
func sqlQueryStart(traceID, spanID uint64, query string)

// emits SQLQueryEnd event.
func sqlQueryEnd(spanID uint64, rowsAffected int64, errorCode SQLErrorCode)
