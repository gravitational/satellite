/*

Copyright 2019 Gravitational, Inc.
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
	"context"
	"io/ioutil"
	"strings"

	. "gopkg.in/check.v1"
)

type ReaderSuite struct{}

var _ = Suite(&ReaderSuite{})

func (s *ReaderSuite) TestReadAll(c *C) {
	testString := "Reader Test"

	reader := NewReaderWithContext(context.TODO(), strings.NewReader(testString))
	content, err := ioutil.ReadAll(reader)

	c.Assert(err, IsNil)
	c.Assert(string(content), Equals, testString, Commentf("Read all content"))
}

func (s *ReaderSuite) TestContextCanceled(c *C) {
	testString := "Reader Test"

	ctx, cancel := context.WithCancel(context.TODO())
	cancel()

	reader := NewReaderWithContext(ctx, strings.NewReader(testString))
	_, err := ioutil.ReadAll(reader)

	c.Assert(err, Not(IsNil))
	c.Assert(err.Error(), Equals, "context canceled", Commentf("Reader context canceled"))
}
