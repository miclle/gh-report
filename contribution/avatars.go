package contribution

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var avatarLoginPattern = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,37}[a-zA-Z0-9])?(?:\[bot\])?$`)

// ParseAvatarUsers 解析可重复的 email=username 映射，邮箱按提交身份规则规范化。
func ParseAvatarUsers(entries []string) (map[string]string, error) {
	users := make(map[string]string, len(entries))
	for _, entry := range entries {
		separator := strings.LastIndex(entry, "=")
		if separator < 1 {
			return nil, fmt.Errorf("头像映射需要 email=username 格式: %q", entry)
		}
		email := strings.ToLower(strings.TrimSpace(entry[:separator]))
		login := strings.ToLower(strings.TrimSpace(entry[separator+1:]))
		local, domain, ok := strings.Cut(email, "@")
		if !ok || local == "" || domain == "" || strings.ContainsAny(email, " \t\r\n") || strings.Contains(domain, "@") || !avatarLoginPattern.MatchString(login) {
			return nil, fmt.Errorf("头像映射中的邮箱或 GitHub 用户名无效: %q", entry)
		}
		if previous, ok := users[email]; ok && previous != login {
			return nil, fmt.Errorf("邮箱 %q 对应多个 GitHub 用户名", email)
		}
		users[email] = login
	}
	return users, nil
}

// LoadGitHubAvatars 根据 noreply 邮箱或显式映射直接下载头像，不调用 API、不读取 Token，不更改身份或统计。
// 单个头像不可用时继续处理其他作者并返回首个错误；取消操作时由调用方终止报告。
func LoadGitHubAvatars(ctx context.Context, report *CommitReport, users map[string]string) (int, error) {
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !githubImageHost(req.URL) {
			return fmt.Errorf("头像请求重定向到不支持的地址")
		}
		return nil
	}}
	return loadGitHubAvatars(ctx, report, client, users)
}

func loadGitHubAvatars(ctx context.Context, report *CommitReport, client *http.Client, users map[string]string) (int, error) {
	report.Rules = append(report.Rules, "GitHub 头像按 noreply 邮箱或显式邮箱映射直接下载，不调用 API、不读取 Token；身份及计数仍按原 Git 作者邮箱归并")
	images := make(map[string][]byte)
	loaded := 0
	var firstErr error
	for i := range report.Users {
		if err := ctx.Err(); err != nil {
			return loaded, err
		}
		user := &report.Users[i]
		avatarURL := githubAvatarURL(user.Email, users)
		if avatarURL == "" {
			continue
		}
		data, cached := images[avatarURL]
		if !cached {
			var err error
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

func githubAvatarURL(email string, users map[string]string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	if login, ok := users[email]; ok {
		if !avatarLoginPattern.MatchString(login) {
			return ""
		}
		return "https://avatars.githubusercontent.com/" + url.PathEscape(strings.ToLower(login))
	}
	local, domain, ok := strings.Cut(email, "@")
	if !ok || domain != "users.noreply.github.com" {
		return ""
	}
	if id, login, ok := strings.Cut(local, "+"); ok {
		number, err := strconv.ParseUint(id, 10, 64)
		if err != nil || number == 0 || !avatarLoginPattern.MatchString(login) {
			return ""
		}
		return "https://avatars.githubusercontent.com/u/" + strconv.FormatUint(number, 10)
	}
	if !avatarLoginPattern.MatchString(local) {
		return ""
	}
	return "https://avatars.githubusercontent.com/" + url.PathEscape(local)
}

func githubImageHost(u *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && u.Port() == "" && u.Hostname() == "avatars.githubusercontent.com"
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
