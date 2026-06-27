package httpxgo

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCircuitBreaker(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Method: %v", r.Method)
		t.Logf("Path: %v", r.URL.Path)

		switch r.URL.Path {
		case "/200":
			w.WriteHeader(http.StatusOK)
			return
		case "/500":
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}))
	defer ts.Close()
	cb := NewCircuitBreaker(nil)
	c := New().SetCircuitBreaker(cb)
	for range defaultFailureThreshold {
		_, err := c.Get(ts.URL + "/500").Exec()
		assertNil(t, err)
	}
	resp, err := c.Get(ts.URL + "/500").Exec()
	assertErrorIs(t, ErrCircuitBreakerOpen, err)
	assertNil(t, resp)
	assertEqual(
		t,
		StateOpen,
		cb.state.Load(),
		"expected open state after reaching failure threshold",
	)
	time.Sleep(defaultTimeout)
	assertEqual(
		t,
		StateHalfOpen,
		cb.state.Load(),
		"expected half-open state",
	)
}

func assertErrorIs(t *testing.T, e, g error, failureMsgs ...string) (r bool) {
	t.Helper()
	if !errors.Is(g, e) {
		t.Errorf("Expected [%v], got [%v]. Message: %v", e, g, strings.Join(failureMsgs, " "))
	}

	return true
}

func assertNil(t *testing.T, v any, failureMsgs ...string) {
	t.Helper()
	if !isNil(v) {
		t.Errorf("[%v] was expected to be nil. Message: %v", v, strings.Join(failureMsgs, " "))
	}
}

func isNil(v any) bool {
	if v == nil {
		return true
	}

	rv := reflect.ValueOf(v)
	kind := rv.Kind()
	if kind >= reflect.Chan && kind <= reflect.Slice && rv.IsNil() {
		return true
	}

	return false
}

func assertEqual(t *testing.T, e, g any, failureMsgs ...string) (r bool) {
	t.Helper()
	if !reflect.DeepEqual(e, g) {
		t.Errorf("Expected [%v], got [%v]. Message: %v", e, g, strings.Join(failureMsgs, " "))
	}

	return r
}
