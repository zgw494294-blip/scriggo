// run

// Copyright 2026 The Scriggo Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file checks the behavior of range-over-function loops (Go 1.23):
// func(func() bool), func(func(V) bool) and func(func(K, V) bool).

package main

func seq0(yield func() bool) {
	if !yield() {
		return
	}
	if !yield() {
		return
	}
	yield()
}

func seq1(yield func(int) bool) {
	for i := 1; i <= 3; i++ {
		if !yield(i) {
			return
		}
	}
}

func seq2(yield func(string, int) bool) {
	if !yield("a", 1) {
		return
	}
	if !yield("b", 2) {
		return
	}
	yield("c", 3)
}

func count(n int) func(func(int) bool) {
	return func(yield func(int) bool) {
		for i := 0; i < n; i++ {
			if !yield(i) {
				return
			}
		}
	}
}

func empty(yield func(int) bool) {}

type Pair struct {
	x int
}

type Seq func(yield func(int) bool)

// pairSeq yields struct values.
func pairSeq(yield func(Pair) bool) {
	yield(Pair{1})
	yield(Pair{2})
}

// repeated uses a range loop and is called multiple times to check that the
// static frame layout works for every invocation.
func repeated(xs []int, base int) int {
	sum := base
	for v := range func(yield func(int) bool) {
		for _, x := range xs {
			if !yield(x) {
				return
			}
		}
	} {
		sum += v
	}
	return sum
}

// brk returns false from yield after the second value and runs code and a
// deferred call before returning.
func brk(yield func(int) bool) {
	defer func() { println("brk defer") }()
	for i := 0; i < 5; i++ {
		if !yield(i) {
			println("brk after false")
			return
		}
	}
	println("brk end")
}

func withDefer() (r int) {
	defer func() { println("f defer", r) }()
	for v := range seq1 {
		defer func() { println("body defer", v) }()
		if v == 2 {
			r = 100
			return
		}
	}
	return 0
}

func sumSeq2() int {
	sum := 0
	for _, v := range seq2 {
		sum += v
	}
	return sum
}

func main() {
	// No variables.
	n := 0
	for range seq0 {
		n++
	}
	println("seq0", n)

	// One and two variables, named functions.
	sum := 0
	for v := range seq1 {
		sum += v
	}
	println("seq1", sum)
	for k, v := range seq2 {
		println("seq2", k, v)
	}
	println("sumSeq2", sumSeq2())

	// Function literal capturing a variable.
	for v := range count(2) {
		println("lit", v)
	}

	// Function value.
	var f func(func(int) bool) = seq1
	for v := range f {
		println("val", v)
	}

	// Zero yields.
	ran := false
	for range empty {
		ran = true
	}
	println("empty0", ran)
	for v := range empty {
		_ = v
		ran = true
	}
	println("empty1", ran)

	// A named function type and struct yielded values.
	var s Seq = seq1
	total := 0
	for v := range s {
		total += v
	}
	println("named", total)
	for p := range pairSeq {
		println("pair", p.x)
	}

	// A function with a range loop called multiple times.
	xs2 := []int{1, 2, 3}
	println("rep1", repeated(xs2, 0))
	println("rep2", repeated(xs2, 10))

	// Blank identifiers.
	for k, _ := range seq2 {
		println("blankk", k)
	}

	// continue and break.
	bs := 0
	for v := range brk {
		if v == 1 {
			continue
		}
		bs += v
		if v == 3 {
			break
		}
	}
	println("brksum", bs)
	println("after loop")

	// return in the body, with iterator and body and function defers.
	ret := withDefer()
	println("ret", ret)

	// Nested range over functions, slices and normal for loops.
	outer := func(yield func(int) bool) {
		yield(1)
		yield(2)
	}
	inner := func(yield func(int) bool) {
		yield(10)
		yield(20)
	}
	for a := range outer {
		for b := range inner {
			println("nest", a+b)
		}
	}
	xs := []int{1, 2}
	for range outer {
		for _, x := range xs {
			println("slice", x)
		}
		for i := 0; i < 2; i++ {
			println("for", i)
		}
	}

	// Per-iteration binding of a := variable captured by a closure: an
	// interpreted helper calls the closure while the loop is active.
	call := func(fn func() int) int { return fn() }
	for v := range seq1 {
		println("bind", call(func() int { return v }))
	}

	// Recover in the function that owns the body, with the iterator unwound
	// first.
	func() {
		defer func() {
			if r := recover(); r != nil {
				println("main recovered", r.(string))
			}
		}()
		for range seq0 {
			panic("boom2")
		}
	}()

	// The iteration function can recover the panic to clean up, but it has
	// to resume panicking; the function that owns the body recovers it.
	func() {
		iter := func(yield func() bool) {
			defer func() {
				if r := recover(); r != nil {
					println("iter cleanup", r.(string))
					panic(r)
				}
			}()
			yield()
		}
		defer func() {
			if r := recover(); r != nil {
				println("main recovered", r.(string))
			}
		}()
		for range iter {
			panic("boom1")
		}
	}()
	println("done")
}
