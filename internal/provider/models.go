package provider

import "context"

// ModelLister is an optional capability: providers that can enumerate the
// models available to the configured account/endpoint implement it. The config
// TUI uses it to offer a pick-list instead of free-text. CLI providers don't
// implement it (the CLI owns model/auth selection).
type ModelLister interface {
	ListModels(ctx context.Context) ([]string, error)
}
