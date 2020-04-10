package main

const llmodule_default_target_datalayout = "e-m:e-i64:64-f80:128-n8:16:32:64-S128"
const llmodule_default_target_triple = "x86_64-unknown-linux-gnu"

func llModuleFrom(ast *Ast) LLModule {
	ret_mod := LLModule{
		target_datalayout: Str(llmodule_default_target_datalayout),
		target_triple:     Str(llmodule_default_target_triple),
		globals:           allocˇLLGlobal(len(ast.defs)),
		funcs:             allocˇLLFunc(len(ast.defs)),
	}
	num_globals, num_funcs, num_exts := 0, 0, 0
	for i := range ast.defs {
		top_def := &ast.defs[i]
		switch ll_sth := llTopLevelFrom(&top_def.body, top_def, ast).(type) {
		case LLGlobal:
			ll_sth.orig_ast_top_def = top_def
			ret_mod.globals[num_globals] = ll_sth
			num_globals++
		case LLFunc:
			ll_sth.orig_ast_top_def = top_def
			ret_mod.funcs[num_funcs] = ll_sth
			num_funcs++
		default:
			fail(string(astDefName(top_def)))
		}
		num_exts++
	}
	ret_mod.globals = ret_mod.globals[0:num_globals]
	ret_mod.funcs = ret_mod.funcs[0:num_funcs]
	for i := range ret_mod.funcs {
		if fn_def := &ret_mod.funcs[i]; !fn_def.external {
			lit_curl_blocks := fn_def.orig_ast_top_def.body.kind.(AstExprForm)[4].kind.(AstExprLitCurl)
			fn_def.basic_blocks = allocˇLLBasicBlock(len(lit_curl_blocks))
			for i := range lit_curl_blocks {
				fn_def.basic_blocks[i] = llBlockFrom(&lit_curl_blocks[i], fn_def.orig_ast_top_def, ast, &ret_mod)
			}
		}
	}
	return ret_mod
}

func llTopLevelFrom(expr *AstExpr, top_def *AstDef, ast *Ast) Any {
	switch it := expr.kind.(type) {
	case AstExprForm:
		callee := astExprSlashed(&it[0])
		assert(len(callee) != 0) // "expected /..."
		kwd := callee[0]
		if 1 == len(callee) {
			if astExprIsIdent(kwd, "global") {
				return llGlobalFrom(expr, top_def, ast)
			} else if astExprIsIdent(kwd, "declare") {
				return llFuncDeclFrom(expr, top_def, ast)
			} else if astExprIsIdent(kwd, "define") {
				return llFuncDefFrom(expr, top_def, ast)
			}
		}
	}
	return nil
}

func llGlobalFrom(full_expr *AstExpr, top_def *AstDef, ast *Ast) LLGlobal {
	expr_form := full_expr.kind.(AstExprForm)
	assert(len(expr_form) == 3)
	lit_curl_opts := expr_form[2].kind.(AstExprLitCurl)
	maybe_constant := astExprsFindKeyedValue(lit_curl_opts, "#constant", ast)
	maybe_external := astExprsFindKeyedValue(lit_curl_opts, "#external", ast)
	assert(maybe_constant != nil || maybe_external != nil)
	assert(!(maybe_constant != nil && maybe_external != nil))

	ret_global := LLGlobal{
		external: maybe_external != nil,
		constant: maybe_constant != nil,
		name:     astDefName(top_def),
		ty:       llTypeFrom(astExprSlashed(&expr_form[1]), full_expr, ast),
	}
	if maybe_external != nil {
		ret_global.name = astExprTaggedIdent(maybe_external)
		assert(len(ret_global.name) != 0)
	} else if maybe_constant != nil {
		ret_global.initializer = llExprFrom(maybe_constant, ast, nil)
		_, is_lit_str := ret_global.initializer.(LLExprLitStr)
		assert(is_lit_str)
	}
	return ret_global
}

func llFuncDeclFrom(full_expr *AstExpr, top_def *AstDef, ast *Ast) LLFunc {
	expr_form := full_expr.kind.(AstExprForm)
	_ = expr_form[2].kind.(AstExprLitCurl)
	lit_curl_params := expr_form[4].kind.(AstExprLitCurl)
	ret_func := LLFunc{
		external:     true,
		ty:           llTypeFrom(astExprSlashed(&expr_form[3]), full_expr, ast),
		name:         astExprTaggedIdent(&expr_form[1]),
		params:       allocˇLLFuncParam(len(lit_curl_params)),
		basic_blocks: nil,
	}
	for i := range lit_curl_params {
		lhs, rhs := astExprFormSplit(&lit_curl_params[i], ":", true, true, true, ast)
		ret_func.params[i].name = astExprTaggedIdent(lhs)
		assert(len(ret_func.params[i].name) != 0) // source lang must provide param names for self-doc reasons...
		ret_func.params[i].name = nil             // ...but destination lang (LLVM-IR) doesn't need them
		ret_func.params[i].ty = llTypeFrom(astExprSlashed(rhs), rhs, ast)
	}
	assert(len(ret_func.name) != 0)
	return ret_func
}

func llFuncDefFrom(full_expr *AstExpr, top_def *AstDef, ast *Ast) LLFunc {
	expr_form := full_expr.kind.(AstExprForm)
	assert(len(expr_form) == 5)
	_ = expr_form[1].kind.(AstExprLitCurl)
	lit_curl_params := expr_form[3].kind.(AstExprLitCurl)
	_ = expr_form[4].kind.(AstExprLitCurl)
	ret_func := LLFunc{
		name:         astDefName(top_def),
		external:     false,
		ty:           llTypeFrom(astExprSlashed(&expr_form[2]), &expr_form[2], ast),
		params:       allocˇLLFuncParam(len(lit_curl_params)),
		basic_blocks: nil,
	}
	for i := range lit_curl_params {
		lhs, rhs := astExprFormSplit(&lit_curl_params[i], ":", true, true, true, ast)
		ret_func.params[i].name = astExprTaggedIdent(lhs)
		assert(len(ret_func.params[i].name) != 0)
		ret_func.params[i].ty = llTypeFrom(astExprSlashed(rhs), rhs, ast)
	}
	assert(len(ret_func.name) != 0)
	return ret_func
}

func llBlockFrom(pair_expr *AstExpr, top_def *AstDef, ast *Ast, ll_mod *LLModule) LLBasicBlock {
	lhs, rhs := astExprFormSplit(pair_expr, ":", true, true, true, ast)
	lit_clip_instrs := rhs.kind.(AstExprLitClip)
	ret_block := LLBasicBlock{name: astExprTaggedIdent(lhs), instrs: allocˇLLInstr(len(lit_clip_instrs))}
	if len(ret_block.name) == 0 {
		counter++
		ret_block.name = uintToStr(counter, 10, 1, Str("b."))
	}
	for i := range lit_clip_instrs {
		ret_block.instrs[i] = llInstrFrom(&lit_clip_instrs[i], top_def, ast, ll_mod)
	}
	return ret_block
}

func llInstrFrom(expr *AstExpr, top_def *AstDef, ast *Ast, ll_mod *LLModule) LLInstr {
	switch it := expr.kind.(type) {
	case AstExprForm:
		callee := astExprSlashed(expr)
		if 1 == len(callee) {
			kwd := callee[0]
			if astExprIsIdent(kwd, "unreachable") {
				return LLInstrUnreachable{}
			}
		}
		callee = astExprSlashed(&it[0])
		if 1 == len(callee) {
			kwd := callee[0]
			if astExprIsIdent(kwd, "ret") {
				assert(len(it) == 2)
				return LLInstrRet{expr: llExprFrom(&it[1], ast, ll_mod).(LLExprTyped)}
			} else if astExprIsIdent(kwd, "let") {
				assert(len(it) == 2)
				lit_curl_pair := it[1].kind.(AstExprLitCurl)
				assert(len(lit_curl_pair) == 1)
				lhs, rhs := astExprFormSplit(&lit_curl_pair[0], ":", true, true, true, ast)

				var instr LLInstr
				if maybe_form, _ := rhs.kind.(AstExprForm); len(maybe_form) == 2 {
					if slashed := astExprSlashed(&maybe_form[0]); len(slashed) == 1 {
						if ident, _ := slashed[0].kind.(AstExprIdent); len(ident) != 0 && ident[0] == 'I' {
							expr_ty := llExprFrom(rhs, ast, ll_mod).(LLExprTyped)
							instr = LLInstrBinOp{
								ty:      expr_ty.ty,
								lhs:     expr_ty.expr,
								rhs:     LLExprLitInt(0),
								op_kind: ll_bin_op_add,
							}
						}
					}
				}
				if instr == nil {
					instr = llInstrFrom(rhs, top_def, ast, ll_mod)
				}
				return LLInstrLet{
					name:  astExprTaggedIdent(lhs),
					instr: instr,
				}
			} else if astExprIsIdent(kwd, "load") {
				assert(len(it) == 4)
				_ = it[1].kind.(AstExprLitCurl)
				ret_load := LLInstrLoad{
					ty:   llTypeFrom(astExprSlashed(&it[2]), expr, ast),
					expr: llExprFrom(&it[3], ast, ll_mod).(LLExprTyped),
				}
				return ret_load
			} else if astExprIsIdent(kwd, "call") {
				assert(len(it) == 5)
				_ = it[1].kind.(AstExprLitCurl)
				lit_clip_args := it[4].kind.(AstExprLitClip)
				ret_call := LLInstrCall{
					ty:     llTypeFrom(astExprSlashed(&it[3]), expr, ast),
					callee: llExprFrom(&it[2], ast, ll_mod),
					args:   allocˇLLExprTyped(len(lit_clip_args)),
				}
				for i := range lit_clip_args {
					ret_call.args[i] = llExprFrom(&lit_clip_args[i], ast, ll_mod).(LLExprTyped)
				}
				return ret_call
			} else if astExprIsIdent(kwd, "switch") {
				assert(len(it) == 4)
				lit_curl_cases := it[3].kind.(AstExprLitCurl)
				ret_switch := LLInstrSwitch{
					comparee:           llExprFrom(&it[1], ast, ll_mod).(LLExprTyped),
					default_block_name: astExprTaggedIdent(&it[2]),
					cases:              allocˇLLSwitchCase(len(lit_curl_cases)),
				}
				for i := range lit_curl_cases {
					lhs, rhs := astExprFormSplit(&lit_curl_cases[i], ":", true, true, true, ast)
					ret_switch.cases[i].expr = llExprFrom(lhs, ast, ll_mod).(LLExprTyped)
					ret_switch.cases[i].block_name = astExprTaggedIdent(rhs)
				}
				return ret_switch
			} else if astExprIsIdent(kwd, "gep") {
				assert(len(it) == 4)
				lit_clip_idxs := it[3].kind.(AstExprLitClip)
				ret_gep := LLInstrGep{
					ty:       llTypeFrom(astExprSlashed(&it[1]), expr, ast),
					base_ptr: llExprFrom(&it[2], ast, ll_mod).(LLExprTyped),
					indices:  allocˇLLExprTyped(len(lit_clip_idxs)),
				}
				for i := range lit_clip_idxs {
					ret_gep.indices[i] = llExprFrom(&lit_clip_idxs[i], ast, ll_mod).(LLExprTyped)
				}
				return ret_gep
			} else if astExprIsIdent(kwd, "brTo") {
				assert(len(it) == 2)
				return LLInstrBrTo{
					block_name: astExprTaggedIdent(&it[1]),
				}
			} else if astExprIsIdent(kwd, "brIf") {
				assert(len(it) == 4)
				return LLInstrBrIf{
					cond:                llExprFrom(&it[1], ast, ll_mod),
					block_name_if_true:  astExprTaggedIdent(&it[2]),
					block_name_if_false: astExprTaggedIdent(&it[3]),
				}
			} else if astExprIsIdent(kwd, "intToPtr") {
				assert(len(it) == 4)
				return LLInstrIntToPtr{
					ty:   llTypeFrom(astExprSlashed(&it[2]), expr, ast),
					expr: llExprFrom(&it[3], ast, ll_mod).(LLExprTyped),
				}
			} else if astExprIsIdent(kwd, "phi") {
				assert(len(it) == 4)
				_ = it[1].kind.(AstExprLitCurl)
				lit_curl_preds := it[3].kind.(AstExprLitCurl)
				ret_phi := LLInstrPhi{
					ty:           llTypeFrom(astExprSlashed(&it[2]), expr, ast),
					predecessors: allocˇLLPhiPred(len(lit_curl_preds)),
				}
				for i := range lit_curl_preds {
					lhs, rhs := astExprFormSplit(&lit_curl_preds[i], ":", true, true, true, ast)
					ret_phi.predecessors[i] = LLPhiPred{
						block_name: astExprTaggedIdent(lhs),
						expr:       llExprFrom(rhs, ast, ll_mod),
					}
					assert(len(ret_phi.predecessors[i].block_name) != 0)
				}
				return ret_phi
			} else if astExprIsIdent(kwd, "alloca") {
				assert(len(it) == 4)
				ret_alloca := LLInstrAlloca{
					ty:        llTypeFrom(astExprSlashed(&it[2]), expr, ast),
					num_elems: llExprFrom(&it[3], ast, ll_mod).(LLExprTyped),
				}
				return ret_alloca
			} else if astExprIsIdent(kwd, "icmp") {
				assert(len(it) == 5)
				ret_cmp := LLInstrCmpI{
					ty:  llTypeFrom(astExprSlashed(&it[2]), expr, ast),
					lhs: llExprFrom(&it[3], ast, ll_mod),
					rhs: llExprFrom(&it[4], ast, ll_mod),
				}
				cmp_kind := astExprTaggedIdent(&it[1])
				if strEql(cmp_kind, Str("eq")) {
					ret_cmp.cmp_kind = ll_cmp_i_eq
				} else if strEql(cmp_kind, Str("ne")) {
					ret_cmp.cmp_kind = ll_cmp_i_ne
				} else if strEql(cmp_kind, Str("ugt")) {
					ret_cmp.cmp_kind = ll_cmp_i_ugt
				} else if strEql(cmp_kind, Str("uge")) {
					ret_cmp.cmp_kind = ll_cmp_i_uge
				} else if strEql(cmp_kind, Str("ult")) {
					ret_cmp.cmp_kind = ll_cmp_i_ult
				} else if strEql(cmp_kind, Str("ule")) {
					ret_cmp.cmp_kind = ll_cmp_i_ule
				} else if strEql(cmp_kind, Str("sgt")) {
					ret_cmp.cmp_kind = ll_cmp_i_sgt
				} else if strEql(cmp_kind, Str("sge")) {
					ret_cmp.cmp_kind = ll_cmp_i_sge
				} else if strEql(cmp_kind, Str("slt")) {
					ret_cmp.cmp_kind = ll_cmp_i_slt
				} else if strEql(cmp_kind, Str("sle")) {
					ret_cmp.cmp_kind = ll_cmp_i_sle
				}
				assert(ret_cmp.cmp_kind != 0)
				return ret_cmp
			} else if astExprIsIdent(kwd, "op2") {
				assert(len(it) == 5)
				ret_op2 := LLInstrBinOp{
					ty:  llTypeFrom(astExprSlashed(&it[2]), expr, ast),
					lhs: llExprFrom(&it[3], ast, ll_mod),
					rhs: llExprFrom(&it[4], ast, ll_mod),
				}
				op_kind := astExprTaggedIdent(&it[1])
				if strEql(op_kind, Str("add")) {
					ret_op2.op_kind = ll_bin_op_add
				} else if strEql(op_kind, Str("udiv")) {
					ret_op2.op_kind = ll_bin_op_udiv
				}
				assert(ret_op2.op_kind != 0)
				return ret_op2
			}
		}
	}
	panic(string(astNodeSrcStr(&expr.base, ast)))
}

func llTypeFrom(expr_callee_slashed []*AstExpr, full_expr_for_err_msg *AstExpr, ast *Ast) LLType {
	if len(expr_callee_slashed) == 0 {
		return nil
	}
	ident, _ := expr_callee_slashed[0].kind.(AstExprIdent)
	if len(ident) == 0 {
		return nil
	}
	if ident[0] == 'P' && len(ident) == 1 {
		sub_type := llTypeFrom(expr_callee_slashed[1:], full_expr_for_err_msg, ast)
		if sub_type == nil {
			sub_type = LLTypeInt{bit_width: 8}
		}
		return LLTypePtr{ty: sub_type}
	} else if ident[0] == 'V' && len(ident) == 1 {
		return LLTypeVoid{}
	} else if ident[0] == 'A' && len(ident) == 1 && len(expr_callee_slashed) >= 3 {
		sub_type := llTypeFrom(expr_callee_slashed[2:], full_expr_for_err_msg, ast)
		if sub_type != nil {
			size_part_lit_int_str := astNodeSrcStr(&expr_callee_slashed[1].base, ast)
			return LLTypeArr{ty: sub_type, size: uintFromStr(size_part_lit_int_str)}
		}
	} else if ident[0] == 'I' {
		var bit_width uint64 = 64
		if len(ident) > 1 {
			bit_width = uintFromStr(ident[1:])
		}
		return LLTypeInt{bit_width: bit_width}
	}
	fail("expected prim type in: ", astNodeSrcStr(&full_expr_for_err_msg.base, ast))
	return nil
}

func llExprFrom(expr *AstExpr, ast *Ast, ll_mod *LLModule) LLExpr {
	switch it := expr.kind.(type) {
	case AstExprLitInt:
		return LLExprLitInt(it)
	case AstExprLitStr:
		return LLExprLitStr(it)
	case AstExprIdent:
		if len(it) == 1 && it[0] == '#' {
			return LLExprLitVoid{}
		}
		panic(string(it))
	case AstExprForm:
		slashed := astExprSlashed(&it[0])
		if len(slashed) != 0 {
			kwd := slashed[0]
			ident, _ := kwd.kind.(AstExprIdent)
			if len(ident) != 0 {
				if len(it) == 2 && ident[0] >= 'A' && ident[0] <= 'Z' {
					ret_expr := LLExprTyped{ty: llTypeFrom(slashed, expr, ast), expr: llExprFrom(&it[1], ast, ll_mod)}
					return ret_expr
				}
			}
		}

		if strEql(astNodeSrcStr(&it[0].base, ast), Str("/@")) {
			assert(len(it) == 2)
			name := it[1].kind.(AstExprIdent) // the usual / default case
			if ll_mod != nil {                // check whether referenced def is external
				found := false
				for i := 0; i < len(ll_mod.funcs) && !found; i++ {
					llf := &ll_mod.funcs[i]
					if llf.external && strEql(astDefName(llf.orig_ast_top_def), name) {
						name = llf.name
						found = true
					}
				}
				for i := 0; i < len(ll_mod.globals) && !found; i++ {
					llg := &ll_mod.globals[i]
					if llg.external && strEql(astDefName(llg.orig_ast_top_def), name) {
						name = llg.name
						break
					}
				}
			}
			return LLExprIdentGlobal(name)
		}

		tag_lit := astExprTaggedIdent(expr)
		if tag_lit != nil {
			return LLExprIdentLocal(tag_lit)
		}
	}
	panic(expr.kind)
}
