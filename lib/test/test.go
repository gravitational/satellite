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

// Diff returns user friendly difference between two objects
func Diff(a, b interface{}) string {
	d := &spew.ConfigState{Indent: " ", DisableMethods: true, DisablePointerMethods: true, DisablePointerAddresses: true}
	return diff.Diff(d.Sdump(a), d.Sdump(b))
}

type deepCompareChecker struct {
	*check.CheckerInfo
}
