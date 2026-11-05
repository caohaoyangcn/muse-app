package providers

import (
	"bytes"
	"context"
	"fmt"
	stdhtml "html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"muse-app/backend/jav"
)

const (
	defaultJavBusBaseURL  = "https://javbus.com"
	defaultUserAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36"
	defaultAccept         = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"
	defaultAcceptLanguage = "zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7,zh-TW;q=0.6"
	defaultTimeout        = 15 * time.Second
	maxResponseBody       = 8 << 20
)

type RequestConfig struct {
	Cookie    string
	UserAgent string
	Accept    string
	Referer   string
	Timeout   time.Duration
}

// JavBusConfig is the public name used by application wiring.
type JavBusConfig = RequestConfig

type JavBus struct {
	client         *http.Client
	baseURL        string
	config         RequestConfig
	limiter        requestLimiter
	refererDefault bool
	mu             sync.RWMutex
	lastPageURL    string
}

func NewJavBus(client *http.Client, baseURL string) *JavBus {
	return NewJavBusWithConfig(client, baseURL, RequestConfig{})
}

func NewJavBusWithConfig(client *http.Client, baseURL string, config RequestConfig) *JavBus {
	return newJavBusWithLimiter(client, baseURL, config, globalJavBusRequestLimiter)
}

func newJavBusWithLimiter(client *http.Client, baseURL string, config RequestConfig, limiter requestLimiter) *JavBus {
	if limiter == nil {
		limiter = globalJavBusRequestLimiter
	}
	if baseURL == "" {
		baseURL = defaultJavBusBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	refererDefault := strings.TrimSpace(config.Referer) == ""
	if config.UserAgent == "" {
		config.UserAgent = defaultUserAgent
	}
	if config.Accept == "" {
		config.Accept = defaultAccept
	}
	if config.Referer == "" {
		config.Referer = baseURL + "/"
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultTimeout
	}
	if client == nil {
		// Leaving Transport as http.DefaultTransport preserves the normal
		// environment proxy behavior, including HTTP(S)_PROXY and NO_PROXY.
		client = &http.Client{Timeout: config.Timeout}
	} else if client.Timeout == 0 && config.Timeout > 0 {
		copy := *client
		copy.Timeout = config.Timeout
		client = &copy
	}
	return &JavBus{client: client, baseURL: baseURL, config: config, limiter: limiter, refererDefault: refererDefault}
}

func (p *JavBus) Name() string { return "javbus" }

// SetCookie replaces the in-memory Cookie used by detail and cover requests.
// It intentionally does not persist or log the value.
func (p *JavBus) SetCookie(cookie string) {
	p.mu.Lock()
	p.config.Cookie = strings.TrimSpace(cookie)
	p.mu.Unlock()
}

// CookieConfigured reports only whether a Cookie is present; it never returns the value.
func (p *JavBus) CookieConfigured() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return strings.TrimSpace(p.config.Cookie) != ""
}

// Fetch performs a request with the same headers, cookie, proxy and timeout
// configuration used by Lookup. The caller owns and must close the body.
func (p *JavBus) Fetch(ctx context.Context, rawURL string) (*http.Response, error) {
	return p.fetch(ctx, rawURL, false)
}

func (p *JavBus) fetch(ctx context.Context, rawURL string, document bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	p.applyHeaders(req, document)
	if err := p.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	res, err := p.client.Do(req)
	if document && err == nil {
		p.setLastPageURL(res)
	}
	return res, err
}

func (p *JavBus) Lookup(ctx context.Context, code string) (*jav.Metadata, error) {
	requestedCode := normalizeCode(code)
	if requestedCode == "" {
		return nil, jav.NewLookupError("parse", 0, fmt.Errorf("empty JavBus code"))
	}
	res, err := p.fetch(ctx, p.baseURL+"/"+url.PathEscape(requestedCode), true)
	if err != nil {
		return nil, jav.NewLookupError("network", 0, err)
	}
	defer func() { _ = res.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBody+1))
	if err != nil {
		return nil, jav.NewLookupError("read", res.StatusCode, err)
	}
	if len(body) > maxResponseBody {
		return nil, jav.NewLookupError("parse", res.StatusCode, fmt.Errorf("response too large"))
	}
	if isAgeVerification(res, body) {
		return nil, jav.NewLookupError("blocked", res.StatusCode, ageVerificationError(p.CookieConfigured()))
	}

	switch {
	case res.StatusCode == http.StatusNotFound:
		return nil, jav.NewLookupError("not_found", res.StatusCode, nil)
	case res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusUnauthorized:
		return nil, jav.NewLookupError("blocked", res.StatusCode, fmt.Errorf("JavBus rejected the request; a valid session Cookie may be required"))
	case res.StatusCode == http.StatusTooManyRequests || res.StatusCode >= 500:
		return nil, jav.NewLookupError("retryable", res.StatusCode, nil)
	case res.StatusCode < 200 || res.StatusCode >= 300:
		return nil, jav.NewLookupError("http", res.StatusCode, nil)
	}

	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, jav.NewLookupError("parse", res.StatusCode, err)
	}
	metadata := parseMetadata(doc, p.baseURL)
	if metadata.code != "" && normalizeCode(metadata.code) != requestedCode {
		return nil, jav.NewLookupError("parse", res.StatusCode, fmt.Errorf("JavBus code mismatch"))
	}
	if metadata.title == "" && metadata.coverURL == "" {
		return nil, jav.NewLookupError("parse", res.StatusCode, fmt.Errorf("metadata not found"))
	}

	result := &jav.Metadata{
		Code:     requestedCode,
		Title:    metadata.title,
		CoverURL: metadata.coverURL,
		Provider: p.Name(),
		Studio:   metadata.studio,
		Series:   metadata.series,
		Actors:   metadata.actors,
		Tags:     metadata.tags,
	}
	if metadata.releaseUnix != nil {
		result.ReleaseUnix = metadata.releaseUnix
	}
	if metadata.durationMin != nil {
		result.DurationMin = metadata.durationMin
	}
	return result, nil
}

func (p *JavBus) applyHeaders(req *http.Request, document bool) {
	p.mu.RLock()
	cookie := strings.TrimSpace(p.config.Cookie)
	p.mu.RUnlock()
	req.Header.Set("User-Agent", p.config.UserAgent)
	req.Header.Set("Accept", p.config.Accept)
	req.Header.Set("Accept-Language", defaultAcceptLanguage)
	req.Header.Set("Referer", p.referer())
	if document {
		req.Header.Set("Sec-Fetch-Dest", "document")
		req.Header.Set("Sec-Fetch-Mode", "navigate")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Sec-Fetch-User", "?1")
		req.Header.Set("Upgrade-Insecure-Requests", "1")
	}
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
}

func (p *JavBus) referer() string {
	if p.refererDefault {
		p.mu.RLock()
		lastPageURL := p.lastPageURL
		p.mu.RUnlock()
		if lastPageURL != "" {
			return lastPageURL
		}
	}
	return p.config.Referer
}

func (p *JavBus) setLastPageURL(res *http.Response) {
	if res == nil || res.Request == nil || res.Request.URL == nil {
		return
	}
	p.mu.Lock()
	p.lastPageURL = res.Request.URL.String()
	p.mu.Unlock()
}

func ageVerificationError(cookieConfigured bool) error {
	if !cookieConfigured {
		return fmt.Errorf("JavBus age verification required; no session Cookie configured")
	}
	return fmt.Errorf("JavBus age verification required; the session Cookie may be expired")
}

func isAgeVerification(res *http.Response, body []byte) bool {
	finalURL := ""
	if res != nil && res.Request != nil && res.Request.URL != nil {
		finalURL = strings.ToLower(res.Request.URL.String())
	}
	if strings.Contains(finalURL, "driver-verify") {
		return true
	}
	lowerBody := strings.ToLower(string(body))
	return strings.Contains(lowerBody, "driver-verify") || strings.Contains(lowerBody, "age verification")
}

type parsedMetadata struct {
	code        string
	title       string
	coverURL    string
	studio      string
	series      string
	releaseUnix *int64
	durationMin *int
	actors      []string
	tags        []string
}

func parseMetadata(doc *html.Node, baseURL string) parsedMetadata {
	meta := parseOpenGraph(doc)
	result := parsedMetadata{
		title:    firstNonEmpty(firstTextByTag(doc, "h3"), meta["og:title"]),
		coverURL: firstNonEmpty(findCoverURL(doc, baseURL), resolveURL(baseURL, meta["og:image"])),
	}

	forEachElement(doc, func(node *html.Node) {
		if node.Data != "p" {
			return
		}
		value, links, label := labeledParagraph(node)
		if label != "" {
			applyField(&result, label, value, links)
		}
	})
	parseLegacyHeaderFields(doc, &result)
	applyOpenGraphFields(&result, meta)

	seenActors := make(map[string]struct{})
	seenTags := make(map[string]struct{})
	forEachElement(doc, func(node *html.Node) {
		if node.Data != "a" {
			return
		}
		href := strings.ToLower(attribute(node, "href"))
		name := cleanText(textContent(node))
		if name == "" {
			return
		}
		switch {
		case strings.Contains(href, "/uncensored/star/"), strings.Contains(href, "/star/"):
			if _, ok := seenActors[name]; !ok {
				seenActors[name] = struct{}{}
				result.actors = append(result.actors, name)
			}
		case strings.Contains(href, "/genre/"):
			if _, ok := seenTags[name]; !ok {
				seenTags[name] = struct{}{}
				result.tags = append(result.tags, name)
			}
		}
	})
	return result
}

func applyOpenGraphFields(result *parsedMetadata, meta map[string]string) {
	if result.releaseUnix == nil {
		for _, key := range []string{"video:release_date", "article:published_time", "release_date"} {
			if value := meta[key]; value != "" {
				result.releaseUnix = parseReleaseDate(value)
				if result.releaseUnix != nil {
					break
				}
			}
		}
	}
	if result.durationMin == nil {
		for _, key := range []string{"video:duration", "og:duration", "duration"} {
			if value := meta[key]; value != "" {
				result.durationMin = parseDuration(value)
				if result.durationMin != nil {
					break
				}
			}
		}
	}
}

func parseLegacyHeaderFields(doc *html.Node, result *parsedMetadata) {
	forEachElement(doc, func(node *html.Node) {
		if node.Data != "span" || !strings.Contains(strings.ToLower(attribute(node, "class")), "header") {
			return
		}
		label := strings.TrimRight(cleanText(textContent(node)), ":： ")
		value, links := legacyHeaderValue(node)
		applyField(result, label, value, links)
	})
}

func legacyHeaderValue(header *html.Node) (string, []*html.Node) {
	var builder strings.Builder
	var links []*html.Node
	for sibling := header.NextSibling; sibling != nil; sibling = sibling.NextSibling {
		if sibling.Type == html.ElementNode && sibling.Data == "span" && strings.Contains(strings.ToLower(attribute(sibling, "class")), "header") {
			break
		}
		builder.WriteString(textContent(sibling))
		builder.WriteByte(' ')
		forEachElement(sibling, func(node *html.Node) {
			if node.Data == "a" {
				links = append(links, node)
			}
		})
	}
	return cleanText(builder.String()), links
}

func applyField(result *parsedMetadata, label, value string, links []*html.Node) {
	switch label {
	case "識別碼", "識别码", "品番", "番号":
		result.code = firstNonEmpty(value, firstLinkText(links))
	case "發行日期", "发行日期", "発売日":
		result.releaseUnix = parseReleaseDate(value)
	case "長度", "长度", "収録時間":
		result.durationMin = parseDuration(value)
	case "製作商", "制作商", "片商", "メーカー":
		result.studio = firstNonEmpty(firstLinkText(links), value)
	case "系列", "シリーズ":
		result.series = firstNonEmpty(firstLinkText(links), value)
	}
}

func parseOpenGraph(doc *html.Node) map[string]string {
	values := make(map[string]string)
	forEachElement(doc, func(node *html.Node) {
		if node.Data != "meta" {
			return
		}
		key := strings.ToLower(strings.TrimSpace(firstNonEmpty(attribute(node, "property"), attribute(node, "name"))))
		value := cleanText(attribute(node, "content"))
		if key != "" && value != "" {
			values[key] = stdhtml.UnescapeString(value)
		}
	})
	return values
}

func labeledParagraph(node *html.Node) (string, []*html.Node, string) {
	text := cleanText(textContent(node))
	for _, label := range []string{"識別碼", "識别码", "品番", "番号", "發行日期", "发行日期", "発売日", "長度", "长度", "収録時間", "製作商", "制作商", "片商", "メーカー", "系列", "シリーズ"} {
		if !strings.HasPrefix(text, label) {
			continue
		}
		value := strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(text, label), ":： "))
		var links []*html.Node
		forEachElement(node, func(child *html.Node) {
			if child.Data == "a" {
				links = append(links, child)
			}
		})
		return value, links, label
	}
	return "", nil, ""
}

func findCoverURL(doc *html.Node, baseURL string) string {
	var result string
	forEachElement(doc, func(node *html.Node) {
		if result != "" {
			return
		}
		switch node.Data {
		case "a", "img":
			for _, attrName := range []string{"href", "src", "data-src"} {
				raw := attribute(node, attrName)
				if isCoverPath(raw) {
					result = resolveURL(baseURL, raw)
					return
				}
			}
		}
	})
	return result
}

func isCoverPath(raw string) bool {
	path := strings.ToLower(strings.TrimSpace(raw))
	return strings.Contains(path, "/imgs/cover/") || strings.Contains(path, "/pics/cover/")
}

func parseReleaseDate(value string) *int64 {
	match := regexp.MustCompile(`(\d{4})[-/](\d{1,2})[-/](\d{1,2})`).FindStringSubmatch(value)
	if len(match) != 4 {
		return nil
	}
	parsed, err := time.Parse("2006-1-2", fmt.Sprintf("%s-%s-%s", match[1], match[2], match[3]))
	if err != nil {
		return nil
	}
	unix := parsed.Unix()
	return &unix
}

func parseDuration(value string) *int {
	match := regexp.MustCompile(`\d+`).FindString(value)
	if match == "" {
		return nil
	}
	var minutes int
	if _, err := fmt.Sscanf(match, "%d", &minutes); err != nil {
		return nil
	}
	return &minutes
}

func firstTextByTag(root *html.Node, tag string) string {
	var result string
	forEachElement(root, func(node *html.Node) {
		if result == "" && node.Data == tag {
			result = cleanText(textContent(node))
		}
	})
	return result
}

func firstLinkText(nodes []*html.Node) string {
	if len(nodes) == 0 {
		return ""
	}
	return cleanText(textContent(nodes[0]))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = cleanText(value); value != "" {
			return value
		}
	}
	return ""
}

func normalizeCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func resolveURL(baseURL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if parsed.IsAbs() {
		return parsed.String()
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return raw
	}
	return base.ResolveReference(parsed).String()
}

func forEachElement(root *html.Node, visit func(*html.Node)) {
	if root == nil {
		return
	}
	if root.Type == html.ElementNode {
		visit(root)
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		forEachElement(child, visit)
	}
}

func textContent(node *html.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == html.TextNode {
		return node.Data
	}
	var builder strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		builder.WriteString(textContent(child))
		builder.WriteByte(' ')
	}
	return builder.String()
}

func attribute(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, name) {
			return attr.Val
		}
	}
	return ""
}

func cleanText(value string) string {
	value = stdhtml.UnescapeString(value)
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
