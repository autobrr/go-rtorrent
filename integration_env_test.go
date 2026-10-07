//go:build integration

package rtorrent

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	defaultTestImage = "docker.io/crazymax/rtorrent-rutorrent:latest"

	// rpcPort is where the image serves XML-RPC, peerPort where rTorrent accepts peer connections.
	rpcPort  = "8000/tcp"
	peerPort = 50000

	completeDir = "/downloads/complete"

	fixtureSize = 16 << 20
	pieceLength = 256 << 10

	// downRateLimitKB keeps a fixture download running for about a minute, so tests can observe it in progress.
	downRateLimitKB = 256
)

// fixture is a generated single-file torrent along with its data.
type fixture struct {
	Name    string
	Hash    string
	Size    int
	Torrent []byte
	Data    []byte
}

// testEnv is a private swarm: the rTorrent instance under test, a second instance acting as its peer and an
// in-process HTTP server that serves the fixture torrent and acts as tracker for both.
type testEnv struct {
	client *Client
	peer   *Client

	// download is seeded by the peer for the instance under test to download.
	download fixture
	// upload is for the instance under test to seed to the peer.
	upload fixture

	server *httptest.Server
	main   *testcontainers.DockerContainer
	other  *testcontainers.DockerContainer

	mu    sync.Mutex
	peers []byte
}

// newTestEnv starts the swarm and seeds the download fixture from the peer. The image under test can be
// overridden with RTORRENT_TEST_IMAGE.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	ctx := context.Background()

	image := os.Getenv("RTORRENT_TEST_IMAGE")
	if image == "" {
		image = defaultTestImage
	}

	env := &testEnv{}

	mux := http.NewServeMux()
	mux.HandleFunc("/announce", env.announce)
	env.server = httptest.NewServer(mux)
	t.Cleanup(env.server.Close)

	env.download = newFixture(t, "go-rtorrent-download.bin", env.serverURL()+"/announce")
	env.upload = newFixture(t, "go-rtorrent-upload.bin", env.serverURL()+"/announce")
	mux.HandleFunc("/"+env.download.Name+".torrent", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-bittorrent")
		_, _ = w.Write(env.download.Torrent)
	})

	nw, err := network.New(ctx)
	require.NoError(t, err)
	testcontainers.CleanupNetwork(t, nw)

	env.main = startContainer(t, image, nw, "main", env.serverPort(),
		// ruTorrent's ratio plugin adds its ratio groups as views some time after the container is healthy
		wait.ForHTTP("/RPC2").
			WithPort(rpcPort).
			WithMethod(http.MethodPost).
			WithHeaders(map[string]string{"Content-Type": "text/xml"}).
			WithBody(strings.NewReader(`<?xml version="1.0"?><methodCall><methodName>view.list</methodName><params></params></methodCall>`)).
			WithResponseMatcher(func(body io.Reader) bool {
				b, err := io.ReadAll(body)
				return err == nil && bytes.Contains(b, []byte("rat_1"))
			}).
			WithStartupTimeout(2*time.Minute),
	)
	env.other = startContainer(t, image, nw, "peer", env.serverPort())

	env.client = NewClient(Config{Addr: rpcAddr(t, env.main)})
	env.peer = NewClient(Config{Addr: rpcAddr(t, env.other)})

	var peers []byte
	for _, c := range []*testcontainers.DockerContainer{env.main, env.other} {
		info, err := c.Inspect(ctx)
		require.NoError(t, err)
		ip := net.ParseIP(info.NetworkSettings.Networks[nw.Name].IPAddress.String()).To4()
		require.NotNil(t, ip)
		peers = append(peers, ip...)
		peers = binary.BigEndian.AppendUint16(peers, peerPort)
	}
	env.mu.Lock()
	env.peers = peers
	env.mu.Unlock()

	_, err = env.client.xmlrpcClient.Call(ctx, "throttle.global_down.max_rate.set_kb", "", downRateLimitKB)
	require.NoError(t, err)

	env.seed(t, env.other, env.peer, env.download)

	return env
}

func startContainer(t *testing.T, image string, nw *testcontainers.DockerNetwork, alias string, hostPort int, strategies ...wait.Strategy) *testcontainers.DockerContainer {
	t.Helper()

	c, err := testcontainers.Run(context.Background(), image,
		network.WithNetwork([]string{alias}, nw),
		testcontainers.WithExposedPorts(rpcPort),
		testcontainers.WithHostPortAccess(hostPort),
		testcontainers.WithWaitStrategy(append([]wait.Strategy{wait.ForHealthCheck().WithStartupTimeout(2 * time.Minute)}, strategies...)...),
	)
	testcontainers.CleanupContainer(t, c)
	require.NoError(t, err)

	return c
}

func rpcAddr(t *testing.T, c *testcontainers.DockerContainer) string {
	t.Helper()

	endpoint, err := c.PortEndpoint(context.Background(), rpcPort, "http")
	require.NoError(t, err)

	return endpoint + "/RPC2"
}

func (env *testEnv) serverPort() int {
	_, port, _ := net.SplitHostPort(env.server.Listener.Addr().String())
	p, _ := strconv.Atoi(port)
	return p
}

// serverURL is the address of the in-process server as seen from inside the containers.
func (env *testEnv) serverURL() string {
	return fmt.Sprintf("http://%s:%d", testcontainers.HostInternal, env.serverPort())
}

// downloadURL is where the instance under test can fetch the download fixture's torrent file.
func (env *testEnv) downloadURL() string {
	return env.serverURL() + "/" + env.download.Name + ".torrent"
}

// announce answers with both containers as peers, whatever the torrent. Seeders get no peers, so they never
// connect to an instance that has not added the torrent, which would show up in its transfer totals.
func (env *testEnv) announce(w http.ResponseWriter, r *http.Request) {
	env.mu.Lock()
	peers := env.peers
	env.mu.Unlock()

	if r.URL.Query().Get("left") == "0" {
		peers = nil
	}

	_, _ = w.Write(bencode(map[string]interface{}{
		"interval": 5,
		"peers":    peers,
	}))
}

// seed puts the data of f into the container and adds f to its rTorrent, then waits until it has checked the data.
func (env *testEnv) seed(t *testing.T, c *testcontainers.DockerContainer, client *Client, f fixture) {
	t.Helper()
	ctx := context.Background()

	err := c.CopyToContainer(ctx, f.Data, completeDir+"/"+f.Name, 0o644)
	require.NoError(t, err)

	err = client.AddTorrent(ctx, f.Torrent, Command("d.directory.set", completeDir))
	require.NoError(t, err)

	for i := 0; ; i++ {
		status, err := client.GetStatus(ctx, Torrent{Hash: f.Hash})
		if err == nil && status.Completed {
			return
		}
		if i == 60 {
			require.NoError(t, errors.Errorf("%s was not seeding in time: %v", f.Name, err))
		}
		<-time.After(time.Second)
	}
}

// newFixture generates fixtureSize bytes of random data and a single-file torrent for it.
func newFixture(t *testing.T, name string, announceURL string) fixture {
	t.Helper()

	data := make([]byte, fixtureSize)
	_, err := rand.Read(data)
	require.NoError(t, err)

	var pieces []byte
	for off := 0; off < len(data); off += pieceLength {
		sum := sha1.Sum(data[off:min(off+pieceLength, len(data))])
		pieces = append(pieces, sum[:]...)
	}

	info := bencode(map[string]interface{}{
		"name":         name,
		"length":       len(data),
		"piece length": pieceLength,
		"pieces":       pieces,
	})
	hash := sha1.Sum(info)

	var torrent bytes.Buffer
	torrent.WriteString("d")
	torrent.Write(bencode("announce"))
	torrent.Write(bencode(announceURL))
	torrent.Write(bencode("info"))
	torrent.Write(info)
	torrent.WriteString("e")

	return fixture{
		Name:    name,
		Hash:    strings.ToUpper(hex.EncodeToString(hash[:])),
		Size:    len(data),
		Torrent: torrent.Bytes(),
		Data:    data,
	}
}

// bencode encodes the subset of types used by torrent files and tracker responses.
func bencode(v interface{}) []byte {
	var b bytes.Buffer
	switch v := v.(type) {
	case int:
		fmt.Fprintf(&b, "i%de", v)
	case string:
		fmt.Fprintf(&b, "%d:%s", len(v), v)
	case []byte:
		fmt.Fprintf(&b, "%d:", len(v))
		b.Write(v)
	case map[string]interface{}:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("d")
		for _, k := range keys {
			b.Write(bencode(k))
			b.Write(bencode(v[k]))
		}
		b.WriteString("e")
	default:
		panic(fmt.Sprintf("bencode: unsupported type %T", v))
	}
	return b.Bytes()
}
