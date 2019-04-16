package atmorepl

import (
	"bufio"
	"io"
	"os"

	"github.com/go-leap/str"
	"github.com/metaleap/atmo"
)

type Repl struct {
	Ctx             atmo.Ctx
	KnownDirectives directives
	IO              struct {
		Stdin           io.Reader
		Stdout          io.Writer
		Stderr          io.Writer
		MultiLineSuffix string

		write              func(string, int)
		writeLns, printLns func(...string)
	}

	// current mutable state during a `Run` loop
	run struct {
		quit                       bool
		indent                     int
		multiLnInputHadLeadingTabs bool
	}
}

func (me *Repl) Run(showWelcomeMsg bool) {
	if me.init(); showWelcomeMsg {
		me.DInfo("")
	}
	me.decoInputStart()
	for multiln, readln := "", bufio.NewScanner(os.Stdin); (!me.run.quit) && readln.Scan(); {
		inputln, numleadingspaces, numleadingtabs := trimAndCountPrefixRunes(readln.Text())
		me.run.multiLnInputHadLeadingTabs = me.run.multiLnInputHadLeadingTabs || (len(multiln) > 0 && numleadingtabs > 0)

		if inputln == "" {
			if me.run.indent > multiLnMinIndent {
				if me.run.indent -= multiLnMinIndent; me.run.indent%2 != 0 {
					me.run.indent++
				}
			}
			me.decoInputAddLine()
			continue
		}

		if ismultiln, isdefbegin := ustr.Suff(inputln, me.IO.MultiLineSuffix), (multiln == "" && ustr.Suff(inputln, ":=")); isdefbegin || ismultiln {
			if multiln == "" {
				if inputln[0] != ':' {
					if me.run.indent, multiln = multiLnMinIndent, inputln[:len(inputln)-len(me.IO.MultiLineSuffix)]+"\n  "; isdefbegin {
						multiln = inputln + "\n  "
					}
					me.decoInputAddLine()
					continue
				}
			} else if multiln, me.run.indent, inputln = "", 0, ustr.Trim(multiln+inputln[:len(inputln)-len(me.IO.MultiLineSuffix)]); inputln == "" {
				me.decoInputDone()
				me.decoInputStart()
				continue
			}
		}

		switch {

		// just another line to add to current multi-line input?
		case multiln != "":
			me.run.indent += numleadingspaces
			multiln += ustr.Times(" ", numleadingspaces) + inputln + "\n" + ustr.Times(" ", me.run.indent)
			me.decoInputAddLine()
			continue

		// else, a directive?
		case inputln[0] == ':':
			me.decoInputDone()
			dname, dargs := ustr.BreakOnFirstOrPref(inputln[1:], " ")
			var found *directive
			if len(dname) > 0 {
				if found = me.KnownDirectives.By(dname); found != nil {
					if dargs = ustr.Trim(dargs); !found.Run(dargs) {
						me.IO.writeLns("directive `:"+dname+"` does not understand `"+dargs+"`,", "as a reminder:", "   :"+found.Desc)
					}
				}
			}
			if found == nil {
				me.IO.writeLns("unknown directive `:" + dname + "` — try: ")
				for i := range me.KnownDirectives {
					me.IO.writeLns("   :" + me.KnownDirectives[i].Desc)
				}
			}
			if !me.run.quit {
				me.decoInputStart()
			}

		// else, input to be EVAL'd now
		default:
			me.decoInputDone()
			if me.run.multiLnInputHadLeadingTabs {
				me.decoAddNotice(false, "multi-line input had leading tabs,note", "that repl auto-indent is based on spaces")
			}
			if out, err := me.Ctx.ReadEvalPrint(inputln); err != nil {
				me.IO.printLns(err.Error())
			} else {
				me.IO.writeLns(out.String())
			}
			me.decoInputStart()
		}
	}
}