package agentproto

import (
	"strconv"
	"time"
)

// timeStamp is a time.Time wrapper that always marshals as RFC3339Nano
// regardless of host locale. The TS SDK consumes it via new Date(...) so
// the canonical form must include nanosecond precision and a Z offset.
type timeStamp time.Time

func (t timeStamp) MarshalJSON() ([]byte, error) {
	formatted := time.Time(t).UTC().Format(time.RFC3339Nano)
	buf := make([]byte, 0, len(formatted)+2)
	buf = append(buf, '"')
	buf = append(buf, formatted...)
	buf = append(buf, '"')
	return buf, nil
}

func (t *timeStamp) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return &timeError{raw: string(data)}
	}
	unquoted, err := strconv.Unquote(string(data))
	if err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339Nano, unquoted)
	if err != nil {
		return err
	}
	*t = timeStamp(parsed)
	return nil
}

func (t timeStamp) Time() time.Time { return time.Time(t) }

type timeError struct {
	raw string
}

func (e *timeError) Error() string { return "agentproto: empty timestamp payload: " + e.raw }
