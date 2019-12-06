package atem

import (
	"os"
)

// OpCode denotes a "primitive instruction", eg. one that is hardcoded in the
// interpreter and invoked when encountering a call to a negative `ExprFuncRef`
// with at least 2 operands on the current `Eval` stack. All `OpCode`-denoted
// primitive instructions consume always exactly 2 operands from said stack.
type OpCode int

const (
	// Addition of 2 `ExprNumInt`s, result 1 `ExprNumInt`
	OpAdd OpCode = -1
	// Subtraction of 2 `ExprNumInt`s, result 1 `ExprNumInt`
	OpSub OpCode = -2
	// Multiplication of 2 `ExprNumInt`s, result 1 `ExprNumInt`
	OpMul OpCode = -3
	// Division of 2 `ExprNumInt`s, result 1 `ExprNumInt`
	OpDiv OpCode = -4
	// Modulo of 2 `ExprNumInt`s, result 1 `ExprNumInt`
	OpMod OpCode = -5
	// Equality test between 2 `Expr`s, result is `StdFuncTrue` or `StdFuncFalse`
	OpEq OpCode = -6
	// Less-than test between 2 `ExprNumInt`s, result is `StdFuncTrue` or `StdFuncFalse`
	OpLt OpCode = -7
	// Greater-than test between 2 `ExprNumInt`s, result is `StdFuncTrue` or `StdFuncFalse`
	OpGt OpCode = -8
	// Writes both `Expr`s (the first one a string-ish `StdFuncCons`tructed linked-list of `ExprNumInt`s) to `OpPrtDst`, result is the right-hand-side `Expr` of the 2 input `Expr` operands
	OpPrt OpCode = -42
)

// OpPrtDst is the output destination for all `OpPrt` primitive instructions.
// Must never be `nil` during any `Prog`s that do potentially invoke `OpPrt`.
var OpPrtDst = os.Stderr.Write

// Eval operates thusly, keeping an internal `stack` of `[]Expr`:
//
// - encountering an `ExprCall`, its `Args` are `append`ed to the `stack` and
// its `Callee` is then `Eval`'d;
//
// - encountering an `ExprFuncRef`, the `stack` is checked for having the
// proper minimum required `len` with regard to the referenced `FuncDef`'s
// number of `Args`. If okay, the pertinent number of args is taken (and
// removed) from the `stack` and the referenced `FuncDef`'s `Body`, rewritten
// with all inner `ExprArgRef`s (including those inside `ExprCall`s) resolved
// to the `stack` entries, is `Eval`'d (with the appropriately reduced `stack`);
//
// - encountering  any other `Expr` type, it is merely returned if the `stack`
// is empty, else a `panic` with the `stack` signals that from the input
// `expr` a non-callable value ended up as the callee of an `ExprCall`.
//
// Corner cases for the `ExprFuncRef` situation: if the `stack` has too small a
// `len`, either an `ExprCall` representing the partial-application closure is
// returned, or just the `ExprFuncRef` in case of a totally empty `stack`;
// if the `ExprFuncRef` is negative and thus referring to a primitive-instruction
// `OpCode`, 2 is the expected minimum required `len` for the `stack` and if
// this is met, the primitive instruction is carried out, its `Expr` result
// then being `Eval`'d with the reduced-by-2 `stack`. Unknown op-codes `panic`
// with a `[3]Expr` of first the `OpCode`-referencing `ExprFuncRef` followed
// by both its operands.
func (me Prog) Eval(expr Expr) Expr { return me.eval(expr, make([]Expr, 0, 1024)) }

func (me Prog) eval(expr Expr, stack []Expr) Expr {
	var lastcall *ExprCall
	for again := true; again; {
		again = false
		switch it := expr.(type) {
		case *ExprCall:
			stack = append(stack, it.Args...)
			again, expr, lastcall = true, it.Callee, it
		case ExprFuncRef:
			numargs, isopcode := 2, (it < 0)
			if !isopcode {
				numargs = len(me[it].Args)
			}
			if len(stack) < numargs { // not enough args on stack:
				if len(stack) > 0 { // then a closure value results
					if len(stack) > len(lastcall.Args) {
						expr, lastcall = &ExprCall{noArgRefs: lastcall.noArgRefs, Callee: it, Args: stack}, nil
					} else {
						expr, lastcall = lastcall, nil
					}
				}
			} else if isopcode {
				lhs, rhs := me.eval(stack[len(stack)-1], nil), me.eval(stack[len(stack)-2], nil)
				switch opcode := OpCode(it); opcode {
				case OpAdd:
					expr = lhs.(ExprNumInt) + rhs.(ExprNumInt)
				case OpSub:
					expr = lhs.(ExprNumInt) - rhs.(ExprNumInt)
				case OpMul:
					expr = lhs.(ExprNumInt) * rhs.(ExprNumInt)
				case OpDiv:
					expr = lhs.(ExprNumInt) / rhs.(ExprNumInt)
				case OpMod:
					expr = lhs.(ExprNumInt) % rhs.(ExprNumInt)
				case OpEq:
					if expr = StdFuncFalse; me.Eq(lhs, rhs, true) {
						expr = StdFuncTrue
					}
				case OpGt, OpLt:
					lt, l, r := (opcode == OpLt), lhs.(ExprNumInt), rhs.(ExprNumInt)
					if expr = StdFuncFalse; (lt && l < r) || ((!lt) && l > r) {
						expr = StdFuncTrue
					}
				case OpPrt:
					expr = rhs
					_, _ = OpPrtDst(append(append(append(ListToBytes(me.ListOfExprs(lhs)), '\t'), me.ListOfExprsToString(rhs)...), '\n'))
				default:
					panic([3]Expr{it, lhs, rhs})
				}
				again, stack = true, stack[:len(stack)-2]
			} else {
				if expr = me[it].Body; me[it].hasArgRefs {
					expr = me.exprRewrittenWithArgRefsResolvedToStackEntries(expr, stack)
				}
				again, stack = true, stack[:len(stack)-numargs]
			}
		default:
			if len(stack) != 0 {
				panic(stack)
			}
		}
	}
	return expr
}

func (me Prog) exprRewrittenWithArgRefsResolvedToStackEntries_NonOptimized(expr Expr, stack []Expr) Expr {
	switch it := expr.(type) {
	case ExprArgRef:
		return stack[len(stack)+int(it)]
	case *ExprCall:
		if it.noArgRefs {
			return it
		}
		callee := me.exprRewrittenWithArgRefsResolvedToStackEntries_NonOptimized(it.Callee, stack)
		call := &ExprCall{noArgRefs: true, Args: make([]Expr, len(it.Args)), Callee: callee}
		for i := len(call.Args) - 1; i > -1; i-- {
			call.Args[i] = me.exprRewrittenWithArgRefsResolvedToStackEntries_NonOptimized(it.Args[i], stack)
		}
		return call
	}
	return expr
}

var NumDrops int

func (me Prog) exprRewrittenWithArgRefsResolvedToStackEntries(expr Expr, stack []Expr) Expr {
	switch it := expr.(type) {
	case ExprArgRef:
		return stack[len(stack)+int(it)]
	case *ExprCall:
		if it.noArgRefs {
			return it
		}
		callee := me.exprRewrittenWithArgRefsResolvedToStackEntries_NonOptimized(it.Callee, stack)
		arsgdiff, call := 0, &ExprCall{noArgRefs: true, Args: make([]Expr, len(it.Args)), Callee: callee}
		fnref, okf := callee.(ExprFuncRef)
		if !okf {
			if subcall, okc := callee.(*ExprCall); okc {
				if fnref, _ = subcall.Callee.(ExprFuncRef); fnref != 0 {
					arsgdiff = len(subcall.Args)
				}
			}
		}
		jmax, neverdrop := -1, fnref <= 0 || me[fnref].allArgsUsed
		if fnref >= 0 {
			jmax = len(me[fnref].Args) - 1
		}
		for j, i := arsgdiff, len(call.Args)-1; i > -1; j, i = j+1, i-1 {
			if neverdrop || j > jmax || me[fnref].Args[j] != 0 {
				call.Args[i] = me.exprRewrittenWithArgRefsResolvedToStackEntries(it.Args[i], stack)
			}
		}
		return call
	}
	return expr
}
