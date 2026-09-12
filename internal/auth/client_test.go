package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoginSuccessRedirect(t *testing.T) {
	var portalHits int32
	var checkHits int32

	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&portalHits, 1)
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}
		if r.FormValue("j_username") != "12345678901" || r.FormValue("j_password") != "secret" {
			t.Errorf("unexpected form values")
		}
		http.Redirect(w, r, "/welcome", http.StatusFound)
	}))
	defer portalServer.Close()

	checkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits := atomic.AddInt32(&checkHits, 1)
		// First check before login reports not connected
		if hits == 1 {
			w.WriteHeader(http.StatusNetworkAuthenticationRequired)
			return
		}
		// Verification check reports connected
		w.WriteHeader(http.StatusNoContent)
	}))
	defer checkServer.Close()

	client := NewClient()
	client.LoginURL = portalServer.URL
	client.CheckURL = checkServer.URL
	client.InitialBackoff = 1 * time.Millisecond
	client.MaxAttempts = 3

	ctx := context.Background()
	err := client.Authenticate(ctx, "12345678901", "secret")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if atomic.LoadInt32(&portalHits) != 1 {
		t.Errorf("expected 1 portal hit, got %d", portalHits)
	}
}

func TestLoginSuccess200OK(t *testing.T) {
	var checkHits int32

	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>Giriş Başarılı</body></html>"))
	}))
	defer portalServer.Close()

	checkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits := atomic.AddInt32(&checkHits, 1)
		if hits == 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer checkServer.Close()

	client := NewClient()
	client.LoginURL = portalServer.URL
	client.CheckURL = checkServer.URL
	client.InitialBackoff = 1 * time.Millisecond

	err := client.Authenticate(context.Background(), "12345678901", "secret")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestLoginInvalidCredentialsRedirect(t *testing.T) {
	var portalHits int32

	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&portalHits, 1)
		http.Redirect(w, r, "/login.html?error=badcredentials", http.StatusFound)
	}))
	defer portalServer.Close()

	checkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNetworkAuthenticationRequired)
	}))
	defer checkServer.Close()

	client := NewClient()
	client.LoginURL = portalServer.URL
	client.CheckURL = checkServer.URL
	client.InitialBackoff = 1 * time.Millisecond
	client.MaxAttempts = 5

	err := client.Authenticate(context.Background(), "12345678901", "wrong_pass")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}

	// Must stop on attempt 1 without retrying!
	if hits := atomic.LoadInt32(&portalHits); hits != 1 {
		t.Errorf("expected exactly 1 attempt on invalid credentials, got %d", hits)
	}
}

func TestLoginInvalidCredentialsBody(t *testing.T) {
	var portalHits int32

	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&portalHits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>Hatalı kullanıcı adı veya şifre girdiniz</body></html>"))
	}))
	defer portalServer.Close()

	checkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNetworkAuthenticationRequired)
	}))
	defer checkServer.Close()

	client := NewClient()
	client.LoginURL = portalServer.URL
	client.CheckURL = checkServer.URL
	client.InitialBackoff = 1 * time.Millisecond
	client.MaxAttempts = 5

	err := client.Authenticate(context.Background(), "12345678901", "wrong_pass")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}

	if hits := atomic.LoadInt32(&portalHits); hits != 1 {
		t.Errorf("expected exactly 1 attempt on invalid credentials, got %d", hits)
	}
}

func TestLoginTransientRetryThenSuccess(t *testing.T) {
	var portalHits int32
	var checkHits int32

	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&portalHits, 1)
		if attempt < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		http.Redirect(w, r, "/welcome", http.StatusFound)
	}))
	defer portalServer.Close()

	checkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits := atomic.AddInt32(&checkHits, 1)
		if hits == 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer checkServer.Close()

	client := NewClient()
	client.LoginURL = portalServer.URL
	client.CheckURL = checkServer.URL
	client.InitialBackoff = 1 * time.Millisecond
	client.MaxBackoff = 5 * time.Millisecond
	client.MaxAttempts = 5

	err := client.Authenticate(context.Background(), "12345678901", "secret")
	if err != nil {
		t.Fatalf("expected success on attempt 3, got %v", err)
	}

	if hits := atomic.LoadInt32(&portalHits); hits != 3 {
		t.Errorf("expected 3 portal attempts, got %d", hits)
	}
}

func TestLoginExceededRetries(t *testing.T) {
	var portalHits int32

	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&portalHits, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer portalServer.Close()

	checkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer checkServer.Close()

	client := NewClient()
	client.LoginURL = portalServer.URL
	client.CheckURL = checkServer.URL
	client.InitialBackoff = 1 * time.Millisecond
	client.MaxAttempts = 3

	err := client.Authenticate(context.Background(), "12345678901", "secret")
	if !errors.Is(err, ErrExceededRetries) {
		t.Fatalf("expected ErrExceededRetries, got %v", err)
	}

	if hits := atomic.LoadInt32(&portalHits); hits != 3 {
		t.Errorf("expected 3 attempts, got %d", hits)
	}
}

func TestLoginContextCancellation(t *testing.T) {
	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer portalServer.Close()

	checkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer checkServer.Close()

	client := NewClient()
	client.LoginURL = portalServer.URL
	client.CheckURL = checkServer.URL
	client.InitialBackoff = 100 * time.Millisecond
	client.MaxAttempts = 5

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after 10ms (during backoff)
	time.AfterFunc(10*time.Millisecond, cancel)

	err := client.Authenticate(ctx, "12345678901", "secret")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestAlreadyConnected(t *testing.T) {
	var portalHits int32

	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&portalHits, 1)
		http.Redirect(w, r, "/welcome", http.StatusFound)
	}))
	defer portalServer.Close()

	checkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Already connected
		w.WriteHeader(http.StatusNoContent)
	}))
	defer checkServer.Close()

	client := NewClient()
	client.LoginURL = portalServer.URL
	client.CheckURL = checkServer.URL

	err := client.Authenticate(context.Background(), "12345678901", "secret")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	// Portal should NOT have been touched at all!
	if hits := atomic.LoadInt32(&portalHits); hits != 0 {
		t.Errorf("expected 0 portal hits when already connected, got %d", hits)
	}
}

func TestCheckConnectivityCases(t *testing.T) {
	ctx := context.Background()

	// 1. Direct 204 success
	s204 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer s204.Close()

	c := NewClient()
	c.CheckURL = s204.URL
	active, err := c.CheckConnectivity(ctx)
	if err != nil || !active {
		t.Errorf("expected active connection on 204, got %t, err=%v", active, err)
	}

	// 2. Captive redirect to IP address
	targetIPServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>Captive Gateway 10.0.0.1</body></html>"))
	}))
	defer targetIPServer.Close()

	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, targetIPServer.URL, http.StatusFound)
	}))
	defer redirectServer.Close()

	c.CheckURL = redirectServer.URL
	active, err = c.CheckConnectivity(ctx)
	if err != nil {
		t.Errorf("unexpected error on redirect: %v", err)
	}
	if active {
		t.Errorf("expected inactive when redirected to another host/IP, got active=true")
	}

	// 3. Direct HTML interception on check endpoint without redirect
	htmlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><head><title>Hotspot Login</title></head><body>Login required</body></html>"))
	}))
	defer htmlServer.Close()

	c.CheckURL = htmlServer.URL
	active, err = c.CheckConnectivity(ctx)
	if err != nil {
		t.Errorf("unexpected error on html interception: %v", err)
	}
	if active {
		t.Errorf("expected inactive when check endpoint returns HTML, got active=true")
	}
}

func TestLogin403RetriesWhenNoCredentialsError(t *testing.T) {
	var portalHits int32

	portalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&portalHits, 1)
		if attempt < 3 {
			// Transient 403 (e.g. rate limit / WAF) without credential error body
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("Rate limit exceeded"))
			return
		}
		http.Redirect(w, r, "/welcome", http.StatusFound)
	}))
	defer portalServer.Close()

	checkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer checkServer.Close()

	client := NewClient()
	client.LoginURL = portalServer.URL
	client.CheckURL = checkServer.URL
	client.InitialBackoff = 1 * time.Millisecond
	client.MaxBackoff = 5 * time.Millisecond
	client.MaxAttempts = 4

	// First connectivity check should report false to allow login attempt
	firstCheck := true
	checkWithGate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if firstCheck {
			firstCheck = false
			w.WriteHeader(http.StatusNetworkAuthenticationRequired)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer checkWithGate.Close()
	client.CheckURL = checkWithGate.URL

	err := client.Authenticate(context.Background(), "12345678901", "secret")
	if err != nil {
		t.Fatalf("expected success after retries on transient 403, got %v", err)
	}

	if hits := atomic.LoadInt32(&portalHits); hits != 3 {
		t.Errorf("expected 3 portal attempts, got %d", hits)
	}
}
