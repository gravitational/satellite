package cstrings

import (
	"regexp"
	"strings"
)

// IsValidDomainName is a primitive domain name checker
// very relaxed and does not conform to spec
func IsValidDomainName(v string) bool {
	if !regexp.MustCompile("^[a-zA-Z0-9.-]+$").MatchString(v) {
		return false
	}
	values := strings.Split(v, ".")
	if len(values) < 2 || values[0] == "" {
		return false
	}
	return true
}
