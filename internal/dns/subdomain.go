package dns

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strings"
	"unicode"
)

type Subdomain interface {
	subdomain() string

	sql.Scanner
	driver.Valuer
	fmt.Stringer
}

type subdomain string

func (s *subdomain) subdomain() string { return string(*s) }
func (s *subdomain) String() string    { return s.subdomain() }

type ParseUserError string

func (err ParseUserError) Error() string { return string(err) }

func ParseSubdomain(raw string) (Subdomain, error) {
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
	sub := subdomain(trimlw)
	return &sub, nil
}

func (s *subdomain) Scan(src any) error {
	str, ok := src.(string)
	if !ok {
		return fmt.Errorf("need string")
	}
	*s = subdomain(str)
	return nil
}

func (s *subdomain) Value() (driver.Value, error) {
	return s.subdomain(), nil
}
