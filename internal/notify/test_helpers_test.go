package notify

import (
	"io"
	"log"
	"time"
)

func testLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

func fixedNow() time.Time {
	return time.Date(2026, time.March, 30, 12, 0, 0, 0, time.UTC)
}
