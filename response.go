package httpxgo

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

var (
	ErrTraceNotEnabled = errors.New("trace is not enabled")
	ErrBodyIsRead      = errors.New("body is already read")
)

// Response contains embdedd [http.Response] object so all the method of [http.Response] are
// accessible. In any case Response object doesn't close the underlying body it's callers
// responsibility to close the response body[This is done deliberately to avoid double closing and
// keeping Go semantic]. In any case if body is already read and method requring it called will
// throw error.
type Response struct {
	*http.Response
	traceInfo           *TraceInfo
	decompressors       *decompressors
	contentTypeDecoders *contentTypeDecoders
	// This set body to already read so can not be read further
	IsRead bool
}

// Success checks wether the response status code is in positive range.
func (r *Response) Success() bool {
	return r.StatusCode > 199 && r.StatusCode < 300
}

func (r *Response) TraceInfo() (*TraceInfo, error) {
	if r.traceInfo == nil {
		return nil, ErrTraceNotEnabled
	}
	return r.traceInfo, nil
}

// Decode will decode given value based on [DecodeOptions] if none provided default will be
// [JSONDecoder]. Make sure body should be pointer to variable you're trying to decode.
//
// WARN: As Decode will store bytes in memory avoid reading large responses.
func (r *Response) Decode(v any) error {
	if r.IsRead {
		return ErrBodyIsRead
	}
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return err
	}
	dec, ok := r.contentTypeDecoders.get(mt)
	if !ok {
		return fmt.Errorf("content type decoder not found for content %s", mt)
	}
	r.IsRead = true
	return dec(v, r.Body)
}

func (r *Response) Bytes() ([]byte, error) {
	if r.IsRead {
		return nil, ErrBodyIsRead
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading the body, err: %w", err)
	}
	r.IsRead = true
	return b, nil
}

// wrapDecompressor decompresses well known format such as gzip, x-gzip, deflate. Other widely used
// format such as brotli, zstd or custom you can set decompressor using client.
func (r *Response) wrapDecompressor() error {
	if r.IsRead {
		return ErrBodyIsRead
	}

	v := strings.TrimSpace(r.Header.Get("Content-Encoding"))
	if v == "" || v == "identity" {
		return nil
	}

	fn, ok := r.decompressors.get(v)
	if !ok {
		return fmt.Errorf("decompressor not found for %s", v)
	}
	dec, err := fn(r.Body)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	r.Body = dec
	r.Header.Del("Content-Encoding")
	r.Header.Del("Content-Length")
	r.ContentLength = -1
	return nil
}

// MultiReadBody Provides body which can auto reset when it hits [io.EOF] error.
func (r *Response) MultiReadBody() (*MultiReadCloser, error) {
	b, err := r.Bytes()
	if err != nil {
		return nil, err
	}
	return &MultiReadCloser{bytes.NewReader(b)}, nil
}

// MultiReadCloser automatically reset the read buffer after reading is complete, Essentially making
// it infinite reader.
type MultiReadCloser struct {
	br *bytes.Reader
}

// Read implments [io.Reader] interface.
func (r *MultiReadCloser) Read(p []byte) (int, error) {
	n, err := r.br.Read(p)
	if err == io.EOF {
		r.br.Seek(0, io.SeekStart)
	}
	return n, err
}

func (r *MultiReadCloser) Close() error {
	return nil
}
