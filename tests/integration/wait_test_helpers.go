package integration

import (
	"os"
	"strconv"
	"testing"
	"time"
)

func adjustedWaitTimeout(base time.Duration) time.Duration {
	raw := os.Getenv("ACERYX_TEST_WAIT_TIMEOUT_MS")
	if raw == "" {
		return base
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return base
	}
	return time.Duration(ms) * time.Millisecond
}

func adjustedWaitInterval(base time.Duration) time.Duration {
	raw := os.Getenv("ACERYX_TEST_WAIT_INTERVAL_MS")
	if raw == "" {
		return base
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return base
	}
	return time.Duration(ms) * time.Millisecond
}

func waitForCondition(t *testing.T, timeout, interval time.Duration, condition func() bool, failure string) {
	t.Helper()
	deadline := time.Now().Add(adjustedWaitTimeout(timeout))
	pollEvery := adjustedWaitInterval(interval)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(pollEvery)
	}
	t.Fatal(failure)
}

func ensureConditionNever(t *testing.T, duration, interval time.Duration, condition func() bool, failure string) {
	t.Helper()
	deadline := time.Now().Add(adjustedWaitTimeout(duration))
	pollEvery := adjustedWaitInterval(interval)
	for time.Now().Before(deadline) {
		if condition() {
			t.Fatal(failure)
		}
		time.Sleep(pollEvery)
	}
}
