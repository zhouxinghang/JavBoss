package logging

import (
	"context"
	"testing"
)

func TestTaskContext(t *testing.T) {
	cases := []struct {
		name     string
		task     string
		wantFrom string
		wantFs   string
	}{
		{name: "unset", wantFrom: "", wantFs: ""},
		{name: "set", task: "censored jav idols enrichment", wantFrom: "censored jav idols enrichment", wantFs: ` task="censored jav idols enrichment"`},
		{name: "trimmed", task: "  cover download  ", wantFrom: "cover download", wantFs: ` task="cover download"`},
		{name: "blank ignored", task: "   ", wantFrom: "", wantFs: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := WithTask(context.Background(), tc.task)
			if got := TaskFrom(ctx); got != tc.wantFrom {
				t.Fatalf("TaskFrom = %q, want %q", got, tc.wantFrom)
			}
			if got := TaskField(ctx); got != tc.wantFs {
				t.Fatalf("TaskField = %q, want %q", got, tc.wantFs)
			}
		})
	}

	if got := TaskFrom(nil); got != "" {
		t.Fatalf("TaskFrom(nil) = %q, want empty", got)
	}
	if got := TaskField(nil); got != "" {
		t.Fatalf("TaskField(nil) = %q, want empty", got)
	}
}

func TestWithTaskPreservesExistingLabel(t *testing.T) {
	ctx := WithTask(context.Background(), "outer")
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if got := TaskFrom(taskCtx); got != "outer" {
		t.Fatalf("derived context lost task label: %q", got)
	}
}
