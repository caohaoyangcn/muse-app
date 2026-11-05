package providers

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"muse-app/backend/jav"
)

const (
	defaultAvmooBaseURL   = "https://avmoo.shop"
	defaultAvmooLanguage  = "cn"
	avmooRequestInterval  = 1500 * time.Millisecond
	avmooSessionTTL       = 30 * time.Minute
	avmooLookupTimeout    = 90 * time.Second
	avmooHTTPTimeout      = 30 * time.Second
	avmooAPIRetryAttempts = 3
	avmooAPIRetryDelay    = 2 * time.Second
	avmooSearchLimit      = 30
	avmooMaxResponseBody  = 8 << 20
	// avmooActorSearchLimit and avmooActorMovieProbes bound the backfill work for
	// one actor: a name search can return dozens of works, and only a handful
	// need to be inspected before the performer shows up in a cast list.
	avmooActorSearchLimit = 20
	avmooActorMovieProbes = 4
	avmooUserAgent        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	avmooHTMLAccept       = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	avmooAPIAccept        = "application/json, text/plain, */*"
	avmooImageAccept      = "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8"
	avmooAcceptLanguage   = "zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7,zh-TW;q=0.6"
)

var avmooCSRFTokenPattern = regexp.MustCompile(`<meta\s+name=["']csrf-token["']\s+content=["']([^"']+)["']`)

// AvmooConfig controls testable/request-level Avmoo settings. Production uses
// the fixed Avmoo address and the browser-like defaults below.
type AvmooConfig struct {
	UserAgent      string
	AcceptLanguage string
	Timeout        time.Duration
	// Proxy overrides the transport's proxy resolver. It is only consulted when
	// no client is injected; nil keeps http.ProxyFromEnvironment.
	Proxy func(*http.Request) (*url.URL, error)
}

// Avmoo is an API-backed Avmoo metadata provider. Session state is deliberately
// kept on the provider instance so test servers and production sessions cannot
// contaminate one another.
type Avmoo struct {
	client  *http.Client
	baseURL string
	config  AvmooConfig
	limiter requestLimiter

	mu        sync.Mutex
	session   avmooSession
	expiresAt time.Time
}

type avmooSession struct {
	csrfToken string
	cookie    string
	referer   string
}

type avmooAPIEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type avmooAPIStatusError struct {
	source  string
	status  int
	message string
}

func (e *avmooAPIStatusError) Error() string {
	if strings.TrimSpace(e.message) != "" {
		return fmt.Sprintf("Avmoo %s status %d: %s", e.source, e.status, e.message)
	}
	return fmt.Sprintf("Avmoo %s status %d", e.source, e.status)
}

type avmooMovie struct {
	MovieID     string      `json:"movieId"`
	MovieFanHao string      `json:"movieFanHao"`
	Title       string      `json:"title"`
	TitleJA     string      `json:"title_ja"`
	TitleEN     string      `json:"title_en"`
	TitleCN     string      `json:"title_cn"`
	TitleTW     string      `json:"title_tw"`
	ReleaseDate string      `json:"releaseDate"`
	Length      int         `json:"length"`
	PosterSmall string      `json:"posterSmall"`
	PosterLarge string      `json:"posterLarge"`
	Studio      *avmooName  `json:"studio"`
	Series      *avmooName  `json:"series"`
	Genre       []avmooName `json:"genre"`
	Star        []avmooStar `json:"star"`
}

// avmooStar is one performer as a movie payload embeds them. Avmoo sends the
// full profile (not just the name) inside the movie object, so the profile comes
// for free with every lookup; getStar returns the same shape for one performer.
type avmooStar struct {
	StarID    string `json:"starId"`
	StarDmmID int64  `json:"starDmmId"`

	StarName   string `json:"starName"`
	StarNameJA string `json:"starName_ja"`
	StarNameEN string `json:"starName_en"`
	StarNameCN string `json:"starName_cn"`
	StarNameTW string `json:"starName_tw"`
	Name       string `json:"name"`
	NameJA     string `json:"name_ja"`
	NameEN     string `json:"name_en"`
	NameCN     string `json:"name_cn"`
	NameTW     string `json:"name_tw"`

	Avatar      string        `json:"avatar"`
	AvatarURL   string        `json:"avatarUrl"`
	Birthday    string        `json:"birthday"`
	Size        avmooStarSize `json:"size"`
	BloodType   string        `json:"bloodType"`
	Hometown    string        `json:"hometown"`
	Hobby       string        `json:"hobby"`
	MovieCount  int           `json:"movieCount"`
	LastRelease string        `json:"lastReleaseDate"`
}

// avmooStarSize carries the measurements as strings. T is height in cm; B, W and
// H are bust, waist and hip; C is the cup size letter.
type avmooStarSize struct {
	Height string `json:"T"`
	Bust   string `json:"B"`
	Cup    string `json:"C"`
	Waist  string `json:"W"`
	Hip    string `json:"H"`
}

type avmooName struct {
	Name   string `json:"name"`
	NameJA string `json:"name_ja"`
	NameEN string `json:"name_en"`
	NameCN string `json:"name_cn"`
	NameTW string `json:"name_tw"`

	// Avmoo uses role-specific base keys in production. These aliases keep the
	// decoder compatible with both the live API and small fixture responses.
	StudioName   string `json:"studioName"`
	StudioNameJA string `json:"studioName_ja"`
	StudioNameEN string `json:"studioName_en"`
	StudioNameCN string `json:"studioName_cn"`
	StudioNameTW string `json:"studioName_tw"`
	SeriesName   string `json:"seriesName"`
	SeriesNameJA string `json:"seriesName_ja"`
	SeriesNameEN string `json:"seriesName_en"`
	SeriesNameCN string `json:"seriesName_cn"`
	SeriesNameTW string `json:"seriesName_tw"`
	GenreName    string `json:"genreName"`
	GenreNameJA  string `json:"genreName_ja"`
	GenreNameEN  string `json:"genreName_en"`
	GenreNameCN  string `json:"genreName_cn"`
	GenreNameTW  string `json:"genreName_tw"`
	StarName     string `json:"starName"`
	StarNameJA   string `json:"starName_ja"`
	StarNameEN   string `json:"starName_en"`
	StarNameCN   string `json:"starName_cn"`
	StarNameTW   string `json:"starName_tw"`
}

func NewAvmoo(client *http.Client, baseURL string) *Avmoo {
	return NewAvmooWithConfig(client, baseURL, AvmooConfig{})
}

func NewAvmooWithConfig(client *http.Client, baseURL string, config AvmooConfig) *Avmoo {
	return newAvmooWithLimiter(client, baseURL, config, newIntervalLimiter(avmooRequestInterval))
}

func newAvmooWithLimiter(client *http.Client, baseURL string, config AvmooConfig, limiter requestLimiter) *Avmoo {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultAvmooBaseURL
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if config.UserAgent == "" {
		config.UserAgent = avmooUserAgent
	}
	if config.AcceptLanguage == "" {
		config.AcceptLanguage = avmooAcceptLanguage
	}
	if config.Timeout <= 0 {
		config.Timeout = avmooHTTPTimeout
	}
	if limiter == nil {
		limiter = newIntervalLimiter(avmooRequestInterval)
	}
	if config.Proxy == nil {
		config.Proxy = http.ProxyFromEnvironment
	}
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = config.Proxy
		transport.ForceAttemptHTTP2 = false
		transport.DisableCompression = true
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS13}
		transport.MaxIdleConns = 50
		transport.MaxIdleConnsPerHost = 5
		transport.MaxConnsPerHost = 5
		client = &http.Client{Transport: transport, Timeout: config.Timeout}
	} else if client.Timeout == 0 {
		copy := *client
		copy.Timeout = config.Timeout
		client = &copy
	}
	return &Avmoo{client: client, baseURL: baseURL, config: config, limiter: limiter}
}

func (p *Avmoo) Name() string { return "avmoo" }

func (p *Avmoo) Lookup(ctx context.Context, code string) (*jav.Metadata, error) {
	requested := strings.ToUpper(strings.TrimSpace(code))
	if requested == "" {
		return nil, jav.NewLookupError("parse", 0, errors.New("empty Avmoo code"))
	}
	lookupCtx, cancel := context.WithTimeout(ctx, avmooLookupTimeout)
	defer cancel()

	session, err := p.cachedSession(lookupCtx, requested)
	if err != nil {
		return nil, p.classifyError(err)
	}
	movie, err := p.lookupWithSession(lookupCtx, session, requested)
	if isAvmooSessionAuthError(err) {
		p.invalidateSession(session)
		session, refreshErr := p.refreshSession(lookupCtx, requested)
		if refreshErr != nil {
			return nil, p.classifyError(refreshErr)
		}
		movie, err = p.lookupWithSession(lookupCtx, session, requested)
	}
	if err != nil {
		return nil, p.classifyError(err)
	}
	metadata := avmooMetadata(movie)
	if metadata == nil || (strings.TrimSpace(metadata.Code) == "" && strings.TrimSpace(metadata.Title) == "" && strings.TrimSpace(metadata.Series) == "" && metadata.ReleaseUnix == nil && metadata.DurationMin == nil && len(metadata.Actors) == 0 && len(metadata.Tags) == 0) {
		return nil, jav.NewLookupError("not_found", http.StatusNotFound, nil)
	}
	if metadata.Code == "" {
		metadata.Code = requested
	}
	return metadata, nil
}

func (p *Avmoo) lookupWithSession(ctx context.Context, session avmooSession, code string) (*avmooMovie, error) {
	searchPayload := []any{map[string]string{"search": code, "lang": defaultAvmooLanguage}, avmooSearchLimit, 1}
	var results []avmooMovie
	if err := p.postAPI(ctx, session, "/jav/data/api/search", searchPayload, &results); err != nil {
		return nil, err
	}
	var match *avmooMovie
	want := normalizeAvmooCode(code)
	for i := range results {
		if normalizeAvmooCode(results[i].MovieFanHao) == want && strings.TrimSpace(results[i].MovieID) != "" {
			match = &results[i]
			break
		}
	}
	if match == nil {
		return nil, errAvmooNotFound
	}
	movie, err := p.getMovieWithSession(ctx, session, match.MovieID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(movie.MovieFanHao) == "" {
		movie.MovieFanHao = match.MovieFanHao
	}
	return movie, nil
}

// getMovieWithSession fetches one work by its Avmoo id.
func (p *Avmoo) getMovieWithSession(ctx context.Context, session avmooSession, movieID string) (*avmooMovie, error) {
	payload := []any{movieID, defaultAvmooLanguage}
	var movie avmooMovie
	if err := p.postAPI(ctx, session, "/jav/data/api/getMovie", payload, &movie); err != nil {
		return nil, err
	}
	if strings.TrimSpace(movie.MovieID) == "" {
		movie.MovieID = movieID
	}
	return &movie, nil
}

// ResolveActor finds one performer's profile by name. The provider has no
// name→performer lookup, so this searches the performer's works and reads the
// profile from the first cast list that names them. A name the provider does
// not know reports not_found, which is what stops the backfill queue from
// retrying it forever.
func (p *Avmoo) ResolveActor(ctx context.Context, name string) (*jav.ActorProfile, error) {
	requested := cleanText(name)
	if requested == "" {
		return nil, jav.NewLookupError("parse", 0, errors.New("empty Avmoo actor name"))
	}
	lookupCtx, cancel := context.WithTimeout(ctx, avmooLookupTimeout)
	defer cancel()

	session, err := p.cachedSession(lookupCtx, requested)
	if err != nil {
		return nil, p.classifyError(err)
	}
	profile, err := p.resolveActorWithSession(lookupCtx, session, requested)
	if isAvmooSessionAuthError(err) {
		p.invalidateSession(session)
		session, refreshErr := p.refreshSession(lookupCtx, requested)
		if refreshErr != nil {
			return nil, p.classifyError(refreshErr)
		}
		profile, err = p.resolveActorWithSession(lookupCtx, session, requested)
	}
	if err != nil {
		return nil, p.classifyError(err)
	}
	return profile, nil
}

func (p *Avmoo) resolveActorWithSession(ctx context.Context, session avmooSession, name string) (*jav.ActorProfile, error) {
	payload := []any{map[string]string{"search": name, "lang": defaultAvmooLanguage}, avmooActorSearchLimit, 1}
	var results []avmooMovie
	if err := p.postAPI(ctx, session, "/jav/data/api/search", payload, &results); err != nil {
		return nil, err
	}
	want := normalizeActorName(name)
	for i := range results {
		if i >= avmooActorMovieProbes {
			break
		}
		if strings.TrimSpace(results[i].MovieID) == "" {
			continue
		}
		movie, err := p.getMovieWithSession(ctx, session, results[i].MovieID)
		if errors.Is(err, errAvmooNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, star := range movie.Star {
			if normalizeActorName(star.displayName()) != want && !star.matchesAlias(want) {
				continue
			}
			profile := star.profile()
			if profile == nil {
				profile = &jav.ActorProfile{Provider: "avmoo", StarID: cleanText(star.StarID)}
			}
			return keyProfileToName(profile, name), nil
		}
	}
	return nil, errAvmooNotFound
}

// keyProfileToName makes the requested spelling the profile's key, because that
// is what the cast lists and the actor index contain. The provider's own
// spelling is kept as an alias when it differs.
func keyProfileToName(profile *jav.ActorProfile, name string) *jav.ActorProfile {
	if profile.Name != "" && profile.Name != name {
		profile.Aliases = appendUnique(profile.Aliases, profile.Name)
	}
	profile.Name = name
	return profile
}

// normalizeActorName compares stage names by ignoring the separators the sites
// are inconsistent about (spaces, including the full-width one) and casing.
func normalizeActorName(name string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(name) {
		if unicode.IsSpace(r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

// matchesAlias reports whether one of the performer's other listed spellings is
// want (already normalized).
func (star avmooStar) matchesAlias(want string) bool {
	for _, alias := range star.aliases(star.displayName()) {
		if normalizeActorName(alias) == want {
			return true
		}
	}
	return false
}

var errAvmooNotFound = errors.New("Avmoo result not found")

func (p *Avmoo) cachedSession(ctx context.Context, code string) (avmooSession, error) {
	p.mu.Lock()
	if p.session.csrfToken != "" && p.session.cookie != "" && time.Now().Before(p.expiresAt) {
		session := p.session
		p.mu.Unlock()
		return session, nil
	}
	p.mu.Unlock()
	return p.refreshSession(ctx, code)
}

func (p *Avmoo) refreshSession(ctx context.Context, code string) (avmooSession, error) {
	pageURL := p.baseURL + "/" + defaultAvmooLanguage + "/search/" + url.PathEscape(code)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return avmooSession{}, err
	}
	p.setHTMLHeaders(req, p.baseURL)
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Dest", "document")
	res, err := p.do(req)
	if err != nil {
		return avmooSession{}, err
	}
	defer func() { _ = res.Body.Close() }()
	body, err := readAvmooBody(res.Body)
	if err != nil {
		return avmooSession{}, err
	}
	if res.StatusCode != http.StatusOK {
		if res.StatusCode == http.StatusNotFound {
			return avmooSession{}, errAvmooNotFound
		}
		return avmooSession{}, &avmooAPIStatusError{source: "session", status: res.StatusCode}
	}
	token := extractAvmooCSRFToken(string(body))
	cookie := cookieHeader(res.Cookies())
	if token == "" || cookie == "" {
		return avmooSession{}, errors.New("Avmoo session missing CSRF token or cookie")
	}
	session := avmooSession{csrfToken: token, cookie: cookie, referer: pageURL}
	p.mu.Lock()
	p.session = session
	p.expiresAt = time.Now().Add(avmooSessionTTL)
	p.mu.Unlock()
	return session, nil
}

func (p *Avmoo) invalidateSession(session avmooSession) {
	p.mu.Lock()
	if p.session.csrfToken == session.csrfToken && p.session.cookie == session.cookie {
		p.session = avmooSession{}
		p.expiresAt = time.Time{}
	}
	p.mu.Unlock()
}

func (p *Avmoo) postAPI(ctx context.Context, session avmooSession, endpoint string, payload any, out any) error {
	var lastErr error
	for attempt := 1; attempt <= avmooAPIRetryAttempts; attempt++ {
		err := p.postAPIOnce(ctx, session, endpoint, payload, out)
		if err == nil || errors.Is(err, errAvmooNotFound) {
			return err
		}
		lastErr = err
		if attempt == avmooAPIRetryAttempts || !shouldRetryAvmoo(err) {
			return err
		}
		timer := time.NewTimer(avmooAPIRetryDelay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func (p *Avmoo) postAPIOnce(ctx context.Context, session avmooSession, endpoint string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", p.config.UserAgent)
	req.Header.Set("Accept", avmooAPIAccept)
	req.Header.Set("Accept-Language", p.config.AcceptLanguage)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", p.baseURL)
	req.Header.Set("Referer", session.referer)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("X-CSRF-Token", session.csrfToken)
	req.Header.Set("Cookie", session.cookie)
	res, err := p.do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := readAvmooBody(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		if res.StatusCode == http.StatusNotFound {
			return errAvmooNotFound
		}
		return &avmooAPIStatusError{source: "http", status: res.StatusCode}
	}
	var envelope avmooAPIEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("Avmoo API response parse: %w", err)
	}
	if envelope.Code == http.StatusNotFound {
		return errAvmooNotFound
	}
	if envelope.Code != http.StatusOK {
		return &avmooAPIStatusError{source: "api", status: envelope.Code, message: envelope.Message}
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return errAvmooNotFound
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("Avmoo API data parse: %w", err)
	}
	return nil
}

func (p *Avmoo) do(req *http.Request) (*http.Response, error) {
	if err := p.limiter.Wait(req.Context()); err != nil {
		return nil, err
	}
	return p.client.Do(req)
}

// Fetch downloads a cover with Avmoo's transport and headers, but never sends
// the session Cookie or CSRF token to an external image host.
func (p *Avmoo) Fetch(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	referer := p.baseURL
	p.mu.Lock()
	if p.session.referer != "" {
		referer = p.session.referer
	}
	p.mu.Unlock()
	req.Header.Set("User-Agent", p.config.UserAgent)
	req.Header.Set("Accept", avmooImageAccept)
	req.Header.Set("Accept-Language", p.config.AcceptLanguage)
	req.Header.Set("Referer", referer)
	return p.do(req)
}

func (p *Avmoo) FetchFor(ctx context.Context, providerName, rawURL string) (*http.Response, error) {
	return p.Fetch(ctx, rawURL)
}

func (p *Avmoo) setHTMLHeaders(req *http.Request, referer string) {
	req.Header.Set("User-Agent", p.config.UserAgent)
	req.Header.Set("Accept", avmooHTMLAccept)
	req.Header.Set("Accept-Language", p.config.AcceptLanguage)
	req.Header.Set("Referer", referer)
}

func readAvmooBody(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, avmooMaxResponseBody+1))
	if err != nil {
		return nil, err
	}
	if len(data) > avmooMaxResponseBody {
		return nil, errors.New("Avmoo response too large")
	}
	return data, nil
}

func extractAvmooCSRFToken(body string) string {
	match := avmooCSRFTokenPattern.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func cookieHeader(cookies []*http.Cookie) string {
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie != nil && strings.TrimSpace(cookie.Name) != "" {
			parts = append(parts, cookie.Name+"="+cookie.Value)
		}
	}
	return strings.Join(parts, "; ")
}

func isAvmooSessionAuthError(err error) bool {
	var statusErr *avmooAPIStatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	switch statusErr.status {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, 419:
		return true
	default:
		return false
	}
}

func shouldRetryAvmoo(err error) bool {
	if err == nil || errors.Is(err, errAvmooNotFound) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var statusErr *avmooAPIStatusError
	if errors.As(err, &statusErr) {
		return statusErr.status == http.StatusTooManyRequests || statusErr.status >= 500
	}
	return false
}

func (p *Avmoo) classifyError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errAvmooNotFound) {
		return jav.NewLookupError("not_found", http.StatusNotFound, nil)
	}
	var statusErr *avmooAPIStatusError
	if errors.As(err, &statusErr) {
		switch {
		case statusErr.status == http.StatusUnauthorized || statusErr.status == http.StatusForbidden || statusErr.status == http.StatusBadRequest || statusErr.status == 419:
			return jav.NewLookupError("blocked", statusErr.status, errors.New("Avmoo session rejected"))
		case statusErr.status == http.StatusTooManyRequests || statusErr.status >= 500:
			return jav.NewLookupError("retryable", statusErr.status, errors.New("Avmoo temporarily unavailable"))
		default:
			return jav.NewLookupError("http", statusErr.status, errors.New("Avmoo request failed"))
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return jav.NewLookupError("retryable", 0, errors.New("Avmoo request timed out"))
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return jav.NewLookupError("retryable", 0, errors.New("Avmoo request timed out"))
		}
		return jav.NewLookupError("network", 0, errors.New("Avmoo network request failed"))
	}
	if strings.Contains(strings.ToLower(err.Error()), "parse") || strings.Contains(strings.ToLower(err.Error()), "response too large") || strings.Contains(strings.ToLower(err.Error()), "missing csrf") {
		return jav.NewLookupError("parse", 0, err)
	}
	return jav.NewLookupError("network", 0, err)
}

func avmooMetadata(movie *avmooMovie) *jav.Metadata {
	if movie == nil {
		return nil
	}
	uncensored := false
	metadata := &jav.Metadata{
		Code:         strings.TrimSpace(movie.MovieFanHao),
		Title:        firstNonEmpty(movie.Title, movie.TitleTW, movie.TitleCN, movie.TitleJA, movie.TitleEN),
		Studio:       avmooNameValue(movie.Studio, "studio"),
		Series:       avmooNameValue(movie.Series, "series"),
		IsUncensored: &uncensored,
		CoverURL:     firstNonEmpty(movie.PosterLarge, movie.PosterSmall),
		Provider:     "avmoo",
	}
	if movie.Length > 0 {
		n := movie.Length
		metadata.DurationMin = &n
	}
	if release := parseAvmooReleaseDate(movie.ReleaseDate); release != nil {
		metadata.ReleaseUnix = release
	}
	for _, genre := range movie.Genre {
		metadata.Tags = appendUnique(metadata.Tags, avmooNameValue(&genre, "genre"))
	}
	// One performer can appear twice in a payload (a duplicate row); the cast
	// list keeps the first occurrence, and so does the profile.
	seenStars := make(map[string]struct{}, len(movie.Star))
	for _, star := range movie.Star {
		name := star.displayName()
		if name == "" {
			continue
		}
		if _, dup := seenStars[name]; dup {
			continue
		}
		seenStars[name] = struct{}{}
		metadata.Actors = append(metadata.Actors, name)
		if profile := star.profile(); profile != nil {
			metadata.ActorProfiles = append(metadata.ActorProfiles, *profile)
		}
	}
	return metadata
}

// displayName picks the performer's name in the same order of preference the
// cast list uses, so the profile is keyed by exactly the string the actor index
// aggregates on.
func (star avmooStar) displayName() string {
	return firstNonEmpty(star.StarName, star.StarNameTW, star.StarNameCN, star.StarNameJA, star.StarNameEN,
		star.Name, star.NameTW, star.NameCN, star.NameJA, star.NameEN)
}

// profile maps the payload's performer object onto the stored profile. It
// returns nil when the payload carried nothing but a name, which keeps a
// name-only cast from creating empty profile rows.
func (star avmooStar) profile() *jav.ActorProfile {
	name := star.displayName()
	if name == "" {
		return nil
	}
	profile := &jav.ActorProfile{
		Name:      name,
		Provider:  "avmoo",
		StarID:    cleanText(star.StarID),
		AvatarURL: strings.TrimSpace(star.AvatarURL),
		Birthday:  cleanText(star.Birthday),
		HeightCM:  parseCentimeters(star.Size.Height),
		BustCM:    parseCentimeters(star.Size.Bust),
		Cup:       cleanText(star.Size.Cup),
		WaistCM:   parseCentimeters(star.Size.Waist),
		HipCM:     parseCentimeters(star.Size.Hip),
		BloodType: cleanText(star.BloodType),
		Hometown:  cleanText(star.Hometown),
		Hobby:     cleanText(star.Hobby),
		Aliases:   star.aliases(name),
	}
	if star.MovieCount > 0 {
		count := star.MovieCount
		profile.MovieCount = &count
	}
	if release := parseAvmooReleaseDate(star.LastRelease); release != nil {
		profile.LastReleaseUnix = release
	}
	if profile.StarID == "" && profile.AvatarURL == "" && profile.Birthday == "" &&
		profile.MovieCount == nil && profile.HeightCM == nil && profile.BustCM == nil &&
		profile.WaistCM == nil && profile.HipCM == nil {
		return nil
	}
	return profile
}

// aliases lists the performer's other spellings. They are not merged
// automatically — 演员合并 stays a user decision — but they let the UI show that
// the provider knows several names for the same person.
func (star avmooStar) aliases(name string) []string {
	var out []string
	for _, candidate := range []string{
		star.StarNameJA, star.StarNameEN, star.StarNameCN, star.StarNameTW,
		star.NameJA, star.NameEN, star.NameCN, star.NameTW,
	} {
		candidate = cleanText(candidate)
		if candidate == "" || candidate == name {
			continue
		}
		out = appendUnique(out, candidate)
	}
	return out
}

// parseCentimeters reads the numeric part of a measurement such as "155" or
// "80cm". Anything else is treated as unknown rather than zero.
func parseCentimeters(value string) *int {
	match := regexp.MustCompile(`\d+`).FindString(value)
	if match == "" {
		return nil
	}
	var centimeters int
	if _, err := fmt.Sscanf(match, "%d", &centimeters); err != nil {
		return nil
	}
	return &centimeters
}

func avmooNameValue(value *avmooName, kind string) string {
	if value == nil {
		return ""
	}
	switch kind {
	case "studio":
		return firstNonEmpty(value.StudioName, value.StudioNameTW, value.StudioNameCN, value.StudioNameJA, value.StudioNameEN, value.Name, value.NameTW, value.NameCN, value.NameJA, value.NameEN)
	case "series":
		return firstNonEmpty(value.SeriesName, value.SeriesNameTW, value.SeriesNameCN, value.SeriesNameJA, value.SeriesNameEN, value.Name, value.NameTW, value.NameCN, value.NameJA, value.NameEN)
	case "genre":
		return firstNonEmpty(value.GenreName, value.GenreNameTW, value.GenreNameCN, value.GenreNameJA, value.GenreNameEN, value.Name, value.NameTW, value.NameCN, value.NameJA, value.NameEN)
	case "star":
		return firstNonEmpty(value.StarName, value.StarNameTW, value.StarNameCN, value.StarNameJA, value.StarNameEN, value.Name, value.NameTW, value.NameCN, value.NameJA, value.NameEN)
	default:
		return firstNonEmpty(value.Name, value.NameTW, value.NameCN, value.NameJA, value.NameEN)
	}
}

func parseAvmooReleaseDate(value string) *int64 {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02", "2006/01/02", "2006-1-2", "2006/1/2"} {
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			unix := parsed.Unix()
			return &unix
		}
	}
	return nil
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func normalizeAvmooCode(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
