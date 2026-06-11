// Package reverseproxy provides a reverse proxy that wraps the standard
// library's httputil.ReverseProxy with support for content injection (e.g.
// livereload scripts) and contextual logging via termlog.
package reverseproxy

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	"github.com/cortesi/termlog"
	"github.com/llimllib/devd/inject"
)

// NewSingleHostReverseProxy returns an httputil.ReverseProxy that forwards
// requests to target, with content injection controlled by ci.
func NewSingleHostReverseProxy(target *url.URL, ci inject.CopyInject) *httputil.ReverseProxy {
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			// SetURL handles scheme, host, path joining, and query
			// merging (same logic as the old Director).
			pr.SetURL(target)
			pr.Out.Host = target.Host

			// Copy inbound X-Forwarded-For so SetXForwarded appends
			// to existing values (matching the old Director behavior).
			if prior := pr.In.Header["X-Forwarded-For"]; len(prior) > 0 {
				pr.Out.Header["X-Forwarded-For"] = prior
			}
			// SetXForwarded sets X-Forwarded-For, -Host, and -Proto.
			// We call it first for X-Forwarded-For, then override
			// -Host and -Proto to preserve inbound values.
			pr.SetXForwarded()

			// Preserve inbound X-Forwarded-Host/Proto if present;
			// otherwise use the original client request values.
			if fh := pr.In.Header.Get("X-Forwarded-Host"); fh != "" {
				pr.Out.Header.Set("X-Forwarded-Host", fh)
			} else {
				pr.Out.Header.Set("X-Forwarded-Host", pr.In.Host)
			}
			if fp := pr.In.Header.Get("X-Forwarded-Proto"); fp != "" {
				pr.Out.Header.Set("X-Forwarded-Proto", fp)
			} else if pr.In.URL.Scheme != "" {
				pr.Out.Header.Set("X-Forwarded-Proto", pr.In.URL.Scheme)
			}

			// Set "identity"-only content encoding so the injector can
			// work on the uncompressed text response.
			pr.Out.Header.Set("Accept-Encoding", "identity")
		},

		ModifyResponse: func(res *http.Response) error {
			inj, err := ci.Sniff(res.Body, res.Header.Get("Content-Type"))
			if err != nil {
				return fmt.Errorf("inject sniff: %w", err)
			}

			if inj.Found() {
				if cl := res.Header.Get("Content-Length"); cl != "" {
					n, err := strconv.ParseInt(cl, 10, 64)
					if err == nil {
						res.Header.Set("Content-Length", strconv.FormatInt(n+int64(inj.Extra()), 10))
						res.ContentLength = n + int64(inj.Extra())
					}
				}
			}

			// Replace the body with one that reads through the injector.
			origBody := res.Body
			res.Body = &injectReadCloser{inj: inj, origBody: origBody}
			return nil
		},

		FlushInterval: 200 * time.Millisecond,

		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},

		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			ctx := req.Context()
			log := termlog.FromContext(ctx)
			log.Shout("reverse proxy error: %v", err)
			rw.WriteHeader(http.StatusInternalServerError)
		},
	}

	return rp
}

// injectReadCloser is an io.ReadCloser that reads from an inject.Injector
// using an io.Pipe to bridge the Copy(Writer) API to a Read API.
type injectReadCloser struct {
	inj      inject.Injector
	origBody io.ReadCloser
	pr       *io.PipeReader
	pw       *io.PipeWriter
	started  bool
}

func (irc *injectReadCloser) Read(p []byte) (int, error) {
	if !irc.started {
		irc.pr, irc.pw = io.Pipe()
		irc.started = true
		go func() {
			_, err := irc.inj.Copy(irc.pw)
			irc.pw.CloseWithError(err) //nolint:errcheck
		}()
	}
	return irc.pr.Read(p)
}

func (irc *injectReadCloser) Close() error {
	var err error
	if irc.pr != nil {
		err = irc.pr.Close()
	}
	if irc.origBody != nil {
		if cerr := irc.origBody.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}
	return err
}
