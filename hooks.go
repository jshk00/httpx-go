package httpxgo

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

func DefaultRequestHook(c *Client, r *Request) error {
	if r.Body != nil && r.isPayloadAllowed() {
		rc, err := handleRequestBody(c, r)
		if err != nil {
			return err
		}
		r.Body = rc
	}
	return buildRequest(c, r)
}

// buildRequest builds the [*Request.RawRequest]
func buildRequest(c *Client, r *Request) error {
	if r.ctx == nil {
		r.ctx = context.Background()
	}
	var (
		req *http.Request
		err error
	)
	body, ok := r.Body.(io.Reader)
	if ok {
		req, err = http.NewRequestWithContext(r.ctx, r.Method, r.URI, body)
	} else {
		req, err = http.NewRequestWithContext(r.ctx, r.Method, r.URI, nil)
	}
	if err != nil {
		return err
	}
	r.RawRequest = req

	// initiate trace once per request if available
	if r.IsTrace || c.trace {
		r.tracer = &TraceInfo{}
		req = req.WithContext(r.tracer.Tracer(req.Context()))
	}

	// Set host, queries and headers
	req.Header = r.Header
	req.URL.RawQuery = r.Queries.Encode()
	if host := req.Header.Get("Host"); host != "" {
		req.Host = host
	}

	r.ctx = req.Context()
	return nil
}

const (
	contentTypeJSON = "application/json"
	contentTypeXML  = "application/xml"
)

// handleRequestBody will handle the automatic encoding of given request body. If the retry is
// desired the body must be replayable and must implement [io.Seeker] interface. In order to
// automatic content type encoding work user must provide correct content type header and
// content type encoder can be registered to support custom content type.
func handleRequestBody(c *Client, r *Request) (io.Reader, error) {
	switch v := r.Body.(type) {
	case io.Reader:
		// Efficient use of bytes.Buffer by converting it into seekable
		if v, ok := v.(*bytes.Buffer); ok {
			return bytes.NewReader(v.Bytes()), nil
		}
		// For retries body must be seekable
		if r.Attempt > 1 {
			if v, ok := v.(io.ReadSeeker); ok {
				v.Seek(0, io.SeekStart)
				return v, nil
			}
			return nil, errors.New("body is not replayable can not be retried")
		}
		return v, nil
	case string:
		return strings.NewReader(v), nil
	case []byte:
		return bytes.NewReader(v), nil
	default:
		if strings.TrimSpace(r.Header.Get("Content-Type")) == "" {
			return nil, errors.New("empty content type cannot encode the body")
		}
		mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			return nil, err
		}
		if mt == contentTypeJSON {
			b, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			return bytes.NewReader(b), nil
		}
		if mt == contentTypeXML {
			b, err := xml.Marshal(v)
			if err != nil {
				return nil, err
			}
			return bytes.NewReader(b), nil
		}
		enc, ok := c.contentTypeEncoders.get(mt)
		if !ok {
			return nil, fmt.Errorf("content type encoder is not found for content type %s", mt)
		}
		return enc(v)
	}
}
