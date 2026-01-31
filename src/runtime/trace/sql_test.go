// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package trace_test

import (
	"bytes"
	"context"
	"runtime/trace"
	"testing"
)

func TestSQLErrorCodeString(t *testing.T) {
	tests := []struct {
		code     trace.SQLErrorCode
		expected string
	}{
		{trace.SQLErrorNone, "none"},
		{trace.SQLErrorTimeout, "timeout"},
		{trace.SQLErrorCanceled, "canceled"},
		{trace.SQLErrorConnection, "connection"},
		{trace.SQLErrorSyntax, "syntax"},
		{trace.SQLErrorConstraint, "constraint"},
		{trace.SQLErrorOther, "other"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.code.String(); got != tt.expected {
				t.Errorf("SQLErrorCode(%d).String() = %q, want %q", tt.code, got, tt.expected)
			}
		})
	}
}

func TestSQLQuery(t *testing.T) {
	if trace.IsEnabled() {
		t.Skip("skipping because tracing is already enabled")
	}

	ctx := context.Background()
	tc := trace.NewTraceContext()

	// When tracing is disabled, should return noop span
	span := trace.SQLQuery(ctx, tc, "SELECT * FROM users")
	if span == nil {
		t.Fatal("SQLQuery() returned nil")
	}

	// End should not panic
	span.End(10)
}

func TestSQLQueryWithTracing(t *testing.T) {
	trace.SetEventFilter(trace.FilterCore | trace.FilterSQL | trace.FilterCustom)
	defer trace.SetEventFilter(trace.FilterDefault)

	var buf bytes.Buffer
	if err := trace.Start(&buf); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	ctx := context.Background()
	tc := trace.NewTraceContext()

	span := trace.SQLQuery(ctx, tc, "SELECT * FROM users WHERE id = 123")
	span.End(1)

	trace.Stop()

	if buf.Len() == 0 {
		t.Error("trace buffer is empty")
	}

	t.Logf("Trace buffer size: %d bytes", buf.Len())
}

func TestSQLSpanSetters(t *testing.T) {
	if trace.IsEnabled() {
		t.Skip("skipping because tracing is already enabled")
	}

	ctx := context.Background()
	tc := trace.NewTraceContext()

	span := trace.SQLQuery(ctx, tc, "SELECT * FROM users")
	span.SetError(trace.SQLErrorTimeout)
	span.End(-1)
}

func TestSanitizeSQL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no literals",
			input:    "SELECT * FROM users",
			expected: "SELECT * FROM users",
		},
		{
			name:     "string literal",
			input:    "SELECT * FROM users WHERE name = 'John'",
			expected: "SELECT * FROM users WHERE name = ?",
		},
		{
			name:     "numeric literal",
			input:    "SELECT * FROM users WHERE id = 123",
			expected: "SELECT * FROM users WHERE id = ?",
		},
		{
			name:     "multiple literals",
			input:    "SELECT * FROM users WHERE name = 'John' AND age = 30",
			expected: "SELECT * FROM users WHERE name = ? AND age = ?",
		},
		{
			name:     "escaped quotes",
			input:    "SELECT * FROM users WHERE name = 'O''Brien'",
			expected: "SELECT * FROM users WHERE name = ?",
		},
		{
			name:     "double quoted string",
			input:    `SELECT * FROM users WHERE name = "John"`,
			expected: "SELECT * FROM users WHERE name = ?",
		},
		{
			name:     "identifier with number",
			input:    "SELECT * FROM table1 WHERE col2 = 5",
			expected: "SELECT * FROM table1 WHERE col2 = ?",
		},
		{
			name:     "float literal",
			input:    "SELECT * FROM products WHERE price = 19.99",
			expected: "SELECT * FROM products WHERE price = ?",
		},
		{
			name:     "scientific notation",
			input:    "SELECT * FROM data WHERE value = 1.5e10",
			expected: "SELECT * FROM data WHERE value = ?",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trace.SanitizeSQL(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeSQL(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSetSQLSanitizer(t *testing.T) {
	// Set custom sanitizer
	trace.SetSQLSanitizer(func(query string) string {
		return "sanitized"
	})

	got := trace.SanitizeSQL("SELECT * FROM users WHERE name = 'secret'")
	if got != "sanitized" {
		t.Errorf("SanitizeSQL() = %q, want %q", got, "sanitized")
	}

	// Reset to nil (default behavior)
	trace.SetSQLSanitizer(nil)

	got = trace.SanitizeSQL("SELECT * FROM users WHERE name = 'John'")
	if got != "SELECT * FROM users WHERE name = ?" {
		t.Errorf("SanitizeSQL() after reset = %q, want %q", got, "SELECT * FROM users WHERE name = ?")
	}
}
