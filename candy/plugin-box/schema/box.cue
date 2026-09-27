// plugin-box's OWN self-contained CUE schema — the SINGLE SOURCE for this
// plugin's declaration surface. There is NO schema-less plugin: every plugin
// ships a non-empty, self-contained schema, served over Describe (the SDK splices
// `base ++ plugin` at the load gate), and this one DOCUMENTS the plugin's command
// surface.
//
// SELF-CONTAINED: it references NO base def, so it compiles STANDALONE — the exact
// property `cue exp gengotypes` needs to generate Go params, AND the property that
// lets the SDK compile it serve-side.
#BoxPlugin: {
	// The command group every word below NESTS under — part of each capability's
	// declared IDENTITY (`command:<word>:box`), keyed by charly's provider registry.
	parent: "box"

	// The command words this plugin serves, all nested under `parent`. A command's
	// args are pass-through CLI tokens (there is no typed plugin_input), so these
	// words ARE this plugin's authored declaration surface.
	commands: ["generate", "validate", "new", "pull", "build", "inspect", "list", "labels", "load", "merge", "reconcile", "feature"]

	// The `list` word's own subcommand catalog (F-CLI-NEST), declared so the host
	// builds a real nested Kong grammar + the `box.list.*` MCP tools.
	listSubcommands: ["boxes", "candies", "targets", "services", "routes", "volumes", "aliases", "tags"]

	// What the plugin does, in one line (the public-docs surface).
	contract: string & !=""
}
