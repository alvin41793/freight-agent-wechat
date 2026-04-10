package handler

import (
	"regexp"
	"strings"

	"freight-agent-wechat/pkg/config"
)

// TextChunk 文本分块结果
type TextChunk struct {
	Content    string // 分块内容
	Region     string // 航线区域（如"美加线"、"东南亚"）
	AgentName  string // 代理名称（如"宁波弘泰"）
	HasPricing bool   // 是否包含价格信息
}

// reChunkBoundary 匹配分块边界：序号+代理名 或 纯代理名
// 例如："1.宁波弘泰"、"2.上海铮航"、"宁波美商"
var reChunkBoundary = regexp.MustCompile(`(?m)^(?:\d+\.\s*)?([\p{Han}]{2,10}(?:公司|货代|物流|航运|国际)?(?:\s*\d+|)$)`)

// reRegionHeader 匹配航线区域标题（单独一行，包含航线关键词）
var reRegionHeader = regexp.MustCompile(`(?m)^(美[东西南北湾]|欧洲|欧基|地中海|东南亚|南美|中东|非洲|澳洲|印巴|红海|亚丁湾|日本|韩国|台澎金马|港澳台)\s*$`)

// rePricePattern 匹配价格模式（用于判断是否包含运价）
var rePricePattern = regexp.MustCompile(`\d{3,5}\s*/\s*\d{3,5}`)

// reNumberedAgent 匹配"序号.代理名"模式
var reNumberedAgent = regexp.MustCompile(`(?m)^(\d+)\.\s*([^\n]+)`)

// SmartSplitText 智能分块：按报价块边界分割长文本
// 返回的分块保留完整的上下文信息（航线区域、代理名等）
func SmartSplitText(text string) []TextChunk {
	if len(text) == 0 {
		return nil
	}

	// 从配置中读取分块阈值
	cfg := config.Get()
	minChunkSize := cfg.TextSplit.MinChunkSize
	maxChunkSize := cfg.TextSplit.MaxChunkSize

	// 如果文本较短，不分块
	if len([]rune(text)) < minChunkSize {
		return []TextChunk{{
			Content:    text,
			HasPricing: rePricePattern.MatchString(text),
		}}
	}

	// 使用双换行符预分割文本块
	preliminaryBlocks := strings.Split(text, "\n\n")

	var chunks []TextChunk
	var currentBlock strings.Builder
	var currentRegion string
	var currentAgent string

	flushBlock := func() {
		if currentBlock.Len() > 0 {
			content := strings.TrimSpace(currentBlock.String())
			if len([]rune(content)) > 10 {
				chunks = append(chunks, TextChunk{
					Content:    content,
					Region:     currentRegion,
					AgentName:  currentAgent,
					HasPricing: rePricePattern.MatchString(content),
				})
			}
			currentBlock.Reset()
		}
	}

	for _, block := range preliminaryBlocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		lines := strings.Split(block, "\n")
		for _, line := range lines {
			trimmedLine := strings.TrimSpace(line)
			if trimmedLine == "" {
				continue
			}

			// 检测航线区域标题
			if regionMatch := reRegionHeader.FindStringSubmatch(trimmedLine); regionMatch != nil {
				flushBlock()
				currentRegion = regionMatch[1]
				continue
			}

			// 检测"序号.代理名"模式
			if matches := reNumberedAgent.FindStringSubmatch(trimmedLine); matches != nil {
				agentName := strings.TrimSpace(matches[2])

				// 判断是否是纯代理名行
				isPureAgentLine := !rePricePattern.MatchString(trimmedLine) &&
					!strings.Contains(strings.ToUpper(trimmedLine), "ETD") &&
					!strings.Contains(strings.ToUpper(trimmedLine), "MSK") &&
					!strings.Contains(strings.ToUpper(trimmedLine), "MSC") &&
					!strings.Contains(strings.ToUpper(trimmedLine), "COSCO") &&
					!strings.Contains(strings.ToUpper(trimmedLine), "CMA") &&
					!strings.Contains(strings.ToUpper(trimmedLine), "ONE")

				if isPureAgentLine && len([]rune(agentName)) <= 10 {
					flushBlock()
					currentAgent = agentName
					continue
				}
			}

			// 添加到当前块
			if currentBlock.Len() > 0 {
				currentBlock.WriteString("\n")
			}
			currentBlock.WriteString(line)

			// 如果当前块超过最大阈值，强制分割
			if len([]rune(currentBlock.String())) > maxChunkSize {
				flushBlock()
			}
		}

		// 每个双换行块结束后刷新
		flushBlock()
	}

	// 刷新最后一个块
	flushBlock()

	return chunks
}

// detectAgentName 从行首检测代理名称
func detectAgentName(line string) string {
	// 匹配常见的代理名模式：城市名+公司名
	// 例如："宁波弘泰"、"上海铮航"、"深圳均辉"
	// 使用 \p{Han} 匹配中文字符
	re := regexp.MustCompile(`^([\p{Han}]{2,10})`)
	if match := re.FindStringSubmatch(line); match != nil {
		name := match[1]
		// 验证是否是合理的代理名（包含城市名特征）
		if strings.ContainsAny(name, "北上广深宁青厦天台重武南") {
			return name
		}
	}
	return ""
}

// splitByDoubleNewline 按双换行符进一步细分
func splitByDoubleNewline(chunk TextChunk) []TextChunk {
	parts := strings.Split(chunk.Content, "\n\n")
	if len(parts) <= 1 {
		return []TextChunk{chunk}
	}

	var result []TextChunk
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len([]rune(part)) > 20 { // 忽略过短的片段
			result = append(result, TextChunk{
				Content:    part,
				Region:     chunk.Region,
				AgentName:  chunk.AgentName,
				HasPricing: rePricePattern.MatchString(part),
			})
		}
	}

	return result
}

// MergeChunks 合并相邻的小块（如果总长度<800字）
func MergeChunks(chunks []TextChunk) []TextChunk {
	if len(chunks) <= 1 {
		return chunks
	}

	var merged []TextChunk
	var current TextChunk

	for _, chunk := range chunks {
		if current.Content == "" {
			current = chunk
			continue
		}

		// 如果合并后不超过800字，且属于同一代理/区域，则合并
		combinedLen := len([]rune(current.Content)) + len([]rune(chunk.Content))
		if combinedLen < 800 && current.AgentName == chunk.AgentName && current.Region == chunk.Region {
			current.Content += "\n\n" + chunk.Content
			current.HasPricing = current.HasPricing || chunk.HasPricing
		} else {
			merged = append(merged, current)
			current = chunk
		}
	}

	if current.Content != "" {
		merged = append(merged, current)
	}

	return merged
}
