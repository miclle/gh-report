package contribution

import (
	"testing"
	"time"

	gh "github.com/google/go-github/v69/github"

	"github.com/miclle/gh-report/github"
)

func TestAggregateContributionAttribution(t *testing.T) {
	a := newAccumulator()
	alice := &gh.User{NodeID: gh.Ptr("U_1"), Login: gh.Ptr("alice")}
	bob := &gh.User{NodeID: gh.Ptr("U_2"), Login: gh.Ptr("bob")}
	bot := &gh.User{NodeID: gh.Ptr("B_3"), Login: gh.Ptr("helper"), Type: gh.Ptr("Bot")}
	for _, c := range []github.CommitActivity{
		{SHA: "a", Author: github.CommitAuthor{User: &struct{ ID, Login string }{"U_1", "alice"}}, Additions: 10, Deletions: 3},
		{SHA: "binary", Author: github.CommitAuthor{User: &struct{ ID, Login string }{"U_1", "alice"}}},
		{SHA: "merge", Merge: true, Additions: 999},
		{SHA: "empty", Empty: true},
		{SHA: "git1", Author: github.CommitAuthor{Name: "Same Name", Email: "OTHER@example.com"}, Additions: 2},
		{SHA: "git2", Author: github.CommitAuthor{Name: "Different Name", Email: "other@example.com"}, Deletions: 1},
		{SHA: "git3", Author: github.CommitAuthor{Name: "Same Name", Email: "third@example.com"}},
		{SHA: "no-email-1", Author: github.CommitAuthor{Name: "Same Name"}},
		{SHA: "no-email-2", Author: github.CommitAuthor{Name: "Same Name"}},
		{SHA: "bot", Author: github.CommitAuthor{User: &struct{ ID, Login string }{"B_3", "helper"}}, Additions: 20},
	} {
		if err := a.addCommit(c); err != nil {
			t.Fatal(err)
		}
	}
	for _, reason := range []string{"completed", "not_planned", "duplicate", ""} {
		a.addIssue(&gh.Issue{User: alice, State: gh.Ptr("closed"), StateReason: gh.Ptr(reason), ClosedBy: bob, Assignees: []*gh.User{bob}})
	}
	a.addIssue(&gh.Issue{User: alice, State: gh.Ptr("open"), StateReason: gh.Ptr("completed")})
	a.addIssue(&gh.Issue{User: bob, State: gh.Ptr("open")})
	a.addIssue(&gh.Issue{User: bob, PullRequestLinks: &gh.PullRequestLinks{}})
	a.addPR(&gh.PullRequest{User: alice, State: gh.Ptr("closed"), MergedAt: &gh.Timestamp{Time: time.Now()}, MergedBy: bob})
	a.addPR(&gh.PullRequest{User: alice, State: gh.Ptr("closed")})
	a.addPR(&gh.PullRequest{User: bot, State: gh.Ptr("open")})
	a.addIssue(&gh.Issue{State: gh.Ptr("closed")})
	report := &Report{Sort: "changes", ExcludeBots: true}
	a.finish(report)
	if len(report.Users) != 7 {
		t.Fatalf("got %d users, want 7 (including issue-only, unknown and separate git authors)", len(report.Users))
	}
	got := report.Users[0]
	if got.Login != "alice" || got.Commits != 2 || got.Changes != 13 || got.IssuesCreated != 5 || got.IssuesAuthoredCompleted != 1 || got.IssuesClosedUnknownReason != 1 || got.PRsCreated != 2 || got.PRsMerged != 1 {
		t.Fatalf("wrong author attribution or completion rules: %+v", got)
	}
	if report.Totals.Commits != 7 || report.Totals.Changes != 16 || report.Totals.IssuesCreated != 7 || report.Totals.IssuesClosedUnknownReason != 2 || report.Totals.PRsCreated != 2 {
		t.Fatalf("totals must match filtered users: %+v", report.Totals)
	}
	if _, ok := a.users["github:U_2"]; !ok || a.users["github:U_2"].IssuesCreated != 1 || a.users["github:U_2"].PRsMerged != 0 {
		t.Fatal("issue-only user missing or credited for another author's PR")
	}
	if a.users["git:other@example.com"].Commits != 2 {
		t.Fatal("same unlinked email must aggregate despite author name changes")
	}
}

func TestSortUsersAndBotSuffix(t *testing.T) {
	for _, test := range []struct{ sort, first string }{
		{"commits", "alice"}, {"changes", "bob"}, {"issues", "bob"}, {"issues-completed", "bob"}, {"prs", "bob"}, {"prs-merged", "bob"}, {"user", "alice"},
	} {
		a := newAccumulator()
		alice := githubIdentity("1", "alice")
		alice.Commits = 3
		bob := githubIdentity("2", "bob")
		bob.Counts = Counts{Commits: 1, Changes: 20, IssuesCreated: 2, IssuesAuthoredCompleted: 1, PRsCreated: 2, PRsMerged: 1}
		a.user(alice)
		a.user(bob)
		a.user(githubIdentity("3", "dependabot[bot]"))
		report := &Report{Sort: test.sort, ExcludeBots: true}
		a.finish(report)
		if len(report.Users) != 2 || report.Users[0].Login != test.first {
			t.Errorf("sort %s: %+v", test.sort, report.Users)
		}
	}
}
