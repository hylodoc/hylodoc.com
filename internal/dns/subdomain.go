package dns

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"unicode"
)

type Subdomain struct{ string }

func (s *Subdomain) String() string { return s.string }

func ParseSubdomain(raw string) (*Subdomain, error) {
	trimlw := strings.ToLower(strings.TrimSpace(raw))
	if len(trimlw) < 1 || len(trimlw) > 63 {
		return nil, ParseUserError(
			"must be between 1 and 63 characters long",
		)
	}
	if trimlw[0] == '-' || trimlw[len(trimlw)-1] == '-' {
		return nil, ParseUserError("cannot start or end with a hyphen")
	}
	if strings.Index(trimlw, "--") != -1 {
		return nil, ParseUserError("cannot contain consecutive hyphens")
	}
	for _, r := range trimlw {
		if unicode.IsSpace(r) {
			return nil, ParseUserError("cannot contain spaces")
		}
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-') {
			return nil, ParseUserError(
				"can only contain letters, numbers, and hyphens",
			)
		}
	}
	return &Subdomain{trimlw}, nil
}

type ParseUserError string

func (err ParseUserError) Error() string { return string(err) }

func (s *Subdomain) Scan(src any) error {
	str, ok := src.(string)
	if !ok {
		return fmt.Errorf("need string")
	}
	*s = Subdomain{str}
	return nil
}

func (s *Subdomain) Value() (driver.Value, error) {
	return s.string, nil
}
