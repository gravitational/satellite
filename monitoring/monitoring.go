package monitoring

import (
	"github.com/gravitational/satellite/agent/health"
	pb "github.com/gravitational/satellite/agent/proto/agentpb"
)

// newChecker creates an instance of health.Checker given checker and a name
func newChecker(checker checker, name string) health.Checker {
	return &defaultChecker{name: name, checker: checker}
}

// Name returns the name of the checker
func (r *defaultChecker) Name() string { return r.name }

// health.Checker
func (r *defaultChecker) Check(reporter health.Reporter) {
	rep := &simpleReporter{Reporter: reporter, checker: r}
	r.checker.check(rep)
}

func (r *simpleReporter) add(err error) {
	r.Reporter.Add(&pb.Probe{
		Checker: r.checker.Name(),
		Error:   err.Error(),
		Status:  pb.Probe_Failed,
	})
}

func (r *simpleReporter) addProbe(probe *pb.Probe) {
	probe.Checker = r.checker.Name()
	r.Reporter.Add(probe)
}

// defaultChecker is a health.Checker with a simplified interface.
type defaultChecker struct {
	name    string
	checker checker
}

// checker is a checker that reports into a simplified reporter.
// This is to enable writing checkers as simple functions returning error
type checker interface {
	check(reporter)
}

// reporter enables implementing checkers as a simple function that returns an error
type reporter interface {
	add(error)
	addProbe(*pb.Probe)
}

// simpleReporter implement reporter interface
type simpleReporter struct {
	health.Reporter
	checker health.Checker
}
