package report

import (
	"testing"
	"time"

	gh "github.com/google/go-github/v69/github"
)

func TestPRDisplayState(t *testing.T) {
	mergedAt := &gh.Timestamp{Time: time.Now()}
	for _, tc := range []struct {
		name     string
		state    string
		draft    bool
		mergedAt *gh.Timestamp
		want     string
	}{
		{name: "open", state: "open", want: "open"},
		{name: "open draft", state: "open", draft: true, want: "draft"},
		{name: "closed", state: "closed", want: "closed"},
		{name: "closed draft", state: "closed", draft: true, want: "closed"},
		{name: "merged", state: "closed", mergedAt: mergedAt, want: "merged"},
		{name: "merged with draft flag", state: "closed", draft: true, mergedAt: mergedAt, want: "merged"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pr := &gh.PullRequest{State: gh.Ptr(tc.state), Draft: gh.Ptr(tc.draft), MergedAt: tc.mergedAt}
			if got := prDisplayState(pr); got != tc.want {
				t.Errorf("prDisplayState() = %q, want %q", got, tc.want)
			}
		})
	}
}
