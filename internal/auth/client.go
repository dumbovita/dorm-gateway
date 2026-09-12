package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrPortalUnavailable  = errors.New("portal unavailable")
	ErrNetworkUnavailable = errors.New("network unavailable")
	ErrTimeout            = errors.New("authentication timed out")
	ErrAmbiguousResponse  = errors.New("ambiguous portal response")
	ErrExceededRetries    = errors.New("exceeded maximum retry attempts")
)

const (
	DefaultLoginURL  = "https://wifi.gsb.gov.tr/j_spring_security_check"
	DefaultCheckURL  = "http://connectivitycheck.gstatic.com/generate_204"
	DefaultUserAgent = "dorm-gateway/1.0 (+https://github.com/dumbovita/dorm-gateway)"
	MaxResponseBody  = 64 * 1024 // 64 KB read cap
)

// Client handles GSB WiFi captive portal authentication.
type Client struct {
	HTTPClient     *http.Client
	LoginURL       string
	CheckURL       string
	UserAgent      string
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Logger         func(format string, args ...any)
}

// NewClient creates a new GSB WiFi authentication client with a clean cookie jar and system TLS.
func NewClient() *Client {
	jar, _ := cookiejar.New(nil)
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	httpClient := &http.Client{
		Jar:       jar,
		Transport: transport,
		Timeout:   10 * time.Second,
		// Do not automatically follow redirects on the login POST,
		// so we can semantically evaluate the 302 Location header.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return &Client{
		HTTPClient:     httpClient,
		LoginURL:       DefaultLoginURL,
		CheckURL:       DefaultCheckURL,
		UserAgent:      DefaultUserAgent,
		MaxAttempts:    5,
		InitialBackoff: 300 * time.Millisecond,
		MaxBackoff:     2 * time.Second,
	}
}

func (c *Client) log(format string, args ...any) {
	if c.Logger != nil {
		c.Logger(format, args...)
	}
}

// CheckConnectivity verifies if internet access is already established.
// Returns true if the check URL responds with HTTP 204 or 200 without captive redirection.
func (c *Client) CheckConnectivity(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.CheckURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	// Use a client that allows following redirects to detect captive portals
	checkClient := &http.Client{
		Transport: c.HTTPClient.Transport,
		Timeout:   4 * time.Second,
	}

	resp, err := checkClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return true, nil
	}

	if resp.StatusCode == http.StatusOK {
		// If redirected away from probe host (to portal IP, hostname, etc.), it is captive interception
		probeURL, _ := url.Parse(c.CheckURL)
		if resp.Request != nil && resp.Request.URL != nil && probeURL != nil {
			if !strings.EqualFold(resp.Request.URL.Host, probeURL.Host) {
				return false, nil
			}
		}

		// Verify body is not an intercepted HTML page
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		bodyLower := strings.ToLower(string(body))
		if strings.Contains(bodyLower, "<html") || strings.Contains(bodyLower, "<body") || strings.Contains(bodyLower, "<head") {
			return false, nil
		}

		return true, nil
	}

	return false, nil
}

// Authenticate performs bounded, retried login to the GSB WiFi portal with semantic verification.
func (c *Client) Authenticate(ctx context.Context, username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username (TC Kimlik No) cannot be empty")
	}
	if password == "" {
		return errors.New("password cannot be empty")
	}

	// 1. Check if already online
	c.log("Checking current network connectivity...")
	if online, _ := c.CheckConnectivity(ctx); online {
		c.log("Network already has active Internet connectivity.")
		return nil
	}

	c.log("Starting portal authentication for user %s...", maskUser(username))

	formData := url.Values{}
	formData.Set("j_username", username)
	formData.Set("j_password", password)

	backoff := c.InitialBackoff
	if backoff <= 0 {
		backoff = 300 * time.Millisecond
	}

	for attempt := 1; attempt <= c.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		c.log("Authentication attempt %d/%d...", attempt, c.MaxAttempts)

		attemptErr := c.doLoginAttempt(ctx, formData)
		if attemptErr == nil {
			// Login attempt looked successful; verify data-path with connectivity check
			c.log("Portal accepted login. Verifying Internet connectivity...")
			if verified, _ := c.CheckConnectivity(ctx); verified {
				c.log("Internet connectivity verified successfully.")
				return nil
			}
			// If verification failed, portal might need a moment or returned a false positive
			attemptErr = fmt.Errorf("%w: connectivity check did not pass after portal response", ErrAmbiguousResponse)
		}

		// Immediate stop on permanent failures
		if errors.Is(attemptErr, ErrInvalidCredentials) {
			c.log("Authentication failed: invalid username or password.")
			return ErrInvalidCredentials
		}

		c.log("Attempt %d failed: %v", attempt, attemptErr)

		if attempt >= c.MaxAttempts {
			break
		}

		// Calculate backoff with jitter
		jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
		sleepDuration := backoff + jitter
		c.log("Waiting %v before retry...", sleepDuration.Round(time.Millisecond))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleepDuration):
		}

		backoff *= 2
		if backoff > c.MaxBackoff && c.MaxBackoff > 0 {
			backoff = c.MaxBackoff
		}
	}

	return fmt.Errorf("%w: failed after %d attempts", ErrExceededRetries, c.MaxAttempts)
}

func (c *Client) doLoginAttempt(ctx context.Context, formData url.Values) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.LoginURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return classifyNetworkError(err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBody))
	bodyText := strings.ToLower(string(bodyBytes))

	// Semantic classification
	switch resp.StatusCode {
	case http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect:
		location := resp.Header.Get("Location")
		return classifyRedirect(location, bodyText)

	case http.StatusOK:
		return classifyOKResponse(bodyText)

	case http.StatusUnauthorized, http.StatusForbidden:
		if containsAny(bodyText, "hatalı", "geçersiz", "yanlış", "kullanıcı bulunamadı", "bad credentials", "giriş başarısız") {
			return ErrInvalidCredentials
		}
		return fmt.Errorf("%w: portal returned HTTP %d", ErrAmbiguousResponse, resp.StatusCode)

	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return fmt.Errorf("%w: portal server returned HTTP %d", ErrPortalUnavailable, resp.StatusCode)

	default:
		return fmt.Errorf("%w: unexpected HTTP %d", ErrAmbiguousResponse, resp.StatusCode)
	}
}

func classifyRedirect(location, bodyText string) error {
	locLower := strings.ToLower(location)

	// Check for explicit error flags in redirect destination
	if strings.Contains(locLower, "error") ||
		strings.Contains(locLower, "badcredentials") ||
		strings.Contains(locLower, "hata") ||
		strings.Contains(locLower, "fail") {
		return ErrInvalidCredentials
	}

	// If redirecting back to login page without error parameter, check body
	if strings.Contains(locLower, "login") {
		if containsAny(bodyText, "hatalı", "geçersiz", "yanlış", "bad credential", "giriş başarısız") {
			return ErrInvalidCredentials
		}
		return fmt.Errorf("%w: redirected back to login page: %s", ErrAmbiguousResponse, location)
	}

	// Redirect to target, success page, or root is considered success
	return nil
}

func classifyOKResponse(bodyText string) error {
	if containsAny(bodyText, "hatalı", "geçersiz", "yanlış", "kullanıcı bulunamadı", "bad credentials", "giriş başarısız") {
		return ErrInvalidCredentials
	}
	if containsAny(bodyText, "giriş başarılı", "başarılı", "welcome", "hoş geldiniz", "oturum açıldı") {
		return nil
	}
	return fmt.Errorf("%w: 200 OK without recognized success or error confirmation", ErrAmbiguousResponse)
}

func classifyNetworkError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ErrTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrTimeout
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return fmt.Errorf("%w: %v", ErrNetworkUnavailable, opErr)
	}
	return fmt.Errorf("%w: %v", ErrPortalUnavailable, err)
}

func containsAny(s string, targets ...string) bool {
	for _, t := range targets {
		if strings.Contains(s, t) {
			return true
		}
	}
	return false
}

func maskUser(u string) string {
	if len(u) == 11 {
		return u[:3] + "******" + u[9:]
	}
	if len(u) > 4 {
		return u[:2] + "***" + u[len(u)-2:]
	}
	return "***"
}
