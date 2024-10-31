package subdomain

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

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
		log.Println("running subdomain middleware...")
		log.Println("received request for: ", r.URL)
		filepath, err := getblogfilepath(r, uwm.store)
		if err != nil {
			log.Println("no subdomain found: ", err)
			next.ServeHTTP(w, r)
			return
		}
		log.Println("filepath", filepath)
		http.ServeFile(w, r, filepath)
	})
}

func getblogfilepath(r *http.Request, s *model.Store) (string, error) {
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

	log.Println("subdomain", subdomain)
	log.Println("url", url)

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
	log.Printf("X-Forwarded-Host: %s\n", host)
	if host == "" {
		return r.Host // Fallback to the Host header
	}
	return host
}
