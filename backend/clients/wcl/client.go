// Package wcl is the Warcraft Logs v2 API client: tokens, queries with retries, the points budget, and the Mythic+
// fetchers (encounters, spec rankings, a run's damage taken), cached in CacheDir.
package wcl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"wow-live/backend/clients/cachefile"
	"wow-live/backend/models"
)

const (
	TokenURL = "https://www.warcraftlogs.com/oauth/token"
	APIURL   = "https://www.warcraftlogs.com/api/v2/client"
	// Mythic is Warcraft Logs' difficulty number for mythic raids.
	Mythic = 5
)

// RetryWaits are the pauses before each retry when Warcraft Logs times out (it's slow at peak times).
var RetryWaits = []time.Duration{10 * time.Second, 30 * time.Second}

// ErrRateLimited means the account's hourly points are used up until the hour resets.
var ErrRateLimited = errors.New("Warcraft Logs rate limit reached (the API allows so many points per hour); " +
	"try again later")

// Credentials are one Warcraft Logs API client.
type Credentials = models.Credentials

// Client talks to the Warcraft Logs API.
type Client struct {
	HTTP     *http.Client
	TokenURL string
	APIURL   string
	Main     Credentials
	// CacheDir is where answers are cached (logs/cache/ in the repo, the app's cache folder when installed).
	CacheDir string
	// Now and Sleep are the clock, replaceable in tests.
	Now   func() time.Time
	Sleep func(time.Duration)
	Waits []time.Duration

	mu    sync.Mutex
	token string
}

// New returns a client for WCL_CLIENT_ID and WCL_CLIENT_SECRET in the environment.
func New(cacheDir string) (*Client, error) {
	main := Credentials{ID: os.Getenv("WCL_CLIENT_ID"), Secret: os.Getenv("WCL_CLIENT_SECRET")}
	if main.ID == "" || main.Secret == "" {
		return nil, errors.New("WCL_CLIENT_ID and WCL_CLIENT_SECRET must be set; add them to .env")
	}
	return &Client{Main: main, CacheDir: cacheDir}, nil
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Client) sleep(d time.Duration) {
	if c.Sleep != nil {
		c.Sleep(d)
		return
	}
	time.Sleep(d)
}

func orDefault(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

// fetchToken gets a client-credentials token for the client.
func (c *Client) fetchToken(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, orDefault(c.TokenURL, TokenURL),
		strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", fmt.Errorf("building token request: %w", err)
	}
	req.SetBasicAuth(c.Main.ID, c.Main.Secret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http().Do(req)
	if err != nil {
		return "", fmt.Errorf("getting a Warcraft Logs token: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusBadRequest, http.StatusUnauthorized:
		return "", fmt.Errorf("getting a Warcraft Logs token: %w (%s)", models.ErrBadCredentials, resp.Status)
	default:
		return "", fmt.Errorf("getting a Warcraft Logs token: %s", resp.Status)
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("reading the Warcraft Logs token: %w", err)
	}
	if body.AccessToken == "" {
		return "", errors.New("Warcraft Logs answered without a token")
	}
	return body.AccessToken, nil
}

// currentToken is the token in use, fetched on first use.
func (c *Client) currentToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" {
		return c.token, nil
	}
	if c.Main.ID == "" || c.Main.Secret == "" {
		return "", models.ErrNoCredentials
	}
	token, err := c.fetchToken(ctx)
	if err != nil {
		return "", err
	}
	c.token = token
	return token, nil
}

// Query runs a GraphQL query and decodes its `data` into out. It retries when Warcraft Logs times out; a rate limit
// is ErrRateLimited.
func (c *Client) Query(ctx context.Context, gql string, variables map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": gql, "variables": variables})
	if err != nil {
		return fmt.Errorf("encoding the query: %w", err)
	}
	waits := c.Waits
	if waits == nil {
		waits = RetryWaits
	}
	for attempt := 1; ; attempt++ {
		data, retry, err := c.post(ctx, body)
		if err == nil {
			return decodeData(data, out)
		}
		switch {
		case retry && attempt <= len(waits):
			c.sleep(waits[attempt-1])
		case retry:
			return fmt.Errorf("Warcraft Logs timed out %d times in a row; try again later: %w", attempt, err)
		default:
			return err
		}
	}
}

// post sends one query and says whether a failure is a timeout worth retrying after a wait.
func (c *Client) post(ctx context.Context, body []byte) ([]byte, bool, error) {
	token, err := c.currentToken(ctx)
	if err != nil {
		return nil, false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, orDefault(c.APIURL, APIURL), bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("building query request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http().Do(req)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, true, fmt.Errorf("querying Warcraft Logs: %w", err)
		}
		return nil, false, fmt.Errorf("querying Warcraft Logs: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("reading the Warcraft Logs response: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, false, ErrRateLimited
	case resp.StatusCode == http.StatusGatewayTimeout:
		return nil, true, fmt.Errorf("querying Warcraft Logs: %s", resp.Status)
	case resp.StatusCode != http.StatusOK:
		return nil, false, fmt.Errorf("querying Warcraft Logs: %s", resp.Status)
	}
	return data, false, nil
}

// decodeData reads a GraphQL response's data, failing on GraphQL errors.
func decodeData(body []byte, out any) error {
	var result struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("reading the Warcraft Logs response: %w", err)
	}
	if len(result.Errors) > 0 {
		messages := make([]string, len(result.Errors))
		for i, e := range result.Errors {
			messages[i] = e.Message
		}
		return fmt.Errorf("Warcraft Logs query failed: %s", strings.Join(messages, "; "))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(result.Data, out); err != nil {
		return fmt.Errorf("reading the Warcraft Logs data: %w", err)
	}
	return nil
}

// RateLimit is the account's points budget this hour (about 1 point to ask).
func (c *Client) RateLimit(ctx context.Context) (models.RateLimit, error) {
	var data struct {
		RateLimitData models.RateLimit `json:"rateLimitData"`
	}
	err := c.Query(ctx, "query { rateLimitData { limitPerHour pointsSpentThisHour pointsResetIn } }", nil, &data)
	return data.RateLimitData, err
}

// SetCredentials switches the client to creds (the desktop app's saved keys) for the queries that follow.
func (c *Client) SetCredentials(creds Credentials) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Main, c.token = creds, ""
}

// Test tries creds without switching to them: a token, then the points they have left this hour.
func (c *Client) Test(ctx context.Context, creds Credentials) (models.RateLimit, error) {
	probe := &Client{HTTP: c.HTTP, TokenURL: c.TokenURL, APIURL: c.APIURL, Main: creds, Sleep: c.Sleep, Waits: c.Waits}
	return probe.RateLimit(ctx)
}

// PointsLine is one line of logs/points.log, e.g. "2026-11-03 23:01 sim used 12.4, 380.1 of 1000 spent this
// hour". The used count shows "?" when the hour reset mid-run.
func PointsLine(when time.Time, script string, start float64, budget models.RateLimit) string {
	used := budget.PointsSpentThisHour - start
	usedText := "?"
	if used >= 0 {
		usedText = fmt.Sprintf("%.1f", used)
	}
	return fmt.Sprintf("%s %s used %s, %.1f of %g spent this hour", when.Format("2006-01-02 15:04"), script,
		usedText, budget.PointsSpentThisHour, budget.LimitPerHour)
}

// LogPoints appends this run's points line to logFile (logs/points.log) and returns it.
func (c *Client) LogPoints(ctx context.Context, logFile, script string, start float64) (string, error) {
	budget, err := c.RateLimit(ctx)
	if err != nil {
		return "", err
	}
	line := PointsLine(c.now(), script, start, budget)
	return line, cachefile.AppendLine(logFile, line)
}
