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

package monitoring

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gravitational/satellite/agent/health"
	"github.com/gravitational/satellite/lib/httplib"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/gravitational/trace"
)

// NewAWSHasProfileChecker returns a new checker, that checks that the instance
// has a node profile assigned to it.
// TODO(knisbet): look into enhancing this to check the contents of the profile
// for missing permissions. However, for now this exists just as a basic check
// for instances that accidently lose their profile assignment.
func NewAWSHasProfileChecker() health.Checker {
	return &awsHasProfileChecker{}
}

type awsHasProfileChecker struct{}

// Name returns this checker name
// Implements health.Checker
func (c *awsHasProfileChecker) Name() string {
	return awsHasProfileCheckerID
}

// Check will check the metadata API to see if an IAM profile is assigned to the node
// Implements health.Checker
func (c *awsHasProfileChecker) Check(ctx context.Context, reporter health.Reporter) {
	probes, err := c.check(ctx)
	if err != nil {
		reporter.Add(NewProbeFromErr(c.Name(), "failed to validate IAM profile", err))
		return
	}
	health.AddFrom(reporter, probes)
}

// check will check the metadata API to see if an IAM profile is assigned to the node.
func (c *awsHasProfileChecker) check(ctx context.Context) (probes health.Reporter, err error) {
	probes = &health.Probes{}

	sess, err := session.NewSession()
	if err != nil {
		return probes, trace.Wrap(err, "failed to create session")
	}

	config := sess.ClientConfig(ec2metadata.ServiceName)
	config.Config.HTTPClient = &http.Client{
		Transport: httplib.NewTransportWithContext(ctx),
		Timeout:   5 * time.Second,
	}

	metadata := ec2metadata.NewClient(*config.Config, config.Handlers, config.Endpoint, config.SigningRegion)

	_, err = metadata.IAMInfo()
	if isContextCanceledError(err) {
		return probes, trace.Wrap(err, "context canceled")
	}
	if err != nil {
		return probes, trace.Wrap(err, "failed to determine node IAM profile")
	}

	probes.Add(NewSuccessProbe(c.Name()))
	return probes, nil
}

// IsRunningOnAWS attempts to use the AWS metadata API to determine if the
// currently running node is an AWS node or not
func IsRunningOnAWS() bool {
	session := session.New()
	metadata := ec2metadata.New(session)
	return metadata.Available()
}

// isContextCanceledError returns true if the error is a context canceled error.
func isContextCanceledError(err error) bool {
	if err == nil {
		return false
	}
	switch err := unwrapAWSError(err).(type) {
	case *url.Error:
		return err.Err == context.Canceled
	default:
		return strings.Contains(err.Error(), "context canceled")
	}
}

// unwrapAWSError unwraps the aws error and returns the orignal error. Returns
// the provided error if it is not an aws error.
func unwrapAWSError(err error) error {
	if err != nil {
		switch origErr := err.(type) {
		case awserr.Error:
			err = origErr.OrigErr()
		default:
			return err
		}
	}
	return err
}

const awsHasProfileCheckerID = "aws"
