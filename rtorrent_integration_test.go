//go:build integration

package rtorrent

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestRTorrent(t *testing.T) {
	// These tests start their own rTorrent containers with Docker, see newTestEnv.
	env := newTestEnv(t)
	client := env.client
	download := env.download
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

	t.Run("add", func(t *testing.T) {
		t.Run("by url", func(t *testing.T) {
			err := client.Add(ctx, env.downloadURL())
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
				require.Equal(t, download.Hash, torrents[0].Hash)
				require.Equal(t, download.Name, torrents[0].Name)
				require.Equal(t, "", torrents[0].Label)
				require.Equal(t, download.Size, torrents[0].Size)
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
			err := client.AddStopped(ctx, env.downloadURL(), label)
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
				require.Equal(t, download.Hash, torrents[0].Hash)
				require.Equal(t, download.Name, torrents[0].Name)
				require.Equal(t, label.Value, torrents[0].Label)
				require.Equal(t, download.Size, torrents[0].Size)
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
			b := download.Torrent

			err := client.AddTorrent(ctx, b)
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
				require.Equal(t, download.Hash, torrents[0].Hash)
				require.Equal(t, download.Name, torrents[0].Name)
				require.Equal(t, "", torrents[0].Label)
				require.Equal(t, download.Size, torrents[0].Size)
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
			b := download.Torrent

			label := DLabel.SetValue("test-label")
			err := client.AddTorrentStopped(ctx, b, label)
			require.NoError(t, err)

			t.Run("get torrent", func(t *testing.T) {
				// It will take some time to appear, so retry a few times
				<-time.After(time.Second)
				torrents, err := client.GetTorrents(ctx, ViewMain)
				require.NoError(t, err)

				require.NotEmpty(t, torrents)
				require.Len(t, torrents, 1)
				require.Equal(t, download.Hash, torrents[0].Hash)
				require.Equal(t, download.Name, torrents[0].Name)
				require.Equal(t, label.Value, torrents[0].Label)
				require.Equal(t, download.Size, torrents[0].Size)

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

		t.Run("with data (stopped) in ratio group with priority", func(t *testing.T) {
			b := download.Torrent

			err := client.AddTorrentStopped(ctx, b, Command("view.set_visible", "rat_1"), DPriority.SetValue("3"))
			require.NoError(t, err)

			var torrents []Torrent
			for i := 0; i <= maxRetries; i++ {
				<-time.After(time.Second)
				torrents, err = client.GetTorrents(ctx, View("rat_1"))
				require.NoError(t, err)
				if len(torrents) > 0 {
					break
				}
				if i == maxRetries {
					require.NoError(t, errors.Errorf("torrent did not show up in ratio group in time"))
				}
			}
			require.Len(t, torrents, 1)

			views, err := client.xmlrpcClient.Call(ctx, "d.views", torrents[0].Hash)
			require.NoError(t, err)
			require.Equal(t, []interface{}{[]interface{}{"rat_1"}}, views)

			priority, err := client.xmlrpcClient.Call(ctx, "d.priority", torrents[0].Hash)
			require.NoError(t, err)
			require.Equal(t, []interface{}{3}, priority)

			err = client.Delete(ctx, torrents[0])
			require.NoError(t, err)
		})

		t.Run("with data in directory and seed", func(t *testing.T) {
			upload := env.upload

			// seed copies the data into the container and adds the torrent with
			// Command("d.directory.set", ...) pointing at it
			env.seed(t, env.main, client, upload)

			torrent, err := client.GetTorrent(ctx, upload.Hash)
			require.NoError(t, err)
			require.Equal(t, upload.Name, torrent.Name)
			require.Equal(t, completeDir, torrent.Path)
			require.True(t, torrent.Completed)

			err = env.peer.AddTorrent(ctx, upload.Torrent)
			require.NoError(t, err)

			var total int
			for i := 0; i <= maxRetries; i++ {
				<-time.After(time.Second)
				total, err = client.UpTotal(ctx)
				require.NoError(t, err)
				if total >= upload.Size {
					break
				}
				if i == maxRetries {
					require.NoError(t, errors.Errorf("peer did not download the seeded torrent in time, uploaded %d bytes", total))
				}
			}

			err = env.peer.Delete(ctx, Torrent{Hash: upload.Hash})
			require.NoError(t, err)
			err = client.Delete(ctx, torrent)
			require.NoError(t, err)
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
