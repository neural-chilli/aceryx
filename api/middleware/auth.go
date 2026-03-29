package middleware

import "net/http"

// Auth validates the request authentication.
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: implement authentication check
		next.ServeHTTP(w, r)
	})
}
