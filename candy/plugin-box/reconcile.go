package box

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/opencharly/sdk/deploykit"
	"github.com/opencharly/sdk/kit"
	"github.com/opencharly/spec/refs"
	"github.com/opencharly/spec/spec"
	"gopkg.in/yaml.v3"
)

// reconcile.go — `charly box reconcile`, moved from charly/reconcile.go (Cutover B unit 3+4). It
// aligns cross-repo `@github` git-tag pins so every reference of a repo fetches ONE commit —
// clearing any per-entity-version warning from the resolver (which compares each candy's own
// `version:`, read after fetch). For each distinct repo referenced by the project's versioned
// YAML files, every pin of that repo is rewritten to ONE target version: the newest
// already-referenced version (default) or the newest tag on the remote (`--remote`). Edits are
// comment-preserving (yaml.v3 node API) and idempotent. Operates on the current project (cwd; the
// host's -C / --dir / CHARLY_PROJECT_DIR already resolved os.Getwd before this dispatches).
//
// PER PROJECT BY DEFAULT — and that default is a KNOWN GAP (opencharly/charly#737). Walking only
// <dir>/charly.yml + box/ + candy/ computes the target over THIS project's references alone, so a
// repo pinned once reads "already reconciled" while the closure it composes needs a newer tag, and
// a later partial repin makes the skew worse. `--closure` computes the target over the whole
// closure instead (see reconcile_closure.go); it rewrites only this project's own files — the other
// repos are read-only shared cache exports, so a closure-wide WRITE is impossible from one project —
// and REPORTS the skew that belongs to them. Use `--closure` on any tree that composes an imported
// namespace; a bare per-project run can only ever answer for one project, never for the tree.
//
// This word needs NO host reverse-channel coupling at all — it operates purely over sdk/kit
// (FileExists/IsGitSubmoduleDir/SortStrings/GitLatestTag/RepoGitURL/CompareSemver) +
// sdk/deploykit (IsRemoteCandyRef/StripVersion) + sdk/spec (ParseRemoteRef) + stdlib
// filesystem/YAML, exactly like the `new`/`labels` words above — zero HostBuild, zero
// InvokeProvider, zero core reentry.

// reconcileGrammar is the `charly box reconcile [--dry-run] [--remote] [--closure]` CLI surface.
type reconcileGrammar struct {
	DryRun  bool `name:"dry-run" help:"Print the pin rewrites without modifying any file."`
	Remote  bool `help:"Align each repo's pins to its newest REMOTE tag (git ls-remote) instead of the newest already-referenced version."`
	Closure bool `help:"Compute the alignment target over the project's WHOLE closure (every pinned @github repo's own manifests, transitively), not just this project's own files. Rewrites only this project's files and REPORTS the skew that lives in other repos."`
}

// dispatchReconcile kong-parses the reconcile grammar and runs the two-pass rewrite: collect the
// per-repo referenced version set, compute each repo's target version, then rewrite every pin
// that disagrees with its repo's target.
func dispatchReconcile(args []string) error {
	var g reconcileGrammar
	if done, err := parseLeaf("reconcile", &g, args); err != nil || done {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	files := reconcileCandidateFiles(dir)
	if len(files) == 0 {
		return fmt.Errorf("no opencharly project files found in %s (run from a project directory or use -C)", dir)
	}

	// Pass 1: collect, per repo, the set of pinned versions referenced anywhere.
	refVersions := make(map[string]map[string]bool) // repoPath -> set of versions
	roots := make(map[string]*yaml.Node)            // file -> document root (reused in pass 2)
	sources := make(map[string][]byte)              // file -> ORIGINAL bytes (rewritten in pass 2)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("reading %s: %w", f, err)
		}
		var root yaml.Node
		if err := yaml.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("parsing %s: %w", f, err)
		}
		roots[f] = &root
		sources[f] = data
		// ONE manifest-pin reader (filePins, shared with the closure walk): this pass and the
		// closure scan must never disagree about what a manifest references (R3).
		pins, perr := filePins(f)
		if perr != nil {
			return perr
		}
		for _, p := range pins {
			if refVersions[p.repo] == nil {
				refVersions[p.repo] = make(map[string]bool)
			}
			refVersions[p.repo][p.version] = true
		}
	}
	if len(refVersions) == 0 {
		fmt.Println("no @github pins found — nothing to reconcile")
		return nil
	}

	// CLOSURE MODE (charly#737): recompute the version set over the WHOLE closure — this
	// project's own files PLUS every pinned repo's own manifests, transitively, read out of the
	// fetch cache — so a repo this project pins ONCE is aligned to what the closure actually
	// requires instead of reading "already reconciled" while the tree is skewed.
	var foreign []foreignPin
	if g.Closure {
		closureVersions, fp, cerr := closureScan(refVersions)
		if cerr != nil {
			return cerr
		}
		refVersions = closureVersions
		foreign = fp
	}

	// Compute the target version per repo.
	target := make(map[string]string)
	for repo, vers := range refVersions {
		t, err := reconcileTargetVersion(g.Remote, repo, vers)
		if err != nil {
			return err
		}
		target[repo] = t
	}

	// Pass 2: rewrite every pin whose version != its repo's target.
	type rewrite struct{ file, from, to string }
	var rewrites []rewrite
	for _, f := range files {
		root := roots[f]
		var edits []pinEdit
		walkScalars(root, func(s *yaml.Node) {
			if !deploykit.IsRemoteCandyRef(s.Value) {
				return
			}
			p := spec.ParseRemoteRef(s.Value)
			if p.Version == "" {
				return
			}
			want := target[p.RepoPath]
			if p.Version == want {
				return
			}
			stripped, _ := deploykit.StripVersion(s.Value)
			newRef := stripped + ":" + want
			rewrites = append(rewrites, rewrite{filepath.Base(f), s.Value, newRef})
			edits = append(edits, pinEdit{line: s.Line, from: s.Value, to: newRef})
		})
		if len(edits) > 0 && !g.DryRun {
			out, err := applyPinEdits(sources[f], edits)
			if err != nil {
				return fmt.Errorf("rewriting %s: %w", f, err)
			}
			if err := os.WriteFile(f, out, 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", f, err)
			}
		}
	}

	// Report per-repo targets (only the ones that were at >1 version).
	repos := make([]string, 0, len(target))
	for r := range target {
		repos = append(repos, r)
	}
	sort.Strings(repos)
	for _, r := range repos {
		if len(refVersions[r]) > 1 {
			fmt.Printf("%s -> %s (was at %d versions)\n", r, target[r], len(refVersions[r]))
		}
	}
	// REPORT what this project cannot repair (closure mode only): a pin that lives in another repo
	// of the closure is that repo's to advance, and a ref the walk could not fetch at all leaves
	// the closure only PARTLY known — both are named, so this project's own "already reconciled"
	// can never stand for the whole tree.
	_, unfetched := reportClosure(os.Stdout, foreign, target)

	if len(rewrites) == 0 {
		fmt.Println("already reconciled — this project's own pins are at one version")
		if unfetched > 0 {
			fmt.Printf("…but the closure is only PARTLY known: %d ref(s) could not be fetched (above)\n", unfetched)
		}
		return nil
	}
	if g.DryRun {
		fmt.Printf("would rewrite %d pin(s):\n", len(rewrites))
	} else {
		fmt.Printf("rewrote %d pin(s):\n", len(rewrites))
	}
	for _, rw := range rewrites {
		fmt.Printf("  %s: %s -> %s\n", rw.file, rw.from, rw.to)
	}
	return nil
}

// gitClient is the process-wide centralized git layer (spec/refs.GitClient) — the cached
// newest-tag path (1h TTL, disk-persisted, cross-process). reconcileTargetVersion MUST use
// THIS, not the raw refs.GitLatestTag (charly#736, R3): the raw per-process `git ls-remote`
// path reintroduces the 90-94 concurrent ls-remote fanout the cache was built to avoid, and
// lets `reconcile --remote` disagree with the resolver's cached answer inside one run.
var (
	gitClientOnce     sync.Once
	gitClientInstance *refs.GitClient
)

func gitClient() *refs.GitClient {
	gitClientOnce.Do(func() { gitClientInstance = refs.NewGitClient("") })
	return gitClientInstance
}

// latestTagFn is the newest-tag resolver reconcile uses — a package var so tests can prove
// `reconcile --remote` routes through the CACHED client (gitClient().LatestTag), not the raw
// refs.GitLatestTag (charly#736). Production value is the cached one.
var latestTagFn = func(repo string) (string, error) {
	return gitClient().LatestTag(refs.RepoGitURL(repo))
}

// reconcileTargetVersion picks the version every pin of repo should align to: the newest remote
// tag when remote is set, else the newest already-referenced version (CalVer/semver via
// refs.CompareSemver).
func reconcileTargetVersion(remote bool, repo string, vers map[string]bool) (string, error) {
	if remote {
		latest, err := latestTagFn(repo)
		if err != nil {
			return "", fmt.Errorf("resolving newest remote tag for %s: %w", repo, err)
		}
		if latest == "" {
			return "", fmt.Errorf("no tags on remote %s", repo)
		}
		return latest, nil
	}
	newest := ""
	for v := range vers {
		if newest == "" || refs.CompareSemver(v, newest) > 0 {
			newest = v
		}
	}
	return newest, nil
}

// reconcileCandidateFiles returns the versioned YAML files in dir that may carry `@github` refs.
// charly.yml is the single entry point (it carries the namespaced @github imports and every
// inline kind); the per-box and per-candy charly.yml manifests under the discovered box/ and
// candy/ directories carry the rest.
func reconcileCandidateFiles(dir string) []string {
	seen := map[string]struct{}{}
	if p := filepath.Join(dir, spec.UnifiedFileName); kit.FileExists(p) {
		seen[filepath.Clean(p)] = struct{}{}
	}
	// Scan every YAML under the discovered box/ and candy/ directories. A per-box charly.yml
	// can pin a @github `base:`, and a per-candy charly.yml can pin @github deps in its
	// require:/candy: lists (e.g. the cachyos keepassxc-keyring candy); both must be aligned
	// too — otherwise reconciliation is not FULLY automatic and the resolver still warns about
	// a version it cannot reach from the entry point. filepath.Walk on a missing directory is a
	// clean no-op (the root err arm returns nil).
	for _, sub := range []string{kit.DefaultBoxDir, kit.DefaultCandyDir} {
		root := filepath.Join(dir, sub)
		_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil || info == nil {
				return nil
			}
			if info.IsDir() {
				if kit.IsGitSubmoduleDir(p, root) {
					return filepath.SkipDir
				}
				return nil
			}
			if ext := filepath.Ext(p); ext == ".yml" || ext == ".yaml" {
				seen[filepath.Clean(p)] = struct{}{}
			}
			return nil
		})
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	kit.SortStrings(out)
	return out
}

// walkScalars visits every scalar node in a YAML node tree.
func walkScalars(n *yaml.Node, fn func(*yaml.Node)) {
	if n == nil {
		return
	}
	if n.Kind == yaml.ScalarNode {
		fn(n)
		return
	}
	for _, c := range n.Content {
		walkScalars(c, fn)
	}
}

// pinEdit is one pin rewrite, located by the scalar node's 1-based line.
type pinEdit struct {
	line     int
	from, to string
}

// applyPinEdits rewrites pins IN THE ORIGINAL BYTES, touching only the lines that carry a
// changed ref.
//
// The obvious implementation — mutate the parsed nodes and yaml.Marshal the document — is what
// this replaces, because it round-trips every OTHER node through the emitter. yaml.v3 re-wraps
// block scalars to its own width rather than reproducing the authored line breaks, so a 4-pin
// reconcile rewrote every `description:` in the file and produced a ~400-line diff across
// unrelated entities. That makes the prescribed remedy for a version-mismatch warning unusable:
// the diff it generates is itself a review blocker, and it silently reformats prose the author
// wrote deliberately.
//
// A pin is always a single-line scalar, so a line-local replacement is both sufficient and
// lossless — every byte the reconcile did not intend to change is preserved exactly.
func applyPinEdits(src []byte, edits []pinEdit) ([]byte, error) {
	lines := strings.Split(string(src), "\n")
	for _, e := range edits {
		if e.line < 1 || e.line > len(lines) {
			return nil, fmt.Errorf("pin %q reported at line %d, outside the %d-line file", e.from, e.line, len(lines))
		}
		i := e.line - 1
		if !strings.Contains(lines[i], e.from) {
			// Refuse rather than guess: a position that does not hold the value means the node
			// index and the bytes disagree, and a blind replace elsewhere would corrupt the file.
			return nil, fmt.Errorf("pin %q not present on line %d", e.from, e.line)
		}
		lines[i] = strings.Replace(lines[i], e.from, e.to, 1)
	}
	return []byte(strings.Join(lines, "\n")), nil
}
