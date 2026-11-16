package contribution

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
)

type avatarTransport func(*http.Request) (*http.Response, error)

func (transport avatarTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return transport(req)
}

func avatarResponse(req *http.Request, status int, data []byte) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(data)), Request: req}
}

func TestGitHubAvatarsUseEmailOrMappingWithoutAPI(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "must-not-be-used")
	report := &CommitReport{Totals: CommitCounts{Commits: 4}, Users: []CommitUser{
		{Email: "123+alice@users.noreply.github.com", Name: "Alice"},
		{Email: "alice@users.noreply.github.com", Name: "Alice"},
		{Email: "other@example.com", Name: "Other email"},
		{Email: "unknown@example.com", Name: "Unknown"},
	}}
	for i := range report.Users {
		report.Users[i].Commits = 1
		report.Users[i].Weeks = []CommitWeek{{Week: "2026-01-05", CommitCounts: CommitCounts{Commits: 1}}}
	}
	before, err := json.Marshal(report.Users)
	if err != nil {
		t.Fatal(err)
	}
	var bitmap bytes.Buffer
	pixels := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	pixels.Set(0, 0, color.NRGBA{R: 255, A: 255})
	if err := png.Encode(&bitmap, pixels); err != nil {
		t.Fatal(err)
	}
	paths := make(map[string]int)
	client := &http.Client{Transport: avatarTransport(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "" {
			t.Error("avatar requests must not read a Token")
		}
		if req.URL.Host != "avatars.githubusercontent.com" || req.URL.Query().Get("s") != "80" {
			t.Errorf("unexpected avatar request: %s", req.URL)
		}
		paths[req.URL.Path]++
		return avatarResponse(req, 200, bitmap.Bytes()), nil
	})}
	users, err := ParseAvatarUsers([]string{"other@example.com=alice"})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := loadGitHubAvatars(context.Background(), report, client, users)
	if err != nil || loaded != 3 || len(paths) != 2 || paths["/alice"] != 1 || paths["/u/123"] != 1 {
		t.Fatalf("loaded=%d paths=%v err=%v", loaded, paths, err)
	}
	after, err := json.Marshal(report.Users)
	if err != nil || !bytes.Equal(before, after) || report.Totals.Commits != 4 {
		t.Fatal("avatar enrichment changed identities or statistics")
	}
	var svg bytes.Buffer
	if err := PrintCommitsChart(&svg, report); err != nil {
		t.Fatal(err)
	}
	assertValidChartXML(t, svg.Bytes(), 4)
	decoder := xml.NewDecoder(&svg)
	embedded := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "image" {
			embedded++
			for _, attr := range start.Attr {
				if attr.Name.Local == "href" && !strings.HasPrefix(attr.Value, "data:image/png;base64,") {
					t.Error("SVG relies on external avatar images")
				}
			}
		}
	}
	if embedded != 3 {
		t.Fatalf("embedded %d images, want 3", embedded)
	}
}

func TestGitHubAvatarDownloadFailureFallsBack(t *testing.T) {
	report := &CommitReport{Users: []CommitUser{
		{Email: "alice@users.noreply.github.com", Name: "Alice"},
		{Email: "other@example.com", Name: "Alice"},
		{Email: "unknown@example.com", Name: "Alice"},
	}}
	for i := range report.Users {
		report.Users[i].CommitCounts = CommitCounts{Commits: 1, Additions: 2, Changes: 2}
		report.Users[i].Weeks = []CommitWeek{{Week: "2026-01-05", CommitCounts: report.Users[i].CommitCounts}}
	}
	report.Totals = CommitCounts{Commits: 3, Additions: 6, Changes: 6}
	requests := 0
	client := &http.Client{Transport: avatarTransport(func(req *http.Request) (*http.Response, error) {
		requests++
		return avatarResponse(req, http.StatusTooManyRequests, nil), nil
	})}
	loaded, err := loadGitHubAvatars(context.Background(), report, client, map[string]string{"other@example.com": "alice"})
	if err == nil || loaded != 0 || requests != 1 {
		t.Fatalf("fallback: loaded=%d requests=%d err=%v", loaded, requests, err)
	}
	var chart bytes.Buffer
	if err := PrintCommitsChart(&chart, report); err != nil {
		t.Fatal(err)
	}
	assertValidChartXML(t, chart.Bytes(), 3)
	if strings.Count(chart.String(), ">A</text>") != 3 || strings.Contains(chart.String(), "data:image/png") {
		t.Fatal("unavailable avatars must render initials")
	}
	if !strings.Contains(chart.String(), "3 contributors · 3 commits · +6 / −0 lines") {
		t.Fatal("avatar fallback changed chart totals")
	}
}

func TestGitHubAvatarURLAndMappingValidation(t *testing.T) {
	for _, test := range []struct{ email, want string }{
		{" 123+Renamed@users.noreply.github.com ", "https://avatars.githubusercontent.com/u/123"},
		{"Alice@users.noreply.github.com", "https://avatars.githubusercontent.com/alice"},
		{"41898282+github-actions[bot]@users.noreply.github.com", "https://avatars.githubusercontent.com/u/41898282"},
		{"alice@example.com", ""},
		{"alice@users.noreply.github.com.evil.test", ""},
		{"123+@users.noreply.github.com", ""},
		{"0+alice@users.noreply.github.com", ""},
		{"no-id+alice@users.noreply.github.com", ""},
		{"alice/../../api@users.noreply.github.com", ""},
		{"", ""},
	} {
		if got := githubAvatarURL(test.email, nil); got != test.want {
			t.Errorf("%q: got %q want %q", test.email, got, test.want)
		}
	}
	users, err := ParseAvatarUsers([]string{" Alice@Example.com = Miclle ", "alice@example.com=miclle"})
	if err != nil || githubAvatarURL("ALICE@example.com", users) != "https://avatars.githubusercontent.com/miclle" {
		t.Fatalf("invalid mapping normalization: %v %v", users, err)
	}
	users, err = ParseAvatarUsers([]string{"123+alice@users.noreply.github.com=override"})
	if err != nil || githubAvatarURL("123+alice@users.noreply.github.com", users) != "https://avatars.githubusercontent.com/override" {
		t.Fatal("explicit mapping must take precedence")
	}
	for _, entries := range [][]string{
		{"alice"}, {"alice="}, {"=alice"}, {"a@b=https://example.com"}, {"a@b=../../api"}, {"a@b=alice", "A@b=bob"}, {"a@b@c=alice"},
	} {
		if _, err := ParseAvatarUsers(entries); err == nil {
			t.Errorf("invalid mappings accepted: %v", entries)
		}
	}
}

func TestAvatarRejectsExternalURLsAndNonBitmapContent(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: avatarTransport(func(req *http.Request) (*http.Response, error) {
		requests++
		return avatarResponse(req, 200, []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)), nil
	})}
	for _, address := range []string{"http://avatars.githubusercontent.com/u/1", "https://example.com/avatar.png", "https://avatars.githubusercontent.com.evil.test/u/1", "https://user:pass@avatars.githubusercontent.com/u/1", "https://avatars.githubusercontent.com:444/u/1"} {
		if _, err := downloadGitHubAvatar(context.Background(), client, address); err == nil {
			t.Fatalf("accepted unsupported URL: %s", address)
		}
	}
	if requests != 0 {
		t.Fatal("unsupported URL reached the network")
	}
	if _, err := downloadGitHubAvatar(context.Background(), client, "https://avatars.githubusercontent.com/u/1"); err == nil {
		t.Fatal("accepted active image content")
	}
}
