/*
Copyright 2015-2019 Gravitational, Inc.

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

// Package trace implements utility functions for capturing debugging
// information about file and line in error reports and logs.
package trace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"

	"golang.org/x/net/context"
)

var debug int32

// SetDebug turns on/off debugging mode, that causes Fatalf to panic
func SetDebug(enabled bool) {
	if enabled {
		atomic.StoreInt32(&debug, 1)
	} else {
		atomic.StoreInt32(&debug, 0)
	}
}

// IsDebug returns true if debug mode is on
func IsDebug() bool {
	return atomic.LoadInt32(&debug) == 1
}

// Wrap takes the original error and wraps it into the Trace struct
// memorizing the context of the error.
func Wrap(err error, args ...interface{}) Error {
	if err == nil {
		return nil
	}
	if len(args) > 0 {
		format := args[0]
		args = args[1:]
		return WrapWithMessage(err, format, args...)
	}
	if traceErr, ok := err.(Error); ok {
		return traceErr
	}
	return newTrace(err, 2)
}

// Unwrap returns the original error the given error wraps
func Unwrap(err error) error {
	if err, ok := err.(ErrorWrapper); ok {
		return err.OrigError()
	}
	return err
}

// ErrorWrapper wraps another error
type ErrorWrapper interface {
	// OrigError returns the wrapped error
	OrigError() error
}

// DebugReporter formats an error for display
type DebugReporter interface {
	// DebugReport formats an error for display
	DebugReport() string
}

// UserMessage returns user-friendly part of the error
func UserMessage(err error) string {
	if err == nil {
		return ""
	}
	if wrap, ok := err.(Error); ok {
		return wrap.UserMessage()
	}
	return err.Error()
}

// UserMessageWithFields returns user-friendly error with key-pairs as part of the message
func UserMessageWithFields(err error) string {
	if err == nil {
		return ""
	}
	if wrap, ok := err.(Error); ok {
		if len(wrap.GetFields()) == 0 {
			return wrap.UserMessage()
		}

		var kvps []string
		for k, v := range wrap.GetFields() {
			kvps = append(kvps, fmt.Sprintf("%v=%q", k, v))
		}
		return fmt.Sprintf("%v %v", strings.Join(kvps, " "), wrap.UserMessage())
	}
	return err.Error()
}

// DebugReport returns debug report with all known information
// about the error including stack trace if it was captured
func DebugReport(err error) string {
	if err == nil {
		return ""
	}
	if reporter, ok := err.(DebugReporter); ok {
		return reporter.DebugReport()
	}
	return err.Error()
}

// GetFields returns any fields that have been added to the error message
func GetFields(err error) map[string]interface{} {
	if err == nil {
		return map[string]interface{}{}
	}
	if wrap, ok := err.(Error); ok {
		return wrap.GetFields()
	}
	return map[string]interface{}{}
}

// WrapWithMessage wraps the original error into Error and adds user message if any
func WrapWithMessage(err error, message interface{}, args ...interface{}) Error {
	var trace Error
	if traceErr, ok := err.(Error); ok {
		trace = traceErr
	} else {
		trace = newTrace(err, 3)
	}
	trace.AddUserMessage(message, args...)
	return trace
}

// Errorf is similar to fmt.Errorf except that it captures
// more information about the origin of error, such as
// callee, line number and function that simplifies debugging
func Errorf(format string, args ...interface{}) (err error) {
	err = fmt.Errorf(format, args...)
	return newTrace(err, 2)
}

// Fatalf - If debug is false Fatalf calls Errorf. If debug is
// true Fatalf calls panic
func Fatalf(format string, args ...interface{}) error {
	if IsDebug() {
		panic(fmt.Sprintf(format, args...))
	} else {
		return Errorf(format, args...)
	}
}

func newTrace(err error, depth int) *TraceErr {
	var buf [32]uintptr
	n := runtime.Callers(depth+1, buf[:])
	pcs := buf[:n]
	frames := runtime.CallersFrames(pcs)
	cursor := frameCursor{
		rest: frames,
		n:    n,
	}
	return newTraceFromFrames(cursor, err)
}

func newTraceFromFrames(cursor frameCursor, err error) *TraceErr {
	traces := make(Traces, 0, cursor.n)
	if cursor.current != nil {
		traces = append(traces, frameToTrace(*cursor.current))
	}
	for {
		frame, more := cursor.rest.Next()
		traces = append(traces, frameToTrace(frame))
		if !more {
			break
		}
	}
	return &TraceErr{
		Err:    err,
		Traces: traces,
	}
}

func frameToTrace(frame runtime.Frame) Trace {
	return Trace{
		Func: frame.Function,
		Path: frame.File,
		Line: frame.Line,
	}
}

type frameCursor struct {
	// current specifies the current stack frame.
	// if omitted, rest contains the complete stack
	current *runtime.Frame
	// rest specifies the rest of stack frames to explore
	rest *runtime.Frames
	// n specifies the total number of stack frames
	n int
}

// Traces is a list of trace entries
type Traces []Trace

// SetTraces adds new traces to the list
func (s Traces) SetTraces(traces ...Trace) {
	s = append(s, traces...)
}

// Func returns first function in trace list
func (s Traces) Func() string {
	if len(s) == 0 {
		return ""
	}
	return s[0].Func
}

// Func returns just function name
func (s Traces) FuncName() string {
	if len(s) == 0 {
		return ""
	}
	fn := filepath.ToSlash(s[0].Func)
	idx := strings.LastIndex(fn, "/")
	if idx == -1 || idx == len(fn)-1 {
		return fn
	}
	return fn[idx+1:]
}

// Loc points to file/line location in the code
func (s Traces) Loc() string {
	if len(s) == 0 {
		return ""
	}
	return s[0].String()
}

// String returns debug-friendly representaton of trace stack
func (s Traces) String() string {
	if len(s) == 0 {
		return ""
	}
	out := make([]string, len(s))
	for i, t := range s {
		out[i] = fmt.Sprintf("\t%v:%v %v", t.Path, t.Line, t.Func)
	}
	return strings.Join(out, "\n")
}

// Trace stores structured trace entry, including file line and path
type Trace struct {
	// Path is a full file path
	Path string `json:"path"`
	// Func is a function name
	Func string `json:"func"`
	// Line is a code line number
	Line int `json:"line"`
}

// String returns debug-friendly representation of this trace
func (t *Trace) String() string {
	dir, file := filepath.Split(t.Path)
	dirs := strings.Split(filepath.ToSlash(filepath.Clean(dir)), "/")
	if len(dirs) != 0 {
		file = filepath.Join(dirs[len(dirs)-1], file)
	}
	return fmt.Sprintf("%v:%v", file, t.Line)
}

// MarshalJSON marshals this error as JSON-encoded payload
func (r *TraceErr) MarshalJSON() ([]byte, error) {
	if r == nil {
		return nil, nil
	}
	type marshalableError TraceErr
	err := marshalableError(*r)
	err.Err = &RawTrace{Message: r.Err.Error()}
	return json.Marshal(err)
}

// TraceErr contains error message and some additional
// information about the error origin
type TraceErr struct {
	// Err is the underlying error that TraceErr wraps
	Err error `json:"error"`
	// Traces is a slice of stack trace entries for the error
	Traces `json:"traces,omitempty"`
	// Message is an optional message that can be wrapped with the original error.
	//
	// This field is obsolete, replaced by messages list below.
	Message string `json:"message,omitempty"`
	// Messages is a list of user messages added to this error.
	Messages []string `json:"messages,omitempty"`
	// Fields is a list of key-value-pairs that can be wrapped with the error to give additional context
	Fields map[string]interface{} `json:"fields,omitempty"`
}

// Fields maps arbitrary keys to values inside an error
type Fields map[string]interface{}

// Error returns the error message this trace describes.
// Implements error
func (r *RawTrace) Error() string {
	return r.Message
}

// RawTrace describes the error trace on the wire
type RawTrace struct {
	// Err specifies the original error
	Err json.RawMessage `json:"error,omitempty"`
	// Traces lists the stack traces at the moment the error was recorded
	Traces `json:"traces,omitempty"`
	// Message specifies the optional user-facing message
	Message string `json:"message,omitempty"`
	// Messages is a list of user messages added to this error.
	Messages []string `json:"messages,omitempty"`
	// Fields is a list of key-value-pairs that can be wrapped with the error to give additional context
	Fields map[string]interface{} `json:"fields,omitempty"`
}

// AddUserMessage adds user-friendly message describing the error nature
func (e *TraceErr) AddUserMessage(formatArg interface{}, rest ...interface{}) *TraceErr {
	newMessage := fmt.Sprintf(fmt.Sprintf("%v", formatArg), rest...)
	e.Messages = append(e.Messages, newMessage)
	return e
}

// AddFields adds the given map of fields to the error being reported
func (e *TraceErr) AddFields(fields map[string]interface{}) *TraceErr {
	if e.Fields == nil {
		e.Fields = make(map[string]interface{}, len(fields))
	}
	for k, v := range fields {
		e.Fields[k] = v
	}
	return e
}

// AddField adds a single field to the error wrapper as context for the error
func (e *TraceErr) AddField(k string, v interface{}) *TraceErr {
	if e.Fields == nil {
		e.Fields = make(map[string]interface{}, 1)
	}
	e.Fields[k] = v
	return e
}

// UserMessage returns user-friendly error message
func (e *TraceErr) UserMessage() string {
	if len(e.Messages) > 0 {
		// Format all collected messages in the reverse order, with each error
		// on its own line with appropriate indentation so they form a tree and
		// it's easy to see the cause and effect.
		var buf bytes.Buffer
		fmt.Fprintln(&buf, e.Messages[len(e.Messages)-1])
		index, indent := len(e.Messages)-1, 1
		for ; index > 0; index, indent = index-1, indent+1 {
			fmt.Fprintf(&buf, "%v%v\n", strings.Repeat("\t", indent), e.Messages[index-1])
		}
		fmt.Fprintf(&buf, "%v%v", strings.Repeat("\t", indent), UserMessage(e.Err))
		return buf.String()
	}
	if e.Message != "" {
		// For backwards compatibility return the old user message if it's present.
		return e.Message
	}
	return UserMessage(e.Err)
}

// DebugReport returns developer-friendly error report
func (e *TraceErr) DebugReport() string {
	var buf bytes.Buffer
	err := reportTemplate.Execute(&buf, errorReport{
		OrigErrType:    fmt.Sprintf("%T", e.Err),
		OrigErrMessage: e.Err.Error(),
		Fields:         e.Fields,
		StackTrace:     e.Traces.String(),
		UserMessage:    e.UserMessage(),
	})
	if err != nil {
		return fmt.Sprint("error generating debug report: ", err.Error())
	}
	return buf.String()
}

// Error returns user-friendly error message when not in debug mode
func (e *TraceErr) Error() string {
	return e.UserMessage()
}

func (e *TraceErr) GetFields() map[string]interface{} {
	return e.Fields
}

// OrigError returns original wrapped error
func (e *TraceErr) OrigError() error {
	err := e.Err
	// this is not an endless loop because I'm being
	// paranoid, this is a safe protection against endless
	// loops
	for i := 0; i < maxHops; i++ {
		newerr, ok := err.(Error)
		if !ok {
			break
		}
		if newerr.OrigError() != err {
			err = newerr.OrigError()
		}
	}
	return err
}

// GoString formats this trace object for use with
// with the "%#v" format string
func (e *TraceErr) GoString() string {
	return e.DebugReport()
}

// maxHops is a max supported nested depth for errors
const maxHops = 50

// Error is an interface that helps to adapt usage of trace in the code
// When applications define new error types, they can implement the interface
// So error handlers can use OrigError() to retrieve error from the wrapper
type Error interface {
	error
	ErrorWrapper
	DebugReporter

	// AddMessage adds formatted user-facing message
	// to the error, depends on the implementation,
	// usually works as fmt.Sprintf(formatArg, rest...)
	// but implementations can choose another way, e.g. treat
	// arguments as structured args
	AddUserMessage(formatArg interface{}, rest ...interface{}) *TraceErr

	// AddField adds additional field information to the error
	AddField(key string, value interface{}) *TraceErr

	// AddFields adds a map of additional fields to the error
	AddFields(fields map[string]interface{}) *TraceErr

	// UserMessage returns user-friendly error message
	UserMessage() string

	// GetFields returns any fields that have been added to the error
	GetFields() map[string]interface{}
}

// NewAggregate creates a new aggregate instance from the specified
// list of errors
func NewAggregate(errs ...error) error {
	// filter out possible nil values
	var nonNils []error
	for _, err := range errs {
		if err != nil {
			nonNils = append(nonNils, err)
		}
	}
	if len(nonNils) == 0 {
		return nil
	}
	return newTrace(aggregate(nonNils), 2)
}

// NewAggregateFromChannel creates a new aggregate instance from the provided
// errors channel.
//
// A context.Context can be passed in so the caller has the ability to cancel
// the operation. If this is not desired, simply pass context.Background().
func NewAggregateFromChannel(errCh chan error, ctx context.Context) error {
	var errs []error

Loop:
	for {
		select {
		case err, ok := <-errCh:
			if !ok { // the channel is closed, time to exit
				break Loop
			}
			errs = append(errs, err)
		case <-ctx.Done():
			break Loop
		}
	}

	return NewAggregate(errs...)
}

// Aggregate interface combines several errors into one error
type Aggregate interface {
	error
	// Errors obtains the list of errors this aggregate combines
	Errors() []error
}

// aggregate implements Aggregate
type aggregate []error

// Error implements the error interface
func (r aggregate) Error() string {
	if len(r) == 0 {
		return ""
	}
	output := r[0].Error()
	for i := 1; i < len(r); i++ {
		output = fmt.Sprintf("%v, %v", output, r[i])
	}
	return output
}

// Errors obtains the list of errors this aggregate combines
func (r aggregate) Errors() []error {
	return []error(r)
}

// IsAggregate returns whether this error of Aggregate error type
func IsAggregate(err error) bool {
	_, ok := Unwrap(err).(Aggregate)
	return ok
}

// wrapProxy wraps the specified error as a new error trace
func wrapProxy(err error) Error {
	if err == nil {
		return nil
	}
	return proxyError{
		// Do not include ReadError in the trace
		TraceErr: newTrace(err, 3),
	}
}

// DebugReport formats the underlying error for display
// Implements DebugReporter
func (r proxyError) DebugReport() string {
	var wrappedErr *TraceErr
	var ok bool
	if wrappedErr, ok = r.TraceErr.Err.(*TraceErr); !ok {
		return DebugReport(r.TraceErr)
	}
	var buf bytes.Buffer
	//nolint:errcheck
	reportTemplate.Execute(&buf, errorReport{
		OrigErrType:    fmt.Sprintf("%T", wrappedErr.Err),
		OrigErrMessage: wrappedErr.Err.Error(),
		Fields:         wrappedErr.Fields,
		StackTrace:     wrappedErr.Traces.String(),
		UserMessage:    wrappedErr.UserMessage(),
		Caught:         r.TraceErr.Traces.String(),
	})
	return buf.String()
}

// OrigError returns the original error.
// Implements WrappingError
func (r proxyError) OrigError() error {
	return r.TraceErr.OrigError()
}

// Error returns the error message of the underlying error
func (r proxyError) Error() string {
	return r.TraceErr.Error()
}

// GoString formats this trace object for use with
// with the "%#v" format string
func (r proxyError) GoString() string {
	return r.DebugReport()
}

// proxyError wraps another error
type proxyError struct {
	*TraceErr
}

type errorReport struct {
	// OrigErrType specifies the error type as text
	OrigErrType string
	// OrigErrMessage specifies the original error's message
	OrigErrMessage string
	// Fields lists any additional fields attached to the error
	Fields map[string]interface{}
	// StackTrace specifies the call stack
	StackTrace string
	// UserMessage is the user-facing message (if any)
	UserMessage string
	// Caught optionally specifies the stack trace where the error
	// has been recorded after coming over the wire
	Caught string
}

var reportTemplate = template.Must(template.New("debugReport").Parse(reportTemplateText))
var reportTemplateText = `
ERROR REPORT:
Original Error: {{.OrigErrType}} {{.OrigErrMessage}}
{{if .Fields}}Fields:
{{range $key, $value := .Fields}}  {{$key}}: {{$value}}
{{end}}{{end}}Stack Trace:
{{.StackTrace}}
{{if .Caught}}Caught:
{{.Caught}}
User Message: {{.UserMessage}}
{{else}}User Message: {{.UserMessage}}{{end}}`
