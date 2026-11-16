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

func TestGitHubAvatarsAreAnonymousCachedAndEmbedded(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "must-not-be-used")
	path := newLocalFixture(t)
	fixtureFile(t, path, "file", []byte("one\n"))
	gitFixture(t, path, "add", ".")
	gitFixture(t, path, "commit", "-m", "Alice")
	fixtureFile(t, path, "file", []byte("one\ntwo\n"))
	gitFixture(t, path, "add", ".")
	gitFixture(t, path, "commit", "--author", "Other email <other@example.com>", "-m", "same GitHub account")
	report, err := CollectCommits(context.Background(), CommitOptions{Path: path, Ref: "HEAD", Weekly: true})
	if err != nil {
		t.Fatal(err)
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
	apiRequests, imageRequests := 0, 0
	client := &http.Client{Transport: avatarTransport(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "" {
			t.Error("avatar requests must not read a Token")
		}
		if req.URL.Host == "api.github.com" {
			apiRequests++
			if req.URL.Path != "/repos/team/repo/commits" || req.URL.Query().Get("per_page") != "1" {
				t.Errorf("unexpected lookup: %s", req.URL)
			}
			sha := req.URL.Query().Get("sha")
			if sha == "" {
				t.Fatal("lookup must use a local representative commit")
			}
			data, _ := json.Marshal([]any{map[string]any{"sha": sha, "author": map[string]any{"id": 7, "login": "same-account", "avatar_url": "https://avatars.githubusercontent.com/u/7?v=4"}}})
			return avatarResponse(req, 200, data), nil
		}
		if req.URL.Host != "avatars.githubusercontent.com" || req.URL.Query().Get("s") != "80" {
			t.Errorf("unexpected avatar URL: %s", req.URL)
		}
		imageRequests++
		return avatarResponse(req, 200, bitmap.Bytes()), nil
	})}
	loaded, err := loadGitHubAvatars(context.Background(), report, client, "")
	if err != nil || loaded != 2 || apiRequests != 2 || imageRequests != 1 {
		t.Fatalf("loaded=%d api=%d images=%d err=%v", loaded, apiRequests, imageRequests, err)
	}
	after, err := json.Marshal(report.Users)
	if err != nil || !bytes.Equal(before, after) || report.Totals.Commits != 2 {
		t.Fatal("avatar enrichment changed identities or statistics")
	}
	var svg bytes.Buffer
	if err := PrintCommitsChart(&svg, report); err != nil {
		t.Fatal(err)
	}
	assertValidChartXML(t, svg.Bytes(), 2)
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
	if embedded != 2 {
		t.Fatalf("embedded %d images, want 2", embedded)
	}
}

func TestGitHubAvatarRateLimitFallsBackWithoutMoreLookups(t *testing.T) {
	report := &CommitReport{LocalPath: newLocalFixture(t), Users: []CommitUser{{sampleSHA: "one"}, {sampleSHA: "two"}, {sampleSHA: "three"}}}
	for i := range report.Users {
		report.Users[i].Name = "Alice"
		report.Users[i].CommitCounts = CommitCounts{Commits: 1, Additions: 2, Changes: 2}
		report.Users[i].Weeks = []CommitWeek{{Week: "2026-01-05", CommitCounts: report.Users[i].CommitCounts}}
	}
	report.Totals = CommitCounts{Commits: 3, Additions: 6, Changes: 6}
	requests := 0
	client := &http.Client{Transport: avatarTransport(func(req *http.Request) (*http.Response, error) {
		requests++
		response := avatarResponse(req, http.StatusForbidden, []byte(`{"message":"API rate limit exceeded"}`))
		response.Header.Set("X-RateLimit-Remaining", "0")
		return response, nil
	})}
	loaded, err := loadGitHubAvatars(context.Background(), report, client, "")
	if err == nil || loaded != 0 || requests != 1 {
		t.Fatalf("rate limit fallback: loaded=%d requests=%d err=%v", loaded, requests, err)
	}
	if strings.Contains(err.Error(), "https://") || !strings.Contains(err.Error(), "请求限流") {
		t.Fatalf("limit notice should be concise: %v", err)
	}
	for _, user := range report.Users {
		if len(user.avatarPNG) != 0 {
			t.Fatal("failed lookup generated an avatar")
		}
	}
	var chart bytes.Buffer
	if err := PrintCommitsChart(&chart, report); err != nil {
		t.Fatal(err)
	}
	assertValidChartXML(t, chart.Bytes(), 3)
	if strings.Count(chart.String(), ">A</text>") != 3 || strings.Contains(chart.String(), "data:image/png") {
		t.Fatal("rate-limited chart must render initials for every unavailable avatar")
	}
	if !strings.Contains(chart.String(), "3 contributors · 3 commits · +6 / −0 lines") {
		t.Fatal("avatar fallback changed chart totals")
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

func TestAvatarTokenIsOnlySentToGitHubAPI(t *testing.T) {
	report := &CommitReport{LocalPath: newLocalFixture(t), Users: []CommitUser{{sampleSHA: "sample"}}}
	var bitmap bytes.Buffer
	if err := png.Encode(&bitmap, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	requests := 0
	client := &http.Client{Transport: avatarTransport(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.URL.Host == "api.github.com" {
			if req.Header.Get("Authorization") != "Bearer avatar-token" {
				t.Error("optional API Token was not used")
			}
			return avatarResponse(req, 200, []byte(`[{"sha":"sample","author":{"id":1,"avatar_url":"https://avatars.githubusercontent.com/u/1"}}]`)), nil
		}
		if req.Header.Get("Authorization") != "" {
			t.Error("Token leaked to the avatar CDN")
		}
		return avatarResponse(req, 200, bitmap.Bytes()), nil
	})}
	loaded, err := loadGitHubAvatars(context.Background(), report, client, "avatar-token")
	if err != nil || loaded != 1 || requests != 2 {
		t.Fatalf("loaded=%d requests=%d err=%v", loaded, requests, err)
	}
}
