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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
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

	// requestTimeout bounds one single-title translation. A scrape never waits
	// on this: the translation runs on its own queue.
	requestTimeout = 60 * time.Second
	// batchRequestTimeout gives a packed request room for several answers.
	// Ten long titles against a busy provider can exceed 90s, so this stays
	// below the jav service's 3-minute outer guard without cutting a slow but
	// successful pack short.
	batchRequestTimeout = 150 * time.Second
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

// TitleVersions returns the current fingerprint for each title: a hash of the
// system prompt, the normalized original title and the configured model. The
// bulk action compares it with the version stored next to the translation and
// skips rows that already match; an explicit per-video re-translate bypasses
// this check.
func (c *Client) TitleVersions(titles []string) ([]string, error) {
	cfg, err := c.loadConfig()
	if err != nil {
		return nil, fmt.Errorf("读取片名翻译设置失败：%w", err)
	}
	if !cfg.Usable() {
		return nil, ErrNotConfigured
	}
	versions := make([]string, len(titles))
	for i, title := range titles {
		versions[i] = titleVersion(cfg.Model, title)
	}
	return versions, nil
}

// TranslateTitles returns one Simplified-Chinese title per input, in the same
// order. Both returned slices have the same length as titles: translations[i]
// is empty when errs[i] explains why that one title could not be translated.
//
// The worker packs a bulk pass into a few titles per request, so the large
// system prompt is paid once per pack instead of once per title. Titles the
// model skipped (a garbled line, a missing number) are retried individually;
// an error that fails the whole request fills every slot, because retrying each
// title would only repeat the same provider failure.
func (c *Client) TranslateTitles(ctx context.Context, titles []string) ([]string, []error) {
	translations := make([]string, len(titles))
	errs := make([]error, len(titles))
	if len(titles) == 0 {
		return translations, errs
	}

	cfg, err := c.loadConfig()
	if err != nil {
		wrapped := fmt.Errorf("读取片名翻译设置失败：%w", err)
		for i := range errs {
			errs[i] = wrapped
		}
		return translations, errs
	}
	if !cfg.Usable() {
		for i := range errs {
			errs[i] = ErrNotConfigured
		}
		return translations, errs
	}
	if len(titles) == 1 {
		translations[0], errs[0] = c.translate(ctx, cfg, titles[0])
		return translations, errs
	}

	// Drop empty slots before packing, but keep their positions so the caller
	// can report a per-item error.
	batchTitles := make([]string, 0, len(titles))
	batchIndexes := make([]int, 0, len(titles))
	for i, title := range titles {
		title = sanitizeTitle(title)
		if title == "" {
			errs[i] = errors.New("标题为空，无法翻译")
			continue
		}
		batchTitles = append(batchTitles, title)
		batchIndexes = append(batchIndexes, i)
	}
	if len(batchTitles) == 0 {
		return translations, errs
	}
	if len(batchTitles) == 1 {
		idx := batchIndexes[0]
		translations[idx], errs[idx] = c.translate(ctx, cfg, batchTitles[0])
		return translations, errs
	}

	raw, err := c.translateBatch(ctx, cfg, batchTitles)
	if err != nil {
		for _, idx := range batchIndexes {
			errs[idx] = err
		}
		return translations, errs
	}

	parsed := parseBatchTranslations(raw, len(batchTitles))
	for i, idx := range batchIndexes {
		if parsed[i] != "" {
			translations[idx] = parsed[i]
			continue
		}
		// The pack came back short or with an unusable line. Ask for this one
		// title alone; guessing a neighbouring translation would silently put
		// the wrong Chinese name on the favorite.
		translations[idx], errs[idx] = c.translate(ctx, cfg, batchTitles[i])
	}
	return translations, errs
}

// titleVersion is the fingerprint persisted next to a translation. It covers
// exactly the inputs the user chose: the system prompt, the normalized original
// title and the model. A change to any of them makes every affected row stale
// without a schema change or a manual "clear all" step.
func titleVersion(model, title string) string {
	return titleFingerprint(systemPrompt, model, title)
}

func titleFingerprint(prompt, model, title string) string {
	key := strings.Join([]string{
		"muse-title-translation-v1",
		prompt,
		sanitizeTitle(title),
		strings.TrimSpace(model),
	}, "\x00")
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:16])
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
	title = sanitizeTitle(title)
	if title == "" {
		return "", errors.New("标题为空，无法翻译")
	}

	raw, err := c.complete(ctx, cfg, []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
		{Role: openai.ChatMessageRoleUser, Content: "原标题：" + title},
	}, requestTimeout)
	if err != nil {
		return "", err
	}

	translated := cleanTranslation(raw)
	if translated == "" {
		return "", errors.New("LLM 返回了空的翻译结果")
	}
	return truncateTranslation(translated), nil
}

// translateBatch asks for several titles in one request and returns the raw
// reply. Parsing decides which numbered rows arrived; a gap is retried alone by
// TranslateTitles.
func (c *Client) translateBatch(ctx context.Context, cfg Config, titles []string) (string, error) {
	return c.complete(ctx, cfg, []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
		{Role: openai.ChatMessageRoleUser, Content: batchPrompt(titles)},
	}, batchRequestTimeout)
}

// complete performs the shared chat-completions call and returns the first
// choice's raw text. Error mapping happens here so every caller shows the same
// actionable Chinese message.
func (c *Client) complete(ctx context.Context, cfg Config, messages []openai.ChatCompletionMessage, timeout time.Duration) (string, error) {
	clientConfig := openai.DefaultConfig(cfg.APIKey)
	clientConfig.BaseURL = cfg.BaseURL
	clientConfig.HTTPClient = c.httpClient
	client := openai.NewClientWithConfig(clientConfig)

	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	response, err := client.CreateChatCompletion(requestCtx, openai.ChatCompletionRequest{
		Model:       cfg.Model,
		Messages:    messages,
		Temperature: 0.2,
	})
	if err != nil {
		return "", describeError(err)
	}
	if len(response.Choices) == 0 {
		return "", errors.New("LLM 没有返回翻译结果")
	}
	return response.Choices[0].Message.Content, nil
}

// batchPrompt numbers each title so the reply can be unpacked by index. Titles
// never contain a newline (sanitizeTitle flattens them), which keeps one line
// per input.
func batchPrompt(titles []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "请把下面 %d 条日文标题分别翻译成简体中文。\n", len(titles))
	b.WriteString("严格按编号逐行输出，每条译文占一行，格式为「编号. 译文」，编号必须与输入一致；")
	b.WriteString("不要输出原文、解释、空行或前言。\n\n")
	for i, title := range titles {
		fmt.Fprintf(&b, "%d. %s\n", i+1, title)
	}
	return b.String()
}

// numberedTranslationLine matches the requested "1. 译文" format, tolerating
// list bullets and the punctuation models use instead of a period.
var numberedTranslationLine = regexp.MustCompile(`^\s*(?:[-+•]\s*)?(\d+)\s*[.、:：)）．。]\s*(.+?)\s*$`)

// parseBatchTranslations maps a numbered reply back onto the input order. An
// empty string means the model did not return a usable line for that index, so
// the caller can retry it alone. If the model ignored the numbering and
// returned exactly one line per title, positional order is used as a fallback.
func parseBatchTranslations(raw string, count int) []string {
	out := make([]string, count)
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	numbered := 0
	for _, line := range lines {
		// Models often bold the number; the decoration is not part of the
		// translation, and stripping it lets the same parser read both.
		line = strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "*"))
		match := numberedTranslationLine.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		index, err := strconv.Atoi(match[1])
		if err != nil || index < 1 || index > count || out[index-1] != "" {
			continue
		}
		translated := cleanTranslation(match[2])
		if translated == "" {
			continue
		}
		out[index-1] = truncateTranslation(translated)
		numbered++
	}
	if numbered > 0 {
		return out
	}

	plain := make([]string, 0, count)
	for _, line := range lines {
		line = trimListBullet(line)
		if line == "" || strings.HasPrefix(line, "```") {
			continue
		}
		if translated := cleanTranslation(line); translated != "" {
			plain = append(plain, truncateTranslation(translated))
		}
	}
	if len(plain) == count {
		copy(out, plain)
	}
	return out
}

// trimListBullet removes a Markdown-style list marker from an unnumbered reply
// line; without it the positional fallback would store "- 译名".
func trimListBullet(line string) string {
	line = strings.TrimSpace(line)
	for _, prefix := range []string{"- ", "* ", "+ ", "• "} {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return line
}

// sanitizeTitle flattens a title onto one line and bounds its length. A newline
// would otherwise shift every following number in a packed request.
func sanitizeTitle(title string) string {
	title = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(title)
	title = strings.TrimSpace(title)
	if utf8.RuneCountInString(title) > maxTitleRunes {
		title = string([]rune(title)[:maxTitleRunes])
	}
	return title
}

func truncateTranslation(translated string) string {
	if utf8.RuneCountInString(translated) > maxTranslationRunes {
		return string([]rune(translated)[:maxTranslationRunes])
	}
	return translated
}

const systemPrompt = `# System Prompt：日文成人向文本专业翻译器（日译中）

## 角色定位
你是一名精通日语与简体中文的专业翻译人员，擅长日本成人向内容、成人视频标题、情色小说、漫画对白及成人题材宣传文案的翻译。

你的任务是将用户提供的日文翻译成简体中文。

## 核心翻译原则

1. **忠实原文**：准确传达原文的含义、语气、情绪、动作及表达强度，不擅自删减、弱化或添加内容。
2. **自然流畅**：使用符合中文母语者习惯的表达方式，避免生硬的日语直译和日式中文。
3. **保留语气**：保留原文的夸张、煽情、挑逗、戏剧化及宣传性语气。
4. **术语准确**：准确识别成人题材中的身体部位、生理反应、性行为、快感及相关专业词汇。
5. **语境优先**：结合上下文理解日语中的省略、双关、拟声词、拟态词及隐喻。
6. **强度对等**：原文表达强烈时，中文也应保持相应的表达强度，不得无故净化或委婉化。

## 特殊翻译要求

- 日语成人向词汇应翻译为中文语境中自然、准确的对应表达。
- 对于夸张的日式宣传文案，优先采用中文成人题材文案常见的表达习惯。
- 保留原文中的感叹号、强调语气及特殊标点所表达的情绪。
- 对于日语复合词，应拆解词义后结合语境重新组织中文句子。
- 不要逐字硬译，应以准确传达整体语义为第一优先级。
- 不要遗漏原文中的动作、状态、程度和情绪信息。

## 输出格式

默认只输出最终中文译文。

不要输出：
- 日文原文
- 翻译过程
- 词汇解释
- 无关说明
- 多余的前言或结语

如果原文存在无法确定的歧义，优先选择最符合上下文的解释，不要随意编造。

## 最终目标

让中文译文看起来像由熟悉日本成人文化及中文表达习惯的专业本地化译者直接创作，而不是机器逐字翻译。`

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
