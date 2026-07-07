// Package tui implements strata's read-only terminal UI: three tabs
// (Layers / Files / Vars & Rules) plus a per-file drilldown, following the
// design handoff in docs/design/tui.
package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"strata/internal/config"
	"strata/internal/engine"
	"strata/internal/layers"
	"strata/internal/perms"
	"strata/internal/state"
	"strata/internal/subst"
)

// VarUse is one substituted variable as shown in the drilldown.
type VarUse struct{ Name, Value, From string }

// Row is one file in the Files tab: the union of what every OS would manage.
type Row struct {
	Rel             string
	Badge           string            // "{{ }} ⚙ 600" composed
	Winner          string            // layer winning on this machine; "" if none
	Mac, Linux, Win string            // winning layer per OS; "" if absent there
	Resolved        bool              // has a winner on this machine
	Status          engine.FileStatus // valid only if Resolved
	Item            engine.Item       // valid only if Resolved
	Providers       []string          // layers providing this rel, stack order
	SubstVars       []VarUse
	Perm            string // "600 (dots.toml)" or "644 (default)"
	Hook            string // "" if none
	LastHash        string // short last-applied hash, "" if untracked
}

type LayerFile struct {
	Short        string
	Badge        string // may include ▲ (overrides an earlier layer)
	OverriddenBy string // active later layer that wins; "" if shown normally
}

type Layer struct {
	Name   string
	Kind   string // base | os | distro | role
	Active bool
	Files  []LayerFile
}

type VarRow struct {
	Name, Value, From, Default string
	Overridden                 bool
}

// Snapshot is everything the TUI renders, computed once at launch.
type Snapshot struct {
	RepoPath    string // display form (~-shortened)
	MachineName string
	ActiveText  string
	Layers      []Layer
	Rows        []Row
	Vars        []VarRow
	UsedBy      [][2]string // substituted file → var list ("—" if none)
	Hooks       [][2]string
	Perms       [][2]string
	Kind        map[string]string // layer name → kind (for coloring)
	RoleIndex   map[string]int    // role layer name → position (color cycling)
}

func inList(s string, list []string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// layerOf extracts the layer name from an absolute source path under repoDir.
func layerOf(repoDir, src string) string {
	if src == "" {
		return ""
	}
	rel, err := filepath.Rel(repoDir, src)
	if err != nil {
		return ""
	}
	return strings.Split(filepath.ToSlash(rel), "/")[0]
}

// canonicalColumns lists every layer dir in the repo in stack-display order:
// base, mac, linux, <distro/other dirs>, windows, <roles in machine order>.
func canonicalColumns(repoDir string, roles []string) ([]string, map[string]string) {
	entries, _ := os.ReadDir(repoDir)
	dirs := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs[e.Name()] = true
		}
	}
	kind := map[string]string{"base": "base", "mac": "os", "linux": "os", "windows": "os"}
	roleSet := map[string]bool{}
	for _, r := range roles {
		roleSet[r] = true
		kind[r] = "role"
	}
	var others []string
	for d := range dirs {
		if kind[d] == "" && !roleSet[d] {
			others = append(others, d)
			kind[d] = "distro"
		}
	}
	sort.Strings(others)

	var cols []string
	add := func(names ...string) {
		for _, n := range names {
			if dirs[n] {
				cols = append(cols, n)
			}
		}
	}
	add("base", "mac", "linux")
	add(others...)
	add("windows")
	add(roles...)
	return cols, kind
}

// Build assembles the Snapshot from the same packages the CLI commands use.
// goos/osRelease are parameters so tests can simulate any platform.
func Build(rc config.RepoConfig, mc config.MachineConfig, home string, st state.State, goos, osRelease, hostname string) (*Snapshot, error) {
	cfg := config.Merge(rc, mc)

	hereOrder := layers.Order(cfg.RoleLayers, goos, osRelease)
	activeIdx := map[string]int{}
	for i, n := range hereOrder {
		activeIdx[n] = i
	}

	resHere, err := layers.Resolve(cfg.RepoDir, hereOrder)
	if err != nil {
		return nil, err
	}
	linuxRelease := ""
	if goos == "linux" {
		linuxRelease = osRelease
	}
	resMac, err := layers.Resolve(cfg.RepoDir, layers.Order(cfg.RoleLayers, "darwin", ""))
	if err != nil {
		return nil, err
	}
	resLinux, err := layers.Resolve(cfg.RepoDir, layers.Order(cfg.RoleLayers, "linux", linuxRelease))
	if err != nil {
		return nil, err
	}
	resWin, err := layers.Resolve(cfg.RepoDir, layers.Order(cfg.RoleLayers, "windows", ""))
	if err != nil {
		return nil, err
	}

	items, err := engine.Plan(cfg, home, st, goos, osRelease)
	if err != nil {
		return nil, err
	}
	byRel := map[string]engine.Item{}
	for _, it := range items {
		byRel[it.Rel] = it
	}

	cols, kind := canonicalColumns(cfg.RepoDir, cfg.RoleLayers)
	colFiles := map[string]map[string]string{} // layer → rel → src
	for _, name := range cols {
		m, err := layers.Resolve(cfg.RepoDir, []string{name})
		if err != nil {
			return nil, err
		}
		colFiles[name] = m
	}

	badgeFor := func(rel string) (string, string, bool) {
		var b []string
		if inList(rel, cfg.Substitute) {
			b = append(b, "{{ }}")
		}
		hook := cfg.Hooks[rel]
		if hook != "" {
			b = append(b, "⚙")
		}
		rule, hasRule := perms.RuleFor(rel, cfg.Permissions)
		if hasRule {
			b = append(b, rule)
		}
		return strings.Join(b, " "), rule, hasRule
	}

	// Files tab rows: union across all OS resolutions.
	relSet := map[string]bool{}
	for _, m := range []map[string]string{resHere, resMac, resLinux, resWin} {
		for rel := range m {
			relSet[rel] = true
		}
	}
	rels := make([]string, 0, len(relSet))
	for rel := range relSet {
		rels = append(rels, rel)
	}
	sort.Strings(rels)

	var rows []Row
	for _, rel := range rels {
		r := Row{
			Rel:    rel,
			Winner: layerOf(cfg.RepoDir, resHere[rel]),
			Mac:    layerOf(cfg.RepoDir, resMac[rel]),
			Linux:  layerOf(cfg.RepoDir, resLinux[rel]),
			Win:    layerOf(cfg.RepoDir, resWin[rel]),
		}
		if it, ok := byRel[rel]; ok {
			r.Resolved, r.Status, r.Item = true, it.Status, it
		}
		for _, c := range cols {
			if _, ok := colFiles[c][rel]; ok {
				r.Providers = append(r.Providers, c)
			}
		}
		badge, rule, hasRule := badgeFor(rel)
		r.Badge = badge
		r.Hook = cfg.Hooks[rel]
		if hasRule {
			r.Perm = rule + " (dots.toml)"
		} else {
			r.Perm = "644 (default)"
		}
		if h, ok := st.Files[rel]; ok && len(h) >= 8 {
			r.LastHash = h[:8]
		}
		if inList(rel, cfg.Substitute) {
			src := resHere[rel]
			if src == "" && len(r.Providers) > 0 {
				src = colFiles[r.Providers[0]][rel]
			}
			if src != "" {
				if content, err := os.ReadFile(src); err == nil {
					for _, name := range subst.Tokens(content) {
						vu := VarUse{Name: name, Value: cfg.Vars[name], From: "dots.toml"}
						if _, ok := mc.Vars[name]; ok {
							vu.From = "machine.toml"
						}
						r.SubstVars = append(r.SubstVars, vu)
					}
				}
			}
		}
		rows = append(rows, r)
	}

	// Layers tab columns.
	var lyrs []Layer
	roleIndex := map[string]int{}
	for i, rname := range cfg.RoleLayers {
		roleIndex[rname] = i
	}
	for ci, name := range cols {
		_, active := activeIdx[name]
		ly := Layer{Name: name, Kind: kind[name], Active: active}
		var lrels []string
		for rel := range colFiles[name] {
			lrels = append(lrels, rel)
		}
		sort.Strings(lrels)
		for _, rel := range lrels {
			lf := LayerFile{Short: strings.TrimPrefix(rel, ".config/")}
			overrides := false
			for _, prev := range cols[:ci] {
				if _, ok := colFiles[prev][rel]; ok {
					overrides = true
				}
			}
			if active {
				if w := layerOf(cfg.RepoDir, resHere[rel]); w != "" && w != name {
					if wi, ok := activeIdx[w]; ok && wi > activeIdx[name] {
						lf.OverriddenBy = w
					}
				}
			}
			badge, _, _ := badgeFor(rel)
			var b []string
			if overrides {
				b = append(b, "▲")
			}
			if badge != "" {
				b = append(b, badge)
			}
			lf.Badge = strings.Join(b, " ")
			ly.Files = append(ly.Files, lf)
		}
		lyrs = append(lyrs, ly)
	}

	// Vars tab.
	varNames := map[string]bool{}
	for n := range rc.Vars {
		varNames[n] = true
	}
	for n := range mc.Vars {
		varNames[n] = true
	}
	var names []string
	for n := range varNames {
		names = append(names, n)
	}
	sort.Strings(names)
	var vars []VarRow
	for _, n := range names {
		def, hasDef := rc.Vars[n]
		mval, hasM := mc.Vars[n]
		vr := VarRow{Name: n}
		switch {
		case hasM && hasDef && mval != def:
			vr.Value, vr.From, vr.Default, vr.Overridden = mval, "machine.toml", def, true
		case hasM && hasDef:
			vr.Value, vr.From, vr.Default = mval, "machine.toml", "same"
		case hasM:
			vr.Value, vr.From, vr.Default = mval, "machine.toml", "—"
		default:
			vr.Value, vr.From, vr.Default = def, "dots.toml", "same"
		}
		vars = append(vars, vr)
	}

	rowByRel := map[string]*Row{}
	for i := range rows {
		rowByRel[rows[i].Rel] = &rows[i]
	}
	var usedBy [][2]string
	substSorted := append([]string(nil), cfg.Substitute...)
	sort.Strings(substSorted)
	for _, rel := range substSorted {
		val := "—"
		if r, ok := rowByRel[rel]; ok && len(r.SubstVars) > 0 {
			var vn []string
			for _, v := range r.SubstVars {
				vn = append(vn, v.Name)
			}
			val = strings.Join(vn, ", ")
		}
		usedBy = append(usedBy, [2]string{rel, val})
	}

	var hooks, permsList [][2]string
	for _, k := range sortedKeys(cfg.Hooks) {
		hooks = append(hooks, [2]string{k, cfg.Hooks[k]})
	}
	for _, k := range sortedKeys(cfg.Permissions) {
		permsList = append(permsList, [2]string{k, cfg.Permissions[k]})
	}

	repoPath := cfg.RepoDir
	if uh, err := os.UserHomeDir(); err == nil && strings.HasPrefix(repoPath, uh) {
		repoPath = "~" + repoPath[len(uh):]
	}

	return &Snapshot{
		RepoPath:    repoPath,
		MachineName: hostname,
		ActiveText:  strings.Join(hereOrder, ", "),
		Layers:      lyrs,
		Rows:        rows,
		Vars:        vars,
		UsedBy:      usedBy,
		Hooks:       hooks,
		Perms:       permsList,
		Kind:        kind,
		RoleIndex:   roleIndex,
	}, nil
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
