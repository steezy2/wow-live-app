package wcl

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeWCL is a Warcraft Logs stand-in: a token endpoint and an API endpoint answering with the next scripted reply.
type fakeWCL struct {
	t       *testing.T
	mu      sync.Mutex
	replies []func(w http.ResponseWriter, body string, auth string)
	queries []string
	tokens  []string
	server  *httptest.Server
}

func newFake(t *testing.T, replies ...func(w http.ResponseWriter, body, auth string)) *fakeWCL {
	f := &fakeWCL{t: t, replies: replies}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Path == "/token" {
			id, _, _ := r.BasicAuth()
			f.tokens = append(f.tokens, id)
			json.NewEncoder(w).Encode(map[string]string{"access_token": "token-" + id})
			return
		}
		f.queries = append(f.queries, string(data))
		if len(f.replies) == 0 {
			t.Errorf("unexpected query: %s", data)
			http.Error(w, "no reply scripted", http.StatusInternalServerError)
			return
		}
		reply := f.replies[0]
		f.replies = f.replies[1:]
		reply(w, string(data), r.Header.Get("Authorization"))
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeWCL) client(t *testing.T) *Client {
	return &Client{TokenURL: f.server.URL + "/token", APIURL: f.server.URL + "/api",
		Main: Credentials{ID: "main", Secret: "s1"}, CacheDir: t.TempDir(),
		Sleep: func(time.Duration) {}, Waits: []time.Duration{time.Millisecond}}
}

func data(payload string) func(w http.ResponseWriter, body, auth string) {
	return func(w http.ResponseWriter, _, _ string) { io.WriteString(w, `{"data": `+payload+`}`) }
}

func status(code int) func(w http.ResponseWriter, body, auth string) {
	return func(w http.ResponseWriter, _, _ string) { w.WriteHeader(code) }
}

func TestRetriesATimeoutThenGivesUp(t *testing.T) {
	f := newFake(t, status(http.StatusGatewayTimeout), data(`{"ok": true}`))
	var out struct{ OK bool }
	if err := f.client(t).Query(context.Background(), "{ ok }", nil, &out); err != nil || !out.OK {
		t.Fatalf("Query = %+v, %v", out, err)
	}
	f = newFake(t, status(http.StatusGatewayTimeout), status(http.StatusGatewayTimeout))
	if err := f.client(t).Query(context.Background(), "{ ok }", nil, nil); err == nil ||
		!strings.Contains(err.Error(), "timed out 2 times") {
		t.Fatalf("want a timed-out error, got %v", err)
	}
}

func TestRateLimitIsAnError(t *testing.T) {
	f := newFake(t, status(http.StatusTooManyRequests), data(`{"ok": true}`))
	if err := f.client(t).Query(context.Background(), "{ x }", nil, nil); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want ErrRateLimited, got %v", err)
	}
}

func TestGraphQLErrorsAreReported(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, _, _ string) {
		io.WriteString(w, `{"errors": [{"message": "Unknown report"}]}`)
	})
	if err := f.client(t).Query(context.Background(), "{ x }", nil, nil); err == nil ||
		!strings.Contains(err.Error(), "Unknown report") {
		t.Fatalf("want the GraphQL error, got %v", err)
	}
}

func TestATokenReplyWithoutATokenIsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{}`)
	}))
	defer server.Close()
	c := &Client{TokenURL: server.URL, APIURL: server.URL, Main: Credentials{ID: "main", Secret: "s"}}
	if err := c.Query(context.Background(), "{ x }", nil, nil); err == nil ||
		!strings.Contains(err.Error(), "without a token") {
		t.Fatalf("want a missing-token error, got %v", err)
	}
}
