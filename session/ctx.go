package atmosess

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-leap/fs"
	"github.com/go-leap/str"
	"github.com/go-leap/sys"
	"github.com/metaleap/atmo"
	"github.com/metaleap/atmo/lang"
)

type CtxBgMsg struct {
	Issue bool
	Time  time.Time
	Lines []string
}

// Ctx fields must never be written to from the outside after the `Ctx.Init` call.
type Ctx struct {
	Dirs struct {
		fauxKitsMutex sync.Mutex
		fauxKits      []string
		Cache         string
		Kits          []string
	}
	Kits struct {
		All                                  Kits
		AlwaysEnsureLoadedAsSoonAsDiscovered bool
		RecurringBackgroundWatch             struct {
			ShouldNow func() bool
		}
	}
	state struct {
		cleanUps      []func()
		bgMsgs        []CtxBgMsg
		fileModsWatch struct {
			latestMutex                                 sync.Mutex
			latest                                      []map[string]os.FileInfo
			manuallyCollectFileModsForNextCatchup       func([]string, []string) int
			collectingFileModsAutomaticallyPeriodically bool
			emitMsgsIfManual                            bool
		}
		kitsReprocessing struct {
			needed bool
		}
		initCalled bool
	}
}

// CtxDefaultCacheDirPath returns the default used by `Ctx.Init` if `Ctx.Dirs.Cache`
// was left empty. It returns a platform-specific dir path such as `~/.cache/atmo`,
// `~/.config/atmo` etc. or in the worst case the current user's home directory.
func CtxDefaultCacheDirPath() string {
	return filepath.Join(usys.UserDataDirPath(true), "atmo")
}

func (me *Ctx) maybeInitPanic(initingNow bool) {
	if me.state.initCalled == initingNow {
		panic("atmo.Ctx.Init must be called exactly once only")
	}
}

// Init validates the `Ctx.Dirs` fields currently set, then builds up its
// `Kits` reflective of the structures found in the various `me.Dirs.Kits`
// search paths and from now on in sync with live modifications to those.
func (me *Ctx) Init(clearCacheDir bool, sessionFauxKitDir string) (err error) {
	me.maybeInitPanic(true)
	me.state.initCalled, me.Kits.All = true, make(Kits, 0, 32)
	cachedir := me.Dirs.Cache
	if cachedir == "" {
		cachedir = CtxDefaultCacheDirPath()
	}
	if !ufs.IsDir(cachedir) {
		err = ufs.EnsureDir(cachedir)
	} else if clearCacheDir {
		err = ufs.Del(cachedir)
	}
	if kitsdirs := me.Dirs.Kits; err == nil {
		kitsdirsenv := ustr.Split(os.Getenv(atmo.EnvVarKitsDirs), string(os.PathListSeparator))
		for i := range kitsdirsenv {
			kitsdirsenv[i] = filepath.Clean(kitsdirsenv[i])
		}
		for i := range kitsdirs {
			kitsdirs[i] = filepath.Clean(kitsdirs[i])
		}
		kitsdirsorig := kitsdirs
		kitsdirs = ustr.Merge(kitsdirsenv, kitsdirs, func(ldp string) bool {
			if ldp != "" && !ufs.IsDir(ldp) {
				me.bgMsg(true, "kits-dir "+ldp+" not found")
				return true
			}
			return ldp == ""
		})
		for i := range kitsdirs {
			for j := range kitsdirs {
				if iinj, jini := ustr.Pref(kitsdirs[i], kitsdirs[j]), ustr.Pref(kitsdirs[j], kitsdirs[i]); i != j && (iinj || jini) {
					err = errors.New("conflicting kits-dirs: " + kitsdirs[i] + " vs. " + kitsdirs[j])
					break
				}
			}
			if err != nil {
				break
			}
		}
		if err == nil && len(kitsdirs) == 0 {
			if kitsdirstried := append(kitsdirsenv, kitsdirsorig...); len(kitsdirstried) == 0 {
				err = errors.New("no kits-dirs were specified, neither via env-var " + atmo.EnvVarKitsDirs + " nor via command-line flags")
			} else {
				err = errors.New("none of the specified kits-dirs were found:\n    " + ustr.Join(kitsdirstried, "\n    "))
			}
		}
		if err == nil {
			var autokitexists bool
			for _, kd := range kitsdirs {
				if autokitexists = ufs.DoesDirHaveFilesWithSuffix(filepath.Join(kd, atmo.NameAutoKit), atmo.SrcFileExt); autokitexists {
					break
				}
			}
			if !autokitexists {
				err = errors.New("Standard auto-imported kit `" + atmo.NameAutoKit + "` not found in any of:\n    " + ustr.Join(kitsdirs, "\n    "))
			}
		}
		if err == nil {
			if me.Dirs.Cache, me.Dirs.Kits = cachedir, kitsdirs; len(sessionFauxKitDir) > 0 {
				_, err = me.fauxKitsAddDir(true, sessionFauxKitDir, true)
			}
		}
		if err == nil {
			me.initKits()
		}
	}
	if err != nil {
		me.state.initCalled = false
	}
	return
}

func (me *Ctx) FauxKitsAdd(dirPath string) (is bool, err error) {
	me.Dirs.fauxKitsMutex.Lock()
	was := ustr.In(dirPath, me.Dirs.fauxKits...)
	if is = was; !is {
		is, err = me.fauxKitsAddDir(true, dirPath, false)
	}
	me.Dirs.fauxKitsMutex.Unlock()
	if is && !was {
		me.catchUpOnFileMods(true, nil)
	}
	return
}

func (me *Ctx) fauxKitsAddDir(alreadyLocked bool, dirPath string, forceAcceptEvenIfNoSrcFiles bool) (dirHasSrcFiles bool, err error) {
	if dirPath == "" || dirPath == "." {
		dirPath, err = os.Getwd()
	} else if dirPath[0] == '~' {
		if len(dirPath) == 1 {
			dirPath = usys.UserHomeDirPath()
		} else if dirPath[1] == filepath.Separator {
			dirPath = filepath.Join(usys.UserHomeDirPath(), dirPath[2:])
		}
	}
	if err == nil {
		dirPath, err = filepath.Abs(dirPath)
	}
	if err == nil && !ufs.IsDir(dirPath) {
		err = &os.PathError{Path: dirPath, Op: "directory", Err: os.ErrNotExist}
	}
	if err == nil {
		if dirHasSrcFiles = ufs.DoesDirHaveFilesWithSuffix(dirPath, atmo.SrcFileExt); dirHasSrcFiles || forceAcceptEvenIfNoSrcFiles {
			var in bool
			for _, kitsdirpath := range me.Dirs.Kits {
				if in = ustr.Pref(dirPath, kitsdirpath+string(os.PathSeparator)); in {
					break
				}
			}
			if !alreadyLocked {
				me.Dirs.fauxKitsMutex.Lock()
			}
			if !in {
				in = ustr.In(dirPath, me.Dirs.fauxKits...)
			}
			if !in {
				me.Dirs.fauxKits = append(me.Dirs.fauxKits, dirPath)
			}
			if !alreadyLocked {
				me.Dirs.fauxKitsMutex.Unlock()
			}
		}
	}
	return
}

func (me *Ctx) FauxKitsHas(dirPath string) (isSessionDirFauxKit bool) {
	me.Dirs.fauxKitsMutex.Lock()
	isSessionDirFauxKit = ustr.In(dirPath, me.Dirs.fauxKits...)
	me.Dirs.fauxKitsMutex.Unlock()
	return
}

// Dispose is called when done with the `Ctx`. There may be tickers to halt, etc.
func (me *Ctx) Dispose() {
	me.maybeInitPanic(false)
	for _, cleanup := range me.state.cleanUps {
		if cleanup != nil {
			cleanup()
		}
	}
	me.state.cleanUps = nil
}

func (me *Ctx) bgMsg(issue bool, lines ...string) {
	msg := CtxBgMsg{Issue: issue, Time: time.Now(), Lines: lines}
	me.state.bgMsgs = append(me.state.bgMsgs, msg)
}

func (me *Ctx) BackgroundMessages(clear bool) (msgs []CtxBgMsg) {
	me.maybeInitPanic(false)
	if msgs = me.state.bgMsgs; clear {
		me.state.bgMsgs = nil
	}
	return
}

func (me *Ctx) BackgroundMessagesCount() (count int) {
	me.maybeInitPanic(false)
	count = len(me.state.bgMsgs)
	return
}

func (me *Ctx) onErrs(errors atmo.Errors, errs []error) {
	for i := range errors {
		me.bgMsg(true, errors[i].Error())
	}
	for _, e := range errs {
		me.bgMsg(true, e.Error())
	}
}

func (me *Ctx) CatchUp(checkForFileModsNow bool) {
	me.catchUpOnFileMods(checkForFileModsNow, nil)
}

func (me *Ctx) catchUpOnFileMods(checkForFileModsNow bool, ensureFilesMarkedAsChanged atmolang.AstFiles) {
	me.Dirs.fauxKitsMutex.Lock()
	fauxkitdirpaths := me.Dirs.fauxKits
	me.Dirs.fauxKitsMutex.Unlock()
	if checkForFileModsNow {
		me.state.fileModsWatch.manuallyCollectFileModsForNextCatchup(me.Dirs.Kits, fauxkitdirpaths)
	}
	var latest []map[string]os.FileInfo
	me.state.fileModsWatch.latestMutex.Lock()
	latest, me.state.fileModsWatch.latest = me.state.fileModsWatch.latest, nil
	me.state.fileModsWatch.latestMutex.Unlock()
	if len(ensureFilesMarkedAsChanged) > 0 {
		extra := make(map[string]os.FileInfo, len(ensureFilesMarkedAsChanged))
		for _, srcfile := range ensureFilesMarkedAsChanged {
			var have bool
			for _, modset := range latest {
				if _, have = modset[srcfile.SrcFilePath]; have {
					break
				}
			}
			if !have {
				if fileinfo, _ := os.Stat(srcfile.SrcFilePath); fileinfo != nil {
					extra[srcfile.SrcFilePath] = fileinfo
				}
			}
		}
		if len(extra) > 0 {
			latest = append(latest, extra)
		}
	}
	me.fileModsHandle(me.Dirs.Kits, fauxkitdirpaths, latest)
}

func (me *Ctx) WithInMemFileMod(srcFilePath string, altSrc string, do func()) (recoveredPanic interface{}) {
	var one map[string]string
	if altSrc != "" {
		one = map[string]string{srcFilePath: altSrc}
	}
	return me.WithInMemFileMods(one, do)
}

func (me *Ctx) WithInMemFileMods(srcFilePathsAndAltSrcs map[string]string, do func()) (recoveredPanic interface{}) {
	if len(srcFilePathsAndAltSrcs) > 0 {
		srcfiles := make(atmolang.AstFiles, 0, len(srcFilePathsAndAltSrcs))
		restoreFinally := func() {
			recoveredPanic = recover()
			for _, srcfile := range srcfiles {
				srcfile.Options.TmpAltSrc = nil
			}
			me.catchUpOnFileMods(true, srcfiles)
		}
		defer restoreFinally()

		for srcfilepath, altsrc := range srcFilePathsAndAltSrcs {
			if kit := me.KitByDirPath(filepath.Dir(srcfilepath), false); kit != nil {
				if srcfile := kit.SrcFiles.ByFilePath(srcfilepath); srcfile != nil {
					srcfiles, srcfile.Options.TmpAltSrc = append(srcfiles, srcfile), []byte(altsrc)
				}
			}
		}
		me.catchUpOnFileMods(true, srcfiles)
	}
	do()
	return
}
