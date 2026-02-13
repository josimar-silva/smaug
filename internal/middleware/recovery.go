package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

// NewRecoveryMiddleware returns a middleware that recovers from panics, logs the error,
// and returns a 500 Internal Server Error to the client.
func NewRecoveryMiddleware(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					// Log the panic with stack trace
					stack := debug.Stack()
					log.Error("panic recovered",
						"error", err,
						"stack_trace", string(stack),
						"method", r.Method,
						"path", r.URL.Path,
					)

					// Return 500 Internal Server Error
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
