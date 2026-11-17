// Package llm translates scraped titles into Simplified Chinese through an
// OpenAI-compatible chat-completions endpoint (DeepSeek, OpenAI, Ollama, LM
// Studio, OpenRouter, …).
//
// The request/response plumbing is delegated to
// github.com/sashabaranov/go-openai so Muse only owns the settings shape, the
// prompt and the output cleanup. Credentials never leave the settings store:
// Config is loaded on every call, so editing the provider in Settings applies
// to the next translation without restarting the app.
package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	openai "github.com/sashabaranov/go-openai"
)

const (
	// DefaultBaseURL is the DeepSeek endpoint, the provider most Muse users in
	// mainland China can reach directly. Any OpenAI-compatible base URL works.
	DefaultBaseURL = "https://api.deepseek.com/v1"
	// DefaultModel is DeepSeek's general chat model: cheap, fast enough for one
	// short translation per scrape, and good at Japanese → Chinese titles.
	DefaultModel = "deepseek-chat"

	// requestTimeout bounds one translation. A scrape never waits on this: the
	// translation runs on its own queue.
	requestTimeout = 60 * time.Second
	// maxTitleRunes bounds what is sent to the model. Titles are far shorter;
	// the cap protects the request from a pathological scrape result.
	maxTitleRunes = 400
	// maxTranslationRunes bounds a stored translation so a model that ignores
	// the "only the title" instruction cannot fill the detail panel.
	maxTranslationRunes = 200
	// maxAPIKeyLength bounds a pasted key.
	maxAPIKeyLength = 16 * 1024
)

// ErrNotConfigured means translation is disabled or missing a provider, model
// or API key. It is a state, not a failure: the scrape flow simply skips it.
var ErrNotConfigured = errors.New("title translation is not configured")

// Config is the persisted provider configuration. The API key is kept in
// memory only while a request is built; it is stored encrypted by settings.
type Config struct {
	Enabled bool
	BaseURL string
	Model   string
	APIKey  string
}

// Usable reports whether the provider is complete enough to attempt a request.
// It ignores Enabled: a paused auto-translate still serves the manual actions
// (translate one title, or backfill missing ones).
func (c Config) Usable() bool {
	return strings.TrimSpace(c.BaseURL) != "" &&
		strings.TrimSpace(c.Model) != "" &&
		strings.TrimSpace(c.APIKey) != ""
}

// AutoTranslate reports whether a newly scraped title should be translated
// without being asked: the provider is usable and the user has not paused the
// automatic path.
func (c Config) AutoTranslate() bool {
	return c.Enabled && c.Usable()
}

// Normalize trims and validates a user-edited configuration, filling the
// defaults for an empty endpoint or model. An empty API key is allowed here:
// the settings form can save an endpoint before the key is entered, and the
// store keeps the previously saved key.
func Normalize(c Config) (Config, error) {
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return Config{}, fmt.Errorf("接口地址格式无效：%w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return Config{}, errors.New("接口地址必须以 http:// 或 https:// 开头")
	}
	if u.Hostname() == "" {
		return Config{}, errors.New("接口地址缺少主机名")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return Config{}, errors.New("接口地址不能包含查询参数或片段")
	}

	c.Model = strings.TrimSpace(c.Model)
	if c.Model == "" {
		c.Model = DefaultModel
	}
	if len(c.Model) > 200 {
		return Config{}, errors.New("模型名称过长（最多 200 个字符）")
	}

	c.APIKey = strings.TrimSpace(c.APIKey)
	if strings.ContainsAny(c.APIKey, "\r\n") {
		return Config{}, errors.New("API Key 不能包含换行")
	}
	if len(c.APIKey) > maxAPIKeyLength {
		return Config{}, fmt.Errorf("API Key 过长（最多 %d 个字符）", maxAPIKeyLength)
	}
	return c, nil
}

// Client translates titles with the provider described by the settings store.
// loadConfig is called for every translation, so a configuration change needs
// no rebuild; httpClient is shared so connections are reused across calls.
type Client struct {
	loadConfig func() (Config, error)
	httpClient *http.Client
}

// NewClient builds a translator over loadConfig. A nil httpClient falls back to
// one owned by the client; main passes the scraping proxy transport so LLM
// requests follow the same network route as metadata requests.
func NewClient(loadConfig func() (Config, error), httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Client{loadConfig: loadConfig, httpClient: httpClient}
}

// Configured reports whether a newly scraped title should be translated
// automatically. A settings read error counts as "not configured": translation
// is optional enrichment and must never block a scrape.
func (c *Client) Configured() bool {
	cfg, err := c.loadConfig()
	return err == nil && cfg.AutoTranslate()
}

// Available reports whether a translation request can run at all. Unlike
// Configured it ignores the auto-translate pause, so the detail-panel button
// and the bulk backfill keep working while background translation is off.
func (c *Client) Available() bool {
	cfg, err := c.loadConfig()
	return err == nil && cfg.Usable()
}

// TranslateTitle returns one Simplified-Chinese title. The model is asked for
// the title alone; cleanTranslation removes the decorations models add anyway.
func (c *Client) TranslateTitle(ctx context.Context, title string) (string, error) {
	cfg, err := c.loadConfig()
	if err != nil {
		return "", fmt.Errorf("读取片名翻译设置失败：%w", err)
	}
	if !cfg.Usable() {
		return "", ErrNotConfigured
	}
	return c.translate(ctx, cfg, title)
}

// checkSampleTitle is the short Japanese title used by Check. It has kana and
// kanji but no番号, so it exercises the real translation prompt without
// depending on any scraped record.
const checkSampleTitle = "となりの人妻"

// Check validates an explicit provider configuration by translating one short
// sample title. It is the settings page's 测试连接 action: the values come from
// the form (which may not be saved yet), and the sample translation proves that
// the endpoint, the API key and the model all work together.
//
// Enabled is ignored on purpose: testing a provider that is currently paused
// must still report whether it is correctly configured.
func (c *Client) Check(ctx context.Context, cfg Config) (string, error) {
	normalized, err := Normalize(cfg)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(normalized.APIKey) == "" {
		return "", errors.New("请填写 API Key（留空时使用已保存的 Key）")
	}
	translated, err := c.translate(ctx, normalized, checkSampleTitle)
	if err != nil {
		return "", err
	}
	return translated, nil
}

// translate performs one chat-completions request with an already validated
// configuration. Both the production path (TranslateTitle) and the settings
// smoke test (Check) share it, so the test exercises exactly what a scrape does.
func (c *Client) translate(ctx context.Context, cfg Config, title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", errors.New("标题为空，无法翻译")
	}
	if utf8.RuneCountInString(title) > maxTitleRunes {
		title = string([]rune(title)[:maxTitleRunes])
	}

	clientConfig := openai.DefaultConfig(cfg.APIKey)
	clientConfig.BaseURL = cfg.BaseURL
	clientConfig.HTTPClient = c.httpClient
	client := openai.NewClientWithConfig(clientConfig)

	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	response, err := client.CreateChatCompletion(requestCtx, openai.ChatCompletionRequest{
		Model: cfg.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: "原标题：" + title},
		},
		Temperature: 0.2,
	})
	if err != nil {
		return "", describeError(err)
	}
	if len(response.Choices) == 0 {
		return "", errors.New("LLM 没有返回翻译结果")
	}

	translated := cleanTranslation(response.Choices[0].Message.Content)
	if translated == "" {
		return "", errors.New("LLM 返回了空的翻译结果")
	}
	if utf8.RuneCountInString(translated) > maxTranslationRunes {
		translated = string([]rune(translated)[:maxTranslationRunes])
	}
	return translated, nil
}

const systemPrompt = `你是影视资料编辑，负责把日本影片标题翻译成简体中文片名。
规则：
1. 只输出译名本身，不要引号、书名号、解释、拼音或原标题；
2. 保留作品番号、系列名和人名的通行译法；
3. 标题若已经是中文，原样输出；
4. 译名尽量控制在 40 个汉字以内，要像正式发行的片名，而不是逐字直译。`

// cleanTranslation reduces a model reply to one title line: the first
// non-empty line, without a "中文译名："-style label or wrapping quotes.
func cleanTranslation(raw string) string {
	text := strings.TrimSpace(raw)
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			text = trimmed
			break
		}
	}
	for _, prefix := range []string{
		"中文译名：", "中文译名:", "中文片名：", "中文片名:",
		"译名：", "译名:", "翻译：", "翻译:", "标题：", "标题:",
	} {
		if strings.HasPrefix(text, prefix) {
			text = strings.TrimSpace(strings.TrimPrefix(text, prefix))
			break
		}
	}
	return strings.TrimSpace(strings.Trim(text, "\"'“”‘’「」『』《》"))
}

// describeError turns a provider failure into an actionable Chinese message.
// The raw SDK error stays wrapped for the log, but the detail panel shows this
// message, so it names the likely cause instead of an HTTP status alone.
func describeError(err error) error {
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.HTTPStatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return fmt.Errorf("LLM 鉴权失败（%d）：请检查 API Key 是否有权限", apiErr.HTTPStatusCode)
		case http.StatusNotFound:
			return errors.New("LLM 接口或模型不存在（404）：请检查接口地址与模型名")
		case http.StatusTooManyRequests:
			return errors.New("LLM 请求过于频繁（429）：请稍后再试")
		}
		if apiErr.HTTPStatusCode >= http.StatusInternalServerError {
			return fmt.Errorf("LLM 服务暂时不可用（%d）", apiErr.HTTPStatusCode)
		}
		message := strings.TrimSpace(apiErr.Message)
		if message == "" {
			message = "请求被拒绝"
		}
		return fmt.Errorf("LLM 请求失败（%d）：%s", apiErr.HTTPStatusCode, message)
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return errors.New("LLM 请求超时")
	}
	var requestErr *openai.RequestError
	if errors.As(err, &requestErr) {
		return fmt.Errorf("无法连接 LLM 服务：%w", requestErr.Err)
	}
	return fmt.Errorf("LLM 请求失败：%w", err)
}
