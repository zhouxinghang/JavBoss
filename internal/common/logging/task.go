package logging

import (
	"context"
	"fmt"
	"strings"
)

type taskContextKey struct{}

// WithTask labels ctx with the name of the task performing the work so logs
// emitted deeper in the call stack, including provider HTTP requests, can
// report which task triggered them. An empty name leaves ctx unchanged.
func WithTask(ctx context.Context, name string) context.Context {
	name = strings.TrimSpace(name)
	if ctx == nil || name == "" {
		return ctx
	}
	return context.WithValue(ctx, taskContextKey{}, name)
}

// TaskFrom returns the task label attached by WithTask, or "" when unset.
func TaskFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	name, _ := ctx.Value(taskContextKey{}).(string)
	return name
}

// TaskField renders the attached task label as a leading ` task="..."` log
// field, or "" when no label is set.
func TaskField(ctx context.Context) string {
	name := TaskFrom(ctx)
	if name == "" {
		return ""
	}
	return fmt.Sprintf(" task=%q", name)
}
