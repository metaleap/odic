package atmolang_irfun

import (
	"github.com/go-leap/str"
	"github.com/metaleap/atmo"
	"github.com/metaleap/atmo/lang"
)

func (me *AstDef) initFrom(ctx *ctxAstInit, orig *atmolang.AstDef) (errs atmo.Errors) {
	me.Orig = orig.ToUnary()
	errs.Add(me.initName(ctx))
	errs.Add(me.initArg(ctx))
	errs.Add(me.initMetas(ctx))
	errs.Add(me.initBody(ctx))
	if me.Orig = orig; !me.Orig.IsTopLevel {
		ctx.dynNameDrop(me.Orig.Name.Val)
	}
	return
}

func (me *AstDef) initName(ctx *ctxAstInit) (errs atmo.Errors) {
	tok := me.Orig.Name.Tokens.First(nil) // could have none so dont just Tokens[0]
	var ident IAstExpr
	ident, errs = ctx.newAstIdentFrom(&me.Orig.Name)
	if name, _ := ident.(*AstIdentName); name == nil {
		errs.AddNaming(tok, "invalid def name: `"+tok.Meta.Orig+"`") // Tag or EmptyParens or placeholder etc..
	} else if me.Name = *name; name.Val == "" /*|| ustr.In(name.Val, langReservedOps...)*/ {
		errs.AddNaming(tok, "reserved token not permissible as def name: `"+tok.Meta.Orig+"`")
	}
	if !me.Orig.IsTopLevel {
		ctx.dynNameAdd(me.Orig.Name.Val)
	}
	if me.Orig.NameAffix != nil {
		ctx.addCoercion(&me.Name, errs.AddVia(ctx.newAstExprFrom(me.Orig.NameAffix)).(IAstExpr))
	}
	return
}

func (me *AstDef) initBody(ctx *ctxAstInit) (errs atmo.Errors) {
	me.Body, errs = ctx.newAstExprFrom(me.Orig.Body)

	opeq := B.IdentName("==")
	if len(ctx.coerceFuncs) > 0 {
		if me.Arg != nil {
			if coerce := ctx.coerceFuncs[me.Arg]; coerce != nil {
				appl := B.Appl(ctx.ensureAstAtomFor(coerce), &me.Arg.AstIdentName)
				me.Body = B.Case(B.Appls(ctx, opeq, &me.Arg.AstIdentName, ctx.ensureAstAtomFor(appl)), me.Body)
			}
		}
		if coerce := ctx.coerceFuncs[&me.Name]; coerce != nil {
			appl := B.Appl(ctx.ensureAstAtomFor(coerce), &me.Name)
			me.Body = B.Case(B.Appls(ctx, opeq, &me.Name, ctx.ensureAstAtomFor(appl)), me.Body)
		}
	}
	return
}

func (me *AstDef) initArg(ctx *ctxAstInit) (errs atmo.Errors) {
	if len(me.Orig.Args) > 1 {
		panic(len(me.Orig.Args))
	} else if len(me.Orig.Args) == 1 {
		var arg AstDefArg
		arg.initFrom(ctx, &me.Orig.Args[0])
		me.Arg = &arg
	}
	return
}

func (me *AstDef) initMetas(ctx *ctxAstInit) (errs atmo.Errors) {
	if len(me.Orig.Meta) > 0 {
		errs.AddTodo(&me.Orig.Meta[0].Toks()[0], "def metas")
		for i := range me.Orig.Meta {
			_ = errs.AddVia(ctx.newAstExprFrom(me.Orig.Meta[i]))
		}
	}
	return
}

func (me *AstDefArg) initFrom(ctx *ctxAstInit, orig *atmolang.AstDefArg) (errs atmo.Errors) {
	me.Orig = orig

	var isconstexpr bool
	switch v := orig.NameOrConstVal.(type) {
	case *atmolang.AstIdent:
		if isconstexpr = true; !(v.IsTag || v.Val == "()" || ( /*AstIdentVar*/ v.Val[0] == '_' && len(v.Val) > 1 && v.Val[1] != '_')) {
			if ustr.IsRepeat(v.Val, '_') {
				if isconstexpr, me.AstIdentName.Val, me.AstIdentName.Orig = false, ustr.Int(len(v.Val))+"_"+ctx.nextPrefix(), v; len(v.Val) > 1 {
					errs.AddNaming(&v.Tokens[0], "invalid def arg name")
				}
			} else if cxn, ok1 := errs.AddVia(ctx.newAstIdentFrom(v)).(*AstIdentName); ok1 {
				isconstexpr, me.AstIdentName = false, *cxn
			}
		}
	case *atmolang.AstExprLitFloat, *atmolang.AstExprLitUint, *atmolang.AstExprLitRune, *atmolang.AstExprLitStr:
		isconstexpr = true
	}
	if isconstexpr && orig.Affix != nil {
		errs.AddSyn(&orig.Affix.Toks()[0], "def-arg affix illegal where the arg is itself a constant value")
	}
	if isconstexpr {
		me.AstIdentName.Val = "42" + ctx.nextPrefix() + orig.NameOrConstVal.Toks()[0].Meta.Orig
		ctx.addCoercion(me, B.Appl(B.IdentName("must"), ctx.ensureAstAtomFor(errs.AddVia(ctx.newAstExprFrom(orig.NameOrConstVal)).(IAstExpr))))
	}
	if orig.Affix != nil {
		ctx.addCoercion(me, errs.AddVia(ctx.newAstExprFrom(orig.Affix)).(IAstExpr))
	}
	return
}

func (me *AstCases) initFrom(ctx *ctxAstInit, orig *atmolang.AstExprCases) (errs atmo.Errors) {
	me.Orig = orig

	var scrut IAstExpr
	if orig.Scrutinee != nil {
		scrut = errs.AddVia(ctx.newAstExprFrom(orig.Scrutinee)).(IAstExpr)
	} else {
		scrut = B.IdentTagTrue()
	}
	scrut = B.Appl(B.IdentName("=="), ctx.ensureAstAtomFor(scrut))
	scrutatomic, opeq := ctx.ensureAstAtomFor(scrut), B.IdentName("||")

	me.Ifs, me.Thens = make([]IAstExpr, len(orig.Alts)), make([]IAstExpr, len(orig.Alts))
	for i := range orig.Alts {
		alt := &orig.Alts[i]
		if alt.Body == nil {
			errs.AddSyn(&alt.Tokens[0], "malformed `|?` branching: case has no result expression (or multiple `|?` branchings should be nested but weren't)")
		} else {
			me.Thens[i] = errs.AddVia(ctx.newAstExprFrom(alt.Body)).(IAstExpr)
		}
		for c, cond := range alt.Conds {
			if c == 0 {
				me.Ifs[i] = B.Appl(scrutatomic, ctx.ensureAstAtomFor(errs.AddVia(ctx.newAstExprFrom(cond)).(IAstExpr)))
			} else {
				me.Ifs[i] = B.Appls(ctx, opeq, ctx.ensureAstAtomFor(me.Ifs[i]), ctx.ensureAstAtomFor(
					B.Appl(scrutatomic, ctx.ensureAstAtomFor(errs.AddVia(ctx.newAstExprFrom(cond)).(IAstExpr)))))
			}
		}
	}
	return
}

func (me *AstLitBase) initFrom(ctx *ctxAstInit, orig atmolang.IAstExprAtomic) {
	me.Orig = orig
}

func (me *AstLitFloat) initFrom(ctx *ctxAstInit, orig atmolang.IAstExprAtomic) {
	me.AstLitBase.initFrom(ctx, orig)
	me.Val = orig.Toks()[0].Float
}

func (me *AstLitUint) initFrom(ctx *ctxAstInit, orig atmolang.IAstExprAtomic) {
	me.AstLitBase.initFrom(ctx, orig)
	me.Val = orig.Toks()[0].Uint
}

func (me *AstLitRune) initFrom(ctx *ctxAstInit, orig atmolang.IAstExprAtomic) {
	me.AstLitBase.initFrom(ctx, orig)
	me.Val = orig.Toks()[0].Rune()
}

func (me *AstLitStr) initFrom(ctx *ctxAstInit, orig atmolang.IAstExprAtomic) {
	me.AstLitBase.initFrom(ctx, orig)
	me.Val = orig.Toks()[0].Str
}