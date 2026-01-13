package httpxgo

import (
	"crypto/tls"
	"errors"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Retry struct {
	// static wait time between retry. If Backoff is set then wait won't be used
	Wait time.Duration
	// maxmium polling attempts to be performed before failing
	Count int
	// Cond is condition in retry, all the post processing logic should go here such response
	// parsing and status code checks. If Cond return true then request retried if false then retry
	// stops.
	Cond func(*Response, error) bool
	// Backoff will use exponential backoff with jitter if nil static wait will be used
	Backoff *BackoffWithJitter
}

func NewRetry() *Retry {
	return &Retry{
		Count: 10,
		Wait:  20 * time.Second,
	}
}

const (
	defaultWaitTime    = 100 * time.Millisecond
	defaultMaxWaitTime = 3000 * time.Millisecond
)

// JitterStrategy is base type for jitter stratget. Choose suitable jitter strategy
// from this blog https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter
type JitterStrategy int

const (
	WithoutJitter      JitterStrategy = iota // plane exponential backoffs
	FullJitter                               // random_between(base, exponential)
	EqualJitter                              // random_between(base, (exponential / 2))  + (exponential / 2)
	DecorrelatedJitter                       // minium_between(max_wait, random_between(base, prev_wait * 3))
)

type BackoffWithJitter struct {
	min      time.Duration // min wait time between retry
	max      time.Duration // max wait time between retry
	prev     time.Duration // previous time for DecorrelatedJitter strategy
	rnd      *rand.Rand
	strategy JitterStrategy // JitterStrategy
}

func NewBackoffWithJitter(
	minWait, maxWait time.Duration,
	strategy JitterStrategy,
) *BackoffWithJitter {
	if minWait <= 0 {
		minWait = defaultWaitTime
	}
	if maxWait <= 0 {
		maxWait = defaultMaxWaitTime
	}
	return &BackoffWithJitter{
		rnd:      rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), rand.Uint64())),
		min:      minWait,
		max:      maxWait,
		strategy: strategy,
	}
}

// NextWaitDuration return sleep times for retry to sleep
func (b *BackoffWithJitter) NextWaitDuration(
	res *Response,
	attempt int,
) time.Duration {
	if res != nil {
		if res.StatusCode == http.StatusTooManyRequests ||
			res.StatusCode == http.StatusServiceUnavailable {
			if delay, ok := ParseRetryHeader(res.Header.Get("Retry-After")); ok {
				return delay
			}
		}
	}
	// min(cap, base * 2**attempt)
	exp := time.Duration(min(float64(b.max), float64(b.min)*math.Exp2(float64(attempt))))
	return b.balanceMinMax(b.randDuration(exp))
}

// randDuration will return sleep duration base on jitter strategy. If
// jitter strategy is not set only exponential approach will be used
func (b *BackoffWithJitter) randDuration(exp time.Duration) time.Duration {
	if exp <= 0 {
		return time.Nanosecond
	}
	switch b.strategy {
	case FullJitter:
		// random_between(0, exp)
		return time.Duration(b.rnd.Int64N(int64(exp)))
	case EqualJitter:
		// (exp/2 + random_between(0, exp/2))
		half := int64(exp / 2)
		return time.Duration(half + b.rnd.Int64N(half))
	case DecorrelatedJitter:
		// min(cap, random_between(base, prev*3))
		if b.prev == 0 {
			b.prev = b.min
		}
		next := min(b.max, b.min+time.Duration(b.rnd.Int64N(int64((b.prev*3)-b.min))))
		b.prev = next
		return next
	default:
		return exp
	}
}

// balanceMinMax balances the 0 and negatitve values of delay to provided min max wait times.
func (b *BackoffWithJitter) balanceMinMax(delay time.Duration) time.Duration {
	if delay <= 0 || b.max < delay {
		return b.max
	}
	if delay < b.min {
		return b.min
	}
	return delay
}

// ParseRetryHeader parses the Retry-After header sent from server
func ParseRetryHeader(v string) (time.Duration, bool) {
	if strings.TrimSpace(v) == "" {
		return 0, false
	}
	// Retry-After: 120
	if delay, err := strconv.ParseInt(v, 10, 64); err == nil {
		if delay < 0 {
			return 0, false
		}
		return time.Second * time.Duration(delay), true
	}
	// Retry-After: Fri, 31 Dec 1999 23:59:59 GMT
	retryTime, err := time.Parse(time.RFC1123, v)
	if err != nil {
		return 0, false
	}

	if until := time.Until(retryTime); until > 0 {
		return until, true
	}
	// date is in the past
	return 0, true
}

func defaultRetryCondition(res *Response, err error) bool {
	var (
		certErr *tls.CertificateVerificationError
		urlErr  *url.Error
	)

	if errors.As(err, &certErr) {
		return false
	}

	if errors.As(err, &urlErr) {
		errStr := urlErr.Err.Error()
		if strings.Contains(errStr, "redirects") || strings.Contains(errStr, "invalid header") ||
			strings.Contains(errStr, "unsupported protocol scheme") {
			return false
		}
		return urlErr.Temporary()
	}

	if res == nil {
		return false
	}

	if res.StatusCode == http.StatusTooManyRequests ||
		(res.StatusCode >= 500 && res.StatusCode != http.StatusNotImplemented) ||
		res.StatusCode == 0 {
		return true
	}

	return false
}
