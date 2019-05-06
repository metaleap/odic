package atmolang_irfun

import (
	"strconv"

	"github.com/go-leap/str"
	"github.com/metaleap/atmo"
	"github.com/metaleap/atmo/lang"
)

type IAstNode interface {
	Print() atmolang.IAstNode

	equivTo(IAstNode) bool
	renameIdents(map[string]string)
	refersTo(string) bool
}

type IAstExpr interface {
	IAstNode
	dynName() string
	IsAtomic() bool
}

type astNodeBase struct {
}

type AstDef struct {
	astNodeBase
	Orig *atmolang.AstDef

	Name AstIdentName
	Arg  *AstDefArg
	Body IAstExpr
}

func (me *AstDef) refersTo(name string) bool { return me.Body.refersTo(name) }
func (me *AstDef) renameIdents(ren map[string]string) {
	me.Name.renameIdents(ren)
	if me.Arg != nil {
		me.Arg.renameIdents(ren)
	}
	me.Body.renameIdents(ren)
}
func (me *AstDef) equivTo(node IAstNode) bool {
	cmp, _ := node.(*AstDef)
	return cmp != nil && cmp.Name.equivTo(&me.Name) && cmp.Arg.isEquivTo(me.Arg) && cmp.Body.equivTo(me.Body)
}

type AstDefTop struct {
	AstDef

	TopLevels []*atmolang.AstFileTopLevelChunk
	Errors    atmo.Errors
}

type AstDefArg struct {
	AstIdentName

	Orig *atmolang.AstDefArg
}

func (me *AstDefArg) isEquivTo(cmp *AstDefArg) bool {
	return ((me == nil) == (cmp == nil)) && (me == nil || me.AstIdentName.equivTo(&cmp.AstIdentName))
}

type AstExprBase struct {
	astNodeBase
}

func (*AstExprBase) IsAtomic() bool { return false }

type AstExprAtomBase struct {
	AstExprBase
}

func (me *AstExprAtomBase) IsAtomic() bool                 { return true }
func (me *AstExprAtomBase) renameIdents(map[string]string) {}

type AstLitBase struct {
	AstExprAtomBase
	Orig atmolang.IAstExprAtomic
}

func (me *AstLitBase) refersTo(string) bool { return false }

type AstLitRune struct {
	AstLitBase
	Val rune
}

func (me *AstLitRune) equivTo(node IAstNode) bool {
	cmp, _ := node.(*AstLitRune)
	return cmp != nil && cmp.Val == me.Val
}
func (me *AstLitRune) dynName() string { return strconv.QuoteRune(me.Val) }

type AstLitStr struct {
	AstLitBase
	Val string
}

func (me *AstLitStr) equivTo(node IAstNode) bool {
	cmp, _ := node.(*AstLitStr)
	return cmp != nil && cmp.Val == me.Val
}
func (me *AstLitStr) dynName() string { return strconv.Quote(me.Val) }

type AstLitUint struct {
	AstLitBase
	Val uint64
}

func (me *AstLitUint) equivTo(node IAstNode) bool {
	cmp, _ := node.(*AstLitUint)
	return cmp != nil && cmp.Val == me.Val
}
func (me *AstLitUint) dynName() string { return strconv.FormatUint(me.Val, 10) }

type AstLitFloat struct {
	AstLitBase
	Val float64
}

func (me *AstLitFloat) equivTo(node IAstNode) bool {
	cmp, _ := node.(*AstLitFloat)
	return cmp != nil && cmp.Val == me.Val
}
func (me *AstLitFloat) dynName() string { return strconv.FormatFloat(me.Val, 'g', -1, 64) }

type AstExprLetBase struct {
	letOrig   *atmolang.AstExprLet
	letDefs   AstDefs
	letPrefix string
}

func (me *AstExprLetBase) letDynName() string {
	if len(me.letDefs) == 0 {
		return ""
	}
	return me.letPrefix + "└"
}
func (me *AstExprLetBase) letDefsEquivTo(cmp *AstExprLetBase) bool {
	if len(me.letDefs) == len(cmp.letDefs) {
		for i := range me.letDefs {
			if !me.letDefs[i].equivTo(&cmp.letDefs[i]) {
				return false
			}
		}
		return true
	}
	return false
}
func (me *AstExprLetBase) letDefsRenameIdents(ren map[string]string) {
	for i := range me.letDefs {
		me.letDefs[i].Body.renameIdents(ren)
	}
}
func (me *AstExprLetBase) letDefsReferTo(name string) (refers bool) {
	for i := range me.letDefs {
		if refers = me.letDefs[i].refersTo(name); refers {
			break
		}
	}
	return
}

type AstIdentBase struct {
	AstExprAtomBase
	Val string

	Orig *atmolang.AstIdent
}

func (me *AstIdentBase) refersTo(name string) bool { return name == me.Val }
func (me *AstIdentBase) dynName() string           { return me.Val }

type AstIdentName struct {
	AstIdentBase
	AstExprLetBase
}

func (me *AstIdentName) IsAtomic() bool  { return len(me.letDefs) == 0 }
func (me *AstIdentName) dynName() string { return me.letDynName() + me.Val }
func (me *AstIdentName) refersTo(name string) bool {
	return me.Val == name || me.letDefsReferTo(name)
}
func (me *AstIdentName) equivTo(node IAstNode) bool {
	cmp, _ := node.(*AstIdentName)
	return cmp != nil && cmp.Val == me.Val && cmp.letDefsEquivTo(&me.AstExprLetBase)
}
func (me *AstIdentName) renameIdents(ren map[string]string) {
	if nu, ok := ren[me.Val]; ok {
		me.Val = nu
	}
	me.letDefsRenameIdents(ren)
}

type AstIdentVar struct {
	AstIdentBase
}

func (me *AstIdentVar) equivTo(node IAstNode) bool {
	cmp, _ := node.(*AstIdentVar)
	return cmp != nil && cmp.Val == me.Val
}
func (me *AstIdentVar) dynName() string { return "˘" + me.Val }

type AstIdentTag struct {
	AstIdentBase
}

func (me *AstIdentTag) equivTo(node IAstNode) bool {
	cmp, _ := node.(*AstIdentTag)
	return cmp != nil && cmp.Val == me.Val
}

type AstIdentEmptyParens struct {
	AstIdentBase
}

func (me *AstIdentEmptyParens) equivTo(node IAstNode) bool {
	cmp, _ := node.(*AstIdentEmptyParens)
	return cmp != nil
}

type AstAppl struct {
	AstExprBase
	AstExprLetBase
	Orig         *atmolang.AstExprAppl
	AtomicCallee IAstExpr
	AtomicArg    IAstExpr
}

func (me *AstAppl) dynName() string {
	return me.letDynName() + me.AtomicCallee.dynName() + "─" + me.AtomicArg.dynName()
}
func (me *AstAppl) equivTo(node IAstNode) bool {
	cmp, _ := node.(*AstAppl)
	return cmp != nil && cmp.AtomicCallee.equivTo(me.AtomicCallee) && cmp.AtomicArg.equivTo(me.AtomicArg) && cmp.letDefsEquivTo(&me.AstExprLetBase)
}
func (me *AstAppl) renameIdents(ren map[string]string) {
	me.AtomicCallee.renameIdents(ren)
	me.AtomicArg.renameIdents(ren)
	me.letDefsRenameIdents(ren)
}
func (me *AstAppl) refersTo(name string) bool {
	return me.AtomicCallee.refersTo(name) || me.AtomicArg.refersTo(name) || me.letDefsReferTo(name)
}

type AstCases struct {
	AstExprBase
	AstExprLetBase
	Orig  *atmolang.AstExprCases
	Ifs   []IAstExpr
	Thens []IAstExpr
}

func (me *AstCases) dynName() string { return ustr.Int(len(me.Ifs)) + "┬" }
func (me *AstCases) equivTo(node IAstNode) bool {
	cmp, _ := node.(*AstCases)
	if cmp != nil && len(cmp.Ifs) == len(me.Ifs) && len(cmp.Thens) == len(me.Thens) {
		for i := range cmp.Ifs {
			if !cmp.Ifs[i].equivTo(me.Ifs[i]) {
				return false
			}
		}
		for i := range cmp.Thens {
			if !cmp.Thens[i].equivTo(me.Thens[i]) {
				return false
			}
		}
		return cmp.letDefsEquivTo(&me.AstExprLetBase)
	}
	return false
}
func (me *AstCases) renameIdents(ren map[string]string) {
	for i := range me.Thens {
		me.Thens[i].renameIdents(ren)
		me.Ifs[i].renameIdents(ren)
	}
	me.letDefsRenameIdents(ren)
}
func (me *AstCases) refersTo(name string) bool {
	for i := range me.Thens {
		if me.Thens[i].refersTo(name) {
			return true
		}
		if me.Ifs[i].refersTo(name) {
			return true
		}
	}
	return me.letDefsReferTo(name)
}