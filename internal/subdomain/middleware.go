package subdomain

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
)

type SubdomainMiddleware struct {
	store *model.Store
}

func NewSubdomainMiddleware(s *model.Store) *SubdomainMiddleware {
	return &SubdomainMiddleware{store: s}
}

func (uwm *SubdomainMiddleware) RouteToSubdomains(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)

		logger.Println("running subdomain middleware...")
		logger.Println("received request for: ", r.URL)
		if err := rendersubdomainpath(w, r, uwm.store); err != nil {
			logger.Println("no subdomain found: ")
			next.ServeHTTP(w, r)
			return
		}
	})
}

func rendersubdomainpath(
	w http.ResponseWriter, r *http.Request, s *model.Store,
) error {
	filepath, err := getblogfilepath(r, s)
	if err != nil {
		return fmt.Errorf("cannot get filepath: %w", err)
	}
	logging.Logger(r).Println("filepath", filepath)
	http.ServeFile(w, r, filepath)
	return nil
}

func getblogfilepath(r *http.Request, s *model.Store) (string, error) {
	logger := logging.Logger(r)

	/* XXX: bit dodge but with local development we have subdomains like
	* http://<subdomain>.localhost:7999 whic should also route
	* correctly so we split on both "." and ":" */
	re := regexp.MustCompile(`[.:]`)
	parts := re.Split(
		strings.ReplaceAll(
			gethostorxforwardedhost(r), "127.0.0.1", "localhost",
		),
		-1,
	)
	if len(parts) < 1 {
		return "", fmt.Errorf("dodge regex wrong part count")
	}
	subdomain := parts[0]
	url := r.URL.Path

	logger.Println("subdomain", subdomain)
	logger.Println("url", url)

	gen, err := s.GetLastGenerationBySubdomain(context.TODO(), subdomain)
	if err != nil {
		return "", fmt.Errorf("cannot get generation: %w", err)
	}
	path, err := s.GetBinding(
		context.TODO(),
		model.GetBindingParams{Generation: gen, Url: url},
	)
	if err != nil {
		return "", fmt.Errorf("cannot get binding: %w", err)
	}
	return path, nil
}

func gethostorxforwardedhost(r *http.Request) string {
	host := r.Header.Get("X-Forwarded-Host")
	logging.Logger(r).Printf("X-Forwarded-Host: %s\n", host)
	if host == "" {
		return r.Host // Fallback to the Host header
	}
	return host
}
