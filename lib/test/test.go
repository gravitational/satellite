package test

import (
	"reflect"

	"github.com/davecgh/go-spew/spew"
	"github.com/kylelemons/godebug/diff"
	"gopkg.in/check.v1"
)

var DeepCompare check.Checker = &deepCompareChecker{
	&check.CheckerInfo{Name: "DeepCompare", Params: []string{"obtained", "expected"}},
}

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
