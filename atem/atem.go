// _atem_ is both a minimal and low-level interpreted functional programming
// language IR (_intermediate representation_, ie. not to be hand-written) and
// its reference interpreter implementation (in lib form). It prioritizes
// staying low-LoC enough to be able to port it over to any other current and
// future lang / tech stack swiftly and with ease, over other concerns, by
// design. At the time of writing, the "parsing" / loading in this Go-based
// implementation is ~42 LoCs (the choice of a JSON code format is likewise
// motivated by the stated "no-brainer portability" objective), the interpreting
// / eval'ing parts around ~75 LoCs, AST node type formulations and their
// `JsonSrc()` implementations around ~50 LoCs, and utilities for forcing
// "`Eval` result linked-list-closures" into actual `[]int` or `[]byte` slices
// or `string`s, or the other way around for passing into `Eval`, another ~55
// LoCs. All counts approximate and net (excluding comments, blank lines etc).
//
// This focus doesn't make for the most efficient interpreter in the world, but that
// isn't the objective for _atem_. The goal is to provide the bootstrapping basis
// for **atmo**. An initial compiler from _atmo_ to _atem_ is being coded in my
// [toy Lambda Calculus dialect](https://github.com/metaleap/go-machines/tree/master/toylam)
// and then again in (the initial iteration of) _atmo_ itself. The _atem_
// interpreter will also suffice / go a long way for REPL purposes and later
// on abstract / symbolic interpretation / partial evaluation for the
// experimental type-checking approaches envisioned to be explored within the
// impending ongoing evolution of _atmo_ once its initial incarnation is birthed.
//
// For "ultimate real-world runtime artifact" purposes, _atem_ isn't intended;
// rather, transpilers and / or compilers to 3rd-party mature and widely enjoyed
// interpreters / bytecode VMs or intermediate ASM targets like LLVM-IR would
// be the envisioned generally-preferable direction anyway, except such trans-/
// compilers must naturally be done in _atmo_ as well, so _atem_ is way to get
// from nowhere to _there_, and to be _able_ (not forced) to replicate this
// original bootstrapping on any sort of tech base at any time whenever necessary.
//
// The initial inspiration / iteration for _atem_ was the elegantly minimalist
// [SAPL](https://github.com/metaleap/go-machines/tree/master/sapl) approach
// presented by Jansen / Koopman / Plasmeijer, but unlike the above-linked
// "by-the-paper" implementation, _atem_ diverges even in its initial form in
// various aspects and will continue to evolve various details in tandem with
// the birthing of _atmo_.
//
// SAPL's basics still apply for now: all funcs are top-level (no lambdas or
// other locals), as such support 0 - n args (rather than all-unary). There
// are no names: global funcs and, inside them, their args are referred to by
// integer indices. Thus most expressions are atomic: arg-refs, func-refs,
// and plain integers. The only non-atomic expression is call / application:
// it is composed of two sub-expressions, the callee and the arg. Divergences:
// our func-refs, if negative, denote a binary primitive-instruction op-code
// such as addition, multiply, equality-testing etc. that is handled natively
// by the interpreter. Unlike SAPL, our func-refs don't carry around their
// number-of-args, instead they're looked up in the `Prog`. For applications
// / calls, likely will move from the current unary style to n-ary, if feasible
// without breaking partial-application or degrading our overall LoCs aims.
package atem

import (
	"strconv"
)

// The few standard func defs the interpreter needs to know of as a minimum, and
// their inviolably hereby-decreed standard indices within a `Prog`. Every atem
// code generator must emit implementations for them all, and placed correctly.
const (
	// I combinator aka identity function
	StdFuncId ExprFuncRef = 0
	// K combinator aka konst aka boolish of true
	StdFuncTrue ExprFuncRef = 1
	// K I aka. boolish of false
	StdFuncFalse ExprFuncRef = 2
	// end of linked-list
	StdFuncNil ExprFuncRef = 3
	// next link in linked-list
	StdFuncCons ExprFuncRef = 4
)

type (
	Prog    []FuncDef
	FuncDef struct {
		// Args holds this `FuncDef`'s arguments: each `int` denotes how often the
		// `Body` references this arg (note that the interpreter does not currently
		// use this info), the arg's "identity" however is just its index in `Args`
		Args []int
		Body Expr
		Meta []string // ignored and not used in this lib: but still loaded from JSON and (re)emitted by `FuncDef.JsonSrc()`
	}
	Expr interface {
		// JsonSrc emits the re-`LoadFromJson`able representation of this `Expr`.
		JsonSrc() string
	}
	ExprNumInt  int
	ExprArgRef  int
	ExprFuncRef int
	ExprAppl    struct {
		Callee Expr
		Arg    Expr
	}
	exprCall struct {
		Callee Expr
		Args   []Expr
	}
)

// JsonSrc emits the re-`LoadFromJson`able representation of this `ExprNumInt`.
func (me ExprNumInt) JsonSrc() string { return strconv.Itoa(int(me)) }

// JsonSrc emits a non-re-`LoadFromJson`able representation of this `ExprArgRef`.
func (me ExprArgRef) JsonSrc() string { return "\"" + strconv.Itoa(int(-me)-1) + "\"" }

// JsonSrc emits the re-`LoadFromJson`able representation of this `ExprFuncRef`.
func (me ExprFuncRef) JsonSrc() string { return "[" + strconv.Itoa(int(me)) + "]" }

// JsonSrc emits the re-`LoadFromJson`able representation of this `ExprAppl`.
func (me *ExprAppl) JsonSrc() string { return "[" + me.Callee.JsonSrc() + ", " + me.Arg.JsonSrc() + "]" }

// JsonSrc emits the re-`LoadFromJson`able representation of this `FuncDef`.
func (me *FuncDef) JsonSrc(dropFuncDefMetas bool) string {
	outjson := "[ ["
	if !dropFuncDefMetas {
		for i, mstr := range me.Meta {
			if i > 0 {
				outjson += ","
			}
			outjson += strconv.Quote(mstr)
		}
	}
	outjson += "], ["
	for i, a := range me.Args {
		if i > 0 {
			outjson += ","
		}
		outjson += strconv.Itoa(a)
	}
	return outjson + "],\n\t\t" + me.Body.JsonSrc() + " ]"
}

// JsonSrc emits the re-`LoadFromJson`able representation of this `Prog`.
func (me Prog) JsonSrc(dropFuncDefMetas bool) string {
	outjson := "[ "
	for i, def := range me {
		if i > 0 {
			outjson += ", "
		}
		outjson += def.JsonSrc(dropFuncDefMetas) + "\n"
	}
	return outjson + "]\n"
}
