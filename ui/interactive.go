package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
)

// InteractiveForm 提供交互式参数选择表单。
type InteractiveForm struct {
	// 选择的报告类型
	ReportType string
	// 选择的仓库列表
	Repos []string
	// 输出格式
	Format string
	// 是否使用 AI
	UseAI bool
	// AI 提供商
	AIProvider string
	// 可用仓库列表
	AvailableRepos []string
}

// 报告类型选项
var reportTypeOptions = []huh.Option[string]{
	huh.NewOption("日报（最近 1 天）", "daily"),
	huh.NewOption("周报（最近 14 天）", "weekly"),
	huh.NewOption("月报（最近 60 天）", "monthly"),
	huh.NewOption("年报（最近 730 天）", "yearly"),
}

// 输出格式选项
var formatOptions = []huh.Option[string]{
	huh.NewOption("CSV（原始数据）", "csv"),
	huh.NewOption("Summary（结构化摘要）", "summary"),
}

// AI 提供商选项
var aiProviderOptions = []huh.Option[string]{
	huh.NewOption("Anthropic Claude", "anthropic"),
	huh.NewOption("OpenAI", "openai"),
}


// RunSelectReportType 显示报告类型选择表单。
func RunSelectReportType() (string, error) {
	var reportType string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("选择报告类型").
				Options(reportTypeOptions...).
				Value(&reportType).
				WithTheme(huh.ThemeCharm()),
		),
	)

	if err := form.Run(); err != nil {
		return "", err
	}
	return reportType, nil
}

// RunSelectFormat 显示输出格式选择表单。
func RunSelectFormat() (string, error) {
	var format string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("选择输出格式").
				Options(formatOptions...).
				Value(&format).
				WithTheme(huh.ThemeCharm()),
		),
	)

	if err := form.Run(); err != nil {
		return "", err
	}
	return format, nil
}

// RunSelectAIProvider 显示 AI 提供商选择表单。
func RunSelectAIProvider() (string, error) {
	var provider string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("选择 AI 提供商").
				Options(aiProviderOptions...).
				Value(&provider).
				WithTheme(huh.ThemeCharm()),
		),
	)

	if err := form.Run(); err != nil {
		return "", err
	}
	return provider, nil
}

// RunConfirmAI 显示是否使用 AI 的确认框。
func RunConfirmAI() (bool, error) {
	var useAI bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("是否使用 AI 生成报告？").
				Value(&useAI).
				Affirmative("是").
				Negative("否").
				WithTheme(huh.ThemeCharm()),
		),
	)

	if err := form.Run(); err != nil {
		return false, err
	}
	return useAI, nil
}

// RunInputRepos 显示仓库输入表单。
func RunInputRepos(defaultRepos string) ([]string, error) {
	var reposStr string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("输入仓库列表（逗号分隔，格式: owner/repo）").
				Placeholder("owner/repo1,owner/repo2").
				Value(&reposStr).
				WithTheme(huh.ThemeCharm()),
		),
	)

	if defaultRepos != "" {
		// 如果有默认值，直接返回
		return parseRepos(defaultRepos), nil
	}

	if err := form.Run(); err != nil {
		return nil, err
	}
	return parseRepos(reposStr), nil
}

// RunMultiSelectRepos 显示仓库多选表单。
func RunMultiSelectRepos(availableRepos []string) ([]string, error) {
	if len(availableRepos) == 0 {
		return RunInputRepos("")
	}

	var selected []string
	options := make([]huh.Option[string], len(availableRepos))
	for i, repo := range availableRepos {
		options[i] = huh.NewOption(repo, repo)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("选择要报告的仓库").
				Options(options...).
				Value(&selected).
				WithTheme(huh.ThemeCharm()),
		),
	)

	if err := form.Run(); err != nil {
		return nil, err
	}
	return selected, nil
}

// RunFullForm 显示完整的交互式表单（当未提供必要参数时）。
func RunFullForm(availableRepos []string) (*InteractiveForm, error) {
	form := &InteractiveForm{
		AvailableRepos: availableRepos,
	}

	var groups []*huh.Group

	// 报告类型选择
	groups = append(groups, huh.NewGroup(
		huh.NewSelect[string]().
			Title("选择报告类型").
			Options(reportTypeOptions...).
			Value(&form.ReportType),
	))

	// 仓库选择
	var reposStr string // 用于输入框模式
	if len(availableRepos) > 0 {
		// 如果有可用仓库列表，使用多选
		options := make([]huh.Option[string], len(availableRepos))
		for i, repo := range availableRepos {
			options[i] = huh.NewOption(repo, repo)
		}
		groups = append(groups, huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("选择要报告的仓库").
				Options(options...).
				Value(&form.Repos).
				Filterable(true),
		))
	} else {
		// 否则使用输入框
		groups = append(groups, huh.NewGroup(
			huh.NewInput().
				Title("输入仓库列表（逗号分隔，格式: owner/repo）").
				Placeholder("owner/repo1,owner/repo2").
				Value(&reposStr).
				Validate(func(s string) error {
					if s == "" {
						return fmt.Errorf("仓库列表不能为空")
					}
					return nil
				}),
		))
	}

	// 输出格式选择
	groups = append(groups, huh.NewGroup(
		huh.NewSelect[string]().
			Title("选择输出格式").
			Options(formatOptions...).
			Value(&form.Format),
	))

	// 是否使用 AI
	groups = append(groups, huh.NewGroup(
		huh.NewConfirm().
			Title("是否使用 AI 生成报告？").
			Value(&form.UseAI).
			Affirmative("是").
			Negative("否"),
	))

	// 如果使用 AI，选择提供商
	var aiProvider string
	groups = append(groups, huh.NewGroup(
		huh.NewSelect[string]().
			Title("选择 AI 提供商").
			Options(aiProviderOptions...).
			Value(&aiProvider),
	).WithHideFunc(func() bool {
		return !form.UseAI
	}))

	huhForm := huh.NewForm(groups...).
		WithTheme(huh.ThemeCharm()).
		WithWidth(60)

	if err := huhForm.Run(); err != nil {
		return nil, err
	}

	// 表单运行后解析输入的仓库列表
	if len(availableRepos) == 0 && reposStr != "" {
		form.Repos = parseRepos(reposStr)
	}

	if form.UseAI {
		form.AIProvider = aiProvider
	}

	return form, nil
}

// parseRepos 解析逗号分隔的仓库字符串。
func parseRepos(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	repos := make([]string, 0, len(parts))
	for _, r := range parts {
		r = strings.TrimSpace(r)
		if r != "" {
			repos = append(repos, r)
		}
	}
	return repos
}
