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
	"encoding/json"
	"path"
	"strings"

	"github.com/gravitational/satellite/agent/health"

	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/request"
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
		probes.Add(NewProbeFromErr(c.Name(), "failed to validate IAM profile", err))
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

	metadata := ec2metadata.New(sess)
	err = c.iamInfo(ctx, metadata)
	if err != nil {
		return probes, trace.Wrap(err, "failed to determine node IAM profile")
	}

	probes.Add(NewSuccessProbe(c.Name()))
	return probes, nil
}

// iamInfo checks if an IAM profile is assigned to the node.
// Takes a context for cancelation.
//
// This is a slight modification to `EC2Metadata.IAMInfo(...)`
// https://github.com/aws/aws-sdk-go/blob/13ad2b09f6ff7e16808b4a45c32684e6c66bf86d/aws/ec2metadata/api.go#L92
func (c *awsHasProfileChecker) iamInfo(ctx context.Context, metadata *ec2metadata.EC2Metadata) error {
	resp, err := c.getIAMInfo(ctx, metadata)
	if err != nil {
		return trace.Wrap(err, "failed to get EC2 IAM info")
	}

	var info ec2metadata.EC2IAMInfo
	if err := json.NewDecoder(strings.NewReader(resp)).Decode(&info); err != nil {
		return trace.Wrap(err, "failed to decode EC2 IAM info")
	}

	if info.Code != "Success" {
		return trace.Wrap(err, "failed to get EC2 IAM Info (%s)", info.Code)
	}
	return nil
}

// getIAMInfo sends a request for IAM info and return the response content.
// Takes a context for cancelation and gets IAM info.
//
// This is a slight modification to `EC2Metadata.GetMetadata(...)`
// https://github.com/aws/aws-sdk-go/blob/13ad2b09f6ff7e16808b4a45c32684e6c66bf86d/aws/ec2metadata/api.go#L18
func (c *awsHasProfileChecker) getIAMInfo(ctx context.Context, metadata *ec2metadata.EC2Metadata) (string, error) {
	op := &request.Operation{
		Name:       "GetMetadata",
		HTTPMethod: "GET",
		HTTPPath:   path.Join("/", "meta-data", "iam/info"),
	}
	output := &metadataOutput{}
	req := metadata.NewRequest(op, nil, output)
	req.SetContext(ctx)
	err := req.Send()
	if ctx.Err() != nil {
		return output.Content, trace.Wrap(err, ctx.Err())
	}
	return output.Content, trace.Wrap(err)
}

// metadataOutput contains the returned content when requesting ec2metadata.
type metadataOutput struct {
	Content string
}

// IsRunningOnAWS attempts to use the AWS metadata API to determine if the
// currently running node is an AWS node or not
func IsRunningOnAWS() bool {
	session := session.New()
	metadata := ec2metadata.New(session)
	return metadata.Available()
}

const awsHasProfileCheckerID = "aws"
