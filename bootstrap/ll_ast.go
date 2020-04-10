package main

type LLType interface{ implementsLLType() }
type LLExpr interface{ implementsLLExpr() }
type LLInstr interface{ implementsLLInstr() }

type LLModule struct {
	target_datalayout Str
	target_triple     Str
	globals           []LLGlobal
	funcs             []LLFunc
}

type LLGlobal struct {
	orig_ast_top_def *AstDef
	name             Str
	constant         bool
	external         bool
	ty               LLType
	initializer      LLExpr
}

type LLFunc struct {
	orig_ast_top_def *AstDef
	external         bool
	ty               LLType
	name             Str
	params           []LLFuncParam
	basic_blocks     []LLBasicBlock
}

type LLFuncParam struct {
	name Str
	ty   LLType
}

type LLBasicBlock struct {
	name   Str
	instrs []LLInstr
}

type LLInstrLet struct {
	name  Str
	instr LLInstr
}

type LLInstrRet struct {
	expr LLExprTyped
}

type LLInstrUnreachable struct{}

type LLInstrSwitch struct {
	comparee           LLExprTyped
	default_block_name Str
	cases              []LLSwitchCase
}

type LLSwitchCase struct {
	expr       LLExprTyped
	block_name Str
}

type LLInstrBrTo struct {
	block_name Str
}

type LLInstrBrIf struct {
	cond                LLExpr
	block_name_if_true  Str
	block_name_if_false Str
}

type LLInstrIntToPtr struct {
	ty   LLType
	expr LLExprTyped
}

type LLInstrComment struct {
	comment_text Str
}

type LLExprIdentLocal Str

type LLExprIdentGlobal Str

type LLExprLitInt uint64

type LLExprLitStr Str

type LLExprLitVoid struct{}

type LLExprTyped struct {
	ty   LLType
	expr LLExpr
}

type LLInstrAlloca struct {
	ty        LLType
	num_elems LLExprTyped
}

type LLInstrLoad struct {
	ty   LLType
	expr LLExprTyped
}

type LLInstrCall struct {
	ty     LLType
	callee LLExpr
	args   []LLExprTyped
}

type LLInstrBinOp struct {
	ty      LLType
	lhs     LLExpr
	rhs     LLExpr
	op_kind LLBinOpKind
}

type LLBinOpKind int

const (
	_ LLBinOpKind = iota
	ll_bin_op_add
	ll_bin_op_udiv
)

type LLInstrCmpI struct {
	ty       LLType
	lhs      LLExpr
	rhs      LLExpr
	cmp_kind LLCmpIKind
}

type LLCmpIKind int

const (
	_ LLCmpIKind = iota
	ll_cmp_i_eq
	ll_cmp_i_ne
	ll_cmp_i_ugt
	ll_cmp_i_uge
	ll_cmp_i_ult
	ll_cmp_i_ule
	ll_cmp_i_sgt
	ll_cmp_i_sge
	ll_cmp_i_slt
	ll_cmp_i_sle
)

type LLInstrPhi struct {
	ty           LLType
	predecessors []LLPhiPred
}

type LLPhiPred struct {
	block_name Str
	expr       LLExpr
}

type LLInstrGep struct {
	ty       LLType
	base_ptr LLExprTyped
	indices  []LLExprTyped
}

type LLTypeInt struct {
	bit_width uint64 // u23 really.. we save us some casts here
}

type LLTypeVoid struct{}

type LLTypePtr struct {
	ty LLType
}

type LLTypeArr struct {
	ty   LLType
	size uint64
}

type LLTypeStruct struct {
	fields []LLType
}

type LLTypeFunc struct {
	ty     LLType
	params []LLType
}

func llTypeEql(t1 LLType, t2 LLType) bool {
	assert(t1 != nil && t2 != nil)
	switch tl := t1.(type) {
	case LLTypeVoid:
		_, ok := t2.(LLTypeVoid)
		return ok
	case LLTypeInt:
		if tr, ok := t2.(LLTypeInt); ok {
			return tl.bit_width == tr.bit_width
		}
	case LLTypePtr:
		if tr, ok := t2.(LLTypePtr); ok {
			return llTypeEql(tl.ty, tr.ty)
		}
	case LLTypeArr:
		if tr, ok := t2.(LLTypeArr); ok {
			return tl.size == tr.size && llTypeEql(tl.ty, tr.ty)
		}
	case LLTypeStruct:
		if tr, ok := t2.(LLTypeStruct); ok && len(tl.fields) == len(tr.fields) {
			for i, tl_field_ty := range tl.fields {
				if !llTypeEql(tl_field_ty, tr.fields[i]) {
					return false
				}
			}
			return true
		}
	case LLTypeFunc:
		if tr, ok := t2.(LLTypeFunc); ok && len(tl.params) == len(tr.params) && llTypeEql(tl.ty, tr.ty) {
			for i, tl_param_ty := range tl.params {
				if !llTypeEql(tl_param_ty, tr.params[i]) {
					return false
				}
			}
			return true
		}
	}
	return false
}

func (LLTypeArr) implementsLLType()    {}
func (LLTypeFunc) implementsLLType()   {}
func (LLTypeInt) implementsLLType()    {}
func (LLTypePtr) implementsLLType()    {}
func (LLTypeStruct) implementsLLType() {}
func (LLTypeVoid) implementsLLType()   {}

func (LLExprIdentGlobal) implementsLLExpr() {}
func (LLExprIdentLocal) implementsLLExpr()  {}
func (LLExprLitInt) implementsLLExpr()      {}
func (LLExprLitStr) implementsLLExpr()      {}
func (LLExprLitVoid) implementsLLExpr()     {}
func (LLExprTyped) implementsLLExpr()       {}

func (LLInstrAlloca) implementsLLInstr()      {}
func (LLInstrBinOp) implementsLLInstr()       {}
func (LLInstrCall) implementsLLInstr()        {}
func (LLInstrCmpI) implementsLLInstr()        {}
func (LLInstrGep) implementsLLInstr()         {}
func (LLInstrLoad) implementsLLInstr()        {}
func (LLInstrPhi) implementsLLInstr()         {}
func (LLInstrBrIf) implementsLLInstr()        {}
func (LLInstrBrTo) implementsLLInstr()        {}
func (LLInstrComment) implementsLLInstr()     {}
func (LLInstrLet) implementsLLInstr()         {}
func (LLInstrRet) implementsLLInstr()         {}
func (LLInstrSwitch) implementsLLInstr()      {}
func (LLInstrUnreachable) implementsLLInstr() {}
func (LLInstrIntToPtr) implementsLLInstr()    {}
