package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	at "github.com/metaleap/atmo/atem"
	tl "github.com/metaleap/go-machines/toylam"
)

var (
	mainTopDefQName string
	inProg          tl.Prog
	outProg         at.Prog
	instr2op        = map[tl.Instr]at.OpCode{
		tl.InstrADD: at.OpAdd,
		tl.InstrDIV: at.OpDiv,
		tl.InstrEQ:  at.OpEq,
		tl.InstrGT:  at.OpGt,
		tl.InstrLT:  at.OpLt,
		tl.InstrMOD: at.OpMod,
		tl.InstrMUL: at.OpMul,
		tl.InstrSUB: at.OpSub,
	}
)

func main() {
	srcfilepath, dstdirpath := os.Args[1], os.Args[2]
	if err := os.MkdirAll(dstdirpath, os.ModePerm); err != nil {
		panic(err)
	}
	srcdirpath := filepath.Dir(srcfilepath)
	files, err := ioutil.ReadDir(srcdirpath)
	if err != nil {
		panic(err)
	}
	modules := make(map[string][]byte, len(files))
	for _, file := range files {
		if curfilepath := filepath.Join(srcdirpath, file.Name()); !file.IsDir() {
			if idxdot := strings.LastIndexByte(file.Name(), '.'); (curfilepath == srcfilepath) || (idxdot > 0 && file.Name()[idxdot:] == ".tl") {
				if src, err := ioutil.ReadFile(curfilepath); err == nil {
					modules[file.Name()[:idxdot]] = src
				} else {
					panic(err)
				}
			}
		}
	}
	srcfilename, srcfileext := filepath.Base(srcfilepath), filepath.Ext(srcfilepath)
	mainTopDefQName = srcfilename[:len(srcfilename)-len(srcfileext)] + ".main"
	dstfilepath := filepath.Join(dstdirpath, mainTopDefQName[:len(mainTopDefQName)-len(".main")]+".json")
	inProg.ParseModules(modules, tl.ParseOpts{KeepNameRefs: true, KeepOpRefs: true, KeepRec: true})
	compile()
	ioutil.WriteFile(dstfilepath, []byte(outProg.String()), os.ModePerm)
}
