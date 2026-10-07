package rtorrent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/autobrr/go-rtorrent/xmlrpc"

	"github.com/stretchr/testify/require"
)

type call struct {
	method string
	params []interface{}
}

// fakeServer answers XML-RPC calls with the response registered for the method and records every call it receives.
type fakeServer struct {
	t         *testing.T
	responses map[string]interface{}
	calls     []call
}

func newFakeServer(t *testing.T, responses map[string]interface{}) (*Client, *fakeServer) {
	t.Helper()

	fs := &fakeServer{t: t, responses: responses}
	srv := httptest.NewServer(fs)
	t.Cleanup(srv.Close)

	return NewClient(Config{Addr: srv.URL}), fs
}

func (fs *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	method, params, _, err := xmlrpc.Unmarshal(r.Body)
	if err != nil {
		fs.t.Errorf("unmarshal request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fs.calls = append(fs.calls, call{method: method, params: params})

	var resp interface{}
	if method == "system.multicall" {
		resp = fs.multicall(params)
	} else {
		resp = fs.response(method)
	}

	var args []interface{}
	if _, ok := resp.(noParams); !ok {
		args = append(args, resp)
	}

	w.Header().Set("Content-Type", "text/xml")
	if err := xmlrpc.Marshal(w, "", args...); err != nil {
		fs.t.Errorf("marshal response: %v", err)
	}
}

// noParams makes the fake server answer with a response that has no params at all.
type noParams struct{}

func (fs *fakeServer) response(method string) interface{} {
	resp, ok := fs.responses[method]
	if !ok {
		return xmlrpc.Fault{Code: -506, Message: "method '" + method + "' not defined"}
	}
	return resp
}

// multicall answers every call of a system.multicall request from the responses, the way rTorrent does: a
// one-element array per successful call and a fault struct per failed one.
func (fs *fakeServer) multicall(params []interface{}) interface{} {
	if resp, ok := fs.responses["system.multicall"]; ok {
		return resp
	}
	if len(params) != 1 {
		fs.t.Errorf("system.multicall: want 1 param, got %d", len(params))
		return xmlrpc.Fault{Code: -501, Message: "invalid parameters"}
	}
	calls, ok := params[0].([]interface{})
	if !ok {
		fs.t.Errorf("system.multicall: param isn't array: %v", params[0])
		return xmlrpc.Fault{Code: -501, Message: "invalid parameters"}
	}

	results := make([]interface{}, 0, len(calls))
	for _, c := range calls {
		m, _ := c.(map[string]interface{})
		method, _ := m["methodName"].(string)

		switch resp := fs.response(method).(type) {
		case xmlrpc.Fault:
			results = append(results, map[string]interface{}{"faultCode": resp.Code, "faultString": resp.Message})
		case noParams:
			results = append(results, []interface{}{})
		default:
			results = append(results, []interface{}{resp})
		}
	}
	return results
}

func TestFieldValue_String(t *testing.T) {
	tests := []struct {
		name string
		fv   *FieldValue
		want string
	}{
		{name: "set value", fv: DLabel.SetValue("tv"), want: `d.custom1.set="tv"`},
		{name: "priority", fv: DPriority.SetValue("3"), want: `d.priority.set="3"`},
		{name: "command", fv: Command("view.set_visible", "rat_1"), want: `view.set_visible="rat_1"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.fv.String())
		})
	}
}

func TestField_Query(t *testing.T) {
	require.Equal(t, "d.name=", DName.Query())
	require.Equal(t, "d.name", DName.Cmd())
}

func TestClient_Add(t *testing.T) {
	data := []byte("d8:announce0:e")

	tests := []struct {
		name   string
		method string
		add    func(c *Client, extra ...*FieldValue) error
		target interface{}
	}{
		{
			name:   "url started",
			method: "load.start",
			add: func(c *Client, extra ...*FieldValue) error {
				return c.Add(context.Background(), "https://example.com/a.torrent", extra...)
			},
			target: []byte("https://example.com/a.torrent"),
		},
		{
			name:   "url stopped",
			method: "load.normal",
			add: func(c *Client, extra ...*FieldValue) error {
				return c.AddStopped(context.Background(), "https://example.com/a.torrent", extra...)
			},
			target: []byte("https://example.com/a.torrent"),
		},
		{
			name:   "raw started",
			method: "load.raw_start",
			add: func(c *Client, extra ...*FieldValue) error {
				return c.AddTorrent(context.Background(), data, extra...)
			},
			target: data,
		},
		{
			name:   "raw stopped",
			method: "load.raw",
			add: func(c *Client, extra ...*FieldValue) error {
				return c.AddTorrentStopped(context.Background(), data, extra...)
			},
			target: data,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, fs := newFakeServer(t, map[string]interface{}{tt.method: 0})

			err := tt.add(client, DLabel.SetValue("tv"), Command("view.set_visible", "rat_1"))
			require.NoError(t, err)

			require.Len(t, fs.calls, 1)
			require.Equal(t, tt.method, fs.calls[0].method)
			// Extra commands must be separate params, not wrapped in an array: tinyxml2 builds of rTorrent reject that.
			require.Equal(t, []interface{}{"", tt.target, `d.custom1.set="tv"`, `view.set_visible="rat_1"`}, fs.calls[0].params)
		})
	}
}

func TestClient_Add_Fault(t *testing.T) {
	client, _ := newFakeServer(t, map[string]interface{}{
		"load.start": xmlrpc.Fault{Code: -503, Message: "Could not create download"},
	})

	err := client.Add(context.Background(), "https://example.com/a.torrent")
	require.ErrorContains(t, err, "load.start XMLRPC call failed")
	require.ErrorContains(t, err, "Could not create download")
}

func TestClient_Totals(t *testing.T) {
	client, _ := newFakeServer(t, map[string]interface{}{
		"throttle.global_down.total": 1024,
		"throttle.global_up.total":   512,
		"throttle.global_down.rate":  64,
		"throttle.global_up.rate":    32,
	})
	ctx := context.Background()

	down, err := client.DownTotal(ctx)
	require.NoError(t, err)
	require.Equal(t, 1024, down)

	up, err := client.UpTotal(ctx)
	require.NoError(t, err)
	require.Equal(t, 512, up)

	downRate, err := client.DownRate(ctx)
	require.NoError(t, err)
	require.Equal(t, 64, downRate)

	upRate, err := client.UpRate(ctx)
	require.NoError(t, err)
	require.Equal(t, 32, upRate)
}

func TestClient_Views(t *testing.T) {
	client, fs := newFakeServer(t, map[string]interface{}{
		"view.list": []interface{}{"main", "default", "rat_0", "rat_1"},
	})

	views, err := client.Views(context.Background())
	require.NoError(t, err)
	require.Equal(t, []View{ViewMain, "default", "rat_0", "rat_1"}, views)
	require.Equal(t, "view.list", fs.calls[0].method)
}

func TestClient_Views_NonString(t *testing.T) {
	client, _ := newFakeServer(t, map[string]interface{}{
		"view.list": []interface{}{"main", 1},
	})

	_, err := client.Views(context.Background())
	require.ErrorContains(t, err, "view name isn't string")
}

func TestClient_GetTorrents(t *testing.T) {
	client, fs := newFakeServer(t, map[string]interface{}{
		"d.multicall2": []interface{}{
			[]interface{}{"name-1", 100, "HASH1", "tv", "/downloads/one", 1, 1, 1500, 1700000000, 1700000200, 1700000100},
			[]interface{}{"name-2", 200, "HASH2", "", "/downloads/two", 0, 0, 0, 1700000000, 0, 0},
		},
	})

	torrents, err := client.GetTorrents(context.Background(), ViewSeeding)
	require.NoError(t, err)
	require.Equal(t, []Torrent{
		{
			Hash:      "HASH1",
			Name:      "name-1",
			Path:      "/downloads/one",
			Size:      100,
			Label:     "tv",
			Completed: true,
			Ratio:     1.5,
			Created:   time.Unix(1700000000, 0),
			Finished:  time.Unix(1700000200, 0),
			Started:   time.Unix(1700000100, 0),
		},
		{
			Hash:     "HASH2",
			Name:     "name-2",
			Path:     "/downloads/two",
			Size:     200,
			Created:  time.Unix(1700000000, 0),
			Finished: time.Unix(0, 0),
			Started:  time.Unix(0, 0),
		},
	}, torrents)

	require.Equal(t, "", fs.calls[0].params[0])
	require.Equal(t, "seeding", fs.calls[0].params[1])
}

func TestClient_GetTorrent(t *testing.T) {
	client, _ := newFakeServer(t, map[string]interface{}{
		"d.name":               "name-1",
		"d.size_bytes":         100,
		"d.custom1":            "tv",
		"d.directory":          "/downloads/one",
		"d.complete":           1,
		"d.ratio":              1500,
		"d.creation_date":      1700000000,
		"d.timestamp.finished": 1700000200,
		"d.timestamp.started":  1700000100,
	})

	torrent, err := client.GetTorrent(context.Background(), "HASH1")
	require.NoError(t, err)
	require.Equal(t, Torrent{
		Hash:      "HASH1",
		Name:      "name-1",
		Path:      "/downloads/one",
		Size:      100,
		Label:     "tv",
		Completed: true,
		Ratio:     1.5,
		Created:   time.Unix(1700000000, 0),
		Finished:  time.Unix(1700000200, 0),
		Started:   time.Unix(1700000100, 0),
	}, torrent)
}

func TestClient_GetFiles(t *testing.T) {
	client, fs := newFakeServer(t, map[string]interface{}{
		"f.multicall": []interface{}{
			[]interface{}{"a.mkv", 100},
			[]interface{}{"b.nfo", 2},
		},
	})

	files, err := client.GetFiles(context.Background(), Torrent{Hash: "HASH1"})
	require.NoError(t, err)
	require.Equal(t, []File{{Path: "a.mkv", Size: 100}, {Path: "b.nfo", Size: 2}}, files)
	require.Equal(t, []interface{}{"HASH1", 0, "f.path=", "f.size_bytes="}, fs.calls[0].params)
}

func TestClient_GetStatus(t *testing.T) {
	client, _ := newFakeServer(t, map[string]interface{}{
		"d.complete":        0,
		"d.completed_bytes": 50,
		"d.down.rate":       10,
		"d.up.rate":         5,
		"d.ratio":           250,
		"d.size_bytes":      100,
	})

	status, err := client.GetStatus(context.Background(), Torrent{Hash: "HASH1"})
	require.NoError(t, err)
	require.Equal(t, Status{CompletedBytes: 50, DownRate: 10, UpRate: 5, Ratio: 0.25, Size: 100}, status)
}

func TestClient_SetLabel(t *testing.T) {
	client, fs := newFakeServer(t, map[string]interface{}{"d.custom1.set": "tv"})

	err := client.SetLabel(context.Background(), Torrent{Hash: "HASH1"}, "tv")
	require.NoError(t, err)
	require.Equal(t, call{method: "d.custom1.set", params: []interface{}{"HASH1", "tv"}}, fs.calls[0])
}

func TestClient_TorrentCommands(t *testing.T) {
	tests := []struct {
		method string
		run    func(c *Client, ctx context.Context, t Torrent) error
	}{
		{method: "d.start", run: (*Client).StartTorrent},
		{method: "d.stop", run: (*Client).StopTorrent},
		{method: "d.close", run: (*Client).CloseTorrent},
		{method: "d.open", run: (*Client).OpenTorrent},
		{method: "d.pause", run: (*Client).PauseTorrent},
		{method: "d.resume", run: (*Client).ResumeTorrent},
		{method: "d.erase", run: (*Client).Delete},
	}
	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			client, fs := newFakeServer(t, map[string]interface{}{tt.method: 0})

			err := tt.run(client, context.Background(), Torrent{Hash: "HASH1"})
			require.NoError(t, err)
			require.Equal(t, call{method: tt.method, params: []interface{}{"HASH1"}}, fs.calls[0])
		})
	}
}

func TestClient_BasicAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "user" || pass != "pass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = xmlrpc.Marshal(w, "", "host")
	}))
	t.Cleanup(srv.Close)

	client := NewClient(Config{Addr: srv.URL, BasicUser: "user", BasicPass: "pass"})

	name, err := client.Name(context.Background())
	require.NoError(t, err)
	require.Equal(t, "host", name)
}

func TestClient_WithHTTPClient_BasicAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "user" || pass != "pass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = xmlrpc.Marshal(w, "", "host")
	}))
	t.Cleanup(srv.Close)

	tests := []struct {
		name   string
		client func(cfg Config, hc *http.Client) *Client
	}{
		{
			name: "NewClient WithHTTPClient",
			client: func(cfg Config, hc *http.Client) *Client {
				return NewClient(cfg).WithHTTPClient(hc)
			},
		},
		{
			name: "NewClientWithOpts WithHTTPClient",
			client: func(cfg Config, hc *http.Client) *Client {
				return NewClientWithOpts(cfg).WithHTTPClient(hc)
			},
		},
		{
			name: "WithCustomClient",
			client: func(cfg Config, hc *http.Client) *Client {
				return NewClientWithOpts(cfg, WithCustomClient(hc))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.client(Config{Addr: srv.URL, BasicUser: "user", BasicPass: "pass"}, &http.Client{Timeout: time.Second})

			name, err := client.Name(context.Background())
			require.NoError(t, err)
			require.Equal(t, "host", name)
		})
	}
}

func TestClient_GetTorrent_Multicall(t *testing.T) {
	client, fs := newFakeServer(t, map[string]interface{}{
		"d.name":               "name-1",
		"d.size_bytes":         100,
		"d.custom1":            "tv",
		"d.directory":          "/downloads/one",
		"d.complete":           1,
		"d.ratio":              1500,
		"d.creation_date":      1700000000,
		"d.timestamp.finished": 1700000200,
		"d.timestamp.started":  1700000100,
	})

	_, err := client.GetTorrent(context.Background(), "HASH1")
	require.NoError(t, err)

	require.Len(t, fs.calls, 1)
	require.Equal(t, "system.multicall", fs.calls[0].method)
	require.Equal(t, []interface{}{multicallParams("HASH1",
		"d.name", "d.size_bytes", "d.custom1", "d.directory", "d.complete", "d.ratio",
		"d.creation_date", "d.timestamp.finished", "d.timestamp.started",
	)}, fs.calls[0].params)
}

func TestClient_GetStatus_Multicall(t *testing.T) {
	client, fs := newFakeServer(t, map[string]interface{}{
		"d.complete":        1,
		"d.completed_bytes": 50,
		"d.down.rate":       10,
		"d.up.rate":         5,
		"d.ratio":           250,
		"d.size_bytes":      100,
	})

	status, err := client.GetStatus(context.Background(), Torrent{Hash: "HASH1"})
	require.NoError(t, err)
	require.True(t, status.Completed)

	require.Len(t, fs.calls, 1)
	require.Equal(t, "system.multicall", fs.calls[0].method)
	require.Equal(t, []interface{}{multicallParams("HASH1",
		"d.complete", "d.completed_bytes", "d.down.rate", "d.up.rate", "d.ratio", "d.size_bytes",
	)}, fs.calls[0].params)
}

func multicallParams(hash string, methods ...string) []interface{} {
	calls := make([]interface{}, 0, len(methods))
	for _, m := range methods {
		calls = append(calls, map[string]interface{}{"methodName": m, "params": []interface{}{hash}})
	}
	return calls
}

func TestClient_Multicall_Fault(t *testing.T) {
	client, _ := newFakeServer(t, map[string]interface{}{
		"d.complete":        0,
		"d.completed_bytes": 50,
		"d.down.rate":       10,
		"d.up.rate":         xmlrpc.Fault{Code: -501, Message: "invalid"},
		"d.ratio":           250,
		"d.size_bytes":      100,
	})

	_, err := client.GetStatus(context.Background(), Torrent{Hash: "HASH1"})
	require.ErrorContains(t, err, "d.up.rate XMLRPC call failed")

	var fault *xmlrpc.Fault
	require.True(t, errors.As(err, &fault))
	require.Equal(t, -501, fault.Code)
}

func TestClient_UnexpectedResponses(t *testing.T) {
	ctx := context.Background()
	torrent := Torrent{Hash: "HASH1"}

	// torrentResponses answers every call GetTorrent and GetStatus make; a nil override removes the method.
	torrentResponses := func(override map[string]interface{}) map[string]interface{} {
		responses := map[string]interface{}{
			"d.name":               "name-1",
			"d.size_bytes":         100,
			"d.custom1":            "tv",
			"d.directory":          "/downloads/one",
			"d.complete":           1,
			"d.completed_bytes":    50,
			"d.down.rate":          10,
			"d.up.rate":            5,
			"d.ratio":              1500,
			"d.creation_date":      1700000000,
			"d.timestamp.finished": 1700000200,
			"d.timestamp.started":  1700000100,
		}
		for k, v := range override {
			if v == nil {
				delete(responses, k)
				continue
			}
			responses[k] = v
		}
		return responses
	}

	ip := func(c *Client) error { _, err := c.IP(ctx); return err }
	name := func(c *Client) error { _, err := c.Name(ctx); return err }
	downTotal := func(c *Client) error { _, err := c.DownTotal(ctx); return err }
	downRate := func(c *Client) error { _, err := c.DownRate(ctx); return err }
	upTotal := func(c *Client) error { _, err := c.UpTotal(ctx); return err }
	upRate := func(c *Client) error { _, err := c.UpRate(ctx); return err }
	isActive := func(c *Client) error { _, err := c.IsActive(ctx, torrent); return err }
	isOpen := func(c *Client) error { _, err := c.IsOpen(ctx, torrent); return err }
	state := func(c *Client) error { _, err := c.State(ctx, torrent); return err }
	getTorrent := func(c *Client) error { _, err := c.GetTorrent(ctx, "HASH1"); return err }
	getStatus := func(c *Client) error { _, err := c.GetStatus(ctx, torrent); return err }
	getTorrents := func(c *Client) error { _, err := c.GetTorrents(ctx, ViewMain); return err }
	getFiles := func(c *Client) error { _, err := c.GetFiles(ctx, torrent); return err }

	tests := []struct {
		name      string
		responses map[string]interface{}
		run       func(c *Client) error
		wantErr   string
	}{
		{name: "IP empty", responses: map[string]interface{}{"network.bind_address": noParams{}}, run: ip, wantErr: "network.bind_address"},
		{name: "IP wrong type", responses: map[string]interface{}{"network.bind_address": 1}, run: ip, wantErr: "isn't string"},
		{name: "Name empty", responses: map[string]interface{}{"system.hostname": noParams{}}, run: name, wantErr: "system.hostname"},
		{name: "DownTotal empty", responses: map[string]interface{}{"throttle.global_down.total": noParams{}}, run: downTotal, wantErr: "throttle.global_down.total"},
		{name: "DownTotal wrong type", responses: map[string]interface{}{"throttle.global_down.total": "1"}, run: downTotal, wantErr: "isn't int"},
		{name: "DownRate empty", responses: map[string]interface{}{"throttle.global_down.rate": noParams{}}, run: downRate, wantErr: "throttle.global_down.rate"},
		{name: "UpTotal empty", responses: map[string]interface{}{"throttle.global_up.total": noParams{}}, run: upTotal, wantErr: "throttle.global_up.total"},
		{name: "UpRate empty", responses: map[string]interface{}{"throttle.global_up.rate": noParams{}}, run: upRate, wantErr: "throttle.global_up.rate"},
		{name: "IsActive empty", responses: map[string]interface{}{"d.is_active": noParams{}}, run: isActive, wantErr: "d.is_active"},
		{name: "IsActive wrong type", responses: map[string]interface{}{"d.is_active": "yes"}, run: isActive, wantErr: "isn't int"},
		{name: "IsOpen empty", responses: map[string]interface{}{"d.is_open": noParams{}}, run: isOpen, wantErr: "d.is_open"},
		{name: "State wrong type", responses: map[string]interface{}{"d.state": []interface{}{1}}, run: state, wantErr: "isn't int"},
		{name: "GetTorrents empty", responses: map[string]interface{}{"d.multicall2": noParams{}}, run: getTorrents, wantErr: "d.multicall2"},
		{name: "GetTorrents row isn't list", responses: map[string]interface{}{"d.multicall2": []interface{}{"name-1"}}, run: getTorrents, wantErr: "isn't list"},
		{name: "GetTorrents short row", responses: map[string]interface{}{"d.multicall2": []interface{}{[]interface{}{"name-1", 100}}}, run: getTorrents, wantErr: "d.hash"},
		{name: "GetTorrents wrong type", responses: map[string]interface{}{"d.multicall2": []interface{}{[]interface{}{"name-1", "100", "HASH1", "", "/", 0, 0, 0, 0, 0, 0}}}, run: getTorrents, wantErr: "d.size_bytes"},
		{name: "GetFiles empty", responses: map[string]interface{}{"f.multicall": noParams{}}, run: getFiles, wantErr: "f.multicall"},
		{name: "GetFiles short row", responses: map[string]interface{}{"f.multicall": []interface{}{[]interface{}{"a.mkv"}}}, run: getFiles, wantErr: "f.size_bytes"},
		{name: "GetFiles wrong type", responses: map[string]interface{}{"f.multicall": []interface{}{[]interface{}{1, 100}}}, run: getFiles, wantErr: "f.path"},
		{name: "GetTorrent fault", responses: torrentResponses(map[string]interface{}{"d.ratio": nil}), run: getTorrent, wantErr: "d.ratio XMLRPC call failed"},
		{name: "GetTorrent empty entry", responses: torrentResponses(map[string]interface{}{"d.custom1": noParams{}}), run: getTorrent, wantErr: "d.custom1"},
		{name: "GetTorrent wrong type", responses: torrentResponses(map[string]interface{}{"d.size_bytes": "100"}), run: getTorrent, wantErr: "d.size_bytes"},
		{name: "GetTorrent multicall empty", responses: map[string]interface{}{"system.multicall": noParams{}}, run: getTorrent, wantErr: "system.multicall"},
		{name: "GetTorrent multicall too few entries", responses: map[string]interface{}{"system.multicall": []interface{}{[]interface{}{"name-1"}}}, run: getTorrent, wantErr: "system.multicall"},
		{name: "GetStatus fault", responses: torrentResponses(map[string]interface{}{"d.up.rate": xmlrpc.Fault{Code: -501, Message: "invalid"}}), run: getStatus, wantErr: "d.up.rate XMLRPC call failed: -501: invalid"},
		{name: "GetStatus wrong type", responses: torrentResponses(map[string]interface{}{"d.complete": "yes"}), run: getStatus, wantErr: "d.complete"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newFakeServer(t, tt.responses)

			var err error
			require.NotPanics(t, func() { err = tt.run(client) })
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}
