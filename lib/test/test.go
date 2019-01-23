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

package test

import (
	"reflect"
	"sort"

	"github.com/davecgh/go-spew/spew"
	"github.com/kylelemons/godebug/diff"
	"gopkg.in/check.v1"
)

// DeepCompare is a gocheck checker that provides a readable diff in case
// comparison fails.
var DeepCompare check.Checker = &deepCompareChecker{
	&check.CheckerInfo{Name: "DeepCompare", Params: []string{"obtained", "expected"}},
}

// Check expects two items in params (obtained and expected) and compares them using reflection.
// If comparison fails, it returns a readable diff in error.
// Implements gocheck checker interface
func (checker *deepCompareChecker) Check(params []interface{}, names []string) (result bool, error string) {
	result = reflect.DeepEqual(params[0], params[1])
	if !result {
		error = Diff(params[0], params[1])
	}
	return result, error
}

// SortedSliceEquals is a gocheck checker that compares two slices after sorting them.
// It expects the slice parameters to implement sort.Interface
var SortedSliceEquals check.Checker = &sliceEqualsChecker{
	&check.CheckerInfo{Name: "SortedSliceEquals", Params: []string{"obtained", "expected"}},
}

// Check expects two slices in params (obtained and expected).
// The slices are sorted before comparison, hence they are expected to implement sort.Interface.
// If comparison fails, it returns a readable diff in error.
// Implements gocheck checker interface
func (checker *sliceEqualsChecker) Check(params []interface{}, names []string) (result bool, error string) {
	obtained := params[0].(sort.Interface)
	sort.Sort(obtained)
	expected := params[1].(sort.Interface)
	sort.Sort(expected)

	result = reflect.DeepEqual(obtained, expected)
	if !result {
		error = Diff(obtained, expected)
	}
	return result, error
}

// Diff returns user friendly difference between two objects
func Diff(a, b interface{}) string {
	d := &spew.ConfigState{Indent: " ", DisableMethods: true, DisablePointerMethods: true, DisablePointerAddresses: true}
	return diff.Diff(d.Sdump(a), d.Sdump(b))
}

type sliceEqualsChecker struct {
	*check.CheckerInfo
}

type deepCompareChecker struct {
	*check.CheckerInfo
}
