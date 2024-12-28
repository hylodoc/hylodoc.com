package session

import (
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"
)

type anonymousLogger struct {
	*log.Logger
}

func newAnonymousLogger(id string) *anonymousLogger {
	return &anonymousLogger{prefixedLogger(id)}
}

func prefixedLogger(prefix string) *log.Logger {
	return log.New(
		log.Writer(),
		fmt.Sprintf("[%s] ", prefix),
		log.LstdFlags|log.Lmsgprefix,
	)
}

func newAnonymousLoggerFromRequest(r *http.Request) *anonymousLogger {
	logger := newAnonymousLogger(uuid.New().String())
	if id := r.Header.Get("X-Request-ID"); id != "" {
		logger.Printf("X-Request-ID: %s\n", id)
	}
	return logger
}

func (logger *anonymousLogger) toSessionLogger(sessionid string) *log.Logger {
	logger.Printf("Session: %s\n", sessionid)
	return prefixedLogger(sessionid)
}
