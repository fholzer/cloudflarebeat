package logpull

import (
	"regexp"
	"strings"

	cloudflare "github.com/cloudflare/cloudflare-go"
	pkgerrors "github.com/pkg/errors"
)

var (
	idMatcher, _   = regexp.Compile("^[0-9a-f]{32}$")
	requiredFields = []string{
		"RayID",
		"EdgeStartTimestamp",
	}
)

func orgId(orgs []cloudflare.Organization, organization *string) *cloudflare.Organization {
	org := strings.ToLower(*organization)

	for _, o := range orgs {
		if o.ID == org || strings.ToLower(o.Name) == org {
			return &o
		}
	}
	return nil
}

func getRootCause(err error) error {
	res := err
	for {
		if e := pkgerrors.Cause(res); res != e {
			res = e
		} else {
			return res
		}
	}
}

func ensureRequiredFields(fields []string) (effectiveFields []string, unpublishedFields []string) {
	unpubFields := make([]string, 0)
	for i := range requiredFields {
		found := false
		for j := range fields {
			if requiredFields[i] == fields[j] {
				found = true
				break
			}
		}
		if !found {
			unpubFields = append(unpubFields, requiredFields[i])
		}
	}

	if len(unpubFields) < 1 {
		return fields, nil
	}

	resFields := make([]string, len(fields)+len(unpubFields))
	copy(resFields[0:], fields)
	copy(resFields[len(fields):], unpubFields[:])
	return resFields, unpubFields
}
