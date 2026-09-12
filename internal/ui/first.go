package ui

import (
	"encoding/json"
	"time"
)

// firstEntryInfo reads the timestamp and cwd of a transcript line; both are
// optional and zero when absent.
func firstEntryInfo(line string) (time.Time, string) {
	var e struct {
		Timestamp time.Time `json:"timestamp"`
		Cwd       string    `json:"cwd"`
	}
	_ = json.Unmarshal([]byte(line), &e)
	return e.Timestamp, e.Cwd
}
