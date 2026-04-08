package renderer

import (
	"context"
	"fmt"
	"time"

	"freight-agent-wechat/internal/quote"

	"github.com/chromedp/chromedp"
)

// ImageRenderer 图片渲染器
type ImageRenderer struct {
	htmlRenderer *HTMLRenderer
	timeout      time.Duration
}

// NewImageRenderer 创建图片渲染器
func NewImageRenderer() (*ImageRenderer, error) {
	htmlRenderer, err := NewHTMLRenderer()
	if err != nil {
		return nil, err
	}

	return &ImageRenderer{
		htmlRenderer: htmlRenderer,
		timeout:      30 * time.Second,
	}, nil
}

// RenderToImage 将报价单渲染为图片
func (r *ImageRenderer) RenderToImage(ctx context.Context, htmlContent string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	// 创建 chromedp 上下文
	allocCtx, allocCancel := chromedp.NewContext(ctx)
	defer allocCancel()

	var buf []byte
	err := chromedp.Run(allocCtx,
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// 设置页面内容
			script := fmt.Sprintf(`document.open(); document.write(%q); document.close();`, htmlContent)
			return chromedp.Evaluate(script, nil).Do(ctx)
		}),
		chromedp.Sleep(500*time.Millisecond), // 等待页面渲染
		chromedp.FullScreenshot(&buf, 100),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to render image: %w", err)
	}

	return buf, nil
}

// RenderQuoteToImage 渲染报价单为图片
func (r *ImageRenderer) RenderQuoteToImage(ctx context.Context, quoteData *quote.QuoteData) ([]byte, error) {
	html, err := r.htmlRenderer.Render(quoteData)
	if err != nil {
		return nil, fmt.Errorf("failed to render HTML: %w", err)
	}
	return r.RenderToImage(ctx, html)
}
