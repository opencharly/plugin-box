package box

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/opencharly/sdk"
	"github.com/opencharly/sdk/deploykit"
	"github.com/opencharly/spec/spec"
)

// validate_owned_content.go — the engine's ONE entry for the nodes of CONTENT kinds (ops.OpContent;
// opencharly/sdk#598). A kind that declares `contents` owns the rules for its nodes (D-PLUG-1), so the
// engine never interprets a node: it asks each such kind, in PROJECT scope, what is wrong with its
// nodes, and reports the kind's diagnostics. For kind:skill those are the rules this file used to
// hold itself — an image owner (opencharly/sdk#434), an owner that is no candy of the project, a body
// that does not exist at the owner's repo root (opencharly/opencharly#530) — now served by
// opencharly/plugin-harness-kind, which is also what installs and labels the skills in an image, so
// the gate and the build read one rule.
//
// Project scope means composition-independent: every candy of the project and every image, whether
// or not a box composes a node's owner — which is what catches a body moved where nothing reads it in
// a repo that composes no box at all.

// contentAsk is how the engine reaches one content kind; production wires it to InvokeProvider.
type contentAsk func(kind string, req spec.OwnedContentRequest) (*spec.OwnedContentReply, error)

// invokeContentAsk asks kind:<word> over the peer InvokeProvider channel.
func invokeContentAsk(ctx context.Context, exec *sdk.Executor) contentAsk {
	return func(kind string, req spec.OwnedContentRequest) (*spec.OwnedContentReply, error) {
		params, err := json.Marshal(req)
		if err != nil {
			return nil, err
		}
		res, err := exec.InvokeProvider(ctx, "kind", kind, sdk.OpContent, params, nil, sdk.InvokeProviderOpts{})
		if err != nil {
			return nil, err
		}
		var reply spec.OwnedContentReply
		if err := json.Unmarshal(res, &reply); err != nil {
			return nil, fmt.Errorf("decode kind:%s content reply: %w", kind, err)
		}
		return &reply, nil
	}
}

// validateOwnedContent asks every content kind with nodes in the project to diagnose them, in sorted
// kind order so the report is reproducible. A kind that cannot be asked is itself a finding — the
// gate must not read "0 errors" because a rule set was never run.
func validateOwnedContent(cfg *spec.Config, dk map[string]deploykit.CandyModel, kinds []string, ask contentAsk, e *vErr) {
	if cfg == nil {
		return
	}
	sorted := append([]string(nil), kinds...)
	sort.Strings(sorted)
	for _, kind := range sorted {
		if len(cfg.PluginKinds[kind]) == 0 {
			continue
		}
		req, err := deploykit.OwnedContentRequestFor(cfg, dk, kind, "", nil)
		if err != nil {
			e.Add("kind %q: %v", kind, err)
			continue
		}
		reply, err := ask(kind, req)
		if err != nil {
			e.Add("kind %q: the kind could not check its nodes: %v", kind, err)
			continue
		}
		if reply == nil {
			continue
		}
		for _, d := range reply.Diagnostics {
			e.Add("kind %q: %s", kind, d)
		}
	}
}
