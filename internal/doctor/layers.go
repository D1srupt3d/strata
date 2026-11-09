package doctor

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// nearLines is the most changed lines that still count as "the same file
// with a line or two changed": the difference [layer_vars] exists for.
const nearLines = 2

// maxCompare skips big files (images, fonts): dotfiles that differ by a line
// are small, and doctor should stay quick.
const maxCompare = 1 << 20

// layerCopy is one layer's copy of a file.
type layerCopy struct{ layer, path string }

// checkLayers warns about a file copied into several layers when the copies
// are identical or differ in a line or two. Layers are for whole-file
// differences; for a line or two, one copy plus a {{var}} means every edit
// is made once instead of once per layer.
func checkLayers(r *report, in Inputs, l loaded) {
	r.group = "layers"
	if !l.machine || !l.repo || !l.dots {
		r.add(Skip, "copies", "need machine.toml, the repo folder and dots.toml to load", "")
		return
	}
	byLayer, err := filesByLayer(l.cfg.RepoDir, l.cfg.Ignore)
	if err != nil {
		r.add(Skip, "copies", "need the repo's folders to be readable and every ignore pattern valid", "")
		return
	}
	copies := map[string][]layerCopy{}
	for _, lf := range byLayer {
		for rel, path := range lf.files {
			copies[rel] = append(copies[rel], layerCopy{lf.name, path})
		}
	}
	mark := len(r.findings)
	for _, rel := range slices.Sorted(maps.Keys(copies)) {
		cs := copies[rel]
		for i := range cs {
			for _, b := range cs[i+1:] {
				compareCopies(r, rel, cs[i], b)
			}
		}
	}
	r.okIfClean(mark, "copies", "no file is copied between layers with only a line or two changed")
}

// compareCopies reports two copies of rel that are identical or nearly so.
func compareCopies(r *report, rel string, a, b layerCopy) {
	da, okA := readText(a.path)
	db, okB := readText(b.path)
	if !okA || !okB {
		return
	}
	la, lb := lines(da), lines(db)
	changed := 0
	for _, op := range difflib.NewMatcher(la, lb).GetOpCodes() {
		if op.Tag != 'e' {
			changed += max(op.I2-op.I1, op.J2-op.J1)
		}
	}
	subject := fmt.Sprintf("%q in %s and %s", rel, a.layer, b.layer)
	switch {
	case changed == 0:
		r.add(Warn, subject, "the copies are identical, so one of them is redundant",
			"keep one copy: in base/ if every machine should get it, else in the one layer that needs it")
	// Most of a short file changed is a different file, not a changed line.
	case changed <= nearLines && 2*changed <= min(len(la), len(lb)):
		plural := "s"
		if changed == 1 {
			plural = ""
		}
		r.add(Warn, subject, fmt.Sprintf("the copies differ in %d line%s", changed, plural),
			"keep one copy in base/ with a {{var}} on the line that differs, set its values under "+
				"[layer_vars.<layer>] in dots.toml, and add the file to substitute")
	}
}

// lines splits text into lines. Unlike difflib.SplitLines it adds no
// phantom last line, which would make a short file look longer and let
// "2 of 3 lines changed" pass as a small change.
func lines(text string) []string {
	return strings.SplitAfter(strings.TrimSuffix(text, "\n"), "\n")
}

// readText reads a file for comparing, or reports false for one that is
// too big or binary (has a NUL byte) - neither has lines worth a {{var}}.
func readText(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxCompare {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil || bytes.IndexByte(data, 0) >= 0 {
		return "", false
	}
	return string(data), true
}
