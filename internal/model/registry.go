package model

import (
	"context"
	"fmt"
	"strings"

	"github.com/666feiyu666/noise-detection/internal/model/dsp"
	"github.com/666feiyu666/noise-detection/internal/model/rule"
)

// Options carries the settings a factory may need to build a model.
type Options struct {
	// RulesPath is an optional JSON rules file for the local rule model.
	RulesPath string
}

// Factory builds one model. Registry calls it once per requested name.
type Factory func(ctx context.Context, opts Options) (Model, error)

// Registry maps model names to factories, so a caller can select any
// combination of models by name.
type Registry struct {
	factories map[string]Factory
	order     []string
}

// NewRegistry returns a Registry with the built-in models registered.
func NewRegistry() *Registry {
	r := &Registry{factories: map[string]Factory{}}
	r.Register("rule", newRuleModel)
	r.Register("dsp", newDSPModel)
	return r
}

// Register adds or replaces a factory under name. Names keep their first
// registration order.
func (r *Registry) Register(name string, f Factory) {
	if _, ok := r.factories[name]; !ok {
		r.order = append(r.order, name)
	}
	r.factories[name] = f
}

// Names lists the registered model names in registration order.
func (r *Registry) Names() []string {
	return append([]string(nil), r.order...)
}

// Build returns one model per requested name, in the order given. An empty list
// selects every registered model.
func (r *Registry) Build(ctx context.Context, names []string, opts Options) ([]Model, error) {
	if len(names) == 0 {
		names = r.Names()
	}
	out := make([]Model, 0, len(names))
	for _, name := range names {
		f, ok := r.factories[name]
		if !ok {
			return nil, fmt.Errorf("unknown model %q: choose one of %s", name, strings.Join(r.Names(), ", "))
		}
		m, err := f(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("build model %q: %w", name, err)
		}
		out = append(out, m)
	}
	return out, nil
}

// newRuleModel builds the local filename-rule model, optionally from a rules
// file.
func newRuleModel(_ context.Context, opts Options) (Model, error) {
	if opts.RulesPath != "" {
		return rule.NewFromFile(opts.RulesPath)
	}
	return rule.New(), nil
}

// newDSPModel builds the deterministic DSP decision model.
func newDSPModel(_ context.Context, _ Options) (Model, error) {
	return dsp.New(), nil
}
