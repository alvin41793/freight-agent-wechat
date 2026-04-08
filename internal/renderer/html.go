package renderer

import (
	"bytes"
	"fmt"
	"html/template"

	"freight-agent-wechat/internal/quote"
)

// HTMLRenderer HTML 渲染器
type HTMLRenderer struct {
	template *template.Template
}

// QuoteTemplateData 报价单模板数据
type QuoteTemplateData struct {
	QuoteID     string
	Route       quote.RouteInfo
	Items       []QuoteItemData
	Surcharges  []SurchargeData
	Subtotal    float64
	Total       float64
	Currency    string
	ValidUntil  string
	Remarks     string
	CreatedAt   string
	CompanyName string
	CompanyLogo string
}

// QuoteItemData 报价项模板数据
type QuoteItemData struct {
	ContainerType string
	Price         string
	Currency      string
}

// SurchargeData 附加费模板数据
type SurchargeData struct {
	Name  string
	Price string
	Unit  string
}

// NewHTMLRenderer 创建 HTML 渲染器
func NewHTMLRenderer() (*HTMLRenderer, error) {
	tmpl, err := template.New("quote").Parse(quoteTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	return &HTMLRenderer{template: tmpl}, nil
}

// Render 渲染报价单为 HTML
func (r *HTMLRenderer) Render(quoteData *quote.QuoteData) (string, error) {
	data := r.prepareTemplateData(quoteData)

	var buf bytes.Buffer
	if err := r.template.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// prepareTemplateData 准备模板数据
func (r *HTMLRenderer) prepareTemplateData(quoteData *quote.QuoteData) *QuoteTemplateData {
	items := make([]QuoteItemData, len(quoteData.Items))
	for i, item := range quoteData.Items {
		items[i] = QuoteItemData{
			ContainerType: item.ContainerType,
			Price:         fmt.Sprintf("%.2f", item.Price),
			Currency:      item.Currency,
		}
	}

	surcharges := make([]SurchargeData, len(quoteData.Surcharges))
	for i, s := range quoteData.Surcharges {
		unit := "固定"
		if s.Unit == "per_container" {
			unit = "每柜"
		}
		surcharges[i] = SurchargeData{
			Name:  s.Name,
			Price: fmt.Sprintf("%.2f", s.Price),
			Unit:  unit,
		}
	}

	validUntil := ""
	if quoteData.ValidUntil != nil {
		validUntil = quoteData.ValidUntil.Format("2006-01-02")
	}

	return &QuoteTemplateData{
		QuoteID:     quoteData.QuoteID,
		Route:       quoteData.Route,
		Items:       items,
		Surcharges:  surcharges,
		Subtotal:    r.calculateSubtotal(quoteData),
		Total:       quoteData.CalculateTotal(),
		Currency:    quoteData.Currency,
		ValidUntil:  validUntil,
		Remarks:     quoteData.Remarks,
		CreatedAt:   quoteData.CreatedAt.Format("2006-01-02 15:04"),
		CompanyName: "货运报价系统",
	}
}

// calculateSubtotal 计算小计（仅集装箱费用）
func (r *HTMLRenderer) calculateSubtotal(quoteData *quote.QuoteData) float64 {
	var subtotal float64
	for _, item := range quoteData.Items {
		subtotal += item.Price
	}
	return subtotal
}

// quoteTemplate 报价单 HTML 模板
const quoteTemplate = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>货运报价单 - {{.QuoteID}}</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <style>
        @import url('https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap');
        body { font-family: 'Inter', sans-serif; }
    </style>
</head>
<body class="bg-gray-50 min-h-screen p-8">
    <div class="max-w-4xl mx-auto bg-white rounded-xl shadow-lg overflow-hidden">
        <!-- 头部 -->
        <div class="bg-gradient-to-r from-blue-600 to-blue-800 text-white p-8">
            <div class="flex justify-between items-start">
                <div>
                    <h1 class="text-3xl font-bold mb-2">{{.CompanyName}}</h1>
                    <p class="text-blue-100">Freight Quotation</p>
                </div>
                <div class="text-right">
                    <p class="text-sm text-blue-100">报价单号</p>
                    <p class="text-xl font-mono font-semibold">{{.QuoteID}}</p>
                </div>
            </div>
        </div>

        <!-- 主体内容 -->
        <div class="p-8">
            <!-- 航线信息 -->
            <div class="mb-8">
                <h2 class="text-lg font-semibold text-gray-800 mb-4 border-b pb-2">航线信息</h2>
                <div class="grid grid-cols-2 gap-6">
                    <div class="bg-gray-50 rounded-lg p-4">
                        <p class="text-sm text-gray-500 mb-1">起运港 (POL)</p>
                        <p class="text-xl font-semibold text-gray-800">{{.Route.POL}}</p>
                    </div>
                    <div class="bg-gray-50 rounded-lg p-4">
                        <p class="text-sm text-gray-500 mb-1">目的港 (POD)</p>
                        <p class="text-xl font-semibold text-gray-800">{{.Route.POD}}</p>
                    </div>
                </div>
            </div>

            <!-- 报价明细 -->
            <div class="mb-8">
                <h2 class="text-lg font-semibold text-gray-800 mb-4 border-b pb-2">运费明细</h2>
                <table class="w-full">
                    <thead>
                        <tr class="bg-gray-100">
                            <th class="text-left py-3 px-4 rounded-tl-lg">集装箱类型</th>
                            <th class="text-right py-3 px-4 rounded-tr-lg">价格 ({{.Currency}})</th>
                        </tr>
                    </thead>
                    <tbody>
                        {{range .Items}}
                        <tr class="border-b border-gray-100">
                            <td class="py-3 px-4 font-medium">{{.ContainerType}}</td>
                            <td class="py-3 px-4 text-right">{{.Price}}</td>
                        </tr>
                        {{end}}
                    </tbody>
                </table>
                <div class="mt-4 text-right">
                    <span class="text-gray-600">小计：</span>
                    <span class="text-xl font-semibold text-blue-600">{{.Currency}} {{printf "%.2f" .Subtotal}}</span>
                </div>
            </div>

            <!-- 附加费 -->
            {{if .Surcharges}}
            <div class="mb-8">
                <h2 class="text-lg font-semibold text-gray-800 mb-4 border-b pb-2">附加费</h2>
                <table class="w-full">
                    <thead>
                        <tr class="bg-gray-100">
                            <th class="text-left py-3 px-4 rounded-tl-lg">费用名称</th>
                            <th class="text-center py-3 px-4">计费方式</th>
                            <th class="text-right py-3 px-4 rounded-tr-lg">金额 ({{.Currency}})</th>
                        </tr>
                    </thead>
                    <tbody>
                        {{range .Surcharges}}
                        <tr class="border-b border-gray-100">
                            <td class="py-3 px-4">{{.Name}}</td>
                            <td class="py-3 px-4 text-center text-gray-500">{{.Unit}}</td>
                            <td class="py-3 px-4 text-right">{{.Price}}</td>
                        </tr>
                        {{end}}
                    </tbody>
                </table>
            </div>
            {{end}}

            <!-- 总计 -->
            <div class="bg-blue-50 rounded-lg p-6 mb-8">
                <div class="flex justify-between items-center">
                    <span class="text-lg font-semibold text-gray-700">总计</span>
                    <span class="text-3xl font-bold text-blue-700">{{.Currency}} {{printf "%.2f" .Total}}</span>
                </div>
            </div>

            <!-- 备注 -->
            {{if .Remarks}}
            <div class="mb-6">
                <h2 class="text-lg font-semibold text-gray-800 mb-2">备注</h2>
                <p class="text-gray-600 bg-yellow-50 rounded-lg p-4">{{.Remarks}}</p>
            </div>
            {{end}}

            <!-- 有效期 -->
            {{if .ValidUntil}}
            <div class="mb-6">
                <p class="text-sm text-gray-500">
                    <span class="font-semibold">有效期至：</span>{{.ValidUntil}}
                </p>
            </div>
            {{end}}

            <!-- 页脚 -->
            <div class="border-t pt-6 text-center text-sm text-gray-400">
                <p>生成时间：{{.CreatedAt}}</p>
                <p class="mt-1">本报价单仅供参考，具体以正式合同为准</p>
            </div>
        </div>
    </div>
</body>
</html>`
