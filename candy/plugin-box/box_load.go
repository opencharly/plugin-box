package box

import (
	"fmt"
	"os/exec"

	"github.com/opencharly/sdk/deploykit"
	"github.com/opencharly/sdk/kit"
	"github.com/opencharly/spec/container"
	specexec "github.com/opencharly/spec/exec"
)

// box_load.go — `charly box load`, the CONTAINER-venue image-delivery verb: it streams a
// locally-built image out of the host store and into the store of a podman running INSIDE a
// running pod deploy. It is the exact twin of `charly vm cp-box` (the VM venue), and both are
// bindings of the one venue-generic path, deploykit.TransferImageToVenue.
//
// Why this verb has to exist rather than a shell pipeline. A nested candybox — a pod composing
// container-nesting, with its own rootless podman serving an API socket at uid 1000 — has an
// image store that is genuinely separate from the host's. Anything that must run INSIDE that
// boundary (the Factory's incident spikes: an agentteams controller spawning its own
// Manager/Worker containers) needs its images in the NESTED store, because the alternative —
// binding the host's podman socket in — dissolves the boundary: the spawned containers land in
// the host store, beside everything else running there. A registry hop is not an escape either;
// the images in question are private, so an anonymous pull 401s. That leaves local delivery, and
// with no verb for it the only path was a hand-run
// `podman save … | podman --remote --url … load` — a manual container-engine command against a
// charly-managed deploy, which the rulebook forbids outright (R4). The missing capability was the
// bug; this closes it.
//
// The load lands through `podman exec -i` rather than from the host directly because the target
// socket lives in the container's own mount namespace — there is no host path to it.

// loadGrammar is the `charly box load <target> <image> [--as] [--socket] [--instance] [--kind]` CLI surface.
type loadGrammar struct {
	Target   string `arg:"" help:"Running deploy (or box) whose venue receives the image — the same name charly shell/cp accept."`
	Image    string `arg:"" help:"Image ref present in HOST podman storage (build it first with charly box build)."`
	As       string `name:"as" help:"After load, tag the image in the venue under this stable ref (e.g. localhost/charly-agentteams-worker:latest)."`
	Socket   string `name:"socket" default:"/run/user/1000/podman/podman.sock" help:"In-venue podman API socket the nested store is served on. The default is the uid-1000 rootless path the container-nesting composition serves."`
	Instance string `name:"instance" help:"Deploy instance suffix, when the target runs more than one."`
	Kind     string `name:"kind" help:"Deliver into a KUBERNETES/KIND cluster's node containerd instead of a pod's nested podman store. Give the kind cluster name (its node is <name>-control-plane); the image then runs in the cluster with NO registry pull."`
}

// dispatchLoad kong-parses the load grammar and runs the verified transfer.
func dispatchLoad(hc *hostClient, args []string) error {
	var g loadGrammar
	done, err := parseLeaf("load", &g, args)
	if done || err != nil {
		return err
	}
	// Shared with `charly vm cp-box` — both delivery verbs need exactly this resolution, and
	// each had grown its own copy until spec's ResolveDeliverableRef collapsed them (R3, in
	// the cutover whose own thesis is that a second venue costs a constructor, not a copy).
	ref, err := container.ResolveDeliverableRef("podman", g.Image)
	if err != nil {
		return fmt.Errorf("box load: %w", err)
	}

	// A kind/node venue is a DIFFERENT store from a pod's nested podman: the target is a
	// cluster's containerd, not a container's podman. Same verified transfer
	// (deploykit.TransferImageToVenue), same `save | load` streaming — only the constructor
	// differs, which is the seam the `charly box load` cutover was built to allow (R3).
	if g.Kind != "" {
		return loadIntoKindNode(hc, g, ref)
	}

	// Resolve the running container the same way charly shell / charly cp do, so a name that
	// works for those works here — and so a stopped target fails with "is not running" rather
	// than a confusing exec error.
	engine, name, err := deploykit.ResolveContainer(hc.ctx, g.Target, g.Instance)
	if err != nil {
		return fmt.Errorf("box load: %w", err)
	}
	if name == "" {
		return fmt.Errorf("box load: %q resolves to the local host, which has no nested store to load into", g.Target)
	}

	socketURL := "unix://" + g.Socket
	// One prefix drives the load, the integrity probe, the tag and any removal, so none of them
	// can address a different store than the image actually landed in. --remote --url is what
	// pins every one of them to the SOCKET's store rather than to whatever local store the
	// in-container podman would otherwise default to.
	podman := "podman --remote --url " + socketURL

	// The probe jump MUST follow the engine ResolveContainer just returned. The engine is
	// DATA on the jump (NestedJump.Engine) — the ONE JumpContainerExec arm builds
	// `<engine> exec -i`, so the probe hop and the load below address the venue through
	// the SAME resolved binary. The retired per-engine jump arm pair made the two halves
	// address the venue through different binaries on a docker deploy: the load ran
	// `docker exec` and the probes ran `podman exec` against the same container. Both then
	// failed at transport level — and VenueHasImage / VenueImageCorrupt both return FALSE on
	// error, so the verified idempotency and the torn-overlay re-stream did not fail, they
	// silently stopped meaning anything. That is the safety property this verb advertises,
	// going vacuous without a symptom.
	// spec/exec/deploy_chain.go already pairs engine→jump this way; this was a wiring
	// omission, not a design question.
	ctx := hc.ctx
	venue := deploykit.ImageVenue{
		Exec:      &specexec.NestedExecutor{Parent: specexec.ShellExecutor{}, Jump: specexec.NestedJump{Kind: specexec.JumpContainerExec, Engine: engine, Target: name}},
		PodmanCmd: podman,
		Rootless:  true,
		Label:     "box load",
		NewLoadCmd: func() *exec.Cmd {
			return exec.CommandContext(ctx, engine, "exec", "-i", name,
				"podman", "--remote", "--url", socketURL, "load")
		},
	}
	if err := deploykit.TransferImageToVenue(ctx, venue, "podman", ref, g.As, deploykit.EmitOpts{}); err != nil {
		// The hint is attached UNCONDITIONALLY rather than keyed on substrings of the
		// error. The previous form matched "Cannot connect" / "no such file", but a
		// missing in-venue socket surfaces as `load failed: exit status 125` from the
		// exec'd process — so the hint never fired for the case it was written for, and
		// DID fire when the HOST podman binary was missing, telling the operator to
		// compose a candy into the box for a fault on their own machine. A hint that
		// cannot identify its case should not pretend to: it now names the socket it
		// tried and leaves the diagnosis to the reader.
		return fmt.Errorf("%w\n\nif the venue serves no podman API socket at %s, compose the "+
			"nested-podman-socket candy into its box or pass --socket", err, g.Socket)
	}
	return nil
}

// kindControlPlaneNode is the node container `kind` creates for a cluster — the venue
// `charly box load --kind <cluster>` delivers into. kind's default topology is one
// control-plane node, and it names it `<cluster>-control-plane` (the same name
// plugin-kube's kindNodeHasImage probes).
func kindControlPlaneNode(cluster string) string { return cluster + "-control-plane" }

// kindCtrImportArgv is the argv, run on the OPERATOR's engine, that reads a `podman save`
// archive on stdin and imports it into the node's containerd. `-n k8s.io` is the namespace
// kubelet reads, so the imported image is visible to the cluster; `import -` reads stdin,
// so no archive ever lands on the node's disk. Pure so it is unit-pinned.
func kindCtrImportArgv(node string) []string {
	return []string{"exec", "-i", node, "ctr", "-n", "k8s.io", "images", "import", "-"}
}

// loadIntoKindNode delivers a host image into a KUBERNETES (kind) cluster's NODE containerd,
// so the cluster runs it with NO registry pull. It is the second binding of the one
// venue-generic path: `deploykit.TransferImageToVenue` does the verified `save | load`, and
// `deploykit.NewNodeVenue` supplies the store-kind seam that speaks `ctr -n k8s.io`.
//
// A kind node is a container on the operator's engine, named `<cluster>-control-plane`, so
// the venue is reached the SAME way the pod venue is — a NestedExecutor over the engine's
// `exec` — but the probe/tag/load verbs are ctr, not podman. The load side streams the host
// archive straight into `ctr -n k8s.io images import -` (no temp file on the node: an import
// reads stdin).
//
// This is the mechanism `opencharly/charly#809` charters: cache an image once, deliver it
// into a pod or a k8s node locally instead of every run re-pulling quay.io/registry.k8s.io.
func loadIntoKindNode(hc *hostClient, g loadGrammar, ref string) error {
	rt, err := kit.ResolveRuntime()
	if err != nil {
		return fmt.Errorf("box load --kind: %w", err)
	}
	engine := kit.EngineBinary(rt.RunEngine)
	node := kindControlPlaneNode(g.Kind)
	if !kit.ContainerRunning(engine, node) {
		return fmt.Errorf("box load --kind: kind cluster %q has no running node container %q on engine %q — "+
			"create it with `kind create cluster --name %s` (or a target:kindcluster deploy) first",
			g.Kind, node, engine, g.Kind)
	}

	ctx := hc.ctx
	// The in-node store verb. `-n k8s.io` is the namespace kubelet reads, so an image imported
	// here is visible to the cluster. Reused verbatim by the probe (HasImage), the tag, the
	// removal and the load — the ONE place the store scope is decided.
	ctr := "ctr -n k8s.io"
	venue := deploykit.NewNodeVenue(
		&specexec.NestedExecutor{Parent: specexec.ShellExecutor{}, Jump: specexec.NestedJump{Kind: specexec.JumpContainerExec, Engine: engine, Target: node}},
		ctr,
		func() *exec.Cmd {
			// The archive is piped through the engine exec into `ctr images import -` — no
			// intermediate tar on the node.
			return exec.CommandContext(ctx, engine, kindCtrImportArgv(node)...)
		},
		"box load --kind",
	)
	if err := deploykit.TransferImageToVenue(ctx, venue, "podman", ref, g.As, deploykit.EmitOpts{}); err != nil {
		return fmt.Errorf("%w\n\nif the kind node %q serves no containerd, the cluster is not up "+
			"(kind creates the node on engine %q); check `%s ps`", err, node, engine, engine)
	}
	return nil
}
