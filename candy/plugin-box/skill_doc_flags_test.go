package box

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

// skill_doc_flags_test.go — the control that keeps this repo's skill bodies TRUE.
//
// Every `.agents/skills/<verb>/SKILL.md` in this repo is published VERBATIM into the
// opencharly/marketplace corpus, and every harness loads that corpus. A body that documents a flag
// this plugin's grammar does not define therefore publishes a command that fails AT THE FLAG.
// Two spellings have carried that class into the corpus: `--platform`, deleted from the pull
// grammar (opencharly/plugin-box#40) while the body still documented it, and the inner-concurrency
// flag, whose contract spelling (`--stage-parallelism` / `CHARLY_STAGE_PARALLELISM`) the grammar
// did not accept until this cutover (opencharly/plugin-box#65).
//
// The control reads each body from the SAME path the corpus generator reads it from, and asserts
// that the surface the body presents for its OWN command is one this plugin's Kong grammar
// actually defines. Kong is the authority, not a hand-written list: the flag names and the env
// bindings come from the grammar structs themselves, so the surface cannot drift.
//
// The SET of bodies it reads is read from the tree too, because a body the control does not read is
// a body the control cannot check. A hand-kept list of them is a list that drifts the moment a body
// is added, and the drift is invisible: the control stays green over a body it has never seen
// (opencharly/plugin-box#67). So the TREE names the bodies and the cases name what each one
// documents, and the control asserts BOTH directions — every body in the tree has a case, every case
// names a body that exists — and that no body is read vacuously.
//
// It is a Go test rather than a prose review because a prose review cannot fail, and a control that
// cannot fail is not coverage (R7).

// What the control reads — the surface Kong owns, and nothing else:
//
//   - A USAGE CLAIM is a text that presents an invocation of the command: a line inside a fenced
//     code block that invokes the command, or an inline code span that begins with the command.
//     Every `--flag` in a usage claim must be a flag the command accepts.
//   - An ALIAS ROW is a markdown table row whose first cell is a `--flag`. Such a row is the body
//     claiming "this flag has this environment variable", and Kong binds an environment variable
//     only as the alias of a flag — so both names in such a row must be the grammar's.
//   - A FLAG BULLET is a list item under a heading that names flags. Each of these bodies
//     documents exactly ONE command, so a flags section in it is that command's flags. A bullet
//     that names a flag and an environment variable makes the same alias claim as an alias row.
//
// What the control deliberately does NOT read, and why:
//
//   - PROSE that mentions the command without invoking it. The project-directory override paragraph
//     every body carries ("`charly box build` resolves `charly.yml` via `os.Getwd()`. Override with
//     `-C <dir>` / `--dir <dir>` / `CHARLY_PROJECT_DIR=<dir>`") names the HOST's project-directory
//     surface, not this plugin's: `-C`, `--dir` and `CHARLY_PROJECT_DIR` belong to the core's
//     dispatch, so holding the plugin's grammar responsible for them would be a false claim.
//   - An environment variable that an implementation reads directly rather than through Kong (a
//     diagnostic hook such as `CHARLY_MERGE_KEEP_TMP`, whose body cites the code that reads it).
//     Kong does not bind it, and this repo's grammar is not the place that owns it.
//
// Neither exclusion is an allowlist of names: both are stated RULES about which SURFACE is read,
// so a later prose rewrite cannot smuggle a false flag in through a sentence.

// flagToken matches a CLI flag as it is written in prose: `--name`, with the name's characters
// restricted to what Kong accepts for a flag name.
var flagToken = regexp.MustCompile(`--[a-z0-9][a-z0-9-]*`)

// envToken matches an environment variable as it is written in prose.
var envToken = regexp.MustCompile(`CHARLY_[A-Z0-9_]+`)

// flagHeading matches a heading that names flags.
var flagHeading = regexp.MustCompile(`(?i)\bflags?\b`)

// skillDocCase is one skill body and the command surface it documents.
type skillDocCase struct {
	skill   string // the skill's directory under .agents/skills/
	command string // the command whose usage claims the body is scanned for
	grammar any    // the Kong-tagged grammar struct that command parses into (nil: no flag surface)
}

// skillDocCases is the set of bodies this control reads. The grammar is the struct the command
// actually parses into (see dispatchBuild and its siblings), so a flag the grammar defines and a
// flag the body documents are compared against the SAME source of truth.
//
// `list` has no Kong grammar: `charly box list` dispatches its subcommands by hand (dispatchList),
// so it has no flag surface at all and its body may document no flag.
var skillDocCases = []skillDocCase{
	{skill: "build", command: "charly box build", grammar: &buildGrammar{}},
	{skill: "generate", command: "charly box generate", grammar: &generateGrammar{}},
	{skill: "validate", command: "charly box validate", grammar: &validateGrammar{}},
	{skill: "pull", command: "charly box pull", grammar: &pullGrammar{}},
	{skill: "merge", command: "charly box merge", grammar: &mergeGrammar{}},
	{skill: "inspect", command: "charly box inspect", grammar: &inspectGrammar{}},
	{skill: "load", command: "charly box load", grammar: &loadGrammar{}},
	{skill: "reconcile", command: "charly box reconcile", grammar: &reconcileGrammar{}},
	{skill: "new", command: "charly box new", grammar: &newGrammar{}},
	{skill: "list", command: "charly box list", grammar: nil},
}

// skillsRoot is the tree the bodies live in — the SAME path the corpus generator reads them from.
const skillsRoot = "../../.agents/skills"

// skillBodyNames returns the names of the skill bodies in the tree: every directory under root that
// holds a SKILL.md. The set is READ, never listed: a list of bodies is a list that is one body
// short the moment a body is added, and the control that reads a list cannot see the body it does
// not read.
func skillBodyNames(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, e.Name(), "SKILL.md")); err != nil {
			continue
		}
		names = append(names, e.Name())
	}
	return names, nil
}

// skillDocCoverage compares the bodies in the tree with the cases the control reads, and returns
// the bodies NO case covers and the cases that name NO body. Both directions are findings: an
// uncovered body is read by nothing, and a stale case asserts about a body that is not there.
func skillDocCoverage(root string, cases []skillDocCase) (uncovered, stale []string, err error) {
	bodies, err := skillBodyNames(root)
	if err != nil {
		return nil, nil, err
	}
	covered := make(map[string]bool, len(cases))
	for _, tc := range cases {
		covered[tc.skill] = true
	}
	for _, name := range bodies {
		if !covered[name] {
			uncovered = append(uncovered, name)
		}
	}
	for _, tc := range cases {
		if !slices.Contains(bodies, tc.skill) {
			stale = append(stale, tc.skill)
		}
	}
	return uncovered, stale, nil
}

// kongSurface returns the flag names and env bindings a Kong grammar defines. It walks Kong's own
// model, so the answer is what the parser actually accepts — not a list maintained beside it.
func kongSurface(t *testing.T, grammar any) (flags map[string]bool, envs map[string]bool) {
	t.Helper()
	flags = map[string]bool{}
	envs = map[string]bool{}
	if grammar == nil {
		return flags, envs
	}
	parser, err := kong.New(grammar)
	if err != nil {
		t.Fatalf("kong.New(%T): %v", grammar, err)
	}
	add := func(fs []*kong.Flag) {
		for _, f := range fs {
			flags[f.Name] = true
			for _, e := range f.Envs {
				envs[e] = true
			}
		}
	}
	add(parser.Model.Flags)
	var walk func(n *kong.Node)
	walk = func(n *kong.Node) {
		for _, c := range n.Children {
			add(c.Flags)
			walk(c)
		}
	}
	walk(parser.Model.Node)
	return flags, envs
}

// docClaim is one place in a body where the body makes a claim about the command's surface.
type docClaim struct {
	line int    // the 1-based line the claim is on
	text string // the text the claim is made in
}

// invocationText returns the text when it invokes the command — with an optional leading
// environment assignment (`CHARLY_X=1 charly box build …`) and an optional leading backtick
// stripped — and reports false otherwise.
func invocationText(text, command string) (string, bool) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "`")
	if eq := strings.Index(text, "="); eq > 0 && strings.HasPrefix(text, "CHARLY_") {
		text = strings.TrimSpace(text[eq+1:])
	}
	if !strings.HasPrefix(text, command) {
		return "", false
	}
	return text, true
}

// inlineSpans returns the inline code spans of a line, in order.
func inlineSpans(line string) []string {
	var out []string
	rest := line
	for {
		i := strings.Index(rest, "`")
		if i < 0 {
			return out
		}
		rest = rest[i+1:]
		j := strings.Index(rest, "`")
		if j < 0 {
			return out
		}
		out = append(out, rest[:j])
		rest = rest[j+1:]
	}
}

// usageClaims returns the body's usage claims for the command: the lines inside a fenced code
// block that invoke it, and the inline code spans that begin with it. A prose sentence that merely
// mentions the command is not a usage claim — it is prose, and prose is not what a reader copies.
func usageClaims(body []string, command string) []docClaim {
	var out []docClaim
	inBlock := false
	for i, raw := range body {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") {
			inBlock = !inBlock
			continue
		}
		if inBlock {
			if text, ok := invocationText(line, command); ok {
				out = append(out, docClaim{line: i + 1, text: text})
			}
			continue
		}
		for _, span := range inlineSpans(line) {
			if text, ok := invocationText(span, command); ok {
				out = append(out, docClaim{line: i + 1, text: text})
			}
		}
	}
	return out
}

// aliasRows returns the body's alias rows: the markdown table rows whose first cell is a flag.
// Such a row is the body claiming "this flag has this environment variable".
func aliasRows(body []string) []docClaim {
	var out []docClaim
	for i, raw := range body {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) < 3 {
			continue
		}
		flagCell := strings.TrimSpace(cells[1])
		if !flagToken.MatchString(flagCell) {
			continue
		}
		out = append(out, docClaim{line: i + 1, text: line})
	}
	return out
}

// flagSectionBullets returns the body's flag bullets: the list items under a heading that names
// flags. These bodies document one command each, so such a bullet is a claim about that command's
// flags.
func flagSectionBullets(body []string) []docClaim {
	var out []docClaim
	inFlagSection := false
	for i, raw := range body {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "#") {
			inFlagSection = flagHeading.MatchString(line)
			continue
		}
		if !inFlagSection {
			continue
		}
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "- ") && !strings.HasPrefix(line, "* ") && !strings.HasPrefix(line, "+ ") {
			continue
		}
		out = append(out, docClaim{line: i + 1, text: line})
	}
	return out
}

// skillDocIsVacuous reports whether a body's usage claims for the command it documents are
// EMPTY — the state in which the control compares nothing in that body, so every assertion
// about it passes by default. The rule lives in a function so that the guard has its own RED
// ARM: a control that cannot fail is not coverage.
//
// It takes the CLAIMS, not the body: the control needs those claims for the loop below, so
// they are computed ONCE and handed to both the guard and the loop. A body-and-command
// signature would recompute them here — the same claims, read twice, for one decision.
func skillDocIsVacuous(claims []docClaim) bool {
	return len(claims) == 0
}

// TestSkillBodiesDocumentOnlyRealFlags is the control. Every flag the body offers for its own
// command must be a flag the command accepts, and every environment variable it pairs with one of
// its flags must be one of the command's own bindings.
func TestSkillBodiesDocumentOnlyRealFlags(t *testing.T) {
	uncovered, stale, err := skillDocCoverage(skillsRoot, skillDocCases)
	if err != nil {
		t.Fatalf("read %s: %v", skillsRoot, err)
	}
	for _, name := range uncovered {
		t.Errorf("the skill body %s is not covered: add its command and grammar to skillDocCases, or the control reads nothing in it", name)
	}
	for _, name := range stale {
		t.Errorf("skillDocCases names %s, which is no skill body in %s", name, skillsRoot)
	}

	for _, tc := range skillDocCases {
		t.Run(tc.skill, func(t *testing.T) {
			path := filepath.Join(skillsRoot, tc.skill, "SKILL.md")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			body := strings.Split(string(raw), "\n")
			flags, envs := kongSurface(t, tc.grammar)

			claims := usageClaims(body, tc.command)
			if skillDocIsVacuous(claims) {
				// A body that never invokes its own command is read by NOTHING here: the usage
				// claims are what tie a flag in the prose to this command's grammar, and a body
				// with none of them passes every assertion below without being compared to
				// anything. A control that reads nothing is not coverage.
				t.Errorf("%s: the body makes no usage claim for %s, so the control compares nothing in it", path, tc.command)
			}

			for _, claim := range claims {
				for _, m := range flagToken.FindAllString(claim.text, -1) {
					name := strings.TrimPrefix(m, "--")
					if !flags[name] {
						t.Errorf("%s:%d: the body documents %s, but %s does not define that flag",
							path, claim.line, m, tc.command)
					}
				}
			}

			for _, claim := range aliasRows(body) {
				cells := strings.Split(claim.text, "|")
				for _, m := range flagToken.FindAllString(cells[1], -1) {
					name := strings.TrimPrefix(m, "--")
					if !flags[name] {
						t.Errorf("%s:%d: the body pairs %s with an environment variable, but %s does not define that flag",
							path, claim.line, m, tc.command)
					}
				}
				for _, m := range envToken.FindAllString(claim.text, -1) {
					if !envs[m] {
						t.Errorf("%s:%d: the body pairs a flag with %s, but %s does not bind that environment variable",
							path, claim.line, m, tc.command)
					}
				}
			}

			for _, claim := range flagSectionBullets(body) {
				flagNames := flagToken.FindAllString(claim.text, -1)
				for _, m := range flagNames {
					name := strings.TrimPrefix(m, "--")
					if !flags[name] {
						t.Errorf("%s:%d: the body lists %s among the command's flags, but %s does not define that flag",
							path, claim.line, m, tc.command)
					}
				}
				if len(flagNames) == 0 {
					continue
				}
				for _, m := range envToken.FindAllString(claim.text, -1) {
					if !envs[m] {
						t.Errorf("%s:%d: the body pairs a flag with %s, but %s does not bind that environment variable",
							path, claim.line, m, tc.command)
					}
				}
			}
		})
	}
}

// TestSkillDocCoverageIsEffective is the coverage check's own RED ARM: it must fail on a body no
// case covers, and on a case that names no body. A coverage check that cannot fail proves nothing,
// and a coverage check that cannot fail is the defect this control exists to catch wearing a
// different hat — the control reads a set, and a set that is never compared with the tree is a set
// that drifts.
func TestSkillDocCoverageIsEffective(t *testing.T) {
	root := t.TempDir()
	body := filepath.Join(root, "extra")
	if err := os.MkdirAll(body, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", body, err)
	}
	if err := os.WriteFile(filepath.Join(body, "SKILL.md"), []byte("---\nname: extra\ndescription: a body no case covers\n---\n"), 0o644); err != nil {
		t.Fatalf("write the body: %v", err)
	}

	cases := []skillDocCase{{skill: "extra", command: "charly box extra"}}
	uncovered, stale, err := skillDocCoverage(root, cases)
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if len(uncovered) != 0 || len(stale) != 0 {
		t.Fatalf("the covered tree reported uncovered=%v stale=%v", uncovered, stale)
	}

	// The case is dropped: the body in the tree is read by nothing, and the check must say so.
	uncovered, stale, err = skillDocCoverage(root, nil)
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if !slices.Contains(uncovered, "extra") {
		t.Fatalf("the coverage check did not report the uncovered body: %v", uncovered)
	}

	// A case names a body that is not there: the check must say that too.
	cases = append(cases, skillDocCase{skill: "absent", command: "charly box absent"})
	uncovered, stale, err = skillDocCoverage(root, cases)
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if !slices.Contains(stale, "absent") {
		t.Fatalf("the coverage check did not report the stale case: %v", stale)
	}
}

// TestSkillDocVacuityIsEffective is the VACUITY GUARD's RED ARM: the guard must fire on a body
// that never invokes the command it documents, and stay silent on a body that does. The guard is
// what stops a case from passing by comparing nothing, so a guard that cannot fire would leave
// the control's silence indistinguishable from its agreement.
func TestSkillDocVacuityIsEffective(t *testing.T) {
	neverInvokes := []string{
		"# The command",
		"",
		"The command is described in prose, never run.",
		"",
		"```",
		"charly box other --flag",
		"```",
	}
	if !skillDocIsVacuous(usageClaims(neverInvokes, "charly box build")) {
		t.Fatal("the vacuity guard did not fire on a body that never invokes the command it documents")
	}

	invokes := []string{
		"# The command",
		"",
		"```",
		"charly box build --flag",
		"```",
	}
	if skillDocIsVacuous(usageClaims(invokes, "charly box build")) {
		t.Fatal("the vacuity guard fired on a body that DOES invoke the command it documents")
	}
}
