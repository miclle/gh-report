package contribution

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/miclle/gh-report/github"
)

func gitFixture(t *testing.T, path string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", path, "-c", "commit.gpgSign=false", "-c", "core.hooksPath=" + filepath.Join(path, "no-hooks")}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Alice", "GIT_AUTHOR_EMAIL=alice@example.com", "GIT_COMMITTER_NAME=Alice", "GIT_COMMITTER_EMAIL=alice@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newLocalFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "local repo")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	gitFixture(t, path, "init", "-b", "main")
	gitFixture(t, path, "remote", "add", "origin", "git@github.com:team/repo.git")
	return path
}

func fixtureFile(t *testing.T, path, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(path, name), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLocalHistoryDefaultBranchBinaryRenameAndMerge(t *testing.T) {
	path := newLocalFixture(t)
	fixtureFile(t, path, "before", []byte("one\ntwo\n"))
	gitFixture(t, path, "add", ".")
	gitFixture(t, path, "commit", "-m", "root")
	gitFixture(t, path, "commit", "--allow-empty", "-m", "empty")
	gitFixture(t, path, "mv", "before", "name\twith\nnewline")
	gitFixture(t, path, "commit", "-m", "rename")
	fixtureFile(t, path, "binary", []byte{0, 1, 2, 3})
	gitFixture(t, path, "add", "binary")
	gitFixture(t, path, "commit", "-m", "binary")
	gitFixture(t, path, "update-index", "--chmod=+x", "binary")
	gitFixture(t, path, "commit", "-m", "mode")
	gitFixture(t, path, "checkout", "-b", "feature")
	fixtureFile(t, path, "feature-file", []byte("feature\n"))
	gitFixture(t, path, "add", "feature-file")
	gitFixture(t, path, "commit", "--author=Bob <bob@example.com>", "-m", "feature")
	gitFixture(t, path, "checkout", "main")
	fixtureFile(t, path, "main-file", []byte("main\n"))
	gitFixture(t, path, "add", "main-file")
	gitFixture(t, path, "commit", "--author=Carol <carol@example.com>", "-m", "main")
	gitFixture(t, path, "merge", "--no-ff", "feature", "-m", "merge")
	defaultSHA := gitFixture(t, path, "rev-parse", "main")
	gitFixture(t, path, "checkout", "-b", "other")
	fixtureFile(t, path, "unmerged", []byte("not in default branch\n"))
	gitFixture(t, path, "add", "unmerged")
	gitFixture(t, path, "commit", "-m", "unmerged")
	fixtureFile(t, path, "uncommitted", []byte("not committed\n"))
	nested := filepath.Join(path, "nested")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatal(err)
	}
	local, err := openLocalRepository(context.Background(), nested, "team/repo")
	if err != nil {
		t.Fatal(err)
	}
	for _, tracking := range []bool{false, true} {
		if tracking {
			gitFixture(t, path, "update-ref", "refs/remotes/origin/main", defaultSHA)
			gitFixture(t, path, "branch", "-D", "main")
		}
		ref, sha, err := local.branchHead(context.Background(), "main")
		if err != nil || sha != defaultSHA || (tracking && ref != "refs/remotes/origin/main") || (!tracking && ref != "refs/heads/main") {
			t.Fatalf("default branch: ref=%s sha=%s err=%v", ref, sha, err)
		}
		a := newAccumulator()
		if err := local.walkCommits(context.Background(), sha, a.addCommit); err != nil {
			t.Fatal(err)
		}
		report := &Report{Sort: "commits"}
		a.finish(report)
		if report.Totals.Commits != 6 || report.Totals.Additions != 4 || report.Totals.Deletions != 0 || len(report.Users) != 3 || a.users["git:alice@example.com"].Commits != 4 {
			t.Fatalf("wrong local counters (tracking=%t): %+v users=%+v", tracking, report.Totals, report.Users)
		}
		if gitFixture(t, path, "branch", "--show-current") != "other" {
			t.Fatal("statistics must not switch the user's branch")
		}
	}
	if _, _, err := local.branchHead(context.Background(), "missing"); err == nil {
		t.Fatal("missing default branch must fail")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := local.walkCommits(ctx, defaultSHA, func(github.CommitActivity) error { return nil }); err == nil {
		t.Fatal("cancelled walk must fail")
	}
}

func TestLocalRemoteSelectionAndShallowRejection(t *testing.T) {
	path := newLocalFixture(t)
	if _, err := openLocalRepository(context.Background(), path, "different/repo"); err == nil {
		t.Fatal("must not mix local commits with another GitHub repository")
	}
	gitFixture(t, path, "remote", "remove", "origin")
	gitFixture(t, path, "remote", "add", "first", "https://github.com/team/one.git")
	gitFixture(t, path, "remote", "add", "second", "ssh://git@github.com/team/two.git")
	if _, err := openLocalRepository(context.Background(), path, ""); err == nil {
		t.Fatal("ambiguous remotes must fail")
	}
	local, err := openLocalRepository(context.Background(), path, "team/two")
	if err != nil || local.remote != "second" {
		t.Fatalf("explicit repository should select matching remote: %+v %v", local, err)
	}
	if _, _, err := local.branchHead(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	fixtureFile(t, path, "file", []byte("one\n"))
	gitFixture(t, path, "add", ".")
	gitFixture(t, path, "commit", "-m", "root")
	clone := filepath.Join(t.TempDir(), "shallow")
	gitFixture(t, filepath.Dir(clone), "clone", "--depth=1", "file://"+path, clone)
	if _, err := openLocalRepository(context.Background(), clone, ""); err == nil || !strings.Contains(err.Error(), "浅克隆") {
		t.Fatalf("shallow repository must fail explicitly: %v", err)
	}
	if _, err := openLocalRepository(context.Background(), t.TempDir(), ""); err == nil {
		t.Fatal("non-git directory must fail")
	}
}

func TestLocalHistoryIncludesSubmodulesDespiteDiffConfig(t *testing.T) {
	path := newLocalFixture(t)
	fixtureFile(t, path, "file", []byte("one\n"))
	gitFixture(t, path, "add", "file")
	gitFixture(t, path, "commit", "-m", "root")
	for _, message := range []string{"add submodule", "update submodule", "mixed update"} {
		previous := gitFixture(t, path, "rev-parse", "HEAD")
		// 用 gitlink 构造 submodule 指针，不需要克隆外部仓库。
		gitFixture(t, path, "update-index", "--add", "--cacheinfo", "160000", previous, "module")
		if message == "mixed update" {
			fixtureFile(t, path, "file", []byte("one\ntwo\n"))
			gitFixture(t, path, "add", "file")
		}
		gitFixture(t, path, "commit", "-m", message)
	}
	sha := gitFixture(t, path, "rev-parse", "HEAD")
	local := &localRepository{path: path}
	want := Counts{Commits: 4, Additions: 5, Deletions: 2, Changes: 7}
	for _, ignore := range []string{"none", "all"} {
		t.Run(ignore, func(t *testing.T) {
			gitFixture(t, path, "config", "diff.ignoreSubmodules", ignore)
			a := newAccumulator()
			if err := local.walkCommits(context.Background(), sha, a.addCommit); err != nil {
				t.Fatal(err)
			}
			report := &Report{Sort: "commits"}
			a.finish(report)
			if report.Totals != want {
				t.Fatalf("diff.ignoreSubmodules=%s: got %+v, want %+v", ignore, report.Totals, want)
			}
		})
	}
}

func TestReadGitCommitsRejectsTruncation(t *testing.T) {
	for _, input := range []string{
		"\x00GHREPORT-COMMIT\x00sha\x00author\x00email",
		"\x00GHREPORT-COMMIT\x00sha\x00author\x00email\x000\x001\t0\tfile",
		"\x00GHREPORT-COMMIT\x00sha\x00author\x00email\x000\x000\t0\t\x00old\x00",
	} {
		if err := readGitCommits(strings.NewReader(input), func(github.CommitActivity) error { return nil }); err == nil {
			t.Fatalf("truncated input must fail: %q", input)
		}
	}
	if err := readGitCommits(strings.NewReader(""), func(github.CommitActivity) error { return io.ErrClosedPipe }); err != nil {
		t.Fatal(err)
	}
}
