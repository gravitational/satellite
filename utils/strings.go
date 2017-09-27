package utils

import (
	"reflect"
	"sort"
	"strings"
)

func StringInSlice(haystack []string, needle string) bool {
	found := false
	for i := range haystack {
		if haystack[i] == needle {
			found = true
			break
		}
	}
	return found
}

func StringsInSlice(haystack []string, needles ...string) bool {
	for _, needle := range needles {
		found := false
		for i := range haystack {
			if haystack[i] == needle {
				found = true
				break
			}
		}
		if found == false {
			return false
		}
	}
	return true
}

func CompareStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Sort(sort.StringSlice(a))
	sort.Sort(sort.StringSlice(b))
	return reflect.DeepEqual(a, b)
}

// HasOneOfPrefixes returns true if the provided string starts with any of the specified prefixes
func HasOneOfPrefixes(s string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// MatchesLabels determines whether a set of "target" labels matches
// the set of "wanted" labels
func MatchesLabels(targetLabels, wantedLabels map[string]string) bool {
	for k, v := range wantedLabels {
		if targetLabels[k] != v {
			return false
		}
	}
	return true
}
