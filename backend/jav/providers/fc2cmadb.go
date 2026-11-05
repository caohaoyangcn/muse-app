package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	stdhtml "html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"muse-app/backend/jav"
)

const (
	defaultFC2CMADBBaseURL = "https://fc2cmadb.com"
	fc2CMADBTimeout        = 20 * time.Second
	fc2CMADBMaxResponse    = 8 << 20
	fc2CMADBRequestGap     = 1500 * time.Millisecond
	fc2CMADBUserAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36 Edg/152.0.0.0"
	fc2CMADBAccept         = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"
	fc2CMADBAcceptLanguage = "zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7,zh-TW;q=0.6"
	fc2CMADBPriority       = "u=0, i"
	fc2CMADBSecCHUA        = `"Chromium";v="152", "Not?A_Brand";v="24", "Microsoft Edge";v="152"`
	fc2CMADBSecCHPlatform  = `"Windows"`
)

var fc2CodeInputPattern = regexp.MustCompile(`(?i)^(?:FC2[-_ ]?(?:PPV[-_ ]?)?|FC2[-_ ]?)([0-9]{5,10})$`)
var fc2DatePattern = regexp.MustCompile(`\b(20[0-9]{2})[-年/]([0-9]{1,2})[-月/]([0-9]{1,2})日?\b`)
var fc2DurationPattern = regexp.MustCompile(`(?i)(?:^|[^0-9])([0-9]{1,4})\s*(?:分|分钟|min|minutes?)(?:$|[^a-z])`)
var fc2SecondsPattern = regexp.MustCompile(`(?i)(?:^|[^0-9])([0-9]{1,8})\s*(?:秒|秒钟|sec|secs|second|seconds)(?:$|[^a-z])`)

// FC2CMADBConfig controls the FC2CMADB request headers and session cookie.
type FC2CMADBConfig struct {
	Cookie    string
	UserAgent string
	Timeout   time.Duration
}

// FC2CMADB accesses article detail pages using an instance-local session.
// Cookie values are intentionally kept only in memory by this provider.
type FC2CMADB struct {
	client  *http.Client
	baseURL *url.URL
	config  FC2CMADBConfig
	limiter requestLimiter
	log     *slog.Logger
	mu      sync.RWMutex
}

func NewFC2CMADB(client *http.Client, baseURL string, config FC2CMADBConfig) *FC2CMADB {
	return newFC2CMADBWithLimiter(client, baseURL, config, newIntervalLimiter(fc2CMADBRequestGap))
}

func NewFC2CMADBWithLogger(client *http.Client, baseURL string, config FC2CMADBConfig, log *slog.Logger) *FC2CMADB {
	return newFC2CMADBWithLogger(client, baseURL, config, newIntervalLimiter(fc2CMADBRequestGap), log)
}

func newFC2CMADBWithLimiter(client *http.Client, baseURL string, config FC2CMADBConfig, limiter requestLimiter) *FC2CMADB {
	return newFC2CMADBWithLogger(client, baseURL, config, limiter, nil)
}

func newFC2CMADBWithLogger(client *http.Client, baseURL string, config FC2CMADBConfig, limiter requestLimiter, log *slog.Logger) *FC2CMADB {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultFC2CMADBBaseURL
	}
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		parsed, _ = url.Parse(defaultFC2CMADBBaseURL)
	}
	if config.UserAgent == "" {
		config.UserAgent = fc2CMADBUserAgent
	}
	if config.Timeout <= 0 {
		config.Timeout = fc2CMADBTimeout
	}
	if client == nil {
		client = &http.Client{Transport: http.DefaultTransport, Timeout: config.Timeout}
	} else if client.Timeout == 0 {
		copy := *client
		copy.Timeout = config.Timeout
		client = &copy
	}
	if limiter == nil {
		limiter = newIntervalLimiter(fc2CMADBRequestGap)
	}
	config.Cookie = normalizeProviderCookie(config.Cookie)
	return &FC2CMADB{client: client, baseURL: parsed, config: config, limiter: limiter, log: log}
}

func (p *FC2CMADB) Name() string { return "fc2cmadb" }

func (p *FC2CMADB) SetCookie(cookie string) {
	p.mu.Lock()
	p.config.Cookie = normalizeProviderCookie(cookie)
	p.mu.Unlock()
}

func normalizeProviderCookie(cookie string) string {
	cookie = strings.TrimSpace(cookie)
	if len(cookie) >= len("Cookie:") && strings.EqualFold(cookie[:len("Cookie:")], "Cookie:") {
		cookie = strings.TrimSpace(cookie[len("Cookie:"):])
	}
	return cookie
}

func (p *FC2CMADB) debug(message string, args ...any) {
	if p != nil && p.log != nil {
		p.log.Info(message, args...)
	}
}

func cookieHasKey(cookie, key string) bool {
	for _, part := range strings.Split(cookie, ";") {
		name := strings.TrimSpace(strings.SplitN(part, "=", 2)[0])
		if strings.EqualFold(name, key) {
			return true
		}
	}
	return false
}

func cookieHasSessionKey(cookie string) bool {
	for _, part := range strings.Split(cookie, ";") {
		name := strings.ToLower(strings.TrimSpace(strings.SplitN(part, "=", 2)[0]))
		if strings.Contains(name, "session") || strings.Contains(name, "auth") {
			return true
		}
	}
	return false
}

func lookupErrorKind(err error) string {
	var lookupErr *jav.LookupError
	if errors.As(err, &lookupErr) {
		return lookupErr.Kind
	}
	return "unknown"
}

func (p *FC2CMADB) CookieConfigured() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return strings.TrimSpace(p.config.Cookie) != ""
}

func (p *FC2CMADB) Lookup(ctx context.Context, code string) (*jav.Metadata, error) {
	articleID, ok := normalizeFC2ArticleID(code)
	if !ok {
		return nil, jav.NewLookupError("parse", 0, errors.New("invalid FC2 code"))
	}
	requestURL := p.baseURL.ResolveReference(&url.URL{Path: "/articles/" + articleID})
	p.debug("fc2 lookup start", "code", code, "articleId", articleID)
	body, response, err := p.get(ctx, requestURL, true)
	if err != nil {
		p.debug("fc2 lookup transport failure", "code", code, "errorKind", lookupErrorKind(err))
		return nil, err
	}
	if response.StatusCode == http.StatusNotFound {
		p.debug("fc2 lookup response", "code", code, "status", response.StatusCode, "bodyBytes", len(body), "classification", "not_found")
		return nil, jav.NewLookupError("not_found", response.StatusCode, nil)
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		p.debug("fc2 lookup response", "code", code, "status", response.StatusCode, "bodyBytes", len(body), "classification", "provider_auth")
		return nil, jav.NewLookupError("provider-auth", response.StatusCode, errors.New("FC2CMADB session rejected"))
	}
	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
		p.debug("fc2 lookup response", "code", code, "status", response.StatusCode, "bodyBytes", len(body), "classification", "retryable")
		return nil, jav.NewLookupError("retryable", response.StatusCode, nil)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		p.debug("fc2 lookup response", "code", code, "status", response.StatusCode, "bodyBytes", len(body), "classification", "http")
		return nil, jav.NewLookupError("http", response.StatusCode, nil)
	}
	classification := fc2PageClassification(response, body)
	if classification != "ok" {
		p.debug("fc2 lookup response", "code", code, "status", response.StatusCode, "bodyBytes", len(body), "classification", classification)
		return nil, jav.NewLookupError("provider-auth", response.StatusCode, errors.New("FC2CMADB age verification or challenge required"))
	}

	var metadata *jav.Metadata
	inertia, inertiaErr := parseFC2InertiaDocument(body, requestURL.String(), articleID)
	if inertiaErr == nil && inertia != nil {
		if inertia.notFound {
			p.debug("fc2 lookup response", "code", code, "status", response.StatusCode, "bodyBytes", len(body), "classification", "not_found")
			return nil, jav.NewLookupError("not_found", response.StatusCode, nil)
		}
		metadata = inertia.metadata

		if metadata != nil && strings.TrimSpace(metadata.Title) != "" && hasFC2String(inertia.deferred, "actresses") {
			partialBody, partialResponse, partialErr := p.getInertiaPartial(ctx, requestURL, inertia.version, "Articles/Show", "actresses")
			if partialErr == nil && partialResponse != nil && partialResponse.StatusCode >= 200 && partialResponse.StatusCode < 300 {
				if actors, ok := parseFC2InertiaActors(partialBody); ok {
					metadata.Actors = uniqueFC2(append(metadata.Actors, actors...))
				}
			} else {
				p.debug("fc2 deferred props unavailable", "code", code, "status", response.StatusCode, "partialStatus", fc2ResponseStatus(partialResponse), "errorKind", lookupErrorKind(partialErr))
			}
		}
	} else if inertiaErr != nil {
		p.debug("fc2 inertia parse fallback", "code", code, "error", inertiaErr)
	}

	legacy := parseFC2Metadata(body, requestURL.String(), articleID)
	metadata = mergeFC2Metadata(metadata, legacy)
	if metadata == nil || strings.TrimSpace(metadata.Title) == "" {
		p.debug("fc2 lookup response", "code", code, "status", response.StatusCode, "bodyBytes", len(body), "classification", "parse_failure")
		return nil, jav.NewLookupError("parse", response.StatusCode, errors.New("FC2CMADB article metadata not found"))
	}
	p.debug("fc2 lookup response", "code", code, "status", response.StatusCode, "bodyBytes", len(body), "classification", "ok")
	return metadata, nil

}

func (p *FC2CMADB) Fetch(ctx context.Context, rawURL string) (*http.Response, error) {
	return p.fetchURL(ctx, rawURL, false)
}

func (p *FC2CMADB) FetchFor(ctx context.Context, providerName, rawURL string) (*http.Response, error) {
	if strings.EqualFold(strings.TrimSpace(providerName), p.Name()) {
		return p.Fetch(ctx, rawURL)
	}
	return nil, fmt.Errorf("provider %q cannot fetch FC2 cover", providerName)
}

func (p *FC2CMADB) get(ctx context.Context, target *url.URL, document bool) ([]byte, *http.Response, error) {
	response, err := p.do(ctx, target, document)
	if err != nil {
		return nil, nil, jav.NewLookupError("network", 0, err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, fc2CMADBMaxResponse+1))
	if err != nil {
		return nil, response, jav.NewLookupError("network", response.StatusCode, err)
	}
	if len(body) > fc2CMADBMaxResponse {
		return nil, response, jav.NewLookupError("parse", response.StatusCode, errors.New("FC2CMADB response too large"))
	}
	return body, response, nil
}

type fc2InertiaEnvelope struct {
	Component string `json:"component"`
	Version   string `json:"version"`
	Props     struct {
		Article   json.RawMessage `json:"article"`
		Actresses json.RawMessage `json:"actresses"`
	} `json:"props"`
	DeferredProps struct {
		Default []string `json:"default"`
	} `json:"deferredProps"`
}

type fc2InertiaArticle struct {
	Title           string          `json:"title"`
	VideoID         json.RawMessage `json:"video_id"`
	Censored        json.RawMessage `json:"censored"`
	UncensoredImage json.RawMessage `json:"uncensored_image"`
	NotFound        json.RawMessage `json:"not_found"`
	ReleaseDate     string          `json:"release_date"`
	Duration        json.RawMessage `json:"duration"`
	ImageURL        string          `json:"image_url"`
	Tags            []any           `json:"tags"`
	Writer          json.RawMessage `json:"writer"`
}

type fc2InertiaDocument struct {
	metadata *jav.Metadata
	version  string
	deferred []string
	notFound bool
}

func parseFC2InertiaDocument(body []byte, pageURL, articleID string) (*fc2InertiaDocument, error) {
	root, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	var raw []byte
	forEachFC2Element(root, func(node *html.Node) {
		if len(raw) > 0 || node.Data != "script" || attributeFC2(node, "data-page") != "app" {
			return
		}
		if strings.EqualFold(attributeFC2(node, "type"), "application/json") {
			raw = []byte(stdhtml.UnescapeString(strings.TrimSpace(textContentFC2(node))))
		}
	})
	if len(raw) == 0 {
		return nil, errors.New("FC2CMADB Inertia data-page script not found")
	}
	var envelope fc2InertiaEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	if envelope.Component != "Articles/Show" || len(envelope.Props.Article) == 0 || string(envelope.Props.Article) == "null" {
		return nil, errors.New("FC2CMADB Inertia article props not found")
	}
	var article fc2InertiaArticle
	if err := json.Unmarshal(envelope.Props.Article, &article); err != nil {
		return nil, err
	}
	metadata := &jav.Metadata{Code: "FC2-" + articleID, Provider: "fc2cmadb"}
	metadata.Title = cleanFC2Title(article.Title, metadata.Code)
	metadata.ReleaseUnix = parseFC2Release(article.ReleaseDate)
	metadata.DurationMin = parseFC2DurationRaw(article.Duration)
	metadata.Studio = fc2RawNamedString(article.Writer)
	metadata.CoverURL = firstFC2Cover(pageURL,
		fc2RawString(article.UncensoredImage),
		article.ImageURL,
	)
	metadata.Tags = uniqueFC2(fc2AnyStrings(article.Tags))
	if censored, ok := parseFC2Censored(article.Censored); ok {
		metadata.IsUncensored = &censored
	}
	notFound, _ := parseFC2Bool(article.NotFound)
	return &fc2InertiaDocument{metadata: metadata, version: envelope.Version, deferred: envelope.DeferredProps.Default, notFound: notFound}, nil
}

func parseFC2InertiaActors(body []byte) ([]string, bool) {
	var envelope struct {
		Props struct {
			Actresses json.RawMessage `json:"actresses"`
		} `json:"props"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || len(envelope.Props.Actresses) == 0 || string(envelope.Props.Actresses) == "null" {
		return nil, false
	}
	return uniqueFC2(fc2RawStringList(envelope.Props.Actresses)), true
}

func (p *FC2CMADB) getInertiaPartial(ctx context.Context, target *url.URL, version, component, data string) ([]byte, *http.Response, error) {
	response, err := p.doRequest(ctx, target, false, &fc2InertiaRequest{version: version, component: component, data: data})
	if err != nil {
		return nil, nil, jav.NewLookupError("network", 0, err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, fc2CMADBMaxResponse+1))
	if err != nil {
		return nil, response, jav.NewLookupError("network", response.StatusCode, err)
	}
	if len(body) > fc2CMADBMaxResponse {
		return nil, response, jav.NewLookupError("parse", response.StatusCode, errors.New("FC2CMADB response too large"))
	}
	return body, response, nil
}

func mergeFC2Metadata(primary, fallback *jav.Metadata) *jav.Metadata {
	if primary == nil {
		return fallback
	}
	if fallback == nil {
		return primary
	}
	primary.Title = firstNonEmptyFC2(primary.Title, fallback.Title)
	primary.Studio = firstNonEmptyFC2(primary.Studio, fallback.Studio)
	primary.Series = firstNonEmptyFC2(primary.Series, fallback.Series)
	primary.ReleaseUnix = firstNonNilFC2(primary.ReleaseUnix, fallback.ReleaseUnix)
	primary.DurationMin = firstNonNilDurationFC2(primary.DurationMin, fallback.DurationMin)
	if primary.IsUncensored == nil {
		primary.IsUncensored = fallback.IsUncensored
	}
	primary.CoverURL = firstNonEmptyFC2(primary.CoverURL, fallback.CoverURL)
	primary.Tags = uniqueFC2(append(primary.Tags, fallback.Tags...))
	primary.Actors = uniqueFC2(append(primary.Actors, fallback.Actors...))
	return primary
}

func fc2ResponseStatus(response *http.Response) int {
	if response == nil {
		return 0
	}
	return response.StatusCode
}

func hasFC2String(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}

func fc2RawNamedString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return cleanFC2Text(value)
	}
	var object map[string]any
	if json.Unmarshal(raw, &object) == nil {
		for _, key := range []string{"name", "title", "label"} {
			if value, ok := object[key].(string); ok {
				return cleanFC2Text(value)
			}
		}
	}
	return ""
}

func fc2RawString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	var object map[string]any
	if json.Unmarshal(raw, &object) == nil {
		for _, key := range []string{"url", "path", "src", "image"} {
			if value, ok := object[key].(string); ok {
				return value
			}
		}
	}
	return ""
}

func fc2RawStringList(raw json.RawMessage) []string {
	var values []any
	if json.Unmarshal(raw, &values) == nil {
		return fc2AnyStrings(values)
	}
	var wrapped map[string]any
	if json.Unmarshal(raw, &wrapped) == nil {
		for _, key := range []string{"data", "items", "actresses", "actors"} {
			if values, ok := wrapped[key].([]any); ok {
				return fc2AnyStrings(values)
			}
		}
	}
	return nil
}

func parseFC2DurationRaw(raw json.RawMessage) *int {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return parseFC2DurationValue(value)
}

func parseFC2Censored(raw json.RawMessage) (bool, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return false, false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false, false
	}
	if boolean, ok := parseFC2Bool(value); ok {
		return !boolean, true
	}
	text, ok := value.(string)
	if !ok {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "無", "なし", "none", "no", "false", "0":
		return true, true
	case "有", "あり", "yes", "true", "1":
		return false, true
	default:
		return false, false
	}
}

func parseFC2Bool(value any) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case float64:
		return v != 0, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "yes", "1":
			return true, true
		case "false", "no", "0":
			return false, true
		}
	}
	return false, false
}

func (p *FC2CMADB) fetchURL(ctx context.Context, rawURL string, document bool) (*http.Response, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || !p.allowedCoverURL(parsed) {
		return nil, errors.New("FC2CMADB cover URL is not allowed")
	}
	return p.do(ctx, parsed, document)
}

type fc2InertiaRequest struct {
	version   string
	component string
	data      string
}

func (p *FC2CMADB) do(ctx context.Context, target *url.URL, document bool) (*http.Response, error) {
	return p.doRequest(ctx, target, document, nil)
}

func (p *FC2CMADB) doRequest(ctx context.Context, target *url.URL, document bool, inertia *fc2InertiaRequest) (*http.Response, error) {
	if document && !p.allowedURL(target) {
		return nil, errors.New("FC2CMADB request URL is not allowed")
	}
	if !document && !p.allowedCoverURL(target) {
		return nil, errors.New("FC2CMADB cover URL is not allowed")
	}
	if err := p.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", p.config.UserAgent)
	req.Header.Set("Accept", fc2CMADBAccept)
	req.Header.Set("Accept-Language", fc2CMADBAcceptLanguage)
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("DNT", "1")
	req.Header.Set("Priority", fc2CMADBPriority)
	req.Header.Set("Sec-CH-UA", fc2CMADBSecCHUA)
	req.Header.Set("Sec-CH-UA-Mobile", "?0")
	req.Header.Set("Sec-CH-UA-Platform", fc2CMADBSecCHPlatform)
	req.Header.Set("Referer", p.baseURL.String()+"/")
	if document {
		req.Header.Set("Sec-Fetch-Dest", "document")
		req.Header.Set("Sec-Fetch-Mode", "navigate")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Sec-Fetch-User", "?1")
		req.Header.Set("Upgrade-Insecure-Requests", "1")
	}
	if inertia != nil {
		req.Header.Set("X-Inertia", "true")
		req.Header.Set("X-Inertia-Version", inertia.version)
		req.Header.Set("X-Inertia-Partial-Component", inertia.component)
		req.Header.Set("X-Inertia-Partial-Data", inertia.data)
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Accept", "text/html, application/xhtml+xml")
		req.Header.Set("Referer", target.String())
	}
	p.mu.RLock()
	cookie := strings.TrimSpace(p.config.Cookie)
	p.mu.RUnlock()
	hasAgeVerified, hasSession := cookieHasKey(cookie, "ageVerified"), cookieHasSessionKey(cookie)
	cookieSent := cookie != "" && p.sameSite(target)
	if cookieSent {
		req.Header.Set("Cookie", cookie)
	}
	p.debug("fc2 request", "host", target.Hostname(), "path", target.EscapedPath(), "document", document, "cookieConfigured", cookie != "", "cookieSent", cookieSent, "hasAgeVerified", hasAgeVerified, "hasSession", hasSession)
	client := *p.client
	client.CheckRedirect = func(next *http.Request, prior []*http.Request) error {
		sameSite := p.sameSite(next.URL)
		cookieStripped := !sameSite && next.Header.Get("Cookie") != ""
		allowed := p.allowedCoverURL(next.URL)
		if document || inertia != nil {
			allowed = p.allowedURL(next.URL)
		}
		if !allowed {

			p.debug("fc2 redirect rejected", "host", next.URL.Hostname(), "path", next.URL.EscapedPath(), "sameSite", sameSite)
			return http.ErrUseLastResponse
		}
		if !sameSite {
			next.Header.Del("Cookie")
		}
		p.debug("fc2 redirect", "host", next.URL.Hostname(), "path", next.URL.EscapedPath(), "sameSite", sameSite, "cookieStripped", cookieStripped)
		return nil
	}
	response, err := client.Do(req)
	if response != nil && response.Request != nil && response.Request.URL != nil {
		p.debug("fc2 response", "status", response.StatusCode, "finalHost", response.Request.URL.Hostname(), "finalPath", response.Request.URL.EscapedPath(), "redirected", response.Request.URL.String() != target.String())
	}
	return response, err
}

func (p *FC2CMADB) sameSite(target *url.URL) bool {
	return target != nil && strings.EqualFold(target.Hostname(), p.baseURL.Hostname())
}

func (p *FC2CMADB) allowedURL(target *url.URL) bool {
	return target != nil && (target.Scheme == "https" || target.Scheme == "http") && p.sameSite(target)
}

func (p *FC2CMADB) allowedCoverURL(target *url.URL) bool {
	if target == nil || (target.Scheme != "https" && target.Scheme != "http") {
		return false
	}
	if p.sameSite(target) {
		return true
	}
	host := strings.ToLower(strings.TrimSuffix(target.Hostname(), "."))
	return strings.HasSuffix(host, ".fc2.com") || strings.HasSuffix(host, ".fc2usercontent.com")
}

func normalizeFC2ArticleID(code string) (string, bool) {
	match := fc2CodeInputPattern.FindStringSubmatch(strings.TrimSpace(code))
	if len(match) != 2 {
		return "", false
	}
	id := strings.TrimLeft(match[1], "0")
	if id == "" {
		id = "0"
	}
	return id, true
}

func fc2PageClassification(response *http.Response, body []byte) string {
	if response == nil {
		return "parse_failure"
	}
	if response.StatusCode == http.StatusNotFound {
		return "not_found"
	}
	visible := strings.ToLower(fc2VisibleText(body))
	if fc2BlockedPage(response, body) {
		if strings.Contains(visible, "18歳以上です") || strings.Contains(visible, "年齢確認") || strings.Contains(visible, "age verification") || strings.Contains(visible, "age gate") {
			return "age_gate"
		}
		return "cloudflare_challenge"
	}
	return "ok"
}

func fc2BlockedPage(response *http.Response, body []byte) bool {
	visible := strings.ToLower(fc2VisibleText(body))
	if response != nil && response.Request != nil && response.Request.URL != nil {
		path := strings.ToLower(response.Request.URL.Path)
		if strings.Contains(path, "/cdn-cgi/challenge") {
			return true
		}
	}
	for _, marker := range []string{"18歳以上です", "年齢確認", "age verification", "age gate", "just a moment...", "checking your browser"} {
		if strings.Contains(visible, marker) {
			return true
		}
	}
	return strings.Contains(visible, "cloudflare") && strings.Contains(visible, "challenge")
}

func fc2VisibleText(body []byte) string {
	root, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return ""
	}
	var builder strings.Builder
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node == nil || fc2HiddenNode(node) {
			return
		}
		if node.Type == html.ElementNode {
			switch node.Data {
			case "script", "style", "noscript", "template":
				return
			}
		}
		if node.Type == html.TextNode {
			builder.WriteByte(' ')
			builder.WriteString(node.Data)
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	return cleanFC2Text(builder.String())
}

func fc2HiddenNode(node *html.Node) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}
	if attributeFC2(node, "hidden") != "" || strings.EqualFold(attributeFC2(node, "aria-hidden"), "true") {
		return true
	}
	style := strings.ToLower(strings.ReplaceAll(attributeFC2(node, "style"), " ", ""))
	if strings.Contains(style, "display:none") || strings.Contains(style, "visibility:hidden") {
		return true
	}
	class := strings.ToLower(attributeFC2(node, "class"))
	for _, token := range strings.Fields(class) {
		if token == "hidden" || token == "invisible" {
			return true
		}
	}
	return false
}

type fc2ArticleJSON struct {
	Title       string `json:"title"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	Description string `json:"description"`
	Cover       string `json:"cover_url"`
	Image       string `json:"image"`
	ReleaseDate string `json:"release_date"`
	Duration    any    `json:"duration"`
	Studio      string `json:"studio"`
	Series      string `json:"series"`
	Tags        []any  `json:"tags"`
	Actors      []any  `json:"actors"`
}

func parseFC2Metadata(body []byte, pageURL, articleID string) *jav.Metadata {
	root, _ := html.Parse(bytes.NewReader(body))
	metadata := &jav.Metadata{Code: "FC2-" + articleID, Provider: "fc2cmadb"}
	if root != nil {
		meta := parseFC2MetaTags(root)
		metadata.Title = firstNonEmptyFC2(meta["og:title"], meta["twitter:title"], fc2Text(root, "h1"), fc2Text(root, "h2"), fc2Text(root, "title"))
		metadata.CoverURL = firstFC2Cover(pageURL, meta["og:image"], meta["twitter:image"], fc2Attr(root, "img", "src"))
		metadata.ReleaseUnix = parseFC2Release(meta["article:published_time"], fc2TextByLabel(root, []string{"発売日", "发行日期", "发布日期", "release"}))
		metadata.DurationMin = parseFC2Duration(meta["video:duration"], fc2TextByLabel(root, []string{"时间", "時長", "长度", "duration"}))
		metadata.Studio = fc2TextByLabel(root, []string{"片商", "メーカー", "studio"})
		metadata.Series = fc2TextByLabel(root, []string{"系列", "シリーズ", "series"})
		metadata.Tags = fc2LinksByHint(root, []string{"tag", "genre", "标签", "ジャンル"})
		metadata.Actors = fc2LinksByHint(root, []string{"actor", "star", "女优", "女優", "出演"})
		parseFC2JSONScripts(root, metadata, pageURL)
	}
	metadata.Title = cleanFC2Title(metadata.Title, metadata.Code)
	metadata.Tags = uniqueFC2(metadata.Tags)
	metadata.Actors = uniqueFC2(metadata.Actors)
	return metadata
}

func parseFC2JSONScripts(root *html.Node, metadata *jav.Metadata, pageURL string) {
	forEachFC2Element(root, func(node *html.Node) {
		if node.Data != "script" {
			return
		}
		typeAttr := strings.ToLower(attributeFC2(node, "type"))
		text := strings.TrimSpace(textContentFC2(node))
		if text == "" || (typeAttr != "application/ld+json" && !strings.Contains(text, "article")) {
			return
		}
		var raw map[string]any
		if json.Unmarshal([]byte(stdhtml.UnescapeString(text)), &raw) != nil {
			return
		}
		candidate := fc2ArticleJSON{}
		encoded, _ := json.Marshal(raw)
		if json.Unmarshal(encoded, &candidate) != nil {
			return
		}
		metadata.Title = firstNonEmptyFC2(metadata.Title, candidate.Title, candidate.Name)
		metadata.CoverURL = firstNonEmptyFC2(metadata.CoverURL, firstFC2Cover(pageURL, candidate.Cover, candidate.Image))

		metadata.Studio = firstNonEmptyFC2(metadata.Studio, candidate.Studio)
		metadata.Series = firstNonEmptyFC2(metadata.Series, candidate.Series)
		metadata.ReleaseUnix = firstNonNilFC2(metadata.ReleaseUnix, parseFC2Release(candidate.ReleaseDate))
		metadata.DurationMin = firstNonNilDurationFC2(metadata.DurationMin, parseFC2DurationValue(candidate.Duration))
		metadata.Tags = append(metadata.Tags, fc2AnyStrings(candidate.Tags)...)
		metadata.Actors = append(metadata.Actors, fc2AnyStrings(candidate.Actors)...)
	})
}

func parseFC2MetaTags(root *html.Node) map[string]string {
	out := make(map[string]string)
	forEachFC2Element(root, func(node *html.Node) {
		if node.Data != "meta" {
			return
		}
		key := strings.ToLower(firstNonEmptyFC2(attributeFC2(node, "property"), attributeFC2(node, "name"), attributeFC2(node, "itemprop")))
		value := strings.TrimSpace(stdhtml.UnescapeString(attributeFC2(node, "content")))
		if key != "" && value != "" {
			out[key] = value
		}
	})
	return out
}

func fc2Text(root *html.Node, tag string) string {
	var result string
	forEachFC2Element(root, func(node *html.Node) {
		if result == "" && node.Data == tag {
			result = cleanFC2Text(textContentFC2(node))
		}
	})
	return result
}

func fc2TextByLabel(root *html.Node, labels []string) string {
	var result string
	forEachFC2Element(root, func(node *html.Node) {
		if result != "" || node.Type != html.ElementNode {
			return
		}
		text := cleanFC2Text(textContentFC2(node))
		lower := strings.ToLower(text)
		for _, label := range labels {
			if strings.HasPrefix(lower, strings.ToLower(label)) {
				result = strings.TrimSpace(strings.TrimLeft(text[len(label):], ":： "))
				return
			}
		}
	})
	return result
}

func fc2Attr(root *html.Node, tag, attr string) string {
	var result string
	forEachFC2Element(root, func(node *html.Node) {
		if result == "" && node.Data == tag {
			result = attributeFC2(node, attr)
		}
	})
	return result
}

func fc2LinksByHint(root *html.Node, hints []string) []string {
	values := make([]string, 0)
	forEachFC2Element(root, func(node *html.Node) {
		if node.Data != "a" {
			return
		}
		haystack := strings.ToLower(attributeFC2(node, "href") + " " + attributeFC2(node, "class"))
		for _, hint := range hints {
			if strings.Contains(haystack, strings.ToLower(hint)) {
				if value := cleanFC2Text(textContentFC2(node)); value != "" {
					values = append(values, value)
				}
				return
			}
		}
	})
	return values
}

func parseFC2Release(values ...string) *int64 {
	for _, value := range values {
		value = strings.TrimSpace(value)
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02", "2006/01/02"} {
			if parsed, err := time.Parse(layout, value); err == nil {
				parsed = parsed.UTC()
				unix := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC).Unix()
				return &unix
			}
		}
		if match := fc2DatePattern.FindStringSubmatch(value); len(match) == 4 {
			month, _ := strconv.Atoi(match[2])
			day, _ := strconv.Atoi(match[3])
			parsed := time.Date(mustAtoiFC2(match[1]), time.Month(month), day, 0, 0, 0, 0, time.UTC)
			if parsed.Year() == mustAtoiFC2(match[1]) && parsed.Month() == time.Month(month) && parsed.Day() == day {
				unix := parsed.Unix()
				return &unix
			}
		}
	}
	return nil
}

func parseFC2Duration(values ...string) *int {
	for _, value := range values {
		if minutes := parseFC2DurationText(value); minutes != nil {
			return minutes
		}
	}
	return nil
}

func parseFC2DurationText(value string) *int {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if parts := strings.Split(value, ":"); len(parts) == 2 || len(parts) == 3 {
		var numbers [3]int
		for i, part := range parts {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || n < 0 || (i > 0 && n >= 60) {
				return nil
			}
			numbers[i] = n
		}
		if len(parts) == 3 {
			minutes := numbers[0]*60 + numbers[1]
			return &minutes
		}
		minutes := numbers[0]
		return &minutes
	}
	if match := fc2DurationPattern.FindStringSubmatch(value); len(match) == 2 {
		minutes, _ := strconv.Atoi(match[1])
		return &minutes
	}
	if match := fc2SecondsPattern.FindStringSubmatch(value); len(match) == 2 {
		seconds, _ := strconv.Atoi(match[1])
		minutes := seconds / 60
		return &minutes
	}
	if number, err := strconv.Atoi(value); err == nil && number >= 0 {
		if number <= 300 {
			return &number
		}
		minutes := number / 60
		return &minutes
	}
	return nil
}

func parseFC2DurationValue(value any) *int {
	switch v := value.(type) {
	case float64:
		if v < 0 || v > 86400 {
			return nil
		}
		return parseFC2DurationText(strconv.Itoa(int(v)))
	case string:
		return parseFC2DurationText(v)
	case map[string]any:
		for _, key := range []string{"seconds", "duration_seconds", "durationSeconds"} {
			if seconds, ok := v[key].(float64); ok && seconds >= 0 && seconds <= 86400 {
				minutes := int(seconds) / 60
				return &minutes
			}
		}
		for _, key := range []string{"minutes", "duration_minutes", "durationMinutes"} {
			if minutes, ok := v[key].(float64); ok && minutes >= 0 && minutes <= 1440 {
				n := int(minutes)
				return &n
			}
		}
	}
	return nil
}

func firstFC2Cover(pageURL string, values ...string) string {
	for _, value := range values {
		resolved := resolveFC2URL(pageURL, value)
		if resolved == "" {
			continue
		}
		path := strings.ToLower(resolved)
		if strings.HasSuffix(path, "/storage/images/article/no-image.jpg") || strings.HasSuffix(path, "/storage/images/article/no-image.png") {
			continue
		}
		return resolved
	}
	return ""
}

func resolveFC2URL(pageURL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	base, err := url.Parse(pageURL)
	if err != nil {
		return ""
	}
	return base.ResolveReference(parsed).String()
}

func cleanFC2Title(value, code string) string {
	value = cleanFC2Text(value)
	value = strings.TrimSuffix(value, " - FC2CMADB")
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, code) {
		return ""
	}
	for _, prefix := range []string{code, "FC2-PPV-" + strings.TrimPrefix(code, "FC2-"), "FC2PPV" + strings.TrimPrefix(code, "FC2-")} {
		if len(value) > len(prefix) && strings.EqualFold(value[:len(prefix)], prefix) {
			value = strings.TrimSpace(strings.TrimLeft(value[len(prefix):], "-:： "))
			break
		}
	}
	return value
}

func cleanFC2Text(value string) string {
	return strings.Join(strings.Fields(stdhtml.UnescapeString(value)), " ")
}

func firstNonEmptyFC2(values ...string) string {
	for _, value := range values {
		if value = cleanFC2Text(value); value != "" {
			return value
		}
	}
	return ""
}

func uniqueFC2(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = cleanFC2Text(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func fc2AnyStrings(values []any) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		switch v := value.(type) {
		case string:
			out = append(out, v)
		case map[string]any:
			for _, key := range []string{"name", "title", "label"} {
				if text, ok := v[key].(string); ok {
					out = append(out, text)
					break
				}
			}
		}
	}
	return out
}

func firstNonNilFC2(values ...*int64) *int64 {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstNonNilDurationFC2(values ...*int) *int {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func mustAtoiFC2(value string) int {
	n, _ := strconv.Atoi(value)
	return n
}

func attributeFC2(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func textContentFC2(node *html.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == html.TextNode {
		return node.Data
	}
	var builder strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		builder.WriteString(textContentFC2(child))
	}
	return builder.String()
}

func forEachFC2Element(root *html.Node, visit func(*html.Node)) {
	if root == nil {
		return
	}
	if root.Type == html.ElementNode {
		visit(root)
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		forEachFC2Element(child, visit)
	}
}
