package httpxgo

import (
	"errors"
	"net/http"
	"sync/atomic"
	"time"
)

type BreakerConfig struct {
	SuccessThreshold uint32
	FailureThreshold uint32
	Timeout          time.Duration
	TripFunc         func(*http.Response) bool
}

var ErrCircuitBreakerOpen = errors.New("httpx: circuit breaker open")

type CircuitBreakerState int

func (s CircuitBreakerState) String() string {
	return [...]string{"closed", "open", "half-open"}[s]
}

const (
	StateClosed CircuitBreakerState = iota
	StateOpen
	StateHalfOpen
)

// CircuitBreaker is implements circuit breaking pattern for improving system resiliency
// CircuitBreaker is only used as client
type CircuitBreaker struct {
	config        BreakerConfig
	failureCount  atomic.Uint32
	successCount  atomic.Uint32
	state         atomic.Value
	lastFailureAt atomic.Value
}

const (
	defaultFailureThreshold uint32 = 3
	defaultSuccessThreshold uint32 = 1
	defaultTimeout                 = 2 * time.Second
)

func NewCircuitBreaker(config BreakerConfig) *CircuitBreaker {
	if config.FailureThreshold == 0 {
		config.FailureThreshold = defaultFailureThreshold
	}
	if config.SuccessThreshold == 0 {
		config.SuccessThreshold = defaultSuccessThreshold
	}
	if config.Timeout == 0 {
		config.Timeout = defaultTimeout
	}
	if config.TripFunc == nil {
		config.TripFunc = defaultTripFunc
	}
	cb := &CircuitBreaker{config: config}
	cb.state.Store(StateClosed)
	return cb
}

func (cb *CircuitBreaker) Execute(r *http.Response, err error) {
	if cb.config.TripFunc(r) || err != nil {
		cb.OnFailure()
		return
	}
	cb.OnSuccess()
}

func (cb *CircuitBreaker) OnSuccess() {
	switch cb.state.Load() {
	case StateClosed:
		cb.successCount.Add(1)
		if cb.successCount.Load() >= cb.config.SuccessThreshold {
			cb.state.Store(StateClosed)
		}
	case StateHalfOpen:
		cb.failureCount.Store(0)
	}
}

func (cb *CircuitBreaker) OnFailure() {
	switch cb.state.Load() {
	case StateClosed:
		if cb.failureCount.Add(1) >= cb.config.FailureThreshold {
			cb.state.Store(StateOpen)
		}
	case StateHalfOpen:
		cb.lastFailureAt.Store(time.Now().UnixNano())
		cb.state.Store(StateOpen)
	}
}

func (cb *CircuitBreaker) PreRequest() error {
	if cb.state.Load() == StateOpen {
		if time.Since(cb.lastFailureAt.Load().(time.Time)) >= cb.config.Timeout {
			cb.state.Store(StateHalfOpen)
			return nil
		}
		return ErrCircuitBreakerOpen
	}
	return nil
}

func defaultTripFunc(r *http.Response) bool {
	return r.StatusCode > 499
}
