//go:build integration

package rtorrent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const maxRetries = 60

func TestRTorrent(t *testing.T) {
	// These tests start their own rTorrent containers with Docker, see newTestEnv.
	env := newTestEnv(t)
	client := env.client
	download := env.download

	ctx := context.Background()

	t.Run("get ip", func(t *testing.T) {
		_, err := client.IP(ctx)
		require.NoError(t, err)
		// Don't assert anything about the response, differs based upon the environment
	})

	t.Run("get name", func(t *testing.T) {
		name, err := client.Name(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, name)
	})

	t.Run("down total", func(t *testing.T) {
		total, err := client.DownTotal(ctx)
		require.NoError(t, err)
		require.Zero(t, total, "expected no data to be transferred yet")
	})

	t.Run("up total", func(t *testing.T) {
		total, err := client.UpTotal(ctx)
		require.NoError(t, err)
		require.Zero(t, total, "expected no data to be transferred yet")
	})

	t.Run("down rate", func(t *testing.T) {
		rate, err := client.DownRate(ctx)
		require.NoError(t, err)
		require.Zero(t, rate, "expected no download yet")
	})

	t.Run("up rate", func(t *testing.T) {
		rate, err := client.UpRate(ctx)
		require.NoError(t, err)
		require.Zero(t, rate, "expected no upload yet")
	})

	t.Run("views", func(t *testing.T) {
		views, err := client.Views(ctx)
		require.NoError(t, err)
		require.Contains(t, views, ViewMain)
		// ruTorrent's ratio plugin inserts its ratio groups as persistent views rat_0 to rat_7
		require.Contains(t, views, View("rat_1"))
	})

	t.Run("get no torrents", func(t *testing.T) {
		torrents, err := client.GetTorrents(ctx, ViewMain)
		require.NoError(t, err)
		require.Empty(t, torrents, "expected no torrents to be added yet")
	})

	t.Run("add by url", func(t *testing.T) {
		err := client.Add(ctx, env.downloadURL())
		require.NoError(t, err)

		torrent := waitForTorrents(t, client, ViewMain, 1)[0]
		requireTorrent(t, download, "", torrent)
		require.Equal(t, "/downloads/temp", torrent.Path)
		require.False(t, torrent.Completed)

		t.Run("get files", func(t *testing.T) {
			requireFiles(t, client, torrent)
		})

		t.Run("single get", func(t *testing.T) {
			got, err := client.GetTorrent(ctx, torrent.Hash)
			require.NoError(t, err)
			require.NotEmpty(t, got.Hash)
			require.NotEmpty(t, got.Name)
			require.NotEmpty(t, got.Path)
			require.NotEmpty(t, got.Size)
		})

		t.Run("change label", func(t *testing.T) {
			err := client.SetLabel(ctx, torrent, "TestLabel")
			require.NoError(t, err)

			var torrents []Torrent
			waitFor(t, "torrent label to change", func() (bool, error) {
				torrents, err = client.GetTorrents(ctx, ViewMain)
				if err != nil {
					return false, err
				}
				require.Len(t, torrents, 1)
				return torrents[0].Label != "", nil
			})
			require.Equal(t, "TestLabel", torrents[0].Label)
		})

		t.Run("get status", func(t *testing.T) {
			var status Status
			waitFor(t, "torrent to start downloading", func() (bool, error) {
				var err error
				status, err = client.GetStatus(ctx, torrent)
				t.Logf("Status = %+v", status)
				return status.CompletedBytes > 0, err
			})
			require.False(t, status.Completed)
			require.NotZero(t, status.CompletedBytes)
			require.NotZero(t, status.DownRate)
			require.NotZero(t, status.Size)
		})

		t.Run("delete torrent", func(t *testing.T) {
			deleteTorrent(t, client, torrent)
		})
	})

	t.Run("add by url (stopped)", func(t *testing.T) {
		label := DLabel.SetValue("test-label")
		err := client.AddStopped(ctx, env.downloadURL(), label)
		require.NoError(t, err)

		torrent := waitForTorrents(t, client, ViewStopped, 1)[0]
		requireTorrent(t, download, label.Value, torrent)
		require.Equal(t, "/downloads/temp", torrent.Path)
		require.False(t, torrent.Completed)

		t.Run("get status", func(t *testing.T) {
			<-time.After(time.Second)
			status, err := client.GetStatus(ctx, torrent)
			require.NoError(t, err)
			t.Logf("Status = %+v", status)

			require.False(t, status.Completed)
			require.Zero(t, status.CompletedBytes)
			require.Zero(t, status.DownRate)
			require.NotZero(t, status.Size)
		})

		t.Run("start torrent", func(t *testing.T) {
			require.NoError(t, client.StartTorrent(ctx, torrent))
			waitForState(t, client, torrent, true, true, 1)

			// let it download for a while, so the totals post activity are not zero
			<-time.After(time.Second * 10)
		})

		t.Run("pause torrent", func(t *testing.T) {
			require.NoError(t, client.PauseTorrent(ctx, torrent))
			waitForState(t, client, torrent, true, false, 1)
		})

		t.Run("resume torrent", func(t *testing.T) {
			require.NoError(t, client.ResumeTorrent(ctx, torrent))
			waitForState(t, client, torrent, true, true, 1)
		})

		t.Run("stop torrent", func(t *testing.T) {
			require.NoError(t, client.StopTorrent(ctx, torrent))
			waitForState(t, client, torrent, true, false, 0)
		})

		t.Run("close torrent", func(t *testing.T) {
			require.NoError(t, client.CloseTorrent(ctx, torrent))
			waitForState(t, client, torrent, false, false, 0)
		})

		t.Run("open torrent", func(t *testing.T) {
			require.NoError(t, client.OpenTorrent(ctx, torrent))
			waitFor(t, "torrent to open", func() (bool, error) {
				return client.IsOpen(ctx, torrent)
			})
		})

		t.Run("re-close torrent", func(t *testing.T) {
			require.NoError(t, client.CloseTorrent(ctx, torrent))
			waitForState(t, client, torrent, false, false, 0)
		})

		t.Run("delete torrent", func(t *testing.T) {
			deleteTorrent(t, client, torrent)
		})
	})

	t.Run("add with data", func(t *testing.T) {
		err := client.AddTorrent(ctx, download.Torrent)
		require.NoError(t, err)

		torrent := waitForTorrents(t, client, ViewMain, 1)[0]
		requireTorrent(t, download, "", torrent)
		require.Equal(t, "/downloads/temp", torrent.Path)
		require.False(t, torrent.Completed)

		t.Run("get files", func(t *testing.T) {
			requireFiles(t, client, torrent)
		})

		t.Run("delete torrent", func(t *testing.T) {
			deleteTorrent(t, client, torrent)
		})
	})

	t.Run("add with data (stopped)", func(t *testing.T) {
		label := DLabel.SetValue("test-label")
		err := client.AddTorrentStopped(ctx, download.Torrent, label)
		require.NoError(t, err)

		torrent := waitForTorrents(t, client, ViewMain, 1)[0]
		requireTorrent(t, download, label.Value, torrent)

		deleteTorrent(t, client, torrent)
	})

	t.Run("add with data (stopped) in ratio group with priority", func(t *testing.T) {
		err := client.AddTorrentStopped(ctx, download.Torrent, Command("view.set_visible", "rat_1"), DPriority.SetValue("3"))
		require.NoError(t, err)

		torrent := waitForTorrents(t, client, View("rat_1"), 1)[0]

		views, err := client.xmlrpcClient.Call(ctx, "d.views", torrent.Hash)
		require.NoError(t, err)
		require.Equal(t, []interface{}{[]interface{}{"rat_1"}}, views)

		priority, err := client.xmlrpcClient.Call(ctx, "d.priority", torrent.Hash)
		require.NoError(t, err)
		require.Equal(t, []interface{}{3}, priority)

		require.NoError(t, client.Delete(ctx, torrent))
	})

	t.Run("add with data in directory and seed", func(t *testing.T) {
		upload := env.upload

		// seed copies the data into the container and adds the torrent with
		// Command("d.directory.set", ...) pointing at it
		env.seed(t, env.main, client, upload)

		torrent, err := client.GetTorrent(ctx, upload.Hash)
		require.NoError(t, err)
		require.Equal(t, upload.Name, torrent.Name)
		require.Equal(t, completeDir, torrent.Path)
		require.True(t, torrent.Completed)

		require.NoError(t, env.peer.AddTorrent(ctx, upload.Torrent))

		var total int
		defer func() {
			if t.Failed() {
				t.Logf("uploaded %d of %d bytes", total, upload.Size)
			}
		}()
		waitFor(t, "peer to download the seeded torrent", func() (bool, error) {
			total, err = client.UpTotal(ctx)
			return total >= upload.Size, err
		})

		require.NoError(t, env.peer.Delete(ctx, Torrent{Hash: upload.Hash}))
		require.NoError(t, client.Delete(ctx, torrent))
	})

	t.Run("down total post activity", func(t *testing.T) {
		total, err := client.DownTotal(ctx)
		require.NoError(t, err)
		require.NotZero(t, total, "expected data to be transferred")
	})

	t.Run("up total post activity", func(t *testing.T) {
		total, err := client.UpTotal(ctx)
		require.NoError(t, err)
		require.NotZero(t, total, "expected data to be transferred")
	})
}

// waitFor polls fn every second until it reports true, failing the test on an error or after maxRetries.
func waitFor(t *testing.T, what string, fn func() (bool, error)) {
	t.Helper()

	for i := 0; i <= maxRetries; i++ {
		<-time.After(time.Second)
		ok, err := fn()
		require.NoError(t, err)
		if ok {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

// waitForTorrents waits until view holds n torrents and returns them.
func waitForTorrents(t *testing.T, client *Client, view View, n int) []Torrent {
	t.Helper()

	var torrents []Torrent
	waitFor(t, "torrents in view "+string(view), func() (bool, error) {
		var err error
		torrents, err = client.GetTorrents(context.Background(), view)
		return len(torrents) >= n, err
	})
	require.Len(t, torrents, n)

	return torrents
}

// waitForState waits until the torrent's open, active and state flags match.
func waitForState(t *testing.T, client *Client, torrent Torrent, open, active bool, state int) {
	t.Helper()
	ctx := context.Background()

	waitFor(t, "torrent state", func() (bool, error) {
		isOpen, err := client.IsOpen(ctx, torrent)
		if err != nil {
			return false, err
		}
		isActive, err := client.IsActive(ctx, torrent)
		if err != nil {
			return false, err
		}
		s, err := client.State(ctx, torrent)
		if err != nil {
			return false, err
		}
		return isOpen == open && isActive == active && s == state, nil
	})
}

// deleteTorrent deletes the torrent and waits until no torrents are left.
func deleteTorrent(t *testing.T, client *Client, torrent Torrent) {
	t.Helper()
	ctx := context.Background()

	require.NoError(t, client.Delete(ctx, torrent))

	torrents, err := client.GetTorrents(ctx, ViewMain)
	require.NoError(t, err)
	require.Empty(t, torrents)

	waitFor(t, "torrent to be deleted", func() (bool, error) {
		torrents, err := client.GetTorrents(ctx, ViewMain)
		return len(torrents) == 0, err
	})
}

func requireTorrent(t *testing.T, want fixture, label string, got Torrent) {
	t.Helper()

	require.Equal(t, want.Hash, got.Hash)
	require.Equal(t, want.Name, got.Name)
	require.Equal(t, label, got.Label)
	require.Equal(t, want.Size, got.Size)
}

func requireFiles(t *testing.T, client *Client, torrent Torrent) {
	t.Helper()

	files, err := client.GetFiles(context.Background(), torrent)
	require.NoError(t, err)
	require.Len(t, files, 1)
	for _, f := range files {
		require.NotEmpty(t, f.Path)
		require.NotZero(t, f.Size)
	}
}
