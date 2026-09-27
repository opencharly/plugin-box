// plugin-box's OWN self-contained CUE schema — the SINGLE SOURCE for this plugin's
// served declaration surface (there is no schema-less plugin: every plugin ships a
// non-empty schema over Describe).
//
// SELF-CONTAINED and PACKAGE-LESS: it references no base def and carries no package
// clause, so it compiles STANDALONE — the property the SDK's serve-side compile needs
// and the property that lets the host splice `base ++ plugin` at the load gate
// (registerPluginUnitSchema); a self-contained schema that will not splice is a LOUD
// load failure.
//
// NO GO CONSUMER: the plugin declares no typed `plugin_input` (its authored input is
// its pass-through CLI grammar), so this schema generates NO `params` package and has
// NO `cue exp gengotypes` artifact — it is the SERVED documentation/config surface,
// not a code-generation source.
//
// It DOCUMENTS the `command:<word>:box` surface: the `box` parent, its twelve words, and `list`'s own subcommand catalog.
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
