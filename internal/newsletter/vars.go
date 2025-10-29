package newsletter

import (
	"strings"
	"time"
)

// ExpandVars performs simple placeholder substitutions for template strings
// used in config-provided text fields (e.g., title, preface, postscript).
//
// Supported variables:
// - {.CurrentDate} => formatted as YYYY-MM-DD (UTC)
func ExpandVars(s string, now time.Time) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	date := now.UTC().Format("2006-01-02")
	out := strings.ReplaceAll(s, "{.CurrentDate}", date)
	return out
}
