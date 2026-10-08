package box

// reconcile_closure.go — the CLOSURE-aware half of `charly box reconcile` (opencharly/charly#737).
//
// THE GAP THIS CLOSES. `reconcileCandidateFiles` walks only <dir>/charly.yml + box/ + candy/, so
// the per-repo target is computed over the CURRENT PROJECT's own references alone. A repo that
// this project happens to pin ONCE reads "already reconciled" while the closure the project
// actually composes needs a newer tag — and the next partial repin makes the skew WORSE, not
// better (measured: `distro-cachyos#123` advanced one manifest and left the old ref behind in
// another, one day before the audit that filed this).
//
// THE SHAPE OF THE FIX, and why a closure-wide WRITE is impossible. The other repos in the
// closure are READ-ONLY shared cache exports (`spec/refs` derives a view from them and never
// mutates them), so a single project cannot rewrite their pins — attempting it would corrupt the
// pristine cache. So `--closure` does the only correct thing a single project can:
//
//   1. COMPUTE the target over the WHOLE closure — the project's own files PLUS every pinned
//      repo's own manifests, transitively, walked read-only out of the fetch cache;
//   2. REWRITE only the project's OWN files to that closure-wide target; and
//   3. REPORT the skew it cannot repair — every foreign pin that disagrees with the target, with
//      its repo@tag and the file that carries it, so the reader knows exactly which repo owes the
//      change instead of being told "already reconciled" about a tree that is not.
//
// That third leg is the honest exit: the tool now states what it could not fix rather than
// asserting a property it never checked.

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/opencharly/sdk/deploykit"
	"github.com/opencharly/sdk/kit"
	"github.com/opencharly/spec/refs"
	"github.com/opencharly/spec/spec"
	"gopkg.in/yaml.v3"
)

// closureFetchFn is the closure walk's fetch seam — the cached repo fetch
// (spec/refs.DownloadRepo) in production, a package var so tests drive the walk against fixture
// exports without touching the network (the same seam shape as latestTagFn).
var closureFetchFn = refs.DownloadRepo

// foreignPin is one pin found OUTSIDE the project (in a fetched closure repo's own manifest),
// with the repo@version it names. The project cannot rewrite it — it belongs to another repo —
// so it is REPORTED.
type foreignPin struct {
	source  string // the fetched repo the manifest came from, "repo@version"
	file    string // the manifest path within that export
	ref     string // the ref as authored
	repo    string
	version string
}

// closureScan walks the project's WHOLE closure read-only: the project's own manifests (already
// collected as `local`) plus every pinned @github repo's own manifests, transitively. It returns
// the closure-wide version set per repo (the input the alignment target must be computed over)
// and every FOREIGN pin it saw, which the caller reports rather than rewrites.
//
// A ref that cannot be fetched is REPORTED as an unresolved foreign pin, never silently skipped:
// "I could not look at this part of the closure" is a fact the reader needs, and hiding it is the
// same class of defect as the silent ref drop this cluster also fixes.
func closureScan(local []string) (map[string]map[string]bool, []foreignPin, error) {
	versions := map[string]map[string]bool{}
	var foreign []foreignPin

	note := func(repo, version string) {
		if versions[repo] == nil {
			versions[repo] = map[string]bool{}
		}
		versions[repo][version] = true
	}

	// Pass 1: the project's own files — the same refs the caller already collected, re-read here
	// so this function owns ONE definition of "what the closure references".
	var queue []string
	for _, f := range local {
		pins, err := filePins(f)
		if err != nil {
			return nil, nil, err
		}
		for _, p := range pins {
			note(p.repo, p.version)
			queue = append(queue, p.repo+"@"+p.version)
		}
	}

	// Pass 2: transitive fix-point over the fetched closure. Each fetched export is walked
	// READ-ONLY out of the fetch cache; its refs are both folded into the target set and recorded
	// as foreign pins (they live in another repo, so this project cannot rewrite them).
	seen := map[string]bool{}
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		if seen[key] {
			continue
		}
		seen[key] = true

		repo, version := splitRepoVersion(key)
		if repo == "" || version == "" || repo == "github.com/opencharly/charly" {
			continue // an unpinned ref has no export to walk; the in-repo charly form is not a repo
		}
		export, err := closureFetchFn(repo, version)
		if err != nil {
			// Reported, not swallowed: the closure is partially unknown and the caller says so.
			foreign = append(foreign, foreignPin{source: key, file: "<unfetchable>", repo: repo, version: version})
			continue
		}
		for _, f := range closureManifests(export) {
			pins, perr := filePins(f)
			if perr != nil {
				return nil, nil, perr
			}
			for _, p := range pins {
				note(p.repo, p.version)
				foreign = append(foreign, foreignPin{source: key, file: relTo(export, f), ref: refString(p, f), repo: p.repo, version: p.version})
				queue = append(queue, p.repo+"@"+p.version)
			}
		}
	}
	return versions, foreign, nil
}

// closureManifests returns the versioned manifests of one fetched export: its root charly.yml, a
// box/<name>/charly.yml and a candy/<name>/charly.yml. That is the same discovery convention the
// loader and the repo walker use, so the closure is computed over exactly what the closure
// composes.
func closureManifests(dir string) []string {
	var out []string
	if p := filepath.Join(dir, spec.UnifiedFileName); kit.FileExists(p) {
		out = append(out, p)
	}
	for _, sub := range []string{kit.DefaultBoxDir, kit.DefaultCandyDir} {
		ents, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			continue
		}
		for _, de := range ents {
			if !de.IsDir() {
				continue
			}
			p := filepath.Join(dir, sub, de.Name(), spec.UnifiedFileName)
			if kit.FileExists(p) {
				out = append(out, p)
			}
		}
	}
	return out
}

// refPin is one remote ref found in a manifest, with the repo path and version it names.
type refPin struct {
	repo    string
	version string
	raw     string
}

// filePins returns every pinned remote candy ref in one manifest (an unpinned ref names no export,
// so it cannot be part of the alignment set).
func filePins(path string) ([]refPin, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	var out []refPin
	walkScalars(&root, func(s *yaml.Node) {
		if !deploykit.IsRemoteCandyRef(s.Value) {
			return
		}
		p := spec.ParseRemoteRef(s.Value)
		if p.Version == "" {
			return
		}
		out = append(out, refPin{repo: p.RepoPath, version: p.Version, raw: s.Value})
	})
	return out, nil
}

// splitRepoVersion splits a "repo@version" closure key back into its parts. The version part may
// itself contain '@'? It may not — a ref is a tag, branch or SHA — so the FIRST '@' separates.
func splitRepoVersion(key string) (string, string) {
	for i := 0; i < len(key); i++ {
		if key[i] == '@' {
			return key[:i], key[i+1:]
		}
	}
	return key, ""
}

// relTo renders a manifest path relative to its export root, so a report names the manifest the
// way its own repo would.
func relTo(root, path string) string {
	if r, err := filepath.Rel(root, path); err == nil {
		return r
	}
	return path
}

// refString is the ref as the reader would find it in `file`.
func refString(p refPin, _ string) string {
	return p.raw
}
