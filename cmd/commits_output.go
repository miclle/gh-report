package cmd

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/miclle/gh-report/contribution"
)

func saveCommitReport(report *contribution.CommitReport, format, output string, showRules, chart bool) error {
	if info, err := os.Stat(output); err == nil && info.IsDir() {
		return fmt.Errorf("报告输出路径不能是目录: %s", output)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("检查报告输出路径: %w", err)
	}
	var document bytes.Buffer
	if chart {
		var svg bytes.Buffer
		if err := contribution.PrintCommitsChart(&svg, report); err != nil {
			return err
		}
		// 内容指纹避免先写 SVG、后写 Markdown 失败时破坏旧报告引用的图表。
		digest := sha256.Sum256(svg.Bytes())
		base := filepath.Base(output)
		imageName := fmt.Sprintf("%s-%x.svg", strings.TrimSuffix(base, filepath.Ext(base)), digest[:8])
		if err := contribution.PrintCommitsChartMarkdown(&document, report, imageName, showRules); err != nil {
			return err
		}
		if err := writeCommitReportFile(filepath.Join(filepath.Dir(output), imageName), svg.Bytes()); err != nil {
			return fmt.Errorf("保存 SVG 图表: %w", err)
		}
	} else if err := contribution.PrintCommits(&document, report, format, showRules); err != nil {
		return err
	}
	return writeCommitReportFile(output, document.Bytes())
}

func writeCommitReportFile(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".gh-report-*")
	if err != nil {
		return fmt.Errorf("创建报告临时文件: %w", err)
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("写入报告: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("关闭报告文件: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("保存报告文件: %w", err)
	}
	return nil
}

func saveCommitJobSummary(report *contribution.CommitReport, output string, avatarUsers map[string]string, showRules bool) error {
	var document bytes.Buffer
	if err := contribution.PrintCommitsJobSummary(&document, report, avatarUsers, showRules); err != nil {
		return err
	}
	return writeCommitReportFile(output, document.Bytes())
}
