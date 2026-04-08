package renderer

import (
	"fmt"

	"freight-agent-wechat/internal/quote"

	"github.com/xuri/excelize/v2"
)

// ExcelRenderer Excel 渲染器
type ExcelRenderer struct{}

// NewExcelRenderer 创建 Excel 渲染器
func NewExcelRenderer() *ExcelRenderer {
	return &ExcelRenderer{}
}

// Render 渲染报价单为 Excel
func (r *ExcelRenderer) Render(quoteData *quote.QuoteData) ([]byte, error) {
	f := excelize.NewFile()

	// 设置工作表名称
	sheetName := "报价单"
	f.SetSheetName("Sheet1", sheetName)

	// 设置列宽
	f.SetColWidth(sheetName, "A", "D", 20)

	// 标题
	f.SetCellValue(sheetName, "A1", "货运报价单")
	f.SetCellValue(sheetName, "A2", fmt.Sprintf("报价单号：%s", quoteData.QuoteID))

	// 合并标题单元格
	f.MergeCell(sheetName, "A1", "D1")

	// 设置标题样式
	titleStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold:  true,
			Size:  16,
			Color: "FFFFFF",
		},
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"2563EB"},
			Pattern: 1,
		},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
		},
	})
	f.SetCellStyle(sheetName, "A1", "D1", titleStyle)

	// 航线信息
	row := 4
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "航线信息")
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row+1), "起运港 (POL)")
	f.SetCellValue(sheetName, fmt.Sprintf("B%d", row+1), quoteData.Route.POL)
	f.SetCellValue(sheetName, fmt.Sprintf("C%d", row+1), "目的港 (POD)")
	f.SetCellValue(sheetName, fmt.Sprintf("D%d", row+1), quoteData.Route.POD)

	// 运费明细
	row = 7
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "运费明细")
	row++
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "集装箱类型")
	f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), fmt.Sprintf("价格 (%s)", quoteData.Currency))

	// 表头样式
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{
			Type:    "pattern",
			Color:   []string{"E5E7EB"},
			Pattern: 1,
		},
	})
	f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), headerStyle)

	// 数据行
	row++
	for _, item := range quoteData.Items {
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), item.ContainerType)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), item.Price)
		row++
	}

	// 小计
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "小计")
	f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), calculateSubtotal(quoteData))

	// 附加费
	if len(quoteData.Surcharges) > 0 {
		row += 2
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "附加费")
		row++
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "费用名称")
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), "计费方式")
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), fmt.Sprintf("金额 (%s)", quoteData.Currency))
		f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("C%d", row), headerStyle)
		row++

		for _, surcharge := range quoteData.Surcharges {
			f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), surcharge.Name)
			unit := "固定"
			if surcharge.Unit == "per_container" {
				unit = "每柜"
			}
			f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), unit)
			f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), surcharge.Price)
			row++
		}
	}

	// 总计
	row += 1
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "总计")
	f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), quoteData.CalculateTotal())

	// 总计样式
	totalStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{
			Bold:  true,
			Size:  14,
			Color: "2563EB",
		},
	})
	f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), totalStyle)

	// 备注
	if quoteData.Remarks != "" {
		row += 2
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "备注")
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row+1), quoteData.Remarks)
		f.MergeCell(sheetName, fmt.Sprintf("A%d", row+1), fmt.Sprintf("D%d", row+1))
	}

	// 页脚
	row += 3
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("生成时间：%s", quoteData.CreatedAt.Format("2006-01-02 15:04:05")))
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row+1), "本报价单仅供参考，具体以正式合同为准")

	// 写入缓冲区
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write excel to buffer: %w", err)
	}

	return buf.Bytes(), nil
}

// calculateSubtotal 计算小计
func calculateSubtotal(quoteData *quote.QuoteData) float64 {
	var subtotal float64
	for _, item := range quoteData.Items {
		subtotal += item.Price
	}
	return subtotal
}
