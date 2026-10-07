# go-rtorrent
[![GoDoc](https://godoc.org/github.com/mrobinsn/go-rtorrent/rtorrent?status.svg)](https://godoc.org/github.com/mrobinsn/go-rtorrent/rtorrent)
[![Coverage Status](https://coveralls.io/repos/github/mrobinsn/go-rtorrent/badge.svg?branch=master)](https://coveralls.io/github/mrobinsn/go-rtorrent?branch=master)
[![MIT license](http://img.shields.io/badge/license-MIT-brightgreen.svg)](http://opensource.org/licenses/MIT)

> rTorrent XMLRPC Bindings for Go (golang)

Fork of [github.com/mrobinsn/go-rtorrent](github.com/mrobinsn/go-rtorrent).

## Documentation
[GoDoc](https://godoc.org/github.com/mrobinsn/go-rtorrent/rtorrent)

## Features
- Get IP, Name, Up/Down totals
- Get torrents within a view
- Get torrent by hash
- Get files for torrents
- Set the label on a torrent
- Add a torrent by URL or by metadata
- Delete a torrent (including files)

## Installation
To install the package, run `go get github.com/autobrr/go-rtorrent`

To use it in application, import `"github.com/autobrr/go-rtorrent"`

## Library Usage

```golang
client := rtorrent.NewClient(rtorrent.Config{Addr: "http://my-rtorrent.com/RPC2"})
name, _ := client.Name(context.Background())
fmt.Printf("My rTorrent's name: %v", name)
```

You can connect to a server using Basic Authentication by adding User and Pass to the config:

```golang
client := rtorrent.NewClient(rtorrent.Config{Addr: "http://my-rtorrent.com/RPC2", BasicUser: "user", BasicPass: "pass"})
```

## Contributing

Pull requests are welcome, please ensure you add relevant tests for any new/changed functionality.

### Testing

Unit tests need nothing but Go:

```sh
go test ./...
```

Integration tests need Docker. They start two rTorrent containers with [Testcontainers](https://golang.testcontainers.org/), one under test and one as its peer, and transfer generated torrents between them, so they don't need internet access:

```sh
go test -tags integration ./...
```

They default to `crazymax/rtorrent-rutorrent:latest`. Set `RTORRENT_TEST_IMAGE` to test another image.
