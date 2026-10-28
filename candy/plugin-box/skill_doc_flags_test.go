package box

import (
	"os"
	"path/filepath"
	"regexp"
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

// TestSkillBodiesDocumentOnlyRealFlags is the control. Every flag the body offers for its own
// command must be a flag the command accepts, and every environment variable it pairs with one of
// its flags must be one of the command's own bindings.
func TestSkillBodiesDocumentOnlyRealFlags(t *testing.T) {
	for _, tc := range skillDocCases {
		t.Run(tc.skill, func(t *testing.T) {
			path := filepath.Join("..", "..", ".agents", "skills", tc.skill, "SKILL.md")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			body := strings.Split(string(raw), "\n")
			flags, envs := kongSurface(t, tc.grammar)

			for _, claim := range usageClaims(body, tc.command) {
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
