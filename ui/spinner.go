package ui

import (
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SpinnerModel 是带文本的 spinner 模型。
type SpinnerModel struct {
	spinner  spinner.Model
	text     string
	done     bool
	program  *tea.Program
	doneChan chan struct{}
}

// NewSpinner 创建新的 spinner。
func NewSpinner(text string) *SpinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))

	return &SpinnerModel{
		spinner:  s,
		text:     text,
		doneChan: make(chan struct{}),
	}
}

// Init 实现 tea.Model 接口。
func (m *SpinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update 实现 tea.Model 接口。
func (m *SpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case struct{}: // 完成信号
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

// View 实现 tea.Model 接口。
func (m *SpinnerModel) View() string {
	if m.done {
		return ""
	}
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	return fmt.Sprintf("%s %s", m.spinner.View(), textStyle.Render(m.text))
}

// Start 启动 spinner。
func (m *SpinnerModel) Start() {
	m.program = tea.NewProgram(m, tea.WithOutput(os.Stderr))
	go func() {
		_, _ = m.program.Run()
		close(m.doneChan)
	}()
}

// Stop 停止 spinner。
func (m *SpinnerModel) Stop() {
	if m.program != nil {
		m.program.Send(struct{}{})
		<-m.doneChan
	}
}

// RunSpinnerWithAction 运行 spinner 并执行指定操作。
// spinner 会在操作完成后自动停止。
func RunSpinnerWithAction(text string, action func() error) error {
	s := NewSpinner(text)
	s.Start()
	defer s.Stop()

	err := action()
	return err
}

// RunSpinnerWithResult 运行 spinner 并执行指定操作，返回结果。
func RunSpinnerWithResult[T any](text string, action func() (T, error)) (T, error) {
	var zero T
	s := NewSpinner(text)
	s.Start()
	defer s.Stop()

	result, err := action()
	if err != nil {
		return zero, err
	}
	return result, nil
}

// PrintSuccess 打印成功消息。
func PrintSuccess(msg string) {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("2")).
		Bold(true)
	fmt.Fprintln(os.Stderr, style.Render("✓ "+msg))
}

// PrintError 打印错误消息。
func PrintError(msg string) {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("1")).
		Bold(true)
	fmt.Fprintln(os.Stderr, style.Render("✗ "+msg))
}

// PrintInfo 打印信息消息。
func PrintInfo(msg string) {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("12"))
	fmt.Fprintln(os.Stderr, style.Render("ℹ "+msg))
}

// SilentWriter 返回一个静默的 io.Writer（丢弃所有输出）。
func SilentWriter() io.Writer {
	return io.Discard
}
