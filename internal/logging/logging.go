package logging

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"
)

type contextKey string

const (
	requestIDKey = contextKey("requestID")
	loggerKey    = contextKey("logger")
)

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		/* retrieve existing or make new */
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		/* create logger that prepends */
		logger := log.New(
			log.Writer(),
			fmt.Sprintf("[%s] ", requestID),
			log.LstdFlags,
		)

		/* store requestID in context for outgoing calls */
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		/* store logger in cotext */
		ctx = context.WithValue(ctx, loggerKey, logger)
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	})
}

func Logger(r *http.Request) *log.Logger {
	logger, ok := r.Context().Value(loggerKey).(*log.Logger)
	if !ok {
		return log.Default()
	}
	return logger
}
