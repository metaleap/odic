#pragma once
#include "metaleap.h"
#include "at_toks.h"
#include "at_ast.h"




AstExpr parseExprLitInt(Uint const all_toks_idx, Ast const* const ast, Token const* const tok) {
    AstExpr ret_expr = astExpr(all_toks_idx, 1, ast_expr_lit_int);
    ºU64 const maybe = uintParse(tokSrc(tok, ast->src));
    if (!maybe.ok)
        panic(astNodeMsg(str("malformed integer literal"), &ret_expr.node_base, ast));
    ret_expr.kind_lit_int = maybe.it;
    return ret_expr;
}

AstExpr parseExprLitStr(Uint const all_toks_idx, Ast const* const ast, Token const* const tok, U8 const quote_char) {
    AstExpr ret_expr = astExpr(all_toks_idx + 1, 1, ast_expr_lit_str);
    Str const lit_src = tokSrc(tok, ast->src);

    assert(lit_src.len >= 2 && lit_src.at[0] == quote_char && lit_src.at[lit_src.len - 1] == quote_char);
    Str ret_str = newStr(0, lit_src.len - 1);
    for (Uint i = 1; i < lit_src.len - 1; i += 1) {
        if (lit_src.at[i] != '\\')
            ret_str.at[ret_str.len] = lit_src.at[i];
        else {
            Uint const idx_end = i + 4;
            Bool bad_esc = idx_end > lit_src.len - 1;
            if (!bad_esc) {
                Str const base10digits = slice(U8, lit_src, i + 1, idx_end);
                i += 3;
                ºU64 const maybe = uintParse(base10digits);
                bad_esc = (!maybe.ok) || maybe.it >= 256;
                ret_str.at[ret_str.len] = (U8)maybe.it;
            }
            if (bad_esc)
                panic(astNodeMsg(str("expected 3-digit base-10 integer decimal 000-255 following backslash escape"), &ret_expr.node_base, ast));
        }
        ret_str.len += 1;
    }
    ret_str.at[ret_str.len] = 0;

    ret_expr.kind_lit_str = ret_str;
    return ret_expr;
}

AstExpr parseExpr(Tokens const, Uint const, Ast const* const);

AstExprs parseExprsDelimited(Tokens const toks, Uint const all_toks_idx, TokenKind const tok_kind_sep, Ast const* const ast) {
    if (toks.len == 0)
        return (AstExprs) {.len = 0, .at = NULL};
    Tokenss const per_elem_toks = toksSplit(toks, tok_kind_sep);
    AstExprs ret_exprs = make(AstExpr, 0, per_elem_toks.len);
    Uint toks_idx = all_toks_idx;
    forEach(Tokens, this_elem_toks, per_elem_toks, {
        if (this_elem_toks->len == 0)
            toks_idx += 1; // 1 for eaten delimiter
        else {
            append(ret_exprs, parseExpr(*this_elem_toks, toks_idx, ast));
            toks_idx += (1 + this_elem_toks->len); // 1 for eaten delimiter
        }
    });
    return ret_exprs;
}

AstExpr parseExpr(Tokens const expr_toks, Uint const all_toks_idx, Ast const* const ast) {
    assert(expr_toks.len != 0);
    AstExprs ret_acc = make(AstExpr, 0, expr_toks.len);
    Bool const whole_form_throng = (expr_toks.len > 1) && (tokThrong(expr_toks, 0, ast->src) == expr_toks.len - 1);

    for (Uint i = 0; i < expr_toks.len; i += 1) {
        Uint const idx_throng_end = whole_form_throng ? i : tokThrong(expr_toks, i, ast->src);
        if (idx_throng_end > i) {
            append(ret_acc, parseExpr(slice(Token, expr_toks, i, idx_throng_end + 1), all_toks_idx + i, ast));
            i = idx_throng_end; // loop header will increment
        } else
            switch (expr_toks.at[i].kind) {

                case tok_kind_comment: panic("unreachable"); break;

                case tok_kind_lit_num_prefixed: {
                    append(ret_acc, parseExprLitInt(all_toks_idx + 1, ast, &expr_toks.at[i]));
                } break;

                case tok_kind_lit_str_qdouble: {
                    append(ret_acc, parseExprLitStr(all_toks_idx + 1, ast, &expr_toks.at[i], '\"'));
                } break;

                case tok_kind_lit_str_qsingle: {
                    AstExpr expr_lit = parseExprLitStr(all_toks_idx + 1, ast, &expr_toks.at[i], '\"');
                    if (expr_lit.kind_lit_str.len != 1)
                        panic(astNodeMsg(str("currently only supporting single-byte char literals"), &expr_lit.node_base, ast));

                    expr_lit.kind = ast_expr_lit_int;
                    expr_lit.kind_lit_int = expr_lit.kind_lit_str.at[0];
                    append(ret_acc, expr_lit);
                } break;

                case tok_kind_sep_bcurly_open:  // fall through to:
                case tok_kind_sep_bsquare_open: // fall through to:
                case tok_kind_sep_bparen_open: {
                    TokenKind const tok_kind = expr_toks.at[i].kind;
                    ºUint idx_closing = toksIndexOfMatchingBracket(slice(Token, expr_toks, i, expr_toks.len));
                    assert(idx_closing.ok); // the other-case will have been caught already by toksCheckBrackets
                    idx_closing.it += i;

                    if (tok_kind == tok_kind_sep_bparen_open) {
                        Tokens const toks_inside_parens = slice(Token, expr_toks, i + 1, idx_closing.it);
                        if (toks_inside_parens.len == 0) {
                            AstExpr expr_ident = astExpr(all_toks_idx + i, 2, ast_expr_ident);
                            expr_ident.kind_ident = str("()");
                            append(ret_acc, expr_ident);
                        } else {
                            AstExpr expr_inside_parens = parseExpr(toks_inside_parens, all_toks_idx + i + 1, ast);
                            expr_inside_parens.anns.parensed += 1;
                            // still want the parens toks captured in node base:
                            expr_inside_parens.node_base.toks_idx = all_toks_idx + i;
                            expr_inside_parens.node_base.toks_len += 2;
                            append(ret_acc, expr_inside_parens);
                        }
                    } else { // no parens: either square brackets or curly braces
                        AstExprs const exprs_inside =
                            parseExprsDelimited(slice(Token, expr_toks, i + 1, idx_closing.it), all_toks_idx + i + 1, tok_kind_sep_comma, ast);
                        Bool const is_braces = (tok_kind == tok_kind_sep_bcurly_open);
                        Bool const is_bracket = (tok_kind == tok_kind_sep_bsquare_open);
                        assert(is_braces || is_bracket); // always true right now obviously, but for future overpaced refactorers..
                        AstExpr expr_brac =
                            astExpr(all_toks_idx + i, 1 + (idx_closing.it - i), is_bracket ? ast_expr_lit_bracket : ast_expr_lit_braces);
                        if (is_braces)
                            expr_brac.kind_braces = exprs_inside;
                        else
                            expr_brac.kind_bracket = exprs_inside;
                    }
                    i = idx_closing.it;
                } break;

                case tok_kind_ident: {
                    AstExpr expr_ident = astExpr(all_toks_idx + i, 1, ast_expr_ident);
                    expr_ident.kind_ident = tokSrc(&expr_toks.at[i], ast->src);
                    append(ret_acc, expr_ident);
                } break;

                default: {
                    Token* const t = &expr_toks.at[i];
                    panic("unrecognized token in line %zu: %s", t->line_nr + 1, tokSrc(t, ast->src));
                } break;
            }
    }

    assert(ret_acc.len != 0);
    if (ret_acc.len == 1)
        return ret_acc.at[0];

    AstExpr ret_expr = astExpr(all_toks_idx, expr_toks.len, ast_expr_form);
    ret_expr.kind_form = ret_acc;
    ret_expr.anns.toks_throng = whole_form_throng;
    return ret_expr;
}
