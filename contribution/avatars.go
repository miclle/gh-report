package contribution

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	gh "github.com/google/go-github/v69/github"
)

// LoadGitHubAvatars 为可关联的 Git 作者加载头像，token 可为空，不更改身份或统计。
// 单个头像不可用时继续处理其他作者并返回首个错误；取消操作时由调用方终止报告。
func LoadGitHubAvatars(ctx context.Context, report *CommitReport, token string) (int, error) {
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !githubImageHost(req.URL) || req.URL.Hostname() != via[0].URL.Hostname() {
			return fmt.Errorf("头像请求重定向到不支持的地址")
		}
		return nil
	}}
	return loadGitHubAvatars(ctx, report, client, token)
}

func loadGitHubAvatars(ctx context.Context, report *CommitReport, client *http.Client, token string) (int, error) {
	report.Rules = append(report.Rules, "启用 GitHub 头像：联网查询提交作者并下载头像；身份及计数仍按原 Git 作者邮箱归并")
	if len(report.Users) == 0 {
		return 0, nil
	}
	local, err := openLocalRepository(ctx, report.LocalPath, "")
	if err != nil {
		return 0, err
	}
	parts := strings.SplitN(local.repo, "/", 2)
	api := gh.NewClient(client)
	if token != "" {
		api = api.WithAuthToken(token)
	}
	images := make(map[string][]byte)
	loaded := 0
	var firstErr error
	for i := range report.Users {
		if err := ctx.Err(); err != nil {
			return loaded, err
		}
		user := &report.Users[i]
		if user.sampleSHA == "" {
			continue
		}
		commits, response, err := api.Repositories.ListCommits(ctx, parts[0], parts[1], &gh.CommitsListOptions{SHA: user.sampleSHA, ListOptions: gh.ListOptions{PerPage: 1}})
		if err != nil {
			var rateLimit *gh.RateLimitError
			var abuseLimit *gh.AbuseRateLimitError
			if errors.As(err, &rateLimit) || errors.As(err, &abuseLimit) || (response != nil && response.StatusCode == http.StatusTooManyRequests) {
				return loaded, fmt.Errorf("GitHub API 请求限流，已停止获取头像")
			}
			if response != nil && response.StatusCode == http.StatusForbidden {
				return loaded, fmt.Errorf("GitHub API 拒绝请求（HTTP 403），已停止获取头像")
			}
			if firstErr == nil {
				firstErr = fmt.Errorf("查询公开提交头像: %w", err)
			}
			continue
		}
		if len(commits) != 1 || commits[0].GetSHA() != user.sampleSHA || commits[0].GetAuthor().GetID() == 0 {
			continue
		}
		avatarURL := commits[0].GetAuthor().GetAvatarURL()
		data, cached := images[avatarURL]
		if !cached {
			data, err = downloadGitHubAvatar(ctx, client, avatarURL)
			images[avatarURL] = data
			if err != nil && firstErr == nil {
				firstErr = fmt.Errorf("下载 GitHub 头像: %w", err)
			}
		}
		if len(data) > 0 {
			user.avatarPNG = data
			loaded++
		}
	}
	return loaded, firstErr
}

func githubImageHost(u *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && u.Port() == "" && (u.Hostname() == "api.github.com" || u.Hostname() == "avatars.githubusercontent.com")
}

func downloadGitHubAvatar(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	avatar, err := url.Parse(rawURL)
	if err != nil || !githubImageHost(avatar) || avatar.Hostname() != "avatars.githubusercontent.com" {
		return nil, fmt.Errorf("不支持的 GitHub 头像地址")
	}
	query := avatar.Query()
	query.Set("s", "80")
	avatar.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, avatar.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "image/png,image/jpeg,image/gif")
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("头像服务返回 HTTP %d", response.StatusCode)
	}
	const limit = 1 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("头像文件超过 1 MiB")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("头像不是支持的位图: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > 1024 || config.Height > 1024 {
		return nil, fmt.Errorf("头像尺寸超出支持范围")
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	// 固定为小尺寸 PNG，SVG 只嵌入位图快照，不依赖外部图片或可执行内容。
	const size = 80
	resized := image.NewNRGBA(image.Rect(0, 0, size, size))
	bounds := source.Bounds()
	if bounds.Dx() == size && bounds.Dy() == size {
		draw.Draw(resized, resized.Bounds(), source, bounds.Min, draw.Src)
	} else {
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				resized.Set(x, y, source.At(bounds.Min.X+x*bounds.Dx()/size, bounds.Min.Y+y*bounds.Dy()/size))
			}
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, resized); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
