package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/miclle/gh-report/anthropic"
	"github.com/miclle/gh-report/github"
	"github.com/miclle/gh-report/report"
)

func main() {
	var (
		configFile = flag.String("config", "", "Path to YAML config file")
		repos      = flag.String("repos", "", "Comma-separated list of repositories (owner/repo)")
		days       = flag.Int("days", 0, "Number of days to look back")
		user       = flag.String("user", "", "Filter by user login (optional)")
		token      = flag.String("token", "", "GitHub token (default: $GITHUB_TOKEN)")
		format     = flag.String("format", "", "Output format: csv (default) or summary")
		ai         = flag.Bool("ai", false, "Use Claude API to generate daily report")
		anthroKey  = flag.String("anthropic-key", "", "Anthropic API key (default: $ANTHROPIC_API_KEY)")
		anthroURL  = flag.String("anthropic-base-url", "", "Anthropic API base URL (default: $ANTHROPIC_BASE_URL)")
		model      = flag.String("model", "", "Claude model name (default: claude-sonnet-4-20250514)")
	)
	flag.Parse()

	// Load config file if specified
	var cfg Config
	if *configFile != "" {
		c, err := LoadConfig(*configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		cfg = *c
	}

	// CLI flags override config file values
	if *repos != "" {
		cfg.Repos = nil
		for _, r := range strings.Split(*repos, ",") {
			cfg.Repos = append(cfg.Repos, strings.TrimSpace(r))
		}
	}
	if *days > 0 {
		cfg.Days = *days
	}
	if *user != "" {
		cfg.User = *user
	}
	if *token != "" {
		cfg.Token = *token
	}
	if *format != "" {
		cfg.Format = *format
	}
	if *ai {
		cfg.AI = true
	}
	if *anthroKey != "" {
		cfg.AnthropicKey = *anthroKey
	}
	if *anthroURL != "" {
		cfg.AnthropicBaseURL = *anthroURL
	}
	if *model != "" {
		cfg.Model = *model
	}

	// Apply defaults
	if cfg.Days == 0 {
		cfg.Days = 1
	}

	if len(cfg.Repos) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no repositories specified (use -repos flag or config file)")
		flag.Usage()
		os.Exit(1)
	}

	// Resolve token: flag/config > env
	ghToken := cfg.Token
	if ghToken == "" {
		ghToken = os.Getenv("GITHUB_TOKEN")
	}
	if ghToken == "" {
		fmt.Fprintln(os.Stderr, "Error: no GitHub token provided (use -token flag, config file, or GITHUB_TOKEN env)")
		os.Exit(1)
	}

	client := github.NewClient(ghToken)
	ctx := context.Background()

	opts := report.Options{
		Repos: cfg.Repos,
		Days:  cfg.Days,
		User:  cfg.User,
	}

	reports, err := report.Collect(ctx, client, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error collecting data: %v\n", err)
		os.Exit(1)
	}

	now := time.Now()
	since := now.AddDate(0, 0, -cfg.Days)

	switch cfg.Format {
	case "summary":
		if cfg.AI {
			// 解析 Anthropic API Key: flag > config > 环境变量
			apiKey := cfg.AnthropicKey
			if apiKey == "" {
				apiKey = os.Getenv("ANTHROPIC_API_KEY")
			}
			if apiKey == "" {
				fmt.Fprintln(os.Stderr, "Error: no Anthropic API key provided (use -anthropic-key flag, config file, or ANTHROPIC_API_KEY env)")
				os.Exit(1)
			}

			modelName := cfg.Model
			if modelName == "" {
				modelName = "claude-sonnet-4-20250514"
			}

			// 解析 Anthropic Base URL: flag > config > 环境变量
			baseURL := cfg.AnthropicBaseURL
			if baseURL == "" {
				baseURL = os.Getenv("ANTHROPIC_BASE_URL")
			}

			prompt := report.BuildSummaryPrompt(reports, since, now, cfg.User)
			aiClient := anthropic.NewClient(apiKey, modelName, baseURL)
			result, err := aiClient.CreateMessage(ctx, prompt)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error calling Claude API: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintln(os.Stdout, result)
		} else {
			report.PrintSummaryData(os.Stdout, reports, since, now, cfg.User)
		}
	default:
		report.Print(os.Stdout, reports, since, now)
	}
}
