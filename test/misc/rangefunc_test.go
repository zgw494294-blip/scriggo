// Copyright 2026 The Scriggo Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package misc

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/open2b/scriggo"
	"github.com/open2b/scriggo/internal/fstest"
	"github.com/open2b/scriggo/native"
)

// rangeFuncs returns native packages that expose synchronous iterator
// functions and logging functions, for use in the range-over-function tests.
func rangeFuncs(log *[]string) native.CombinedImporter {
	var held []func() int
	return native.CombinedImporter{
		native.Packages{
			"it": native.Package{
				Name: "it",
				Declarations: native.Declarations{
					"Seq0": func(yield func() bool) {
						yield()
						yield()
						yield()
					},
					"Seq1": func(yield func(int) bool) {
						for i := 1; i <= 3; i++ {
							if !yield(i) {
								*log = append(*log, "iterator stopped")
								return
							}
						}
						*log = append(*log, "iterator completed")
					},
					"Seq2": func(yield func(string, int) bool) {
						if !yield("a", 1) {
							return
						}
						if !yield("b", 2) {
							return
						}
						yield("c", 3)
					},
					"Seq1Break": func(yield func(int) bool) {
						defer func() { *log = append(*log, "iterator defer") }()
						for i := 0; i < 5; i++ {
							cont := yield(i)
							*log = append(*log, "yield returned")
							if !cont {
								*log = append(*log, "after break in iterator")
								return
							}
						}
					},
					"Log": func(s string) {
						*log = append(*log, s)
					},
					"LogInt": func(n int) {
						*log = append(*log, strconv.Itoa(n))
					},
					"LogFloat": func(f float64) {
						*log = append(*log, strconv.FormatFloat(f, 'f', -1, 64))
					},
					"Hold": func(f func() int) {
						held = append(held, f)
					},
					"CallHeld": func() {
						for _, f := range held {
							*log = append(*log, strconv.Itoa(f()))
						}
					},
					"SeqFloat": func(yield func(float64, string) bool) {
						yield(1.5, "x")
						yield(2.5, "y")
					},
					"MakeIter": func() func(func(int) bool) {
						*log = append(*log, "MakeIter")
						return func(yield func(int) bool) { yield(7) }
					},
					"Boom": func() { panic("helper boom") },
				},
			},
		},
	}
}

// runRangeProgram builds and runs a program with the given packages.
func runRangeProgram(t *testing.T, src string, packages native.Importer) error {
	t.Helper()
	prog, err := scriggo.Build(fstest.Files{"main.go": src}, &scriggo.BuildOptions{Packages: packages})
	if err != nil {
		return err
	}
	return prog.Run(nil)
}

// TestRangeOverFuncInterpreted tests range over interpreted named functions,
// function literals (closures) and function values.
func TestRangeOverFuncInterpreted(t *testing.T) {

	const src = `
package main

import "it"

// seq0 yields three times without values.
func seq0(yield func() bool) {
	yield()
	yield()
	yield()
}

// seq1 yields the integers from 1 to 3. It returns false as bool value
// through the log when the body stops.
func seq1(yield func(int) bool) {
	for i := 1; i <= 3; i++ {
		if !yield(i) {
			it.Log("seq1 stopped")
			return
		}
	}
	it.Log("seq1 completed")
}

// seq2 yields key/value pairs.
func seq2(yield func(string, int) bool) {
	yield("a", 1)
	yield("b", 2)
	yield("c", 3)
}

// count returns an iterator function literal that captures n.
func count(n int) func(func(int) bool) {
	return func(yield func(int) bool) {
		for i := 0; i < n; i++ {
			if !yield(i) {
				return
			}
		}
	}
}

// empty never calls yield.
func empty(yield func(int) bool) {}

func main() {
	// No variables.
	n := 0
	for range seq0 {
		n++
	}
	it.LogInt(n)

	// One variable, named function.
	sum := 0
	for v := range seq1 {
		sum += v
	}
	it.LogInt(sum)

	// Two variables, named function.
	s := ""
	for k, v := range seq2 {
		it.Log(k)
		s += k
		_ = v
	}
	it.Log(s)

	// Blank key.
	for _, v := range seq2 {
		it.LogInt(v)
	}

	// Function literal that captures a variable.
	for v := range count(2) {
		it.LogInt(v)
	}

	// Function value.
	var f func(func(int) bool) = seq1
	for v := range f {
		it.LogInt(v)
	}

	// Zero yields: the body is not executed.
	for range empty {
		it.Log("must not be logged")
	}
	for v := range empty {
		_ = v
		it.Log("must not be logged")
	}
	it.Log("done")
}
`

	var log []string
	err := runRangeProgram(t, src, rangeFuncs(&log))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"3",
		"seq1 completed", "6",
		"a", "b", "c", "abc",
		"1", "2", "3",
		"0", "1",
		"1", "2", "3",
		"seq1 completed",
		"done",
	}
	if strings.Join(log, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected log:\n%v\nwant:\n%v", log, want)
	}

}

// TestRangeOverFuncAssignment tests the := and = forms, blank identifiers and
// the per-iteration binding of := variables captured by closures.
func TestRangeOverFuncAssignment(t *testing.T) {

	const src = `
package main

import "it"

func seq(yield func(int) bool) {
	yield(1)
	yield(2)
	yield(3)
}

func main() {
	// = updates an existing variable.
	v := 0
	sum := 0
	for v = range seq {
		sum += v
	}
	it.LogInt(v)
	it.LogInt(sum)

	// Per-iteration binding: a closure that captures the iteration variable
	// captures the value of the current iteration. The closures are held by
	// the host and called after the loop has terminated.
	for v := range seq {
		it.Hold(func() int { return v })
	}
	it.CallHeld()
	it.Log("done")
}
`
	var log []string
	err := runRangeProgram(t, src, rangeFuncs(&log))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"3", "6", "1", "2", "3", "done"}
	if strings.Join(log, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected log:\n%v\nwant:\n%v", log, want)
	}

}

// TestRangeOverFuncBreakContinue tests break and continue, including the
// false value returned to yield and the code and deferred calls executed in
// the iterator after a break.
func TestRangeOverFuncBreakContinue(t *testing.T) {

	const src = `
package main

import "it"

func seq(yield func(int) bool) {
	defer func() { it.Log("iterator defer") }()
	for i := 0; i < 5; i++ {
		if !yield(i) {
			it.Log("iterator after false")
			return
		}
	}
	it.Log("iterator end")
}

func main() {
	// continue.
	s := 0
	for v := range seq {
		if v == 1 {
			continue
		}
		s += v
		if v == 3 {
			break
		}
	}
	it.LogInt(s)
	it.Log("after loop")
}
`
	var log []string
	err := runRangeProgram(t, src, rangeFuncs(&log))
	if err != nil {
		t.Fatal(err)
	}
	// i=0 yield returns true; i=1 continue (true); i=2 (true); i=3 break
	// (false), the iterator runs the code after the call and its defer, then
	// the loop continues.
	want := []string{"iterator after false", "iterator defer", "5", "after loop"}
	if strings.Join(log, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected log:\n%v\nwant:\n%v", log, want)
	}

}

// TestRangeOverFuncNested tests range-over-function loops nested with other
// range loops, normal loops and function calls.
func TestRangeOverFuncNested(t *testing.T) {

	const src = `
package main

import "it"

func outer(yield func(int) bool) {
	for i := 1; i <= 2; i++ {
		if !yield(i) {
			return
		}
	}
}

func inner(yield func(int) bool) {
	yield(10)
	yield(20)
}

func add(a, b int) int { return a + b }

func main() {
	// Nested range-over-function loops.
	for a := range outer {
		for b := range inner {
			it.LogInt(add(a, b))
		}
	}
	// Range over a slice in a range-over-function body.
	xs := []int{1, 2}
	for range outer {
		for _, x := range xs {
			it.LogInt(x)
		}
		for i := 0; i < 2; i++ {
			it.LogInt(i)
		}
	}
}
`
	var log []string
	err := runRangeProgram(t, src, rangeFuncs(&log))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"11", "21", "12", "22",
		"1", "2", "0", "1",
		"1", "2", "0", "1",
	}
	if strings.Join(log, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected log:\n%v\nwant:\n%v", log, want)
	}

}

// TestRangeOverFuncReturn tests a return statement in the body: the result is
// computed, the active iteration functions exit running their deferred calls,
// then the deferred calls of the owning function run and the function
// returns.
func TestRangeOverFuncReturn(t *testing.T) {

	const src = `
package main

import "it"

func iter(yield func(int) bool) {
	defer func() { it.Log("iter defer") }()
	for i := 0; i < 10; i++ {
		if !yield(i) {
			it.Log("unexpected false")
			return
		}
	}
}

func f() int {
	defer func() { it.Log("f defer") }()
	for v := range iter {
		defer func() { it.Log("body defer") }()
		if v == 2 {
			it.Log("returning")
			return 100
		}
	}
	return 0
}

// g has named result parameters.
func g() (n int) {
	defer func() { it.LogInt(n) }()
	for v := range iter {
		if v == 1 {
			n = 42
			return
		}
	}
	return
}

func main() {
	it.LogInt(f())
	it.LogInt(g())
	it.Log("main done")
}
`
	var log []string
	err := runRangeProgram(t, src, rangeFuncs(&log))
	if err != nil {
		t.Fatal(err)
	}
	// f: body defer registered on the iteration that returns must not run at
	// each yield, but on f's return, after the iterator defer.
	want := []string{
		"returning", "iter defer",
		"body defer", "body defer", "body defer",
		"f defer", "100",
		"iter defer", "42", "42",
		"main done",
	}
	if strings.Join(log, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected log:\n%v\nwant:\n%v", log, want)
	}

}

// TestRangeOverFuncReturnNested tests a return in nested range loops: all the
// active iteration functions, interpreted and native, exit before the owning
// function returns.
func TestRangeOverFuncReturnNested(t *testing.T) {

	const src = `
package main

import "it"

func outer(yield func(int) bool) {
	defer func() { it.Log("outer defer") }()
	yield(1)
}

func inner(yield func(int) bool) {
	defer func() { it.Log("inner defer") }()
	yield(10)
	it.Log("inner after yield")
}

func f() string {
	for range outer {
		for range inner {
			for _, v := range it.Seq2 {
				if v == 2 {
					return "result"
				}
			}
		}
		it.Log("must not be logged")
	}
	return "zero"
}

func main() {
	it.Log(f())
}
`
	var log []string
	err := runRangeProgram(t, src, rangeFuncs(&log))
	if err != nil {
		t.Fatal(err)
	}
	// The native iterator is unwound by the panic, then the interpreted
	// inner and outer iterators run their defers.
	want := []string{"inner defer", "outer defer", "result"}
	if strings.Join(log, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected log:\n%v\nwant:\n%v", log, want)
	}

}

// TestRangeOverFuncBodyDefer tests that the deferred calls registered in the
// body belong to the owning function and are not executed at every yield.
func TestRangeOverFuncBodyDefer(t *testing.T) {

	const src = `
package main

import "it"

func iter(yield func(int) bool) {
	yield(1)
	yield(2)
	yield(3)
}

func main() {
	it.Log("before")
	for v := range iter {
		defer func() { it.Log("defer") }()
		it.LogInt(v)
	}
	it.Log("after")
}
`
	var log []string
	err := runRangeProgram(t, src, rangeFuncs(&log))
	if err != nil {
		t.Fatal(err)
	}
	// main returns after the loop and the three deferred calls run in LIFO
	// order.
	want := []string{"before", "1", "2", "3", "after", "defer", "defer", "defer"}
	if strings.Join(log, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected log:\n%v\nwant:\n%v", log, want)
	}

}

// TestRangeOverFuncNative tests range over iterator functions registered as
// host functions through BuildOptions.Packages.
func TestRangeOverFuncNative(t *testing.T) {

	const src = `
package main

import "it"

func main() {
	n := 0
	for range it.Seq0 {
		n++
	}
	it.LogInt(n)

	sum := 0
	for v := range it.Seq1 {
		sum += v
		if v == 2 {
			break
		}
	}
	it.LogInt(sum)

	for k, v := range it.Seq2 {
		it.Log(k)
		_ = v
	}

	for v := range it.Seq1Break {
		if v == 2 {
			break
		}
		it.LogInt(v)
	}
	it.Log("end")
}
`
	var log []string
	err := runRangeProgram(t, src, rangeFuncs(&log))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"3",
		"iterator stopped", "3",
		"a", "b", "c",
		"0", "yield returned",
		"1", "yield returned",
		"yield returned",
		"after break in iterator", "iterator defer",
		"end",
	}
	if strings.Join(log, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected log:\n%v\nwant:\n%v", log, want)
	}

}

// TestRangeOverFuncNativeReturn tests a return in the body when ranging over
// a host iterator function.
func TestRangeOverFuncNativeReturn(t *testing.T) {

	const src = `
package main

import "it"

func f() int {
	defer func() { it.Log("f defer") }()
	for v := range it.Seq1 {
		if v == 2 {
			return 20
		}
	}
	return 0
}

func main() {
	it.LogInt(f())
}
`
	var log []string
	err := runRangeProgram(t, src, rangeFuncs(&log))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"f defer", "20"}
	if strings.Join(log, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected log:\n%v\nwant:\n%v", log, want)
	}

}

// TestRangeOverFuncPanic tests that a panic in the body or in the iterator
// still behaves as an ordinary panic, with defer and recover, and that a
// subsequent run of the same program is not affected.
func TestRangeOverFuncPanic(t *testing.T) {

	const srcPanicBody = `
package main

import "it"

func iter(yield func(int) bool) {
	defer func() {
		if r := recover(); r != nil {
			// Clean up and resume panicking, as required by the
			// range-over-function protocol.
			it.Log("iter recovered")
			panic(r)
		}
	}()
	yield(1)
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			it.Log("main recovered")
		}
	}()
	for range iter {
		panic("boom")
	}
}
`
	var log []string
	pkgs := rangeFuncs(&log)
	err := runRangeProgram(t, srcPanicBody, pkgs)
	if err != nil {
		t.Fatalf("expected the panic to be recovered, got %v", err)
	}
	want := []string{"iter recovered", "main recovered"}
	if strings.Join(log, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected log:\n%v\nwant:\n%v", log, want)
	}

	// If the iterator does not recover the panic, it propagates to the
	// function that owns the body and can be recovered there.
	const srcPanicMain = `
package main

import "it"

func iter(yield func() bool) {
	defer func() { it.Log("iter defer") }()
	yield()
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			it.Log("main recovered")
		}
	}()
	for range iter {
		panic("boom")
	}
}
`
	log = nil
	err = runRangeProgram(t, srcPanicMain, pkgs)
	if err != nil {
		t.Fatalf("expected the panic to be recovered, got %v; log: %v", err, log)
	}
	want = []string{"iter defer", "main recovered"}
	if strings.Join(log, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected log:\n%v\nwant:\n%v", log, want)
	}

	// A panic that is not recovered is returned as a PanicError.
	const srcPanicIter = `
package main

import "it"

func iter(yield func() bool) {
	panic("iter boom")
}

func main() {
	for range iter {
		it.Log("must not be logged")
	}
}
`
	log = nil
	var pe *scriggo.PanicError
	err = runRangeProgram(t, srcPanicIter, rangeFuncs(&log))
	if !errors.As(err, &pe) {
		t.Fatalf("expected *scriggo.PanicError, got %T: %v", err, err)
	}

	// Running a new program after a failed run is not affected.
	const srcOK = `
package main

import "it"

func main() {
	for range it.Seq0 {
		it.Log("ok")
	}
}
`
	log = nil
	err = runRangeProgram(t, srcOK, rangeFuncs(&log))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(log, "|") != "ok|ok|ok" {
		t.Fatalf("unexpected log: %v", log)
	}

}

// TestRangeOverFuncBuildErrors tests that invalid iterator signatures, too
// many iteration variables and incompatible assignments are build errors.
func TestRangeOverFuncBuildErrors(t *testing.T) {
	cases := map[string]string{
		"not a function": `
package main

func main() {
	for range 42 {}
}
`,
		"function with results": `
package main

func main() {
	f := func(yield func() bool) int { return 0 }
	for range f {}
}
`,
		"yield without bool result": `
package main

func main() {
	f := func(yield func()) {}
	for range f {}
}
`,
		"too many parameters": `
package main

func main() {
	f := func(yield func(int, int, int) bool) {}
	for range f {}
}
`,
		"too many variables, no values": `
package main

func main() {
	f := func(yield func() bool) {}
	for x := range f { _ = x }
}
`,
		"too many variables, one value": `
package main

func main() {
	f := func(yield func(int) bool) {}
	for a, b := range f { _, _ = a, b }
}
`,
		"incompatible assignment": `
package main

func main() {
	f := func(yield func(string) bool) {}
	var n int
	for n = range f {}
}
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := scriggo.Build(fstest.Files{"main.go": src}, nil)
			if err == nil {
				t.Fatal("expected a build error, got nil")
			}
			var be *scriggo.BuildError
			if !errors.As(err, &be) {
				t.Fatalf("expected *scriggo.BuildError, got %T: %v", err, err)
			}
		})
	}
}

// TestRangeOverFuncTypesAndMisc covers different kinds of yielded values,
// the one-time evaluation of the range expression, a panic in a helper
// called from the body and a switch statement with break in the body.
func TestRangeOverFuncTypesAndMisc(t *testing.T) {

	const src = `
package main

import "it"

func seq0(yield func() bool) {
	if !yield() {
		return
	}
	if !yield() {
		return
	}
	yield()
}

func helper() {
	it.Boom()
}

func find(xs []int, x int) bool {
	for _, v := range xs {
		switch {
		case v == x:
			return true
		default:
		}
	}
	return false
}

func main() {
	// Float and string yielded values, with a blank value.
	for k, _ := range it.SeqFloat {
		it.LogFloat(k)
	}
	for _, v := range it.SeqFloat {
		it.Log(v)
	}

	// The range expression is evaluated once.
	for v := range it.MakeIter() {
		it.LogInt(v)
	}

	// A blank variable with a one-value iterator is allowed.
	for _ = range func(yield func(int) bool) { yield(9) } {
	}

	// break in a no-value loop.
	n := 0
	for range seq0 {
		n++
		if n == 1 {
			break
		}
	}
	it.LogInt(n)

	// switch with break in the body of a range over a slice.
	xs := []int{1, 2, 3}
	sum := 0
	for i := 0; i < 2; i++ {
		for _, v := range xs {
			switch v {
			case 2:
				break
			default:
				sum += v
			}
		}
	}
	it.LogInt(sum)
	if find(xs, 2) {
		it.Log("found")
	}

	// A panic in a helper called from the body propagates and is recovered
	// by the owning function.
	func() {
		defer func() {
			if r := recover(); r != nil {
				it.Log("helper recovered")
			}
		}()
		for range seq0 {
			helper()
		}
	}()
	it.Log("done")
}
`
	var log []string
	err := runRangeProgram(t, src, rangeFuncs(&log))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"1.5", "2.5",
		"x", "y",
		"MakeIter", "7",
		"1",
		"8",
		"found",
		"helper recovered",
		"done",
	}
	if strings.Join(log, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected log:\n%v\nwant:\n%v", log, want)
	}

}
