/*
Copyright 2015 Gravitational, Inc.

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
package cxml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"testing"

	. "gopkg.in/check.v1"
)

func TestSchema(t *testing.T) { TestingT(t) }

type SchemaSuite struct {
}

var _ = Suite(&SchemaSuite{})

func (s *SchemaSuite) TestRecordNodes(c *C) {
	tree := `<xml><a><b/><c/><data>value</data></a></xml>`
	buffer := &bytes.Buffer{}
	type node struct {
		node    string
		parents []string
	}
	nodes := []node{}
	err := TransformXML(
		xml.NewDecoder(strings.NewReader(tree)),
		xml.NewEncoder(buffer),
		func(parents *NodeList, el xml.Token) []xml.Token {
			n := node{}
			switch e := el.(type) {
			case xml.CharData:
				n.node = string(e)
			case xml.StartElement:
				n.node = fmt.Sprintf("<%v>", e.Name.Local)
			default:
				return []xml.Token{el}
			}
			if len(parents.nodes) != 0 {
				for _, name := range parents.nodes {
					n.parents = append(n.parents, fmt.Sprintf("<%v>", name.Name.Local))
				}
			}
			nodes = append(nodes, n)
			return []xml.Token{el}
		})
	c.Assert(err, IsNil)
	expected := []node{
		{node: "<xml>"},
		{node: "<a>", parents: []string{"<xml>"}},
		{node: "<b>", parents: []string{"<xml>", "<a>"}},
		{node: "<c>", parents: []string{"<xml>", "<a>"}},
		{node: "<data>", parents: []string{"<xml>", "<a>"}},
		{node: "value", parents: []string{"<xml>", "<a>", "<data>"}},
	}
	c.Assert(len(nodes), Equals, len(expected))
	for i := range nodes {
		c.Assert(nodes[i], DeepEquals, expected[i])
	}

}

func (s *SchemaSuite) TestCases(c *C) {
	type tc struct {
		in  string
		out string
		fn  TransformFunc
	}

	testCases := []tc{
		{
			in:  `<xml><source file="before"></source></xml>`,
			out: `<xml><source file="after"></source></xml>`,
			fn: ReplaceAttributeIf("file", "after", func(parents *NodeList, el xml.Token) bool {
				e, ok := el.(xml.StartElement)
				if !ok {
					return false
				}
				return e.Name.Local == "source"
			}),
		},
		{
			in:  `<xml><node></node></xml>`,
			out: `<xml><node></node><disk type="file"></disk></xml>`,
			fn: InjectNodesIf([]xml.Token{
				xml.StartElement{
					Name: xml.Name{Local: "disk"},
					Attr: []xml.Attr{{
						Name:  xml.Name{Local: "type"},
						Value: "file"}}},
				xml.EndElement{Name: xml.Name{Local: "disk"}},
			}, func(parents *NodeList, el xml.Token) bool {
				endEl, ok := el.(xml.EndElement)
				if !ok {
					return false
				}
				return endEl.Name.Local == "xml" && parents.ParentIs(
					xml.Name{Local: "xml"})
			}),
		},
		{
			in:  `<xml><node>prev value</node><other>other</other></xml>`,
			out: `<xml><node>new value</node><other>other</other></xml>`,
			fn: ReplaceCDATAIf(
				[]byte("new value"), ParentIs(xml.Name{Local: "node"})),
		},
	}

	for i, tc := range testCases {
		comment := Commentf("test #%d", i+1)

		out := &bytes.Buffer{}
		err := TransformXML(
			xml.NewDecoder(strings.NewReader(tc.in)),
			xml.NewEncoder(out), tc.fn)
		c.Assert(err, IsNil, comment)
		c.Assert(out.String(), Equals, tc.out)
	}
}
