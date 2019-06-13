package atmosess

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/go-leap/fs"
	"github.com/go-leap/str"
	"github.com/metaleap/atmo"
	"github.com/metaleap/atmo/il"
	"github.com/metaleap/atmo/lang"
)

// Kit is a pile of atmo source files residing in the same directory and
// being interpreted or compiled all together as a unit.
type Kit struct {
	DirPath           string
	ImpPath           string
	WasEverToBeLoaded bool
	Imports           []string

	topLevelDefs atmoil.AstTopDefs
	SrcFiles     atmolang.AstFiles
	state        struct {
		defsGoneIdsNames map[string]string
		defsBornIdsNames map[string]string
	}
	lookups struct {
		tlDefsByID      map[string]*atmoil.AstDefTop
		tlDefIDsByName  map[string][]string
		namesInScopeOwn atmoil.AnnNamesInScope
		namesInScopeExt atmoil.AnnNamesInScope
		namesInScopeAll atmoil.AnnNamesInScope
	}
	Errs struct {
		Stage0DirAccessDuringRefresh error
		Stage0BadImports             []error
	}
}

func (me *Ctx) kitEnsureLoaded(kit *Kit, thenReprocessAffectedDefsIfAnyKitsReloaded bool) {
	me.kitRefreshFilesAndMaybeReload(kit, !kit.WasEverToBeLoaded)
	if thenReprocessAffectedDefsIfAnyKitsReloaded {
		me.reprocessAffectedDefsIfAnyKitsReloaded()
	}
}

func (me *Ctx) KitEnsureLoaded(kit *Kit) {
	me.kitEnsureLoaded(kit, true)
}

func (me *Ctx) KitsEnsureLoaded(plusSessDirFauxKits bool, kitImpPaths ...string) {
	me.maybeInitPanic(false)
	if plusSessDirFauxKits {
		for _, dirsess := range me.Dirs.fauxKits {
			if idx := me.Kits.All.IndexDirPath(dirsess); idx >= 0 {
				kitImpPaths = append(kitImpPaths, me.Kits.All[idx].ImpPath)
			}
		}
	}

	if len(kitImpPaths) > 0 {
		for _, kip := range kitImpPaths {
			if kit := me.Kits.All.ByImpPath(kip); kit != nil {
				me.kitRefreshFilesAndMaybeReload(kit, !kit.WasEverToBeLoaded)
			}
		}
	}

	me.reprocessAffectedDefsIfAnyKitsReloaded()
}

func (me *Ctx) KitByDirPath(dirPath string, tryToAddToFauxKits bool) (kit *Kit) {
	if kit = me.Kits.All.ByDirPath(dirPath); kit == nil && tryToAddToFauxKits {
		if ok, _ := me.FauxKitsAdd(dirPath); ok {
			kit = me.Kits.All.ByDirPath(dirPath)
		}
	}
	return
}

func (me *Ctx) KitByImpPath(impPath string) *Kit {
	me.maybeInitPanic(false)
	idx := me.Kits.All.IndexImpPath(impPath)
	if idx < 0 && (impPath == "" || impPath == "." || impPath == "·") {
		if fauxkitdirs := me.Dirs.fauxKits; len(fauxkitdirs) > 0 {
			idx = me.Kits.All.IndexDirPath(fauxkitdirs[0])
		}
	}
	if idx >= 0 {
		return me.Kits.All[idx]
	}
	return nil
}

func (me *Ctx) kitRefreshFilesAndMaybeReload(kit *Kit, reloadForceInsteadOfAuto bool) {
	var fresherrs []error
	var srcfileschanged bool

	{ // step 1: files refresh
		var diritems []os.FileInfo
		if diritems, kit.Errs.Stage0DirAccessDuringRefresh = ufs.Dir(kit.DirPath); kit.Errs.Stage0DirAccessDuringRefresh != nil {
			kit.SrcFiles, kit.topLevelDefs, fresherrs = nil, nil, append(fresherrs, kit.Errs.Stage0DirAccessDuringRefresh)
			goto end
		}

		// any deleted files get forgotten now
		for i := 0; i < len(kit.SrcFiles); i++ {
			if kit.SrcFiles[i].SrcFilePath != "" && !ufs.IsFile(kit.SrcFiles[i].SrcFilePath) {
				kit.SrcFiles.RemoveAt(i)
				i, srcfileschanged = i-1, true
			}
		}

		// any new files get added
		for _, file := range diritems {
			if (!file.IsDir()) && ustr.Suff(file.Name(), atmo.SrcFileExt) {
				if fp := filepath.Join(kit.DirPath, file.Name()); kit.SrcFiles.Index(fp) < 0 {
					srcfileschanged, kit.SrcFiles = true, append(kit.SrcFiles, &atmolang.AstFile{SrcFilePath: fp})
				}
			}
		}
		if srcfileschanged {
			atmo.SortMaybe(kit.SrcFiles)
		}
	}

	{ // step 2: maybe (re)load
		if kit.WasEverToBeLoaded || reloadForceInsteadOfAuto {
			kit.WasEverToBeLoaded, kit.Errs.Stage0BadImports =
				true, nil

			allunchanged := !srcfileschanged
			for _, sf := range kit.SrcFiles {
				var nochanges bool
				fresherrs = append(fresherrs, sf.LexAndParseFile(true, false, &nochanges)...)
				allunchanged = allunchanged && nochanges
			}

			for _, imp := range kit.Imports {
				if kimp := me.Kits.All.ByImpPath(imp); kimp == nil {
					kit.Errs.Stage0BadImports = append(kit.Errs.Stage0BadImports, errors.New("import not found: `"+imp+"`"))
				} else {
					me.kitEnsureLoaded(kimp, false)
				}
			}
			if len(kit.Errs.Stage0BadImports) > 0 {
				fresherrs = append(fresherrs, kit.Errs.Stage0BadImports...)
			}

			if allunchanged && !reloadForceInsteadOfAuto {
				goto end
			}
			{
				od, nd, fe := kit.topLevelDefs.ReInitFrom(kit.SrcFiles)
				kit.state.defsGoneIdsNames, kit.state.defsBornIdsNames, fresherrs = od, nd, append(fresherrs, fe...)
				if len(od) > 0 || len(nd) > 0 || len(fe) > 0 {
					me.Kits.reprocessingNeeded = true
				}
			}
			kit.lookups.tlDefIDsByName, kit.lookups.tlDefsByID = make(map[string][]string, len(kit.topLevelDefs)), make(map[string]*atmoil.AstDefTop, len(kit.topLevelDefs))
			for _, tldef := range kit.topLevelDefs {
				kit.lookups.tlDefsByID[tldef.Id], kit.lookups.tlDefIDsByName[tldef.Name.Val] =
					tldef, append(kit.lookups.tlDefIDsByName[tldef.Name.Val], tldef.Id)
			}
		}
	}
end:
	kit.ensureErrTldPosOffsets()
	me.onErrs(nil, fresherrs)
}

func (me *Kit) ensureErrTldPosOffsets() {
	for _, srcfile := range me.SrcFiles {
		for i := range srcfile.TopLevel {
			tlc := &srcfile.TopLevel[i]
			tlc.Errs().UpdatePosOffsets(tlc)
		}
	}
	for _, tld := range me.topLevelDefs {
		tld.Errs.Stage0Init.UpdatePosOffsets(tld.OrigTopLevelChunk)
		tld.Errs.Stage1BadNames.UpdatePosOffsets(tld.OrigTopLevelChunk)
	}
}

// Errors collects whatever issues exist in any of the `Kit`'s source files
// (file-system errors, lexing/parsing errors, semantic errors etc).
func (me *Kit) Errors(maybeErrsToSrcs map[error][]byte) (errs []error) {
	if me.Errs.Stage0DirAccessDuringRefresh != nil {
		errs = append(errs, me.Errs.Stage0DirAccessDuringRefresh)
	}
	errs = append(errs, me.Errs.Stage0BadImports...)
	for i := range me.SrcFiles {
		for _, e := range me.SrcFiles[i].Errors() {
			if errs = append(errs, e); maybeErrsToSrcs != nil {
				maybeErrsToSrcs[e] = me.SrcFiles[i].LastLoad.Src
			}
		}
	}
	for i := range me.topLevelDefs {
		deferrs := append(me.topLevelDefs[i].Errs.Stage0Init.Errors(), me.topLevelDefs[i].Errs.Stage1BadNames.Errors()...)
		if maybeErrsToSrcs != nil {
			for _, e := range deferrs {
				maybeErrsToSrcs[e] = me.topLevelDefs[i].OrigTopLevelChunk.SrcFile.LastLoad.Src
			}
		}
		errs = append(errs, deferrs...)
	}
	return
}

func (me *Kit) kitsDirPath() string {
	return kitsDirPathFrom(me.DirPath, me.ImpPath)
}

// HasDefs returns whether any of the `Kit`'s source files define `name`.
func (me *Kit) HasDefs(name string) bool {
	return len(me.lookups.tlDefIDsByName[name]) > 0
}

func (me *Kit) Defs(name string) (defs atmoil.AstTopDefs) {
	for len(name) > 0 && name[0] == '_' {
		name = name[1:]
	}
	if len(name) > 0 {
		for _, id := range me.lookups.tlDefIDsByName[name] {
			if def := me.lookups.tlDefsByID[id]; def != nil {
				defs = append(defs, def)
			}
		}
	}
	return
}

func (me *Kit) AstNodeAt(srcFilePath string, pos0ByteOffset int) (topLevelChunk *atmolang.SrcTopChunk, theNodeAndItsAncestors []atmolang.IAstNode) {
	if srcfile := me.SrcFiles.ByFilePath(srcFilePath); srcfile != nil {
		if topLevelChunk = srcfile.TopLevelChunkAt(pos0ByteOffset); topLevelChunk != nil {
			theNodeAndItsAncestors = topLevelChunk.At(pos0ByteOffset)
		}
	}
	return
}

func (me *Kit) AstNodeIrFunFor(defId string, origNode atmolang.IAstNode) (astDefTop *atmoil.AstDefTop, theNodeAndItsAncestors []atmoil.IAstNode) {
	if astDefTop = me.lookups.tlDefsByID[defId]; astDefTop != nil {
		theNodeAndItsAncestors = astDefTop.FindByOrig(origNode)
	}
	return
}
