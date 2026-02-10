package httpxgo

import (
	"errors"
	"net/http"
	"sync/atomic"
	"time"
)

var ErrCircuitBreakerOpen = errors.New("httpx: circuit breaker open")

const (
	defaultFailureThreshold uint32 = 3
	defaultSuccessThreshold uint32 = 1
	defaultTimeout                 = 2 * time.Second
)

type CircuitBreakerState int

const (
	StateClosed CircuitBreakerState = iota
	StateOpen
	StateHalfOpen
)

func (s CircuitBreakerState) String() string {
	return [...]string{"closed", "open", "half-open"}[s]
}

type BreakerConfig struct {
	SuccessThreshold uint32
	FailureThreshold uint32
	Timeout          time.Duration
	TripFunc         func(*http.Response) bool
}

func (bc *BreakerConfig) validate() {
	if bc.FailureThreshold == 0 {
		bc.FailureThreshold = defaultFailureThreshold
	}
	if bc.SuccessThreshold == 0 {
		bc.SuccessThreshold = defaultSuccessThreshold
	}
	if bc.Timeout == 0 {
		bc.Timeout = defaultTimeout
	}
	if bc.TripFunc == nil {
		bc.TripFunc = DefaultTripFunc
	}
}

type CircuitBreaker struct {
	config        *BreakerConfig
	failureCount  atomic.Uint32
	successCount  atomic.Uint32
	state         atomic.Value // stores CircuitBreakerState
	lastFailureAt atomic.Int64
}

func NewCircuitBreaker(config *BreakerConfig) *CircuitBreaker {
	if config == nil {
		config = &BreakerConfig{}
	}
	config.validate()

	cb := &CircuitBreaker{config: config}
	cb.state.Store(StateClosed)
	return cb
}

func (cb *CircuitBreaker) getState() CircuitBreakerState {
	return cb.state.Load().(CircuitBreakerState) //nolint
}

func (cb *CircuitBreaker) setState(s CircuitBreakerState) {
	cb.state.Store(s)
}

func (cb *CircuitBreaker) Allow() error {
	if cb.getState() == StateOpen {
		return ErrCircuitBreakerOpen
	}
	return nil
}

func (cb *CircuitBreaker) PostReq(r *http.Response) {
	if cb.config.TripFunc(r) {
		cb.onFailure()
		return
	}
	cb.onSuccess()
}

func (cb *CircuitBreaker) onSuccess() {
	switch cb.getState() {
	case StateClosed:
		if cb.failureCount.Load() > 0 {
			cb.failureCount.Store(0)
		}
	case StateHalfOpen:
		if cb.successCount.Add(1) >= cb.config.SuccessThreshold {
			cb.setState(StateClosed)
			cb.successCount.Store(0)
			cb.failureCount.Store(0)
		}
	}
}

func (cb *CircuitBreaker) onFailure() {
	switch cb.getState() {
	case StateClosed:
		if cb.failureCount.Add(1) >= cb.config.FailureThreshold {
			cb.open()
		}
	case StateHalfOpen:
		cb.open()
	}
	cb.lastFailureAt.Store(time.Now().UnixNano())
}

func (cb *CircuitBreaker) open() {
	cb.setState(StateOpen)
	cb.failureCount.Store(0)
	cb.successCount.Store(0)
	go func() {
		time.Sleep(cb.config.Timeout)
		cb.setState(StateHalfOpen)
	}()
}

func DefaultTripFunc(r *http.Response) bool {
	return r.StatusCode > 499
}
