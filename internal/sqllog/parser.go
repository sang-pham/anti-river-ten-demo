package sqllog

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Expected line format (single line):
// DB:<name>,sql:<query>,exec_time_ms:<int>,exec_count:<int>
//
// The SQL query may contain commas, so we use a non-greedy match for the query
// and anchor on the explicit exec_time_ms and exec_count fields.
var lineRE = regexp.MustCompile(`^DB:([^,]+),sql:(.*?),exec_time_ms:(\d+),exec_count:(\d+)\s*$`)

// ParseLine parses one log line into a SQLLog (without ID/CreatedAt).
func ParseLine(s string) (SQLLog, error) {
	line := strings.TrimSpace(s)
	if line == "" {
		return SQLLog{}, fmt.Errorf("empty line")
	}
	m := lineRE.FindStringSubmatch(line)
	if len(m) != 5 {
		return SQLLog{}, fmt.Errorf("invalid line format")
	}
	dbName := strings.TrimSpace(m[1])
	sqlQuery := strings.TrimSpace(m[2])

	execTimeMs, err := strconv.ParseInt(m[3], 10, 64)
	if err != nil {
		return SQLLog{}, fmt.Errorf("invalid exec_time_ms: %w", err)
	}
	execCount, err := strconv.ParseInt(m[4], 10, 64)
	if err != nil {
		return SQLLog{}, fmt.Errorf("invalid exec_count: %w", err)
	}
	if dbName == "" || sqlQuery == "" {
		return SQLLog{}, fmt.Errorf("db or sql is empty")
	}
	if execTimeMs < 0 || execCount < 0 {
		return SQLLog{}, fmt.Errorf("negative values not allowed")
	}
	return SQLLog{
		DBName:     dbName,
		SQLQuery:   sqlQuery,
		ExecTimeMs: execTimeMs,
		ExecCount:  execCount,
	}, nil
}

// ParseStream scans an io.Reader line by line and invokes onEntry for valid lines,
// and onError for bad lines; it does not stop on bad lines.
func ParseStream(ctx context.Context, r io.Reader, onEntry func(SQLLog) error, onError func(error)) error {
	sc := bufio.NewScanner(r)
	// Allow long SQL lines (up to 1 MiB)
	const maxLine = 1 << 20
	buf := make([]byte, 64*1024)
	sc.Buffer(buf, maxLine)

	for sc.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		l := sc.Text()
		rec, err := ParseLine(l)
		if err != nil {
			if onError != nil {
				onError(fmt.Errorf("parse: %w; line=%q", err, l))
			}
			continue
		}
		if onEntry != nil {
			if err := onEntry(rec); err != nil && onError != nil {
				onError(fmt.Errorf("store: %w", err))
			}
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	return nil
}
