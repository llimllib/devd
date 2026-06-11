package reverseproxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/llimllib/devd/inject"
)

func TestReverseProxy(t *testing.T) {
	const backendResponse = "I am the backend"
	const backendStatus = 404
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.TransferEncoding) > 0 {
			t.Errorf("backend got unexpected TransferEncoding: %v", r.TransferEncoding)
		}
		if r.Header.Get("X-Forwarded-For") == "" {
			t.Errorf("didn't get X-Forwarded-For header")
		}
		if c := r.Header.Get("Connection"); c != "" {
			t.Errorf("handler got Connection header value %q", c)
		}
		if c := r.Header.Get("Upgrade"); c != "" {
			t.Errorf("handler got Upgrade header value %q", c)
		}
		if g, e := r.Host, "some-name"; g == e {
			t.Errorf("backend got original Host header %q, expected over-written", g)
		}
		if acceptEncoding := r.Header.Get("Accept-Encoding"); acceptEncoding != "identity" {
			t.Errorf(
				"backend got unexpected or no Accept-Encoding header: %q, expected \"identity\"",
				acceptEncoding,
			)
		}
		w.Header().Set("X-Foo", "bar")
		http.SetCookie(w, &http.Cookie{Name: "flavor", Value: "chocolateChip"})
		w.WriteHeader(backendStatus)
		if _, err := w.Write([]byte(backendResponse)); err != nil {
			t.Errorf("unable to write response")
		}
	}))
	defer backend.Close()
	backendURL, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyHandler := NewSingleHostReverseProxy(backendURL, inject.CopyInject{})
	frontend := httptest.NewServer(proxyHandler)
	defer frontend.Close()

	getReq, _ := http.NewRequest("GET", frontend.URL, nil)
	getReq.Host = "some-name"
	getReq.Header.Set("Connection", "close")
	getReq.Header.Set("Upgrade", "foo")
	getReq.Close = true
	res, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if g, e := res.StatusCode, backendStatus; g != e {
		t.Errorf("got res.StatusCode %d; expected %d", g, e)
	}
	if g, e := res.Header.Get("X-Foo"), "bar"; g != e {
		t.Errorf("got X-Foo %q; expected %q", g, e)
	}
	if g, e := len(res.Header["Set-Cookie"]), 1; g != e {
		t.Fatalf("got %d SetCookies, want %d", g, e)
	}
	if cookie := res.Cookies()[0]; cookie.Name != "flavor" {
		t.Errorf("unexpected cookie %q", cookie.Name)
	}
	bodyBytes, _ := io.ReadAll(res.Body)
	if g, e := string(bodyBytes), backendResponse; g != e {
		t.Errorf("got body %q; expected %q", g, e)
	}
}

func TestXForwardedFor(t *testing.T) {
	const prevForwardedFor = "client ip"
	const backendResponse = "I am the backend"
	const backendStatus = 404
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Forwarded-For") == "" {
			t.Errorf("didn't get X-Forwarded-For header")
		}
		if !strings.Contains(r.Header.Get("X-Forwarded-For"), prevForwardedFor) {
			t.Errorf("X-Forwarded-For didn't contain prior data")
		}
		w.WriteHeader(backendStatus)
		if _, err := w.Write([]byte(backendResponse)); err != nil {
			t.Errorf("unable to write response")
		}
	}))
	defer backend.Close()
	backendURL, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxyHandler := NewSingleHostReverseProxy(backendURL, inject.CopyInject{})
	frontend := httptest.NewServer(proxyHandler)
	defer frontend.Close()

	getReq, _ := http.NewRequest("GET", frontend.URL, nil)
	getReq.Host = "some-name"
	getReq.Header.Set("Connection", "close")
	getReq.Header.Set("X-Forwarded-For", prevForwardedFor)
	getReq.Close = true
	res, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if g, e := res.StatusCode, backendStatus; g != e {
		t.Errorf("got res.StatusCode %d; expected %d", g, e)
	}
	bodyBytes, _ := io.ReadAll(res.Body)
	if g, e := string(bodyBytes), backendResponse; g != e {
		t.Errorf("got body %q; expected %q", g, e)
	}
}

var proxyQueryTests = []struct {
	baseSuffix string // suffix to add to backend URL
	reqSuffix  string // suffix to add to frontend's request URL
	want       string // what backend should see for final request URL (without ?)
}{
	{"", "", ""},
	{"?sta=tic", "?us=er", "sta=tic&us=er"},
	{"", "?us=er", "us=er"},
	{"?sta=tic", "", "sta=tic"},
}

func TestReverseProxyQuery(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Got-Query", r.URL.RawQuery)
		if _, err := w.Write([]byte("hi")); err != nil {
			t.Errorf("unable to write hi")
		}
	}))
	defer backend.Close()

	for i, tt := range proxyQueryTests {
		backendURL, err := url.Parse(backend.URL + tt.baseSuffix)
		if err != nil {
			t.Fatal(err)
		}
		frontend := httptest.NewServer(NewSingleHostReverseProxy(backendURL, inject.CopyInject{}))
		req, _ := http.NewRequest("GET", frontend.URL+tt.reqSuffix, nil)
		req.Close = true
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%d. Get: %v", i, err)
		}
		if g, e := res.Header.Get("X-Got-Query"), tt.want; g != e {
			t.Errorf("%d. got query %q; expected %q", i, g, e)
		}
		_ = res.Body.Close()
		frontend.Close()
	}
}

func TestReverseProxyFlushInterval(t *testing.T) {
	const expected = "hi"
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte(expected)); err != nil {
			t.Errorf("unable to write %s", expected)
		}
	}))
	defer backend.Close()

	backendURL, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}

	proxyHandler := NewSingleHostReverseProxy(backendURL, inject.CopyInject{})

	frontend := httptest.NewServer(proxyHandler)
	defer frontend.Close()

	req, _ := http.NewRequest("GET", frontend.URL, nil)
	req.Close = true
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer res.Body.Close() //nolint:errcheck
	if bodyBytes, _ := io.ReadAll(res.Body); string(bodyBytes) != expected {
		t.Errorf("got body %q; expected %q", bodyBytes, expected)
	}
}

func TestInjection(t *testing.T) {
	ci := inject.CopyInject{
		Within:      1024,
		ContentType: "text/html",
		Marker:      regexp.MustCompile(`</head>`),
		Payload:     []byte(`<script src="/injected.js"></script>`),
	}

	const backendBody = `<html><head><title>Test</title></head><body>Hello</body></html>`
	const expectedBody = `<html><head><title>Test</title><script src="/injected.js"></script></head><body>Hello</body></html>`

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(backendBody)))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(backendBody))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	proxyHandler := NewSingleHostReverseProxy(backendURL, ci)
	frontend := httptest.NewServer(proxyHandler)
	defer frontend.Close()

	res, err := http.Get(frontend.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	if string(body) != expectedBody {
		t.Errorf("got body %q; expected %q", string(body), expectedBody)
	}

	// Content-Length should be adjusted for the injected payload
	if res.ContentLength != int64(len(expectedBody)) {
		t.Errorf("got Content-Length %d; expected %d", res.ContentLength, len(expectedBody))
	}
}

func TestInjectionNoMatch(t *testing.T) {
	ci := inject.CopyInject{
		Within:      1024,
		ContentType: "text/html",
		Marker:      regexp.MustCompile(`</head>`),
		Payload:     []byte(`<script src="/injected.js"></script>`),
	}

	const backendBody = `{"key": "value"}`

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(backendBody))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	proxyHandler := NewSingleHostReverseProxy(backendURL, ci)
	frontend := httptest.NewServer(proxyHandler)
	defer frontend.Close()

	res, err := http.Get(frontend.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	if string(body) != backendBody {
		t.Errorf("got body %q; expected %q (no injection for non-HTML)", string(body), backendBody)
	}
}

func TestInjectionLargeBody(t *testing.T) {
	ci := inject.CopyInject{
		Within:      64,
		ContentType: "text/html",
		Marker:      regexp.MustCompile(`</head>`),
		Payload:     []byte(`<script>injected</script>`),
	}

	// Marker is within the sniff window, but body extends well beyond it
	head := `<html><head></head><body>`
	tail := strings.Repeat("x", 4096) + `</body></html>`
	backendBody := head + tail
	expectedBody := `<html><head><script>injected</script></head><body>` + tail

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(backendBody))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	proxyHandler := NewSingleHostReverseProxy(backendURL, ci)
	frontend := httptest.NewServer(proxyHandler)
	defer frontend.Close()

	res, err := http.Get(frontend.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	if string(body) != expectedBody {
		t.Errorf("body length %d; expected %d", len(body), len(expectedBody))
		if len(body) < 200 && len(expectedBody) < 200 {
			t.Errorf("got %q; expected %q", string(body), expectedBody)
		}
	}
}

func TestForwardedHeaders(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Got-Forwarded-Host", r.Header.Get("X-Forwarded-Host"))
		w.Header().Set("X-Got-Forwarded-Proto", r.Header.Get("X-Forwarded-Proto"))
		w.Write([]byte("ok"))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	proxyHandler := NewSingleHostReverseProxy(backendURL, inject.CopyInject{})
	frontend := httptest.NewServer(proxyHandler)
	defer frontend.Close()

	// Without pre-existing forwarded headers, the proxy should set them
	req, _ := http.NewRequest("GET", frontend.URL, nil)
	req.Host = "myapp.example.com"
	req.Close = true
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	res.Body.Close()

	if got := res.Header.Get("X-Got-Forwarded-Host"); got != "myapp.example.com" {
		t.Errorf("X-Forwarded-Host = %q; want %q", got, "myapp.example.com")
	}

	// With pre-existing X-Forwarded-Host, the proxy should preserve it
	req2, _ := http.NewRequest("GET", frontend.URL, nil)
	req2.Header.Set("X-Forwarded-Host", "original.example.com")
	req2.Close = true
	res2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	res2.Body.Close()

	if got := res2.Header.Get("X-Got-Forwarded-Host"); got != "original.example.com" {
		t.Errorf("X-Forwarded-Host = %q; want %q", got, "original.example.com")
	}
}

func TestBasePath(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Got-Path", r.URL.Path)
		w.Write([]byte("ok"))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL + "/base")
	proxyHandler := NewSingleHostReverseProxy(backendURL, inject.CopyInject{})
	frontend := httptest.NewServer(proxyHandler)
	defer frontend.Close()

	req, _ := http.NewRequest("GET", frontend.URL+"/dir", nil)
	req.Close = true
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	res.Body.Close()

	if got, want := res.Header.Get("X-Got-Path"), "/base/dir"; got != want {
		t.Errorf("got path %q; want %q", got, want)
	}
}

func TestBackendError(t *testing.T) {
	// Point at a URL that will refuse connections
	backendURL, _ := url.Parse("http://127.0.0.1:1")
	proxyHandler := NewSingleHostReverseProxy(backendURL, inject.CopyInject{})
	frontend := httptest.NewServer(proxyHandler)
	defer frontend.Close()

	res, err := http.Get(frontend.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	res.Body.Close()

	if res.StatusCode != http.StatusInternalServerError {
		t.Errorf("got status %d; want %d", res.StatusCode, http.StatusInternalServerError)
	}
}
