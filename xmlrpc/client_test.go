package xmlrpc

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient(Config{Addr: srv.URL})
}

func writeResponse(t *testing.T, w http.ResponseWriter, args ...interface{}) {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, Marshal(&buf, "", args...))
	w.Header().Set("Content-Type", "text/xml")
	_, _ = w.Write(buf.Bytes())
}

func TestClient_Call_OK(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		name, params, _, err := Unmarshal(r.Body)
		require.NoError(t, err)
		require.Equal(t, "system.client_version", name)
		require.Empty(t, params)
		require.Equal(t, "text/xml", r.Header.Get("Content-Type"))
		writeResponse(t, w, "0.9.8")
	})

	result, err := c.Call(context.Background(), "system.client_version")
	require.NoError(t, err)
	require.Equal(t, []interface{}{"0.9.8"}, result)
}

func TestClient_Call_UnexpectedStatus(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>\n<head><title>502 Bad Gateway</title></head>\n<body>\n<center><h1>502 Bad Gateway</h1></center>\n<hr><center>nginx</center>\n</body>\n</html>\n"))
	})

	_, err := c.Call(context.Background(), "system.client_version")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected status: 502 Bad Gateway")
	require.Contains(t, err.Error(), "<title>502 Bad Gateway</title>")
	require.NotContains(t, err.Error(), "required token")
}

func TestClient_Call_UnexpectedStatus_TruncatesBody(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(strings.Repeat("x", 10000)))
	})

	_, err := c.Call(context.Background(), "system.client_version")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected status: 500 Internal Server Error")
	require.Less(t, len(err.Error()), 1024)
}

func TestClient_Call_Fault(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeResponse(t, w, Fault{Code: -506, Message: "method 'nope' not defined"})
	})

	_, err := c.Call(context.Background(), "nope")
	require.Error(t, err)
	require.Equal(t, "-506: method 'nope' not defined", err.Error())

	var fault *Fault
	require.True(t, errors.As(err, &fault))
	require.Equal(t, -506, fault.Code)
	require.Equal(t, "method 'nope' not defined", fault.Message)

	// the root package wraps errors with pkg/errors
	wrapped := pkgerrors.Wrap(err, "nope failed")
	require.Equal(t, "nope failed: -506: method 'nope' not defined", wrapped.Error())

	fault = nil
	require.True(t, errors.As(wrapped, &fault))
	require.Equal(t, -506, fault.Code)
}

func TestClient_Call_BasicAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "user" || pass != "pass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		writeResponse(t, w, "ok")
	}))
	t.Cleanup(srv.Close)

	c := NewClient(Config{Addr: srv.URL, BasicUser: "user", BasicPass: "pass"})
	result, err := c.Call(context.Background(), "system.client_version")
	require.NoError(t, err)
	require.Equal(t, []interface{}{"ok"}, result)

	c = NewClient(Config{Addr: srv.URL})
	_, err = c.Call(context.Background(), "system.client_version")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected status: 401 Unauthorized")
}

func TestClient_Call_ContextCanceled(t *testing.T) {
	done := make(chan struct{})
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-done:
		}
	})
	t.Cleanup(func() { close(done) })

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := c.Call(ctx, "system.client_version")
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
}
