package atmosess

import (
	"github.com/go-leap/dev/lex"
	. "github.com/metaleap/atmo"
	. "github.com/metaleap/atmo/il"
)

func (me *ctxPreducing) toks(n IIrNode) udevlex.Tokens {
	return me.curNode.owningTopDef.AstOrigToks(n)
}

func (me *Ctx) rePreduceTopLevelDefs(defIds map[*IrDef]*Kit) (freshErrs Errors) {
	for def := range defIds {
		def.Anns.Preduced, def.Errs.Stage3Preduce = nil, nil
	}
	ctxpred := ctxPreducing{curSessCtx: me}
	for def, kit := range defIds {
		ctxpred.curNode.owningKit, ctxpred.curNode.owningTopDef = kit, def
		_ = ctxpred.preduce(def) // does set def.Anns.Preduced, and it must happen there not here
	}
	return
}

func (me *Ctx) Preduce(nodeOwningKit *Kit, maybeNodeOwningTopDef *IrDef, node IIrNode) IPreduced {
	ctxpreduce := &ctxPreducing{curSessCtx: me}
	ctxpreduce.curNode.owningKit, ctxpreduce.curNode.owningTopDef = nodeOwningKit, maybeNodeOwningTopDef
	return ctxpreduce.preduce(node)
}

func (me *ctxPreducing) preduce(node IIrNode) (ret IPreduced) {
	switch this := node.(type) {

	case *IrDef:
		if this.Anns.Preduced == nil && this.Errs.Stage3Preduce == nil { // only actively preduce if not already there --- both set to nil preparatorily in rePreduceTopLevelDefs
			this.Errs.Stage3Preduce = make(Errors, 0, 0) // not nil anymore now
			if !this.HasErrors() {
				prevtopdef := me.curNode.owningTopDef
				me.curNode.owningTopDef = this
				{
					me.dbgIndent++
					this.Anns.Preduced = me.preduce(this.Body)
					me.dbgIndent--
				}
				me.curNode.owningTopDef = prevtopdef
			}
		}
		ret = this.Anns.Preduced

	case IrDefRef:
		prevkit := me.curNode.owningKit
		me.curNode.owningKit = this.Kit
		ret = me.preduce(this.IrDef)
		me.curNode.owningKit = prevkit

	case *IrLitFloat:
		ret = &PPrimAtomicConstFloat{Val: this.Val}

	case *IrLitUint:
		ret = &PPrimAtomicConstUint{Val: this.Val}

	case *IrLitTag:
		ret = &PPrimAtomicConstTag{Val: this.Val}

	case *IrIdentName:
		me.dbgIndent++
		switch len(this.Anns.Candidates) {
		case 0:
			ret = &PErr{Err: ErrNaming(4321, me.toks(this).First1(), "notInScope")}
		case 1:
			if abs, isabs := this.Anns.Candidates[0].(*IrAbs); isabs {
				ret = me.preduce(&abs.Arg)
			} else {
				ret = me.preduce(this.Anns.Candidates[0])
			}
		default:
			ret = &PErr{Err: ErrNaming(1234, me.toks(this).First1(), "ambiguous")}
		}
		me.dbgIndent--

	case *IrAbs:
		ret = &PFunc{Orig: this, Arg: &PMeta{}, Ret: &PMeta{}}

	case *IrArg:
		ret = &PMeta{}

	case *IrAppl:
		rcallee := me.preduce(this.Callee)
		if rcallee != nil {
			switch rc := rcallee.(type) {
			case *PErr:
				ret = rc
			case *PAbyss:
				ret = rc
			case *PMeta:
				ret = rc
			case *PFunc:
				isoutermost := (me.envStack == nil)
				if isoutermost {
				}

				if isoutermost {
					me.envStack = nil
				}
				ret = rc.Ret
			default:
				ret = &PErr{Err: ErrNaming(6789, me.toks(this).First1(), "notCallable: "+rc.SummaryCompact())}
			}
		}

	default:
		panic(this)
	}
	return
}
