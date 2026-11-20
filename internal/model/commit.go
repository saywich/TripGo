package model

import "context"

type (
	commitKey   struct{}
	commitHooks struct{ callbacks []func() }
)

func WithCommitHooks(ctx context.Context) (context.Context, func()) {
	hooks := &commitHooks{}
	return context.WithValue(ctx, commitKey{}, hooks), func() {
		for _, fn := range hooks.callbacks {
			fn()
		}
	}
}

func AfterCommit(ctx context.Context, fn func()) {
	if hooks, ok := ctx.Value(commitKey{}).(*commitHooks); ok {
		hooks.callbacks = append(hooks.callbacks, fn)
		return
	}
	fn()
}
