package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNormalizeFillsDefaultsAndValidates(t *testing.T) {
	tests := []struct {
		name    string
		in      Config
		want    Config
		wantErr bool
	}{
		{
			name: "empty endpoint and model use defaults",
			in:   Config{Enabled: true, APIKey: "key"},
			want: Config{Enabled: true, BaseURL: DefaultBaseURL, Model: DefaultModel, APIKey: "key"},
		},
		{
			name: "trailing slash and spaces are trimmed",
			in:   Config{Enabled: true, BaseURL: " https://example.com/v1/ ", Model: " m ", APIKey: " key "},
			want: Config{Enabled: true, BaseURL: "https://example.com/v1", Model: "m", APIKey: "key"},
		},
		{
			name:    "scheme is required",
			in:      Config{BaseURL: "api.example.com/v1"},
			wantErr: true,
		},
		{
			name:    "host is required",
			in:      Config{BaseURL: "https:///v1"},
			wantErr: true,
		},
		{
			name:    "query is rejected",
			in:      Config{BaseURL: "https://example.com/v1?x=1"},
			wantErr: true,
		},
		{
			name:    "newline in key is rejected",
			in:      Config{APIKey: "abc\ndef"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Normalize(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Normalize(%+v) = %+v, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Normalize(%+v): %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("Normalize(%+v) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestUsableRequiresEveryProviderPart(t *testing.T) {
	base := Config{Enabled: true, BaseURL: DefaultBaseURL, Model: DefaultModel, APIKey: "key"}
	if !base.Usable() || !base.AutoTranslate() {
		t.Fatal("complete config should be usable and auto-translate")
	}
	// A paused auto-translate keeps the provider usable for manual actions.
	paused := base
	paused.Enabled = false
	if !paused.Usable() {
		t.Fatal("a paused provider should still serve manual requests")
	}
	if paused.AutoTranslate() {
		t.Fatal("a paused provider must not auto-translate")
	}
	for name, mutate := range map[string]func(*Config){
		"no url":   func(c *Config) { c.BaseURL = "" },
		"no model": func(c *Config) { c.Model = "" },
		"no key":   func(c *Config) { c.APIKey = "" },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := base
			mutate(&cfg)
			if cfg.Usable() {
				t.Fatalf("%+v should not be usable", cfg)
			}
		})
	}
}

// chatServer returns an OpenAI-compatible endpoint and records request bodies.
func chatServer(t *testing.T, status int, content string) (*httptest.Server, *atomic.Value) {
	t.Helper()
	var body atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("request path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}
		requestBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request: %v", err)
		}
		body.Store(string(requestBody))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status != http.StatusOK {
			_, _ = fmt.Fprintf(w, `{"error":{"message":"invalid api key","type":"invalid_request_error"}}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"id":"1","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}]}`, content)
	}))
	t.Cleanup(server.Close)
	return server, &body
}

func TestTranslateTitleCleansModelOutput(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "plain", content: "邻家的人妻", want: "邻家的人妻"},
		{name: "labelled and quoted", content: "中文译名：“邻家的人妻”", want: "邻家的人妻"},
		{name: "multi line", content: "译名： 邻家的人妻\n说明：……", want: "邻家的人妻"},
		{name: "surrounding whitespace", content: "  邻家的人妻  ", want: "邻家的人妻"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, _ := chatServer(t, http.StatusOK, tt.content)
			client := NewClient(func() (Config, error) {
				return Config{Enabled: true, BaseURL: server.URL + "/v1", Model: "m", APIKey: "test-key"}, nil
			}, server.Client())
			got, err := client.TranslateTitle(context.Background(), "となりの人妻")
			if err != nil {
				t.Fatalf("TranslateTitle: %v", err)
			}
			if got != tt.want {
				t.Fatalf("TranslateTitle = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTranslateTitlesPacksTitlesAndUnpacksNumbering(t *testing.T) {
	server, body := chatServer(t, http.StatusOK, "1. 甲\n2. 乙\n3. 丙")
	client := NewClient(func() (Config, error) {
		return Config{Enabled: true, BaseURL: server.URL + "/v1", Model: "deepseek-chat", APIKey: "test-key"}, nil
	}, server.Client())

	translations, errs := client.TranslateTitles(context.Background(), []string{"一", "二", "三"})
	if len(translations) != 3 || len(errs) != 3 {
		t.Fatalf("result lengths = %d, %d; want 3, 3", len(translations), len(errs))
	}
	for i := range errs {
		if errs[i] != nil {
			t.Fatalf("errs[%d] = %v, want nil", i, errs[i])
		}
	}
	want := []string{"甲", "乙", "丙"}
	for i := range want {
		if translations[i] != want[i] {
			t.Fatalf("translations[%d] = %q, want %q", i, translations[i], want[i])
		}
	}

	request := body.Load().(string)
	for _, fragment := range []string{"请把下面 3 条", "1. 一", "2. 二", "3. 三"} {
		if !strings.Contains(request, fragment) {
			t.Fatalf("batch request missing %q: %s", fragment, request)
		}
	}
	// The packed path must not reuse the one-title user message.
	if strings.Contains(request, "原标题：") {
		t.Fatalf("batch request used the single-title prompt: %s", request)
	}
}

func TestTranslateTitlesRetriesMissingRowsIndividually(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		reply := "1. 甲\n3. 丙" // row 2 is missing from the packed reply
		if call > 1 {
			reply = "乙"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"1","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}]}`, reply)
	}))
	t.Cleanup(server.Close)

	client := NewClient(func() (Config, error) {
		return Config{Enabled: true, BaseURL: server.URL + "/v1", Model: "m", APIKey: "test-key"}, nil
	}, server.Client())
	translations, errs := client.TranslateTitles(context.Background(), []string{"一", "二", "三"})
	for i := range errs {
		if errs[i] != nil {
			t.Fatalf("errs[%d] = %v, want nil", i, errs[i])
		}
	}
	want := []string{"甲", "乙", "丙"}
	for i := range want {
		if translations[i] != want[i] {
			t.Fatalf("translations = %#v, want %#v", translations, want)
		}
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("provider calls = %d, want one packed request plus one retry", got)
	}
}

func TestTranslateTitlesReportsProviderFailureForEveryTitle(t *testing.T) {
	server, _ := chatServer(t, http.StatusUnauthorized, "")
	client := NewClient(func() (Config, error) {
		return Config{Enabled: true, BaseURL: server.URL + "/v1", Model: "m", APIKey: "test-key"}, nil
	}, server.Client())

	translations, errs := client.TranslateTitles(context.Background(), []string{"一", "二"})
	if len(translations) != 2 || len(errs) != 2 {
		t.Fatalf("result lengths = %d, %d; want 2, 2", len(translations), len(errs))
	}
	for i := range errs {
		if errs[i] == nil || !strings.Contains(errs[i].Error(), "API Key") {
			t.Fatalf("errs[%d] = %v, want the actionable API-key error", i, errs[i])
		}
	}
}

func TestTranslateTitlesEmptyInputSkipsTheProvider(t *testing.T) {
	client := NewClient(func() (Config, error) {
		t.Fatal("empty input must not read the provider settings")
		return Config{}, nil
	}, nil)
	translations, errs := client.TranslateTitles(context.Background(), nil)
	if len(translations) != 0 || len(errs) != 0 {
		t.Fatalf("empty input returned %d translations, %d errors; want none", len(translations), len(errs))
	}
}

func TestParseBatchTranslations(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		count int
		want  []string
	}{
		{
			name:  "numbered lines",
			raw:   "1. 甲\n2. 乙\n3. 丙",
			count: 3,
			want:  []string{"甲", "乙", "丙"},
		},
		{
			name:  "numbering punctuation and labels",
			raw:   "1、中文译名：「甲」\n2：乙",
			count: 2,
			want:  []string{"甲", "乙"},
		},
		{
			name:  "fullwidth punctuation and bold numbers",
			raw:   "**1．甲**\n**2。乙**",
			count: 2,
			want:  []string{"甲", "乙"},
		},
		{
			name:  "list bullets before numbers",
			raw:   "- 1. 甲\n- 2. 乙",
			count: 2,
			want:  []string{"甲", "乙"},
		},
		{
			name:  "list bullets without numbers",
			raw:   "- 甲\n- 乙",
			count: 2,
			want:  []string{"甲", "乙"},
		},
		{
			name:  "a missing row stays empty",
			raw:   "1. 甲\n3. 丙",
			count: 3,
			want:  []string{"甲", "", "丙"},
		},
		{
			name:  "unnumbered lines use positional order",
			raw:   "甲\n乙",
			count: 2,
			want:  []string{"甲", "乙"},
		},
		{
			name:  "code fences and prose are ignored",
			raw:   "以下是译文：\n```\n1. 甲\n2. 乙\n```",
			count: 2,
			want:  []string{"甲", "乙"},
		},
		{
			name:  "a duplicate number keeps the first line",
			raw:   "1. 甲\n1. 乙\n2. 丙",
			count: 2,
			want:  []string{"甲", "丙"},
		},
		{
			name:  "an out-of-range number is ignored",
			raw:   "1. 甲\n9. 多余\n2. 乙",
			count: 2,
			want:  []string{"甲", "乙"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseBatchTranslations(tt.raw, tt.count)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseBatchTranslations(%q) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestTranslateTitleSendsPromptAndModel(t *testing.T) {
	server, body := chatServer(t, http.StatusOK, "邻家的人妻")
	client := NewClient(func() (Config, error) {
		return Config{Enabled: true, BaseURL: server.URL + "/v1", Model: "deepseek-chat", APIKey: "test-key"}, nil
	}, server.Client())
	if _, err := client.TranslateTitle(context.Background(), "となりの人妻"); err != nil {
		t.Fatalf("TranslateTitle: %v", err)
	}
	request := body.Load().(string)
	for _, want := range []string{`"model":"deepseek-chat"`, "となりの人妻", "只输出最终中文译文", "日文原文"} {
		if !strings.Contains(request, want) {
			t.Fatalf("request body missing %q: %s", want, request)
		}
	}
}

func TestTranslateTitleRejectsEmptyTitle(t *testing.T) {
	client := NewClient(func() (Config, error) {
		return Config{Enabled: true, BaseURL: DefaultBaseURL, Model: DefaultModel, APIKey: "key"}, nil
	}, nil)
	if _, err := client.TranslateTitle(context.Background(), "   "); err == nil {
		t.Fatal("empty title should fail without a request")
	}
}

func TestTranslateTitleMapsProviderErrors(t *testing.T) {
	server, _ := chatServer(t, http.StatusUnauthorized, "")
	client := NewClient(func() (Config, error) {
		return Config{Enabled: true, BaseURL: server.URL + "/v1", Model: "m", APIKey: "test-key"}, nil
	}, server.Client())
	_, err := client.TranslateTitle(context.Background(), "タイトル")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "API Key") {
		t.Fatalf("error = %v, want an actionable API-key message", err)
	}
}

func TestTranslateTitleUsesCurrentConfig(t *testing.T) {
	server, _ := chatServer(t, http.StatusOK, "译名")
	var enabled atomic.Bool
	enabled.Store(false)
	var apiKey atomic.Value
	apiKey.Store("test-key")
	client := NewClient(func() (Config, error) {
		return Config{Enabled: enabled.Load(), BaseURL: server.URL + "/v1", Model: "m", APIKey: apiKey.Load().(string)}, nil
	}, server.Client())
	if client.Configured() {
		t.Fatal("paused config should not auto-translate")
	}
	if !client.Available() {
		t.Fatal("paused config should still serve manual requests")
	}
	// Manual translation stays available while the automatic path is paused.
	if got, err := client.TranslateTitle(context.Background(), "タイトル"); err != nil || got != "译名" {
		t.Fatalf("TranslateTitle while paused = %q, %v", got, err)
	}
	enabled.Store(true)
	if !client.Configured() {
		t.Fatal("enabled config should auto-translate")
	}
	if got, err := client.TranslateTitle(context.Background(), "タイトル"); err != nil || got != "译名" {
		t.Fatalf("TranslateTitle = %q, %v", got, err)
	}

	apiKey.Store("")
	if client.Available() {
		t.Fatal("a missing key must make the provider unavailable")
	}
	if _, err := client.TranslateTitle(context.Background(), "タイトル"); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("TranslateTitle without a key error = %v, want ErrNotConfigured", err)
	}
}

func TestTranslateTitleFailsWhenSettingsCannotBeRead(t *testing.T) {
	client := NewClient(func() (Config, error) {
		return Config{}, errors.New("database is gone")
	}, nil)
	if client.Configured() {
		t.Fatal("a settings failure must count as not configured")
	}
	if _, err := client.TranslateTitle(context.Background(), "タイトル"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestCheckUsesExplicitConfigWithoutSaving(t *testing.T) {
	server, body := chatServer(t, http.StatusOK, "邻家的人妻")
	// The stored settings are irrelevant: Check must test the form values.
	client := NewClient(func() (Config, error) {
		return Config{}, errors.New("stored settings must not be read")
	}, server.Client())
	got, err := client.Check(context.Background(), Config{
		Enabled: false, // paused auto-translate must still be testable
		BaseURL: server.URL + "/v1",
		Model:   "deepseek-chat",
		APIKey:  "test-key",
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if got != "邻家的人妻" {
		t.Fatalf("Check = %q, want the sample translation", got)
	}
	request := body.Load().(string)
	for _, want := range []string{"となりの人妻", `"model":"deepseek-chat"`, "只输出最终中文译文"} {
		if !strings.Contains(request, want) {
			t.Fatalf("smoke-test request missing %q: %s", want, request)
		}
	}
}

func TestCheckRejectsMissingAPIKey(t *testing.T) {
	client := NewClient(func() (Config, error) { return Config{}, nil }, nil)
	_, err := client.Check(context.Background(), Config{BaseURL: DefaultBaseURL, Model: DefaultModel})
	if err == nil || !strings.Contains(err.Error(), "API Key") {
		t.Fatalf("Check without a key error = %v, want an API-key hint", err)
	}
}

func TestCheckReportsProviderFailure(t *testing.T) {
	server, _ := chatServer(t, http.StatusUnauthorized, "")
	client := NewClient(func() (Config, error) { return Config{}, nil }, server.Client())
	_, err := client.Check(context.Background(), Config{
		BaseURL: server.URL + "/v1",
		Model:   "m",
		APIKey:  "test-key",
	})
	if err == nil || !strings.Contains(err.Error(), "API Key") {
		t.Fatalf("Check 401 error = %v, want an actionable API-key message", err)
	}
}
