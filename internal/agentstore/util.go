package agentstore

import (
	"bufio"
	"errors"
	"io"
	"os"
	"time"
)

// countLines returns the number of newline-terminated lines in path. If
// the file does not exist, returns 0 (a journal is allowed to be absent
// during recovery scenarios).
func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()

	count := 0
	r := bufio.NewReader(f)
	for {
		_, err := r.ReadString('\n')
		if err == io.EOF {
			return count, nil
		}
		if err != nil {
			return 0, err
		}
		count++
	}
}

// parseISO parses an RFC3339Nano timestamp, falling back to RFC3339 for
// values written before the nanosecond format was settled.
func parseISO(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}
