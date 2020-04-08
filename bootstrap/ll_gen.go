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
	num_globals, num_funcs := 0, 0
	for i := range ast.defs {
		top_def := &ast.defs[i]
		switch ll_sth := llNodeFrom(&top_def.body, top_def, ast).(type) {
		case LLGlobal:
			ret_mod.globals[num_globals] = ll_sth
			num_globals++
		case LLFunc:
			ret_mod.funcs[num_funcs] = ll_sth
			num_funcs++
		default:
			println(string(astDefName(top_def)), ll_sth)
		}
	}
	ret_mod.globals = ret_mod.globals[0:num_globals]
	ret_mod.funcs = ret_mod.funcs[0:num_funcs]
	return ret_mod
}

func llNodeFrom(expr *AstExpr, top_def *AstDef, ast *Ast) Any {
	switch it := expr.kind.(type) {
	case AstExprForm:
		callee := astExprSlashed(&it[0])
		if len(callee) == 0 {
			fail("expected some '/...' prim call in:\n", astNodeSrcStr(&expr.base, ast))
		}
		if 1 == len(callee) {
			if astExprIsIdent(callee[0], "global") {
				return llGlobalFrom(expr, top_def, ast)
			} else if astExprIsIdent(callee[0], "declare") {
				return llFuncDeclFrom(expr, top_def, ast)
			} else if astExprIsIdent(callee[0], "define") {
				return llFuncDefFrom(expr, top_def, ast)
			}
		}
		if ident, is_ident := callee[0].kind.(AstExprIdent); is_ident && ident[0] >= 'A' && ident[0] <= 'Z' {
			return llTypeFrom(callee, expr, ast)
		}
	}
	return llExprFrom(expr, ast)
}

func llGlobalFrom(full_expr *AstExpr, top_def *AstDef, ast *Ast) LLGlobal {
	expr_form := full_expr.kind.(AstExprForm)
	assert(len(expr_form) == 3)
	lit_curl := expr_form[2].kind.(AstExprLitCurl)
	maybe_constant := astExprsFindKeyedValue(lit_curl, "#constant", ast)
	maybe_external := astExprsFindKeyedValue(lit_curl, "#external", ast)
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
		ret_global.initializer = llExprFrom(maybe_constant, ast)
		_, is_lit_str := ret_global.initializer.(LLExprLitStr)
		assert(is_lit_str)
	}
	return ret_global
}

func llFuncDeclFrom(full_expr *AstExpr, top_def *AstDef, ast *Ast) LLFunc {
	expr_form := full_expr.kind.(AstExprForm)
	lit_curl_opts, _ := expr_form[2].kind.(AstExprLitCurl)
	assert(lit_curl_opts != nil)
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
	lit_curl_opts, _ := expr_form[1].kind.(AstExprLitCurl)
	assert(lit_curl_opts != nil)
	lit_curl_params := expr_form[3].kind.(AstExprLitCurl)
	lit_curl_blocks := expr_form[4].kind.(AstExprLitCurl)
	ret_func := LLFunc{
		name:         astDefName(top_def),
		external:     false,
		ty:           llTypeFrom(astExprSlashed(&expr_form[2]), &expr_form[2], ast),
		params:       allocˇLLFuncParam(len(lit_curl_params)),
		basic_blocks: allocˇLLBasicBlock(len(lit_curl_blocks)),
	}
	for i := range lit_curl_params {
		lhs, rhs := astExprFormSplit(&lit_curl_params[i], ":", true, true, true, ast)
		ret_func.params[i].name = astExprTaggedIdent(lhs)
		assert(len(ret_func.params[i].name) != 0)
		ret_func.params[i].ty = llTypeFrom(astExprSlashed(rhs), rhs, ast)
	}
	for i := range lit_curl_blocks {
		ret_func.basic_blocks[i] = llBlockFrom(&lit_curl_blocks[i], ast)
	}
	assert(len(ret_func.name) != 0)
	return ret_func
}

func llBlockFrom(pair_expr *AstExpr, ast *Ast) LLBasicBlock {
	lhs, rhs := astExprFormSplit(pair_expr, ":", true, true, true, ast)
	lit_clip := rhs.kind.(AstExprLitClip)
	ret_block := LLBasicBlock{name: astExprTaggedIdent(lhs), stmts: allocˇLLStmt(len(lit_clip))}
	if len(ret_block.name) == 0 {
		counter++
		ret_block.name = uintToStr(counter, 10, 1, Str("blk."))
	}
	for i := range lit_clip {
		ret_block.stmts[i] = llStmtFrom(&lit_clip[i], ast)
	}
	return ret_block
}

func llStmtFrom(stmt_expr *AstExpr, ast *Ast) LLStmt {
	return LLStmtComment{comment_text: astNodeSrcStr(&stmt_expr.base, ast)}
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

func llExprFrom(expr *AstExpr, ast *Ast) LLExpr {
	switch it := expr.kind.(type) {
	case AstExprLitStr:
		return LLExprLitStr(it)
	}
	return nil
}

func llIdentFrom(expr *AstExpr, must_prefix byte, ast *Ast) Str {
	src_str := astNodeSrcStr(&expr.base, ast)
	if must_prefix != 0 && len(src_str) != 0 {
		assert(len(src_str) != 0)
		assert(src_str[0] == must_prefix)
	}
	if len(src_str) == 0 || (len(src_str) == 1 && src_str[0] == must_prefix) {
		counter++
		src_str = uintToStr(counter, 10, 1, Str("-$-"))
	}
	ret_str := allocˇu8(len(src_str))
	for i, c := range src_str {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c == '-' || c == '$' || (c >= '0' && c <= '9' && i > 0) {
			ret_str[i] = c
		} else {
			ret_str[i] = '.'
		}
	}
	return ret_str
}
