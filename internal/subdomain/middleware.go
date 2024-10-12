package subdomain

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
)

type SubdomainMiddleware struct {
	store *model.Store
}

func NewSubdomainMiddleware(s *model.Store) *SubdomainMiddleware {
	return &SubdomainMiddleware{
		store: s,
	}
}

func (uwm *SubdomainMiddleware) RouteToSubdomains(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("running subdomain middleware...")

		log.Println("received request for: ", r.URL)

		/* extract subdomain */
		host := r.Header.Get("X-Forwarded-Host")
		log.Printf("X-Forwarded-Host: %s\n", host)
		if host == "" {
			host = r.Host // Fallback to the Host header
		}
		log.Printf("Host: %s\n", host)

		/* needed for the following splitting to work on localhost */
		host = strings.ReplaceAll(host, "127.0.0.1", "localhost")

		/* XXX: bit dodge but with local development we have subdomains like
		* http://<subdomain>.localhost:7999 whic should also route
		* correctly so we split on both "." and ":" */
		re := regexp.MustCompile(`[.:]`)
		parts := re.Split(host, -1)
		if len(parts) > 2 {
			subdomain := parts[0]
			log.Printf("subdomain: %s\n", subdomain)
			/* path to generated site */
			userSitePath := fmt.Sprintf("%s/%s/", config.Config.Progstack.WebsitesPath, subdomain)
			log.Printf("userSitePath: %s\n", userSitePath)

			/* check if file exists */
			filePath := filepath.Join(userSitePath, r.URL.Path)
			if r.URL.Path == "/" {
				/* no specific file requested */
				filePath = filepath.Join(userSitePath, "index.html")
			}
			log.Printf("filePath: %s\n", filePath)

			http.ServeFile(w, r, filePath)
			return
		}

		/* not a subdomain next middleware */
		next.ServeHTTP(w, r)
	})
}
