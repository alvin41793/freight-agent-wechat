package bot

import (
	"fmt"
	"strings"
)

// FormatQuoteSummary 格式化报价摘要（用于消息回复）
func FormatQuoteSummary(quoteID, routePOL, routePOD string, items []struct {
	ContainerType string
	Price         float64
	Currency      string
}, total float64, currency string) string {
	var sb strings.Builder

	sb.WriteString("当前报价单摘要：\n")
	sb.WriteString("━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("报价单号：%s\n", quoteID))
	sb.WriteString(fmt.Sprintf("航线：%s → %s\n", routePOL, routePOD))
	sb.WriteString("━━━━━━━━━━━━━━━━\n")

	for _, item := range items {
		sb.WriteString(fmt.Sprintf("%s: %s %.0f\n", item.ContainerType, item.Currency, item.Price))
	}

	sb.WriteString("━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("总计：%s %.2f\n", currency, total))
	sb.WriteString("━━━━━━━━━━━━━━━━\n")
	sb.WriteString("\n如需修改，请直接输入指令。\n")
	sb.WriteString("如需导出，请说 '导出图片' 或 '导出Excel'。")

	return sb.String()
}

// FormatHelpMessage 格式化帮助消息
func FormatHelpMessage() string {
	return `货运报价单助手 - 使用说明

【创建报价】
直接输入报价信息，例如：
"上海到洛杉矶，20GP 1000，40HQ 1800"

【修改报价】
"40HQ 改成 2000"
"加一个 45HQ 2200"
"去掉附加费"

【导出报价】
"导出图片" - 生成图片格式
"导出Excel" - 生成Excel文件
"导出文本" - 生成文本格式

【其他操作】
"查看历史" - 查看修改历史
"回滚版本 1" - 回滚到指定版本
"清空" - 开始新的报价

━━━━━━━━━━━━━━━━
如有问题，请联系管理员。`
}

// FormatErrorMessage 格式化错误消息
func FormatErrorMessage(err error) string {
	return fmt.Sprintf("抱歉，处理您的请求时出错：%s\n\n请重试或联系管理员。", err.Error())
}

// FormatWelcomeMessage 格式化欢迎消息
func FormatWelcomeMessage() string {
	return `欢迎使用货运报价单助手！

我可以帮您：
✅ 根据文字描述生成精美报价单
✅ 支持多轮对话修改报价
✅ 导出图片/Excel/文本格式

直接告诉我您的报价信息即可开始！
例如："上海到洛杉矶，20GP 1000，40HQ 1800"`
}

// FormatNoQuoteMessage 格式化无报价单消息
func FormatNoQuoteMessage() string {
	return `当前没有进行中的报价单。

请直接输入报价信息开始，例如：
"上海到洛杉矶，20GP 1000，40HQ 1800"`
}

// FormatHistoryMessage 格式化历史消息
func FormatHistoryMessage(versions []map[string]interface{}) string {
	if len(versions) == 0 {
		return "暂无修改历史。"
	}

	var sb strings.Builder
	sb.WriteString("修改历史：\n")
	sb.WriteString("━━━━━━━━━━━━━━━━\n")

	for _, v := range versions {
		sb.WriteString(fmt.Sprintf("[%d] %s - %s\n",
			v["index"], v["version"], v["timestamp"]))
	}

	sb.WriteString("━━━━━━━━━━━━━━━━\n")
	sb.WriteString("使用 '回滚版本 N' 恢复指定版本。")

	return sb.String()
}
