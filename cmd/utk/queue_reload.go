package main

import (
	"debug/pe"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// A compile that finished is not a compile that loaded. Observed (My-Game W7 P6c): a test file was edited while a long test job
// ran; the next `queue submit compile` answered "COMPILE OK … ran 0s", Library/ScriptAssemblies/*.dll on disk was newer than the
// source, yet the domain never reloaded and every later test ran the old IL until something called RequestScriptReload. Nothing
// in the verb's own status says so ("up_to_date" / "completed" both read as success), so after the refresh the cycle compares the
// module version id (MVID) of every ScriptAssemblies dll on disk with the one the Editor has loaded, forces a reload when they
// differ, and FAILS the job if they still differ.

// loadedMVIDSnippet prints "<dll>=<mvid>;" for every loaded assembly that lives in Library/ScriptAssemblies.
const loadedMVIDSnippet = `var sb=new System.Text.StringBuilder(); foreach (var a in System.AppDomain.CurrentDomain.GetAssemblies()){ if(a.IsDynamic) continue; string l; try{l=a.Location;}catch{continue;} if(string.IsNullOrEmpty(l)||!l.Replace('\\','/').Contains("/Library/ScriptAssemblies/")) continue; sb.Append(System.IO.Path.GetFileName(l)).Append('=').Append(a.ManifestModule.ModuleVersionId).Append(';'); } return sb.ToString();`

const reloadSnippet = `UnityEditor.EditorUtility.RequestScriptReload(); return "reload requested";`

var mvidPair = regexp.MustCompile(`([A-Za-z0-9_.\-]+\.dll)=([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)

// peMVID reads the Module table's Mvid out of a managed assembly (ECMA-335 II.22.30), formatted like Guid.ToString().
func peMVID(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	pf, err := pe.NewFile(f)
	if err != nil {
		return "", err
	}
	var dir pe.DataDirectory
	switch oh := pf.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		dir = oh.DataDirectory[14]
	case *pe.OptionalHeader64:
		dir = oh.DataDirectory[14]
	}
	if dir.VirtualAddress == 0 {
		return "", fmt.Errorf("not a managed assembly")
	}
	off := func(rva uint32) (int64, error) {
		for _, s := range pf.Sections {
			if rva >= s.VirtualAddress && rva < s.VirtualAddress+max32(s.VirtualSize, s.Size) {
				return int64(rva-s.VirtualAddress) + int64(s.Offset), nil
			}
		}
		return 0, fmt.Errorf("rva %#x is in no section", rva)
	}
	read := func(at int64, n int) ([]byte, error) {
		b := make([]byte, n)
		_, err := f.ReadAt(b, at)
		if err != nil && err != io.EOF {
			return nil, err
		}
		return b, nil
	}
	cli, err := off(dir.VirtualAddress)
	if err != nil {
		return "", err
	}
	hdr, err := read(cli, 16)
	if err != nil {
		return "", err
	}
	mdOff, err := off(binary.LittleEndian.Uint32(hdr[8:12]))
	if err != nil {
		return "", err
	}
	root, err := read(mdOff, 4096)
	if err != nil || len(root) < 20 || string(root[:4]) != "BSJB" {
		return "", fmt.Errorf("no metadata root")
	}
	p := 16 + int(binary.LittleEndian.Uint32(root[12:16])) + 4 // version string, flags(2), stream count(2): the stream headers start here
	if p-2 > len(root) {
		return "", fmt.Errorf("bad metadata root")
	}
	n := int(binary.LittleEndian.Uint16(root[p-2 : p]))
	streams := map[string][2]int64{}
	for i := 0; i < n && p+8 < len(root); i++ {
		o, sz := int64(binary.LittleEndian.Uint32(root[p:])), int64(binary.LittleEndian.Uint32(root[p+4:]))
		p += 8
		e := p
		for e < len(root) && root[e] != 0 {
			e++
		}
		streams[string(root[p:e])] = [2]int64{o, sz}
		p = (e + 4) &^ 3
	}
	tab, ok := streams["#~"]
	if !ok {
		tab, ok = streams["#-"]
	}
	gd, ok2 := streams["#GUID"]
	if !ok || !ok2 {
		return "", fmt.Errorf("no #~ or #GUID stream")
	}
	th, err := read(mdOff+tab[0], 64)
	if err != nil {
		return "", err
	}
	heaps := th[6]
	valid := binary.LittleEndian.Uint64(th[8:16])
	rows := 0
	for b := valid; b != 0; b &= b - 1 {
		rows++
	}
	strSize, guidSize := 2, 2
	if heaps&1 != 0 {
		strSize = 4
	}
	if heaps&2 != 0 {
		guidSize = 4
	}
	row, err := read(mdOff+tab[0]+24+int64(rows)*4, 2+strSize+guidSize)
	if err != nil {
		return "", err
	}
	idx := int64(binary.LittleEndian.Uint16(row[2+strSize:]))
	if guidSize == 4 {
		idx = int64(binary.LittleEndian.Uint32(row[2+strSize:]))
	}
	if idx == 0 {
		return "", fmt.Errorf("module has no mvid")
	}
	g, err := read(mdOff+gd[0]+(idx-1)*16, 16)
	if err != nil || len(g) < 16 {
		return "", fmt.Errorf("cannot read the #GUID heap")
	}
	return fmt.Sprintf("%08x-%04x-%04x-%x-%x", binary.LittleEndian.Uint32(g[0:4]), binary.LittleEndian.Uint16(g[4:6]),
		binary.LittleEndian.Uint16(g[6:8]), g[8:10], g[10:16]), nil
}

func max32(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}

// staleAssemblies lists the ScriptAssemblies dlls whose loaded copy differs from the file on disk. A project without that
// folder (or an Editor that cannot be asked) reports none: this check never fails a job on its own doubt.
func staleAssemblies(cwd string) []string {
	files, _ := filepath.Glob(filepath.Join(cwd, "Library", "ScriptAssemblies", "*.dll"))
	if len(files) == 0 {
		return nil
	}
	out, code := queueExec(cwd, time.Minute, nil, utkSelf(), "exec", loadedMVIDSnippet)
	if code != 0 {
		return nil
	}
	loaded := map[string]string{}
	for _, m := range mvidPair.FindAllStringSubmatch(out, -1) {
		loaded[m[1]] = m[2]
	}
	var stale []string
	for _, f := range files {
		name := filepath.Base(f)
		want, ok := loaded[name]
		if !ok {
			continue // not loaded (an Editor-only or unused assembly)
		}
		if disk, err := peMVID(f); err == nil && disk != want {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	return stale
}

// ensureReloaded makes the Editor run the IL that is on disk: one forced reload, then a second look.
func (c *cycle) ensureReloaded() (bool, string) {
	stale := staleAssemblies(c.cwd)
	for try := 0; len(stale) > 0 && try < 2; try++ {
		io.WriteString(c.stderr, "utk queue: WARN the Editor still runs old code ("+strings.Join(stale, ", ")+"); forcing a script reload\n")
		queueExec(c.cwd, time.Minute, nil, utkSelf(), "exec", reloadSnippet)
		queueExec(c.cwd, 6*time.Minute, nil, utkSelf(), "editor", "wait", "--timeout", "300")
		stale = staleAssemblies(c.cwd)
	}
	if len(stale) > 0 {
		return false, "STALE_IL: the Editor still runs old code after a forced reload (" + strings.Join(stale, ", ") + "); the test would run the previous build. Run `utk editor restart`, or look for a modal dialog\n"
	}
	return true, ""
}
