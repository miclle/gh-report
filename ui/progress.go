// Package ui 提供终端用户界面组件。
package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// 仓库名称样式
var repoNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))

// repoProgress 单个仓库的进度状态。
type repoProgress struct {
	total   int
	current int
}

// Progress 实现多仓库并发进度显示。
type Progress struct {
	repos      []string
	progresses []repoProgress
	bars       []progress.Model
	mu         sync.Mutex
	rendered   bool
	isTerm     bool
	lineCount  int
}

// NewProgress 创建新的进度显示组件。
func NewProgress(repos []string) *Progress {
	bars := make([]progress.Model, len(repos))
	progresses := make([]repoProgress, len(repos))

	for i := range repos {
		bars[i] = progress.New(
			progress.WithDefaultGradient(),
			progress.WithWidth(30),
			progress.WithoutPercentage(),
		)
		progresses[i] = repoProgress{total: 6, current: 0}
	}

	return &Progress{
		repos:      repos,
		progresses: progresses,
		bars:       bars,
		isTerm:     term.IsTerminal(int(os.Stderr.Fd())),
		lineCount:  1 + len(repos),
	}
}

// SetTotal 设置指定仓库的总步骤数。
func (p *Progress) SetTotal(repoIndex int, total int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if repoIndex >= 0 && repoIndex < len(p.progresses) {
		p.progresses[repoIndex].total = total
		p.render()
	}
}

// Increment 报告指定仓库完成一个步骤。
func (p *Progress) Increment(repoIndex int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if repoIndex >= 0 && repoIndex < len(p.progresses) {
		p.progresses[repoIndex].current++
		p.render()
	}
}

// SetError 设置错误状态。
func (p *Progress) SetError(err error) {
	// 不需要额外处理
}

// Complete 标记完成。
func (p *Progress) Complete() {
	// 不需要额外处理
}

// Start 启动进度显示。
func (p *Progress) Start() *ProgressWrapper {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.isTerm {
		fmt.Fprintln(os.Stderr, "正在获取 GitHub 数据...")
		return &ProgressWrapper{p}
	}

	// 隐藏光标
	fmt.Fprint(os.Stderr, "\033[?25l")
	p.rendered = true
	p.render()

	// 给进度条一点时间显示第一帧
	time.Sleep(50 * time.Millisecond)

	return &ProgressWrapper{p}
}

// render 渲染进度条。
func (p *Progress) render() {
	if !p.isTerm {
		return
	}

	// 计算仓库名称的最大宽度
	maxRepoWidth := 0
	for _, repo := range p.repos {
		if len(repo) > maxRepoWidth {
			maxRepoWidth = len(repo)
		}
	}

	// 移动光标回到进度区域开头
	// 先移动到行首，然后向上移动 N 行
	lineCount := 1 + len(p.repos)
	fmt.Fprintf(os.Stderr, "\r\033[%dA", lineCount)

	// 构建输出
	var b strings.Builder
	b.WriteString("正在获取 GitHub 数据...\n")

	for i, repo := range p.repos {
		pr := p.progresses[i]
		percent := 0.0
		if pr.total > 0 {
			percent = float64(pr.current) / float64(pr.total)
		}

		repoName := fmt.Sprintf("%-*s", maxRepoWidth, repo)
		b.WriteString(repoNameStyle.Render(repoName))
		b.WriteString("  ")
		b.WriteString(p.bars[i].ViewAs(percent))
		b.WriteString(fmt.Sprintf(" %5.1f%% (%d/%d)", percent*100, pr.current, pr.total))
		b.WriteString("\033[K\n") // 清除行末尾
	}

	fmt.Fprint(os.Stderr, b.String())
}

// Stop 停止进度显示。
func (p *Progress) Stop() {
	if !p.isTerm || !p.rendered {
		return
	}

	// 显示光标
	fmt.Fprint(os.Stderr, "\033[?25h")

	// 清除进度条区域
	// 移动光标到进度区域开头，然后清除
	fmt.Fprint(os.Stderr, "\r\033[J")
}

// ProgressWrapper 包装 Progress 实现 report.Progress 接口。
type ProgressWrapper struct {
	*Progress
}
