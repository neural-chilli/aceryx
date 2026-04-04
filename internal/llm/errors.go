package llm

import (
	"errors"
	"fmt"
)

var (
	ErrNotSupported        = errors.New("not supported")
	ErrRateLimited         = errors.New("rate limited")
	ErrProviderUnavailable = errors.New("provider unavailable")
)

type HTTPStatusError struct {
	StatusCode int
	Body       string
}

func (e *HTTPStatusError) Error() string {
	if e == nil {
		return "http status error"
	}
	if e.Body == "" {
		return fmt.Sprintf("provider http status %d", e.StatusCode)
	}
	return fmt.Sprintf("provider http status %d: %s", e.StatusCode, e.Body)
}

func IsRetryableProviderError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrRateLimited) || errors.Is(err, ErrProviderUnavailable) {
		return true
	}
	var hs *HTTPStatusError
	if errors.As(err, &hs) {
		if hs.StatusCode == 429 || hs.StatusCode >= 500 {
			return true
		}
		return false
	}
	return false
}

func IsClientProviderError(err error) bool {
	if err == nil {
		return false
	}
	var hs *HTTPStatusError
	if errors.As(err, &hs) {
		return hs.StatusCode >= 400 && hs.StatusCode < 500 && hs.StatusCode != 429
	}
	return false
}
