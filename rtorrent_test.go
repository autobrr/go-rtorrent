package rtorrent

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestRTorrent(t *testing.T) {
	/*
		These tests rely on a local instance of rtorrent to be running in a clean state.
		Use the included `test.sh` script to run these tests.
	*/
	addr := os.Getenv("RTORRENT_TEST_URL")

	slog.Info("RTORRENT_TEST_URL from env", slog.String("addr", addr))

	if addr == "" {
		addr = "http://localhost:8000/RPC2"

		slog.Info("RTORRENT_TEST_URL from env empty, fallback to default", slog.String("addr", addr))
	}

	client := NewClient(Config{Addr: addr, TLSSkipVerify: false})
	//client := New("http://localhost:8000", false)
	maxRetries := 60

	ctx := context.Background()

	t.Run("get ip", func(t *testing.T) {
		ctx := context.Background()
		_, err := client.IP(ctx)
		require.NoError(t, err)
		// Don't assert anything about the response, differs based upon the environment
	})

	t.Run("get name", func(t *testing.T) {
		ctx := context.Background()
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

	t.Run("get no torrents", func(t *testing.T) {
		torrents, err := client.GetTorrents(ctx, ViewMain)
		require.NoError(t, err)
		require.Empty(t, torrents, "expected no torrents to be added yet")
	})

	t.Run("add", func(t *testing.T) {
		t.Run("by url", func(t *testing.T) {
			err := client.Add(ctx, "https://releases.ubuntu.com/24.10/ubuntu-24.10-desktop-amd64.iso.torrent")
			require.NoError(t, err)

			t.Run("get torrent", func(t *testing.T) {
				// It will take some time to appear, so retry a few times
				var torrents []Torrent
				var err error
				retries := maxRetries
				for i := 0; i <= retries; i++ {
					<-time.After(time.Second)
					torrents, err = client.GetTorrents(ctx, ViewMain)
					require.NoError(t, err)
					if len(torrents) > 0 {
						break
					}
					if i == retries {
						require.NoError(t, errors.Errorf("torrent did not show up in time"))
					}
				}
				require.NotEmpty(t, torrents)
				require.Len(t, torrents, 1)
				require.Equal(t, "3F9AAC158C7DE8DFCAB171EA58A17AABDF7FBC93", torrents[0].Hash)
				require.Equal(t, "ubuntu-24.10-desktop-amd64.iso", torrents[0].Name)
				require.Equal(t, "", torrents[0].Label)
				require.Equal(t, 5665497088, torrents[0].Size)
				require.Equal(t, "/downloads/temp", torrents[0].Path)
				require.False(t, torrents[0].Completed)

				t.Run("get files", func(t *testing.T) {
					files, err := client.GetFiles(ctx, torrents[0])
					require.NoError(t, err)
					require.NotEmpty(t, files)
					require.Len(t, files, 1)
					for _, f := range files {
						require.NotEmpty(t, f.Path)
						require.NotZero(t, f.Size)
					}
				})

				t.Run("single get", func(t *testing.T) {
					torrent, err := client.GetTorrent(ctx, torrents[0].Hash)
					require.NoError(t, err)
					require.NotEmpty(t, torrent.Hash)
					require.NotEmpty(t, torrent.Name)
					require.NotEmpty(t, torrent.Path)
					require.NotEmpty(t, torrent.Size)
				})

				t.Run("change label", func(t *testing.T) {
					err := client.SetLabel(ctx, torrents[0], "TestLabel")
					require.NoError(t, err)

					// It will take some time to change, so try a few times
					retries := maxRetries
					for i := 0; i <= retries; i++ {
						<-time.After(time.Second)
						torrents, err = client.GetTorrents(ctx, ViewMain)
						require.NoError(t, err)
						require.Len(t, torrents, 1)
						if torrents[0].Label != "" {
							break
						}
						if i == retries {
							require.NoError(t, errors.Errorf("torrent label did not change in time"))
						}
					}
					require.Equal(t, "TestLabel", torrents[0].Label)
				})

				t.Run("get status", func(t *testing.T) {
					var status Status
					var err error
					// It may take some time for the download to start
					retries := maxRetries
					for i := 0; i <= retries; i++ {
						<-time.After(time.Second)
						status, err = client.GetStatus(ctx, torrents[0])
						require.NoError(t, err)
						t.Logf("Status = %+v", status)
						if status.CompletedBytes > 0 {
							break
						}
						if i == retries {
							require.NoError(t, errors.Errorf("torrent did not start in time"))
						}
					}

					require.False(t, status.Completed)
					require.NotZero(t, status.CompletedBytes)
					require.NotZero(t, status.DownRate)
					require.NotZero(t, status.Size)
					// require.NotZero(t, status.UpRate)
					//require.NotZero(t, status.Ratio)
				})

				t.Run("delete torrent", func(t *testing.T) {
					err := client.Delete(ctx, torrents[0])
					require.NoError(t, err)

					torrents, err := client.GetTorrents(ctx, ViewMain)
					require.NoError(t, err)
					require.Empty(t, torrents)

					t.Run("get torrent", func(t *testing.T) {
						// It will take some time to disappear, so retry a few times
						var torrents []Torrent
						var err error
						retries := maxRetries
						for i := 0; i <= retries; i++ {
							<-time.After(time.Second)
							torrents, err = client.GetTorrents(ctx, ViewMain)
							require.NoError(t, err)
							if len(torrents) == 0 {
								break
							}
							if i == retries {
								require.NoError(t, errors.Errorf("torrent did not delete in time"))
							}
						}
						require.Empty(t, torrents)
					})
				})

			})
		})

		t.Run("by url (stopped)", func(t *testing.T) {
			label := DLabel.SetValue("test-label")
			err := client.AddStopped(ctx, "https://releases.ubuntu.com/24.10/ubuntu-24.10-desktop-amd64.iso.torrent", label)
			require.NoError(t, err)

			t.Run("get torrent", func(t *testing.T) {
				// It will take some time to appear, so retry a few times
				var torrents []Torrent
				var err error
				retries := maxRetries
				for i := 0; i <= retries; i++ {
					<-time.After(time.Second)
					torrents, err = client.GetTorrents(ctx, ViewStopped)
					require.NoError(t, err)
					if len(torrents) > 0 {
						break
					}
					if i == retries {
						require.NoError(t, errors.Errorf("torrent did not show up in time"))
					}
				}
				require.NotEmpty(t, torrents)
				require.Len(t, torrents, 1)
				require.Equal(t, "3F9AAC158C7DE8DFCAB171EA58A17AABDF7FBC93", torrents[0].Hash)
				require.Equal(t, "ubuntu-24.10-desktop-amd64.iso", torrents[0].Name)
				require.Equal(t, label.Value, torrents[0].Label)
				require.Equal(t, 5665497088, torrents[0].Size)
				require.Equal(t, "/downloads/temp", torrents[0].Path)
				require.False(t, torrents[0].Completed)

				t.Run("get status", func(t *testing.T) {
					<-time.After(time.Second)
					status, err := client.GetStatus(ctx, torrents[0])
					require.NoError(t, err)
					t.Logf("Status = %+v", status)

					require.False(t, status.Completed)
					require.Zero(t, status.CompletedBytes)
					require.Zero(t, status.DownRate)
					require.NotZero(t, status.Size)
				})

				t.Run("start torrent", func(t *testing.T) {
					err = client.StartTorrent(ctx, torrents[0])
					require.NoError(t, err)

					t.Run("check if started", func(t *testing.T) {
						var isOpen, isActive bool
						var state, retries int = 0, maxRetries
						for i := 0; i <= retries; i++ {
							<-time.After(time.Second)

							isOpen, err = client.IsOpen(ctx, torrents[0])
							require.NoError(t, err)

							isActive, err = client.IsActive(ctx, torrents[0])
							require.NoError(t, err)

							state, err = client.State(ctx, torrents[0])
							require.NoError(t, err)

							if isOpen && isActive && state == 1 {
								break
							}

							if i == retries {
								require.NoError(t, errors.Errorf("torrent did not start in time"))
							}
						}

						require.True(t, isOpen)
						require.True(t, isActive)
						require.Equal(t, 1, state)
					})

					// wait some seconds to properly start to download bytes so
					// to allow testing for up/down total post activity
					<-time.After(time.Second * 10)

					t.Run("pause torrent", func(t *testing.T) {
						err := client.PauseTorrent(ctx, torrents[0])
						require.NoError(t, err)

						var isOpen, isActive bool
						var state, retries int = 0, maxRetries
						for i := 0; i <= retries; i++ {
							<-time.After(time.Second)
							isOpen, err = client.IsOpen(ctx, torrents[0])
							require.NoError(t, err)

							isActive, err = client.IsActive(ctx, torrents[0])
							require.NoError(t, err)

							state, err = client.State(ctx, torrents[0])
							require.NoError(t, err)

							if isOpen && !isActive && state == 1 {
								break
							}

							if i == retries {
								require.NoError(t, errors.Errorf("torrent did not pause in time"))
							}
						}
						require.True(t, isOpen)
						require.False(t, isActive)
						require.Equal(t, 1, state)
					})

					t.Run("resume torrent", func(t *testing.T) {
						err := client.ResumeTorrent(ctx, torrents[0])
						require.NoError(t, err)
						var isOpen, isActive bool
						var state, retries int = 0, maxRetries
						for i := 0; i <= retries; i++ {
							<-time.After(time.Second)
							isOpen, err = client.IsOpen(ctx, torrents[0])
							require.NoError(t, err)

							isActive, err = client.IsActive(ctx, torrents[0])
							require.NoError(t, err)

							state, err = client.State(ctx, torrents[0])
							require.NoError(t, err)

							if isOpen && isActive && state == 1 {
								break
							}

							if i == retries {
								require.NoError(t, errors.Errorf("torrent did not resume in time"))
							}
						}
						require.True(t, isOpen)
						require.True(t, isActive)
						require.Equal(t, 1, state)
					})

				})

				t.Run("stop torrent", func(t *testing.T) {
					err = client.StopTorrent(ctx, torrents[0])
					require.NoError(t, err)

					t.Run("check if stopped", func(t *testing.T) {
						var isOpen, isActive bool
						var state, retries int = 0, maxRetries
						for i := 0; i <= retries; i++ {
							<-time.After(time.Second)

							isOpen, err = client.IsOpen(ctx, torrents[0])
							require.NoError(t, err)

							isActive, err = client.IsActive(ctx, torrents[0])
							require.NoError(t, err)

							state, err = client.State(ctx, torrents[0])
							require.NoError(t, err)

							if isOpen && !isActive && state == 0 {
								break
							}

							if i == retries {
								require.NoError(t, errors.Errorf("torrent did not stop in time"))
							}
						}
						require.True(t, isOpen)
						require.False(t, isActive)
						require.Equal(t, 0, state)
					})
				})

				t.Run("close torrent", func(t *testing.T) {
					err = client.CloseTorrent(ctx, torrents[0])
					require.NoError(t, err)

					t.Run("check if closed", func(t *testing.T) {
						var isOpen, isActive bool
						var state, retries int = 0, maxRetries
						for i := 0; i <= retries; i++ {
							<-time.After(time.Second)

							isOpen, err = client.IsOpen(ctx, torrents[0])
							require.NoError(t, err)

							isActive, err = client.IsActive(ctx, torrents[0])
							require.NoError(t, err)

							state, err = client.State(ctx, torrents[0])
							require.NoError(t, err)

							if !isOpen && !isActive && state == 0 {
								break
							}

							if i == retries {
								require.NoError(t, errors.Errorf("torrent did not close in time"))
							}
						}
						require.False(t, isOpen)
						require.False(t, isActive)
						require.Equal(t, 0, state)
					})
				})

				t.Run("open torrent", func(t *testing.T) {
					err = client.OpenTorrent(ctx, torrents[0])
					require.NoError(t, err)

					t.Run("check if open", func(t *testing.T) {
						var isOpen bool
						var retries int = maxRetries
						for i := 0; i <= retries; i++ {
							<-time.After(time.Second)

							isOpen, err = client.IsOpen(ctx, torrents[0])
							require.NoError(t, err)

							if isOpen {
								break
							}

							if i == retries {
								require.NoError(t, errors.Errorf("torrent did not open in time"))
							}
						}
						require.True(t, isOpen)
					})
				})

				t.Run("re-close torrent", func(t *testing.T) {
					err = client.CloseTorrent(ctx, torrents[0])
					require.NoError(t, err)

					t.Run("check if closed", func(t *testing.T) {
						var isOpen, isActive bool
						var state, retries int = 0, maxRetries
						for i := 0; i <= retries; i++ {
							<-time.After(time.Second)

							isOpen, err = client.IsOpen(ctx, torrents[0])
							require.NoError(t, err)

							isActive, err = client.IsActive(ctx, torrents[0])
							require.NoError(t, err)

							state, err = client.State(ctx, torrents[0])
							require.NoError(t, err)

							if !isOpen && !isActive && state == 0 {
								break
							}

							if i == retries {
								require.NoError(t, errors.Errorf("torrent did not close in time"))
							}
						}
						require.False(t, isOpen)
						require.False(t, isActive)
						require.Equal(t, 0, state)
					})
				})

				t.Run("delete torrent", func(t *testing.T) {
					err := client.Delete(ctx, torrents[0])
					require.NoError(t, err)

					torrents, err := client.GetTorrents(ctx, ViewMain)
					require.NoError(t, err)
					require.Empty(t, torrents)

					t.Run("get torrent", func(t *testing.T) {
						// It will take some time to disappear, so retry a few times
						var torrents []Torrent
						var err error
						retries := maxRetries
						for i := 0; i <= retries; i++ {
							<-time.After(time.Second)
							torrents, err = client.GetTorrents(ctx, ViewMain)
							require.NoError(t, err)
							if len(torrents) == 0 {
								break
							}
							if i == retries {
								require.NoError(t, errors.Errorf("torrent did not delete in time"))
							}
						}
						require.Empty(t, torrents)
					})
				})
			})
		})

		t.Run("with data", func(t *testing.T) {
			b, err := os.ReadFile("testdata/ubuntu-24.10-desktop-amd64.iso.torrent")
			require.NoError(t, err)
			require.NotEmpty(t, b)

			err = client.AddTorrent(ctx, b)
			require.NoError(t, err)

			t.Run("get torrent", func(t *testing.T) {
				// It will take some time to appear, so retry a few times
				var torrents []Torrent
				var err error
				retries := maxRetries
				for i := 0; i <= retries; i++ {
					<-time.After(time.Second)
					torrents, err = client.GetTorrents(ctx, ViewMain)
					require.NoError(t, err)
					if len(torrents) > 0 {
						break
					}
					if i == retries {
						require.NoError(t, errors.Errorf("torrent did not show up in time"))
					}
				}
				require.NotEmpty(t, torrents)
				require.Len(t, torrents, 1)
				require.Equal(t, "3F9AAC158C7DE8DFCAB171EA58A17AABDF7FBC93", torrents[0].Hash)
				require.Equal(t, "ubuntu-24.10-desktop-amd64.iso", torrents[0].Name)
				require.Equal(t, "", torrents[0].Label)
				require.Equal(t, 5665497088, torrents[0].Size)
				require.Equal(t, "/downloads/temp", torrents[0].Path)
				require.False(t, torrents[0].Completed)

				t.Run("get files", func(t *testing.T) {
					files, err := client.GetFiles(ctx, torrents[0])
					require.NoError(t, err)
					require.NotEmpty(t, files)
					require.Len(t, files, 1)
					for _, f := range files {
						require.NotEmpty(t, f.Path)
						require.NotZero(t, f.Size)
					}
				})

				t.Run("delete torrent", func(t *testing.T) {
					err := client.Delete(ctx, torrents[0])
					require.NoError(t, err)

					torrents, err := client.GetTorrents(ctx, ViewMain)
					require.NoError(t, err)
					require.Empty(t, torrents)

					t.Run("get torrent", func(t *testing.T) {
						// It will take some time to disappear, so retry a few times
						var torrents []Torrent
						var err error
						retries := maxRetries
						for i := 0; i <= retries; i++ {
							<-time.After(time.Second)
							torrents, err = client.GetTorrents(ctx, ViewMain)
							require.NoError(t, err)
							if len(torrents) == 0 {
								break
							}
							if i == retries {
								require.NoError(t, errors.Errorf("torrent did not delete in time"))
							}
						}
						require.Empty(t, torrents)
					})
				})
			})
		})

		t.Run("with data (stopped)", func(t *testing.T) {
			b, err := os.ReadFile("testdata/ubuntu-24.10-desktop-amd64.iso.torrent")
			require.NoError(t, err)
			require.NotEmpty(t, b)

			label := DLabel.SetValue("test-label")
			err = client.AddTorrentStopped(ctx, b, label)
			require.NoError(t, err)

			t.Run("get torrent", func(t *testing.T) {
				// It will take some time to appear, so retry a few times
				<-time.After(time.Second)
				torrents, err := client.GetTorrents(ctx, ViewMain)
				require.NoError(t, err)

				require.NotEmpty(t, torrents)
				require.Len(t, torrents, 1)
				require.Equal(t, "3F9AAC158C7DE8DFCAB171EA58A17AABDF7FBC93", torrents[0].Hash)
				require.Equal(t, "ubuntu-24.10-desktop-amd64.iso", torrents[0].Name)
				require.Equal(t, label.Value, torrents[0].Label)
				require.Equal(t, 5665497088, torrents[0].Size)

				t.Run("delete torrent", func(t *testing.T) {
					err := client.Delete(ctx, torrents[0])
					require.NoError(t, err)

					torrents, err := client.GetTorrents(ctx, ViewMain)
					require.NoError(t, err)
					require.Empty(t, torrents)

					t.Run("get torrent", func(t *testing.T) {
						// It will take some time to disappear, so retry a few times
						var torrents []Torrent
						var err error
						retries := maxRetries
						for i := 0; i <= retries; i++ {
							<-time.After(time.Second)
							torrents, err = client.GetTorrents(ctx, ViewMain)
							require.NoError(t, err)
							if len(torrents) == 0 {
								break
							}
							if i == retries {
								require.NoError(t, errors.Errorf("torrent did not delete in time"))
							}
						}
						require.Empty(t, torrents)
					})
				})
			})
		})
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
