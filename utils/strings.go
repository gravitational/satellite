/*
Copyright 2017 Gravitational, Inc.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"reflect"
	"sort"
	"strings"
)

// StringInSlice returns true if needle is within haystack
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

// StringsInSlice returns true if needles are within haystack
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

// CompareStringSlices does two string array comparison
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
