package httpx

import "net/http"

// Middleware wraps an http.Handler to provide cross-cutting concerns.
type Middleware func(http.Handler) http.Handler

// Chain applies middleware so that the first middleware in the slice is outermost (executes first).
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
