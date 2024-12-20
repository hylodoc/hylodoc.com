package certman

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
	"golang.org/x/crypto/acme/autocert"
)

type CertManager struct {
	s        *model.Store
	dircache autocert.DirCache
}

func NewCertManager(s *model.Store) *CertManager {
	return &CertManager{
		s, autocert.DirCache(config.Config.Progstack.CertsPath),
	}
}

func (m *CertManager) CheckDomain(ctx context.Context, host string) error {
	if host == config.Config.Progstack.ServiceName {
		return nil
	}

	/* check for subdomain first because it's the more common case */
	err := checkSubdomain(ctx, host, m.s)
	if err == nil {
		return nil
	}
	if !errors.Is(err, errNoSubdomainFound) {
		return fmt.Errorf("subdomain exists error: %w", err)
	}
	assert.Assert(errors.Is(err, errNoSubdomainFound))

	domainexists, err := m.s.DomainExists(ctx, host)
	if err != nil {
		return fmt.Errorf("domain exists error: %w", err)
	}
	if domainexists {
		return nil
	}

	return fmt.Errorf("no such domain or subdomain")
}

var errNoSubdomainFound = errors.New("no such subdomain")

func checkSubdomain(ctx context.Context, host string, s *model.Store) error {
	/* `.hylodoc.com' (dot followed by service name) must follow host */
	subdomain, found := strings.CutSuffix(
		host,
		fmt.Sprintf(".%s", config.Config.Progstack.ServiceName),
	)
	if !found {
		return fmt.Errorf(
			"service name not found: %w", errNoSubdomainFound,
		)
	}
	exists, err := s.SubdomainExists(ctx, subdomain)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	if !exists {
		return fmt.Errorf("not in db: %w", errNoSubdomainFound)
	}
	return nil
}

func (m *CertManager) Get(ctx context.Context, key string) ([]byte, error) {
	if err := m.CheckDomain(ctx, key); err != nil {
		return nil, autocert.ErrCacheMiss
	}
	return m.dircache.Get(ctx, key)
}

func (m *CertManager) Put(ctx context.Context, key string, data []byte) error {
	return m.dircache.Put(ctx, key, data)
}

func (m *CertManager) Delete(ctx context.Context, key string) error {
	return m.dircache.Delete(ctx, key)
}
