package apitest

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	restURL    = "https://playground.nileslabs.com/api/v1"
	graphqlURL = "https://graphql.postman-echo.com/graphql"
)

type apiClient struct {
	t       *testing.T
	http    *http.Client
	baseURL string
	sandbox string
	trace   []string
}

type response struct {
	status int
	header http.Header
	body   []byte
}

func newClient(t *testing.T, envName, fallback string) *apiClient {
	t.Helper()
	base := strings.TrimRight(os.Getenv(envName), "/")
	if base == "" {
		base = fallback
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		t.Fatalf("Некорректный адрес API: %q", base)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true
	t.Cleanup(transport.CloseIdleConnections)
	c := &apiClient{t: t, http: &http.Client{Timeout: 20 * time.Second, Transport: transport}, baseURL: base}
	t.Cleanup(func() {
		if t.Failed() {
			for _, entry := range c.trace {
				t.Log(entry)
			}
		}
	})
	return c
}

func newREST(t *testing.T) *apiClient {
	t.Helper()
	c := newClient(t, "REST_API_URL", restURL)
	// Отдельная сессия сохраняет изменения между запросами одного теста.
	c.sandbox = randomID(t)
	return c
}

func newGraphQL(t *testing.T) *apiClient {
	t.Helper()
	return newClient(t, "GRAPHQL_API_URL", graphqlURL)
}

func randomID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (c *apiClient) request(method, path string, payload any, token string) response {
	c.t.Helper()
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			c.t.Fatalf("Тело запроса: %v", err)
		}
	}
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		c.t.Fatal(err)
	}
	if c.sandbox != "" {
		q := u.Query()
		q.Set("_sandbox", c.sandbox)
		u.RawQuery = q.Encode()
	}
	req, err := http.NewRequest(method, u.String(), bytes.NewReader(body))
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	entry := fmt.Sprintf("%s %s\nЗаголовки: %s\nТело: %s", method, u, safeHeaders(req.Header), safeJSON(body))
	start := time.Now()
	r, err := c.http.Do(req)
	if err != nil {
		c.trace = append(c.trace, entry+"\nОшибка: "+err.Error())
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	c.trace = append(c.trace, fmt.Sprintf("%s\nОтвет: HTTP %d за %s\nЗаголовки: %s\nТело: %s",
		entry, r.StatusCode, time.Since(start).Round(time.Millisecond), safeHeaders(r.Header), safeJSON(data)))
	if err != nil {
		c.t.Fatalf("Чтение ответа %s %s: %v", method, path, err)
	}
	return response{r.StatusCode, r.Header, data}
}

func checkStatus(t *testing.T, r response, want int) {
	t.Helper()
	if r.status != want {
		t.Fatalf("HTTP status = %d, ожидался %d", r.status, want)
	}
}

func readJSON[T any](t *testing.T, r response) T {
	t.Helper()
	mediaType, _, err := mime.ParseMediaType(r.header.Get("Content-Type"))
	if err != nil || (mediaType != "application/json" && mediaType != "application/graphql-response+json") {
		t.Fatalf("Ожидался JSON, получен Content-Type %q", r.header.Get("Content-Type"))
	}
	var v T
	if err := json.Unmarshal(r.body, &v); err != nil {
		t.Fatalf("Некорректный JSON ответа: %v", err)
	}
	return v
}

func equal[T comparable](t *testing.T, field string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, ожидалось %v", field, got, want)
	}
}

func notEmpty(t *testing.T, field, value string) {
	t.Helper()
	if strings.TrimSpace(value) == "" {
		t.Fatalf("Поле %s пустое или отсутствует", field)
	}
}

// У созданных ресурсов стенд возвращает строковые id, у исходных бывают числа.
func resourceID(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var s string
	if json.Unmarshal(raw, &s) == nil && strings.TrimSpace(s) != "" {
		return s
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		if i, err := n.Int64(); err == nil && i > 0 {
			return n.String()
		}
	}
	t.Fatalf("Ожидался непустой id, получено %s", raw)
	return ""
}

func safeHeaders(h http.Header) string {
	copy := h.Clone()
	for key := range copy {
		if secretKey(key) {
			copy.Set(key, "[скрыто]")
		}
	}
	return fmt.Sprint(copy)
}

func secretKey(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "token") || strings.Contains(key, "password") ||
		strings.Contains(key, "cookie") || key == "authorization" || key == "x-playground-identity"
}

func safeJSON(data []byte) string {
	if len(data) == 0 {
		return "<пусто>"
	}
	var value any
	if json.Unmarshal(data, &value) != nil {
		return string(data)
	}
	var hide func(any)
	hide = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for key, item := range x {
				if secretKey(key) {
					x[key] = "[скрыто]"
				} else {
					hide(item)
				}
			}
		case []any:
			for _, item := range x {
				hide(item)
			}
		}
	}
	hide(value)
	data, _ = json.Marshal(value)
	return string(data)
}
