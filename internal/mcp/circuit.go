package mcp

import (
	"fmt"
	"sync"
	"time"
)

type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

type CircuitBreaker struct {
	mu            sync.Mutex
	state         CircuitState
	failureCount  int
	lastFailure   time.Time
	lastAttempt   time.Time
	coolDown      time.Duration
	failThreshold int
	failWindow    time.Duration
	halfOpenProbe bool
}

func NewCircuitBreaker(coolDown time.Duration, failThreshold int, failWindow time.Duration) *CircuitBreaker {
	if coolDown <= 0 {
		coolDown = 60 * time.Second
	}
	if failThreshold <= 0 {
		failThreshold = 5
	}
	if failWindow <= 0 {
		failWindow = 5 * time.Minute
	}
	return &CircuitBreaker{
		state:         CircuitClosed,
		coolDown:      coolDown,
		failThreshold: failThreshold,
		failWindow:    failWindow,
	}
}

func (cb *CircuitBreaker) Allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	now := time.Now().UTC()
	switch cb.state {
	case CircuitClosed:
		cb.lastAttempt = now
		return nil
	case CircuitOpen:
		if now.Sub(cb.lastFailure) >= cb.coolDown {
			cb.state = CircuitHalfOpen
			cb.halfOpenProbe = false
		} else {
			return fmt.Errorf("circuit breaker open — recent failures")
		}
	}
	if cb.state == CircuitHalfOpen {
		if cb.halfOpenProbe {
			return fmt.Errorf("circuit breaker open — recent failures")
		}
		cb.halfOpenProbe = true
		cb.lastAttempt = now
		return nil
	}
	return nil
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = CircuitClosed
	cb.failureCount = 0
	cb.lastFailure = time.Time{}
	cb.halfOpenProbe = false
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	now := time.Now().UTC()
	if cb.state == CircuitHalfOpen {
		cb.state = CircuitOpen
		cb.lastFailure = now
		cb.halfOpenProbe = false
		return
	}
	if cb.lastFailure.IsZero() || now.Sub(cb.lastFailure) > cb.failWindow {
		cb.failureCount = 0
	}
	cb.failureCount++
	cb.lastFailure = now
	if cb.failureCount >= cb.failThreshold {
		cb.state = CircuitOpen
	}
}

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}
