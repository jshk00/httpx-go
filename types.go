package httpxgo

import (
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"
	"sync"
)

type (
	ResponseHook     func(*Client, *Response) error
	RequestHook      func(*Client, *Request) error
	ContentTypeEncFn func(body any) (io.Reader, error)
	ContentTypeDecFn func(body any, r io.Reader) error
	DecompressFn     func(io.ReadCloser) (io.ReadCloser, error)
)

type contentTypeEncoders struct {
	mu  sync.RWMutex
	enc map[string]ContentTypeEncFn
}

func newContentTypeEncoders() *contentTypeEncoders {
	return &contentTypeEncoders{enc: make(map[string]ContentTypeEncFn)}
}

func (ce *contentTypeEncoders) set(key string, fn ContentTypeEncFn) {
	ce.mu.Lock()
	ce.enc[key] = fn
	ce.mu.Unlock()
}

func (ce *contentTypeEncoders) get(key string) (ContentTypeEncFn, bool) {
	ce.mu.RLock()
	fn, ok := ce.enc[key]
	ce.mu.RUnlock()
	return fn, ok
}

type contentTypeDecoders struct {
	mu  sync.RWMutex
	dec map[string]ContentTypeDecFn
}

func newContentTypeDecoders() *contentTypeDecoders {
	return &contentTypeDecoders{dec: make(map[string]ContentTypeDecFn)}
}

func (ce *contentTypeDecoders) set(key string, fn ContentTypeDecFn) {
	ce.mu.Lock()
	ce.dec[key] = fn
	ce.mu.Unlock()
}

func (ce *contentTypeDecoders) get(key string) (ContentTypeDecFn, bool) {
	ce.mu.RLock()
	fn, ok := ce.dec[key]
	ce.mu.RUnlock()
	return fn, ok
}

// decompressors is concurrent safe map of decompression function.
// It already has gzip, delfate and zlib. User can override it as well.
type decompressors struct {
	mu   sync.RWMutex
	data map[string]DecompressFn
}

func newDecompressor() *decompressors {
	return &decompressors{
		data: map[string]DecompressFn{
			"gzip":    decompressGzip,
			"deflate": decompressFlate,
			"zlib":    decompressZlib,
		},
	}
}

func (ds *decompressors) put(key string, fn DecompressFn) {
	ds.mu.Lock()
	ds.data[key] = fn
	ds.mu.Unlock()
}

func (ds *decompressors) get(key string) (fn DecompressFn, ok bool) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	fn, ok = ds.data[key]
	return fn, ok
}

type decompressor struct {
	s io.ReadCloser
	r io.Reader
}

func (d *decompressor) Read(p []byte) (int, error) {
	return d.r.Read(p)
}

func (d *decompressor) Close() error {
	return d.s.Close()
}

func decompressGzip(r io.ReadCloser) (io.ReadCloser, error) {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	return &decompressor{
		s: r,
		r: gr,
	}, nil
}

func decompressFlate(r io.ReadCloser) (io.ReadCloser, error) {
	return &decompressor{s: r, r: flate.NewReader(r)}, nil
}

func decompressZlib(r io.ReadCloser) (io.ReadCloser, error) {
	zr, err := zlib.NewReader(r)
	if err != nil {
		return nil, err
	}
	return &decompressor{s: r, r: zr}, nil
}
