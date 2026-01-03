package httpxgo

import (
	"fmt"
	"net/http"
)

type Client struct {
	breaker             *CircuitBreaker
	client              *http.Client
	trace               bool
	decompressors       *decompressors
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
	c.breaker = b
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
	c.client.Jar = jar
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
//	// Example for: Content-Encoding: gzip, zlib
//	func(r io.Reader) (io.ReadCloser, error) {
//	    zr, err := zlib.NewReader(r)   // decode last applied encoding first
//	    if err != nil {
//	        return nil, err
//	    }
//	    gr, err := gzip.NewReader(zr)
//	    if err != nil {
//	        return nil, err
//	    }
//	    return gr, nil
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
func (c *Client) Get(url string) *Request {
	return NewRequest().SetMethod(http.MethodGet).SetURL(url)
}

// Head is http head method follows upto 10 redirect
func (c *Client) Head(url string) *Request {
	return NewRequest().SetMethod(http.MethodHead).SetURL(url)
}

// Post is http post method
func (c *Client) Post(url string, body any) *Request {
	return NewRequest().SetMethod(http.MethodPost).SetURL(url).SetBody(body)
}

// Put is http put method
func (c *Client) Put(url string, body any) *Request {
	return NewRequest().SetMethod(http.MethodPut).SetURL(url).SetBody(body)
}

// Patch is http patch method
func (c *Client) Patch(url string, body any) *Request {
	return NewRequest().SetMethod(http.MethodPost).SetURL(url).SetBody(body)
}

// Delete is http delete method
func (c *Client) Delete(url string) *Request {
	return NewRequest().SetMethod(http.MethodDelete).SetURL(url)
}

func (c *Client) exec(r *Request) (*Response, error) {
	// Execute all the request hooks
	for i := 0; i < len(r.requestHook); i++ {
		if err := r.requestHook[i](c, r); err != nil {
			return nil, fmt.Errorf("failed to execute request hook: %w", err)
		}
	}

	res, err := c.client.Do(r.RawRequest) //nolint:bodyClose
	if err != nil {
		return nil, err
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
	return resp, nil
}
