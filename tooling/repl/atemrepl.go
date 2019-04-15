package atemrepl

import (
	"bufio"
	"io"
	"os"

	"github.com/go-leap/str"
	"github.com/metaleap/atem"
)

type Repl struct {
	Ctx atem.Ctx
	IO  struct {
		Stdin           io.Reader
		Stdout          io.Writer
		Stderr          io.Writer
		MultiLineSuffix string

		write              func(string, int)
		writeLns, printLns func(...string)
	}

	KnownDirectives directives

	quit bool
}

func (me *Repl) Run(showWelcomeMsg bool) (err error) {
	if me.init(); showWelcomeMsg {
		me.DWelcomeMsg("")
	}
	multiln, indent, repl := "", 0, bufio.NewScanner(os.Stdin)
	for me.IO.writeLns("▼"); (!me.quit) && repl.Scan(); {
		readln := repl.Text()
		numleadingspaces := ustr.CountPrefixRunes(readln, ' ')
		if readln = ustr.Trim(readln); readln == "" {
			me.IO.write(" ", indent)
		} else {
			if neat := (multiln == "" && ustr.Suff(readln, " :=")); neat || ustr.Suff(readln, me.IO.MultiLineSuffix) {
				if multiln == "" {
					if readln[0] != ':' {
						if indent, multiln = 2, readln[:len(readln)-len(me.IO.MultiLineSuffix)]+"\n  "; neat {
							multiln = readln + "\n  "
						}
						me.IO.write(" ", indent)
						continue
					}
				} else if multiln, indent, readln = "", 0, ustr.Trim(multiln+readln[:len(readln)-len(me.IO.MultiLineSuffix)]); readln == "" {
					continue
				}
			}
			switch {
			case multiln != "":
				indent += numleadingspaces
				multiln += ustr.Times(" ", numleadingspaces) + readln + "\n" + ustr.Times(" ", indent)
				me.IO.write(" ", indent)
				continue
			case readln[0] == ':':
				dletter, dargs := ustr.BreakOnFirstOrPref(readln[1:], " ")
				var found *directive
				if len(dletter) > 0 {
					if found = me.KnownDirectives.By(dletter[0]); found != nil {
						found.Run(dargs)
					}
				}
				if found == nil {
					me.IO.writeLns("unknown directive `:" + dletter + "` — try: ")
					for i := range me.KnownDirectives {
						me.IO.writeLns("\t:" + me.KnownDirectives[i].Desc)
					}
				}
			default:
				if out, err := me.Ctx.ReadEvalPrint(readln); err != nil {
					me.IO.printLns(err.Error())
				} else {
					me.IO.writeLns(out.String())
				}
			}
			me.IO.writeLns(ustr.If(me.quit, "▲", "▼"))
		}
	}
	return
}
