package httpxgo

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Request struct {
	respHooks               []ResponseHook
	reqHooks                []RequestHook
	client                  *Client
	tracer                  *TraceInfo
	ctx                     context.Context
	cookie                  *http.Cookie
	retry                   *Retry
	URI                     string
	Queries                 url.Values
	Header                  http.Header
	Body                    any
	Method                  string
	IsTrace                 bool
	IsRetry                 bool
	Attempt                 int
	AllowGetPayload         bool
	AlloweDeletePayload     bool
	AllowNonIdempotentRetry bool
	RawRequest              *http.Request
	TotalTime               time.Duration
}

func NewRequest() *Request {
	return &Request{
		Header:   make(http.Header),
		Queries:  make(url.Values),
		reqHooks: []RequestHook{DefaultRequestHook},
	}
}

func (r *Request) WithContext(ctx context.Context) *Request {
	r.ctx = ctx
	return r
}

func (r *Request) Context() context.Context {
	return r.ctx
}

func (r *Request) SetMethod(v string) *Request {
	r.Method = v
	return r
}

func (r *Request) EnableTrace() *Request {
	r.IsTrace = true
	return r
}

func (r *Request) SetRetry(retry *Retry) *Request {
	if retry == nil {
		retry = NewRetry()
	}
	r.retry = retry
	r.IsRetry = true
	return r
}

func (r *Request) SetBody(v any) *Request {
	r.Body = v
	return r
}

func (r *Request) SetURL(uri string) *Request {
	r.URI = uri
	return r
}

func (r *Request) URL() string {
	return r.URI
}

func (r *Request) SetHeader(k, v string) *Request {
	r.Header.Set(k, v)
	return r
}

func (r *Request) SetCookies(c *http.Cookie) *Request {
	r.cookie = c
	return r
}

func (r *Request) SetHeaders(hdrs map[string]string) *Request {
	for k, v := range hdrs {
		r.SetHeader(k, v)
	}
	return r
}

func (r *Request) SetQuery(k, v string) *Request {
	r.Queries.Set(k, v)
	return r
}

func (r *Request) SetQueries(queries map[string]string) *Request {
	for k, v := range queries {
		r.SetQuery(k, v)
	}
	return r
}

func (r *Request) SetRequestHook(hook RequestHook) *Request {
	r.reqHooks = append(r.reqHooks, hook)
	return r
}

func (r *Request) SetResponseHook(hook ResponseHook) *Request {
	r.respHooks = append(r.respHooks, hook)
	return r
}

func (r *Request) SetAllowGetPayload(b bool) *Request {
	r.AllowGetPayload = b
	return r
}

func (r *Request) SetAllowDeletePayload(b bool) *Request {
	r.AlloweDeletePayload = b
	return r
}

func (r *Request) SetAllowNonIdempotentRetry(b bool) *Request {
	r.AllowNonIdempotentRetry = b
	return r
}

func (r *Request) isIdempotent() bool {
	if r.AllowNonIdempotentRetry {
		return true
	}
	switch r.Method {
	case http.MethodGet,
		http.MethodHead,
		http.MethodTrace,
		http.MethodOptions,
		http.MethodDelete,
		http.MethodPut:
		return true
	}
	return false
}

func (r *Request) isPayloadAllowed() bool {
	switch r.Method {
	case "", http.MethodGet:
		return r.AllowGetPayload
	case http.MethodDelete:
		return r.AlloweDeletePayload
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	}
	return false
}

// Hook execution order:
//
//  1. requestHook — runs before sending the request.
//  2. retry — if enabled, takes full control over retries and
//     determines the final response. In this case, responseHook is
//     NOT invoked.
//  3. responseHook — runs only if no retryHook is defined.
//
// Important:
//
//   - If retry is set (custom or default), responseHook will be ignored.
//     This avoids conflicts from reading res.Body multiple times.
//
//   - When using the default retry, place any post-processing logic
//     (e.g. decoding JSON, logging, validation) in the Cond function itself.
func (r *Request) Exec() (*Response, error) {
	var (
		res *Response
		err error
		now = time.Now()
	)

	// If retry is nil set it because we need retry.Count
	if r.retry == nil {
		r.retry = &Retry{}
	}

	if r.retry.Count < 0 {
		r.retry.Count = 0
	}

Loop:
	for attempt := 0; attempt <= r.retry.Count; attempt++ {
		r.Attempt++
		res, err = r.client.exec(r)
		if err != nil {
			ctxErr := r.Context().Err()
			if ctxErr != nil && errors.Is(ctxErr, context.DeadlineExceeded) {
				break
			}
		}

		if r.Attempt-1 == r.retry.Count && r.isIdempotent() {
			break
		}

		if r.IsRetry {
			// Default condition will always be checked
			needsRetry := defaultRetryCondition(res, err)
			// if default condition is false then execute the user one
			if !needsRetry && r.retry.Cond != nil && res != nil {
				needsRetry = r.retry.Cond(res, err)
			}

			if !needsRetry {
				break
			}

			if res != nil && res.Body != nil {
				_, _ = io.Copy(io.Discard, res.Body)
				res.Body.Close()
			}

			if r.retry.Backoff != nil {
				r.retry.Wait = r.retry.Backoff.NextWaitDuration(res, attempt)
			}

			timer := time.NewTimer(r.retry.Wait)
			select {
			case <-r.Context().Done():
				err = r.Context().Err()
				break Loop
			case <-timer.C:
			}
			timer.Stop()
		}
	}
	r.TotalTime = time.Since(now)
	return res, err
}
