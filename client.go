package httpxgo

import (
	"fmt"
	"net/http"
	"net/url"
)

type Client struct {
	breaker             *CircuitBreaker
	client              *http.Client
	trace               bool
	decompressors       *contentTypeDecompressor
	contentTypeEncoders *contentTypeEncoders
	contentTypeDecoders *contentTypeDecoders
}

func New() *Client {
	return (&Client{
		client:              &http.Client{},
		decompressors:       newDecompressor(),
		contentTypeEncoders: newContentTypeEncoders(),
		contentTypeDecoders: newContentTypeDecoders(),
	}).SetTransport(defaultTransport)
}

func (c *Client) SetCircuitBreaker(b *CircuitBreaker) *Client {
	if b != nil {
		c.breaker = b
	}
	return c
}

// SetTransport set the httptransport, if provided transport is nil, default transport will be used.
func (c *Client) SetTransport(t http.RoundTripper) *Client {
	if t != nil {
		c.client.Transport = t
	}
	return c
}

func (c *Client) EnableTrace() *Client {
	c.trace = true
	return c
}

// DisableRedirect disable the redirects in http.Client. By default redirect are not disabled and
// follows upto configured redirects in http client.
func (c *Client) DisableRedirect() *Client {
	c.client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return c
}

// SetCookieJar set cookie jar with contained cookies by default no cookie jar is setup
func (c *Client) SetCookieJar(jar http.CookieJar) *Client {
	if jar != nil {
		c.client.Jar = jar
	}
	return c
}

// SetDecompressor registers a decompression function for the given Content-Encoding name. Keys must
// match the value of the Content-Encoding header exactly after trimming spaces.
//
// The default client provides decompressors for "gzip", "deflate", and "zlib". Calling
// SetDecompressor with an existing key overrides the default implementation.
//
// Multi-encoding responses (e.g. "gzip, zlib") are treated as a single logical encoding. The
// library does not attempt to chain multiple encodings internally. If a server sends multiple
// encodings, register a decompressor using the exact header value (e.g. "gzip, zlib") and implement
// the decoding chain inside the provided function in reverse application order:
//
//	type decompressor struct {
//		s io.ReadCloser
//		r io.Reader
//	}
//
//	func (d *decompressor) Read(p []byte) (n int, err error) {
//		return d.r.Read(p)
//	}
//
//	func (d *decompressor) Close() error {
//		return d.s.Close()
//	}
//
//	func decompressCustom(r io.ReadCloser) (io.ReadCloser, error) {
//		zr, err := zlib.NewReader(r)
//		if err != nil {
//			return err
//		}
//		gr, err := gzip.NewReader(zr)
//		if err != nil {
//			return err
//		}
//		return &decompressor{r: gr, s: r}
//	}
//
// Call SetDecompressor multiple times to register additional encodings.
func (c *Client) SetDecompressor(key string, fn DecompressFn) *Client {
	c.decompressors.put(key, fn)
	return c
}

func (c *Client) SetContentTypeEncoder(key string, fn ContentTypeEncFn) *Client {
	c.contentTypeEncoders.set(key, fn)
	return c
}

func (c *Client) SetContentTypeDecoder(key string, fn ContentTypeDecFn) *Client {
	c.contentTypeDecoders.set(key, fn)
	return c
}

// Get is http get method
func (c *Client) Get(uri string) *Request {
	return c.R().SetMethod(http.MethodGet).SetURL(uri)
}

// Head is http head method follows upto 10 redirect
func (c *Client) Head(uri string) *Request {
	return c.R().SetMethod(http.MethodHead).SetURL(uri)
}

// Post is http post method
func (c *Client) Post(uri string, body any) *Request {
	return c.R().SetMethod(http.MethodPost).SetURL(uri).SetBody(body)
}

// Put is http put method
func (c *Client) Put(uri string, body any) *Request {
	return c.R().SetMethod(http.MethodPut).SetURL(uri).SetBody(body)
}

// Patch is http patch method
func (c *Client) Patch(uri string, body any) *Request {
	return c.R().SetMethod(http.MethodPost).SetURL(uri).SetBody(body)
}

// Delete is http delete method
func (c *Client) Delete(uri string) *Request {
	return c.R().SetMethod(http.MethodDelete).SetURL(uri)
}

func (c *Client) R() *Request {
	return &Request{
		Header:   make(http.Header),
		Queries:  make(url.Values),
		reqHooks: []RequestHook{DefaultRequestHook},
		client:   c,
	}
}

func (c *Client) exec(r *Request) (*Response, error) {
	if c.breaker != nil {
		if err := c.breaker.Allow(); err != nil {
			return nil, err
		}
	}

	// Execute all the request hooks
	for i := 0; i < len(r.reqHooks); i++ {
		if err := r.reqHooks[i](c, r); err != nil {
			return nil, fmt.Errorf("failed to execute request hook: %w", err)
		}
	}

	res, err := c.client.Do(r.RawRequest) //nolint:bodyClose
	if err != nil {
		return nil, err
	}
	if c.breaker != nil && res != nil {
		c.breaker.PostReq(res)
	}
	resp := &Response{
		Response:            res,
		traceInfo:           r.tracer,
		decompressors:       c.decompressors,
		contentTypeDecoders: c.contentTypeDecoders,
	}
	if err := resp.wrapDecompressor(); err != nil {
		return nil, err
	}

	// WARN: In case of retry if body is read in in response hooks
	// then reading body in payload based retry condition will case issue.
	for i := 0; i < len(r.respHooks); i++ {
		if err := r.respHooks[i](c, resp); err != nil {
			return nil, fmt.Errorf("failed to execute response hook: %w", err)
		}
	}
	return resp, nil
}
