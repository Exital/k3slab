package progress

import (
	"strconv"
	"strings"
)

const markerPrefix = "::k3slab-progress::"

// ParseLine extracts percent and message from a progress marker line.
// Accepts an optional "stderr: " prefix from engine drainPipe.
// Returns ok=false if the line is not a progress marker.
func ParseLine(line string) (pct int, message string, ok bool) {
	s := strings.TrimSpace(line)
	s = strings.TrimPrefix(s, "stderr: ")
	if !strings.HasPrefix(s, markerPrefix) {
		return 0, "", false
	}
	rest := strings.TrimPrefix(s, markerPrefix)
	pctStr, msg, found := strings.Cut(rest, "::")
	if !found {
		return 0, "", false
	}
	n, err := strconv.Atoi(pctStr)
	if err != nil {
		return 0, "", false
	}
	return n, msg, true
}
