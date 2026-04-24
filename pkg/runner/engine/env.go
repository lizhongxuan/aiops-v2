package engine

import "context"

type envKey struct{}

func WithEnv(ctx context.Context, env map[string]string) context.Context {
	if len(env) == 0 {
		return ctx
	}
	copied := make(map[string]string, len(env))
	for k, v := range env {
		copied[k] = v
	}
	return context.WithValue(ctx, envKey{}, copied)
}

func envFromContext(ctx context.Context) map[string]string {
	raw := ctx.Value(envKey{})
	if raw == nil {
		return nil
	}
	if env, ok := raw.(map[string]string); ok {
		return env
	}
	return nil
}
