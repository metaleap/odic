package atmolang

import (
	"bytes"
	"io"
	"os"
	"time"

	"github.com/go-leap/dev/lex"
	"github.com/go-leap/std"
	"github.com/metaleap/atmo"
)

func init() {
	udevlex.SepsGroupers, udevlex.SepsOthers, udevlex.RestrictedWhitespace, udevlex.ScannerStringDelims, udevlex.ScannerStringDelimNoEsc =
		"([{}])", ",", true, "\"'", '\''
}

func (me *AstFile) LexAndParseFile(onlyIfModifiedSinceLastLoad bool, stdinIfNoSrcFile bool, noChangesDetected *bool) (freshErrs []error) {
	if me.Options.TmpAltSrc != nil {
		me.LastLoad.Time, me.LastLoad.FileSize = 0, 0
	} else if me.SrcFilePath != "" {
		if srcfileinfo, _ := os.Stat(me.SrcFilePath); srcfileinfo != nil {
			if onlyIfModifiedSinceLastLoad && me.errs.loading == nil && srcfileinfo.Size() == me.LastLoad.FileSize {
				if modtime := srcfileinfo.ModTime().UnixNano(); modtime > 0 && me.LastLoad.Time > modtime {
					if noChangesDetected != nil {
						*noChangesDetected = true
					}
					return
				}
			}
			me.LastLoad.FileSize = srcfileinfo.Size()
		}
	}

	var reader io.Reader
	if me._errs, me.errs.loading = nil, nil; me.Options.TmpAltSrc != nil {
		reader = &ustd.BytesReader{Data: me.Options.TmpAltSrc}
	} else if me.SrcFilePath != "" {
		var srcfile *os.File
		if srcfile, me.errs.loading = os.Open(me.SrcFilePath); me.errs.loading == nil {
			reader = srcfile
			defer srcfile.Close()
		} else {
			freshErrs = append(freshErrs, me.errs.loading)
		}
	} else if stdinIfNoSrcFile {
		reader = os.Stdin
	}
	if me.errs.loading == nil && reader != nil {
		freshErrs = append(freshErrs, me.LexAndParseSrc(reader, noChangesDetected)...)
	}
	return
}

func LexAndParseExpr(fauxSrcFileNameForErrs string, src []byte) (IAstExpr, *atmo.Error) {
	toks, errs := udevlex.Lex(src, fauxSrcFileNameForErrs, 64, ' ')
	for _, e := range errs {
		return nil, atmo.ErrLex(ErrLexing_Other, &e.Pos, e.Msg)
	}

	if expr, err := (&ctxTldParse{}).parseExpr(toks); err != nil { // need this..
		return nil, err // ..explicit branch because else a `nil`..
	} else { // ..`*Error` would turn into a non-nil `error`..
		return expr, nil // ..interface with inner `nil` value *sigh!*
	}
}

func (me *AstFile) LexAndParseSrc(r io.Reader, noChangesDetected *bool) (freshErrs []error) {
	var src []byte
	if src, me.errs.loading = ustd.ReadAll(r, me.LastLoad.FileSize); me.errs.loading != nil {
		freshErrs = append(freshErrs, me.errs.loading)
	} else {
		if bytes.Equal(src, me.LastLoad.Src) {
			if noChangesDetected != nil {
				*noChangesDetected = true
			}
			return
		}
		me.LastLoad.Time, me.LastLoad.Src = time.Now().UnixNano(), src
		if me.topChunksCompare(me.topLevelChunksGatherFrom(src)) {
			if noChangesDetected != nil {
				*noChangesDetected = true
			}
			return
		}

		for i := range me.TopLevel {
			if this := &me.TopLevel[i]; this.srcDirty {
				this.srcDirty, this.errs.parsing, this.errs.lexing, this.Ast.Def.Orig, this.Ast.comments.Leading, this.Ast.comments.Trailing =
					false, nil, nil, nil, nil, nil

				indent, inderr := ' ', false
				if this.preLex.numLinesTabIndented > 0 {
					if this.preLex.numLinesSpaceIndented == 0 {
						indent = '\t'
					} else {
						inderr, freshErrs = true, append(freshErrs,
							this.errs.lexing.AddAt(atmo.ErrCatLexing, ErrLexing_IndentationInconsistent,
								&udevlex.Pos{FilePath: this.SrcFile.SrcFilePath, Col1: 1, Ln1: this.offset.Ln},
								len(this.Src),
								"inconsistent indentation in this top-level block: either replace leading tabs with spaces or vice-versa"))
					}
				}
				if !inderr {
					toks, errs := udevlex.Lex(this.Src, me.SrcFilePath, 64, indent)

					if this.Ast.Tokens = toks; len(errs) > 0 {
						for _, e := range errs {
							freshErrs = append(freshErrs,
								this.errs.lexing.AddLex(ErrLexing_Other, &e.Pos, e.Msg))
						}
					} else {
						freshErrs = append(freshErrs, me.parse(this)...)
					}
				}
			}
		}
	}
	return
}

type topLevelChunk struct {
	src                   []byte
	pos                   int
	line                  int
	numLinesTabIndented   int
	numLinesSpaceIndented int
}

func (me *AstFile) topLevelChunksGatherFrom(src []byte) (tlChunks []topLevelChunk) {
	tlChunks = make([]topLevelChunk, 0, 32)

	var newline bool
	var curline, lastchunkedat, lastchunkedln, numtabs, numspaces int
	var insth, insthn []byte
	allemptysofar, il := true, len(src)-1
	instr1, instr2, incm, incl, instr1n := []byte("\""), []byte("'"), []byte("*/"), []byte("\n"), []byte("\\\"")
	for i, ch := range src {
		isnl := (ch == '\n')
		if isnl {
			curline++
		}
		if allemptysofar && !(isnl || ch == ' ' || ch == '\t') {
			allemptysofar, lastchunkedat, lastchunkedln = false, i, curline
		}

		if insth != nil {
			if i1 := i + 1; bytes.Equal(src[i-len(insth)+1:i1], insth) && (insthn == nil || !bytes.Equal(src[i-len(insthn)+1:i1], insthn)) {
				insth, insthn = nil, nil
			} else {
				continue
			}
		} else if ch == '"' {
			insth, insthn = instr1, instr1n
		} else if ch == '\'' {
			insth = instr2
		} else if ch == '/' && i < il {
			if chnext := src[i+1]; chnext == '*' {
				insth = incm
			} else if chnext == '/' {
				insth = incl
			}
		}
		if ch == '\n' {
			newline = true
		} else if newline {
			newline = false
			if ch == '\t' {
				numtabs++
			} else if ch == ' ' {
				numspaces++
			} else {
				// naive at first: every non-indented line begins its own new chunk. after the loop we merge comments directly prefixed to defs
				if lastchunkedat != i {
					tlChunks = append(tlChunks, topLevelChunk{src: src[lastchunkedat:i], pos: lastchunkedat, line: lastchunkedln, numLinesSpaceIndented: numspaces, numLinesTabIndented: numtabs})
					numspaces, numtabs = 0, 0
				}
				lastchunkedat, lastchunkedln = i, curline
			}
		}
	}
	if me.LastLoad.NumLines = curline; lastchunkedat < il {
		tlChunks = append(tlChunks, topLevelChunk{src: src[lastchunkedat:], pos: lastchunkedat, line: lastchunkedln, numLinesSpaceIndented: numspaces, numLinesTabIndented: numtabs})
	}
	// fix naive tlChunks: stitch together what belongs together
	for i := len(tlChunks) - 1; i > 0; i-- {
		if tlChunks[i-1].line == tlChunks[i].line-1 && /* prev chunk is prev line and begins `//`? */
			len(tlChunks[i-1].src) >= 2 && tlChunks[i-1].src[0] == '/' && tlChunks[i-1].src[1] == '/' {
			tlChunks[i-1].src = append(tlChunks[i-1].src, tlChunks[i].src...)
			for j := i; j < len(tlChunks)-1; j++ {
				tlChunks[j] = tlChunks[j+1]
			}
			tlChunks = tlChunks[0 : len(tlChunks)-1]
		}
	}

	for newidx := range tlChunks { // trim-right \n bytes really helps with not reloading more than necessary
		var numn int
		for j := len(tlChunks[newidx].src) - 1; j > 0; j-- {
			if tlChunks[newidx].src[j] == '\n' {
				numn++
			} else {
				break
			}
		}
		tlChunks[newidx].src = tlChunks[newidx].src[:len(tlChunks[newidx].src)-numn]
	}
	return
}

func (me *AstFile) topChunksCompare(tlChunks []topLevelChunk) (allTheSame bool) {
	// compare gathered `tlChunks` to existing `AstFileTopLevelChunk`s in `me.TopLevel`,
	// dropping those that are gone, adding those that are new, and repositioning others as needed

	srcsame := make(map[int]int, len(tlChunks))
	for oldidx := range me.TopLevel {
		for newidx := range tlChunks {
			if stale, fresh := &me.TopLevel[oldidx], &tlChunks[newidx]; bytes.Equal(stale.Src, fresh.src) {
				srcsame[newidx] = oldidx
				break
			}
		}
	}
	allTheSame = (len(srcsame) == len(me.TopLevel)) && (len(me.TopLevel) == len(tlChunks))
	if allTheSame {
		for newidx, oldidx := range srcsame {
			if newidx != oldidx {
				allTheSame = false
				break
			}
		}
	}
	if !allTheSame {
		oldtlc := me.TopLevel
		me._toks, me.TopLevel = nil, make([]SrcTopChunk, len(tlChunks))
		for newidx := range tlChunks {
			if oldidx, ok := srcsame[newidx]; ok {
				me.TopLevel[newidx] = oldtlc[oldidx]
			} else {
				me.TopLevel[newidx].srcDirty, me.TopLevel[newidx]._id, me.TopLevel[newidx]._errs, me.TopLevel[newidx].Src =
					true, "", nil, tlChunks[newidx].src
				me.TopLevel[newidx].id[1], me.TopLevel[newidx].id[2], _ = ustd.HashTwo(0, 0, me.TopLevel[newidx].Src)
				me.TopLevel[newidx].id[0] = uint64(len(me.TopLevel[newidx].Src))
			}
			me.TopLevel[newidx].SrcFile = me
		}
	}
	for newidx := range tlChunks {
		me.TopLevel[newidx].offset.Ln, me.TopLevel[newidx].offset.B, me.TopLevel[newidx].preLex.numLinesSpaceIndented, me.TopLevel[newidx].preLex.numLinesTabIndented =
			tlChunks[newidx].line, tlChunks[newidx].pos, tlChunks[newidx].numLinesSpaceIndented, tlChunks[newidx].numLinesTabIndented
	}

	return
}
