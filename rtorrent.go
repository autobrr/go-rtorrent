package rtorrent

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/autobrr/go-rtorrent/xmlrpc"

	"github.com/pkg/errors"
)

// Client is used to communicate with a remote rTorrent instance
type Client struct {
	addr         string
	xmlrpcClient *xmlrpc.Client
	cfg          Config

	log *log.Logger
}

type Config struct {
	Addr          string
	TLSSkipVerify bool

	BasicUser string
	BasicPass string

	Log *log.Logger
}

type OptFunc func(*Client)

func WithCustomClient(client *http.Client) OptFunc {
	return func(c *Client) {
		c.xmlrpcClient = xmlrpc.NewClient(xmlrpc.Config{
			Addr:          c.cfg.Addr,
			TLSSkipVerify: c.cfg.TLSSkipVerify,
			BasicUser:     c.cfg.BasicUser,
			BasicPass:     c.cfg.BasicPass,
			Client:        client,
		})
	}
}

// NewClient returns a new instance of `Client`
func NewClient(cfg Config) *Client {
	c := &Client{
		addr: cfg.Addr,
		log:  log.New(io.Discard, "", log.LstdFlags),
		xmlrpcClient: xmlrpc.NewClient(xmlrpc.Config{
			Addr:          cfg.Addr,
			TLSSkipVerify: cfg.TLSSkipVerify,
			BasicUser:     cfg.BasicUser,
			BasicPass:     cfg.BasicPass,
		}),
	}

	// override logger if we pass one
	if cfg.Log != nil {
		c.log = cfg.Log
	}

	return c
}

// WithHTTPClient allows you to a provide a custom http.Client.
func (r *Client) WithHTTPClient(client *http.Client) *Client {
	r.xmlrpcClient = xmlrpc.NewClientWithHTTPClient(r.addr, client)
	return r
}

func NewClientWithOpts(cfg Config, opts ...OptFunc) *Client {
	c := &Client{
		addr: cfg.Addr,
		log:  log.New(io.Discard, "", log.LstdFlags),
		cfg:  cfg,
		xmlrpcClient: xmlrpc.NewClient(xmlrpc.Config{
			Addr:          cfg.Addr,
			TLSSkipVerify: cfg.TLSSkipVerify,
			BasicUser:     cfg.BasicUser,
			BasicPass:     cfg.BasicPass,
		}),
	}

	for _, opt := range opts {
		opt(c)
	}

	// override logger if we pass one
	if cfg.Log != nil {
		c.log = cfg.Log
	}

	return c
}

// FieldValue contains the Field and Value of an attribute on a rTorrent
type FieldValue struct {
	Field Field
	Value string
}

// Torrent represents a torrent in rTorrent
type Torrent struct {
	Hash      string
	Name      string
	Path      string
	Size      int
	Label     string
	Completed bool
	Ratio     float64
	Created   time.Time
	Started   time.Time
	Finished  time.Time
}

// Status represents the status of a torrent
type Status struct {
	Completed      bool
	CompletedBytes int
	DownRate       int
	UpRate         int
	Ratio          float64
	Size           int
}

// File represents a file in rTorrent
type File struct {
	Path string
	Size int
}

// Field represents an attribute on a Client entity that can be queried or set
type Field string

// View represents a "view" within Client
type View string

const (
	// ViewMain represents the "main" view, containing all torrents
	ViewMain View = "main"
	// ViewStarted represents the "started" view, containing only torrents that have been started
	ViewStarted View = "started"
	// ViewStopped represents the "stopped" view, containing only torrents that have been stopped
	ViewStopped View = "stopped"
	// ViewHashing represents the "hashing" view, containing only torrents that are currently hashing
	ViewHashing View = "hashing"
	// ViewSeeding represents the "seeding" view, containing only torrents that are currently seeding
	ViewSeeding View = "seeding"

	// DName represents the name of a "Downloading Items"
	DName Field = "d.name"
	// DLabel represents the label of a "Downloading Item"
	DLabel Field = "d.custom1"
	// DSizeInBytes represents the size in bytes of a "Downloading Item"
	DSizeInBytes Field = "d.size_bytes"
	// DHash represents the hash of a "Downloading Item"
	DHash Field = "d.hash"
	// DBasePath represents the base path of a "Downloading Item"
	DBasePath Field = "d.base_path"
	// DDirectory represents the directory of a "Downloading Item"
	DDirectory Field = "d.directory"
	// DIsActive represents whether a "Downloading Item" is active or not
	DIsActive Field = "d.is_active"
	// DRatio represents the ratio of a "Downloading Item"
	DRatio Field = "d.ratio"
	// DComplete represents whether the "Downloading Item" is complete or not
	DComplete Field = "d.complete"
	// DCompletedBytes represents the total of completed bytes of the "Downloading Item"
	DCompletedBytes Field = "d.completed_bytes"
	// DDownRate represents the download rate of the "Downloading Item"
	DDownRate Field = "d.down.rate"
	// DUpRate represents the upload rate of the "Downloading Item"
	DUpRate Field = "d.up.rate"
	// DCreationTime represents the date the torrent was created
	DCreationTime Field = "d.creation_date"
	// DFinishedTime represents the date the torrent finished downloading
	DFinishedTime Field = "d.timestamp.finished"
	// DStartedTime represents the date the torrent started downloading
	DStartedTime Field = "d.timestamp.started"

	// FPath represents the path of a "File Item"
	FPath Field = "f.path"
	// FSizeInBytes represents the size in bytes of a "File Item"
	FSizeInBytes Field = "f.size_bytes"
)

// Query converts the field to a string which allows it to be queried
// Example:
//
//	DName.Query() // returns "d.name="
func (f Field) Query() string {
	return fmt.Sprintf("%s=", f)
}

// SetValue returns a FieldValue struct which can be used to set the field on a particular item in rTorrent to the specified value
func (f Field) SetValue(value string) *FieldValue {
	return &FieldValue{f, value}
}

// Cmd returns the representation of the field which allows it to be used a command with Client
func (f Field) Cmd() string {
	return string(f)
}

func (f *FieldValue) String() string {
	return fmt.Sprintf("%s.set=\"%s\"", f.Field, f.Value)
}

// Pretty returns a formatted string representing this Torrent
func (t *Torrent) Pretty() string {
	return fmt.Sprintf("Torrent:\n\tHash: %v\n\tName: %v\n\tPath: %v\n\tLabel: %v\n\tSize: %v bytes\n\tCompleted: %v\n\tRatio: %v\n", t.Hash, t.Name, t.Path, t.Label, t.Size, t.Completed, t.Ratio)
}

// Pretty returns a formatted string representing this File
func (f *File) Pretty() string {
	return fmt.Sprintf("File:\n\tPath: %v\n\tSize: %v bytes\n", f.Path, f.Size)
}

// AddStopped adds a new torrent by URL in a stopped state
//
// extraArgs can be any valid rTorrent rpc command. For instance:
//
// Adds the Torrent by URL (stopped) and sets the label on the torrent
//
//	AddStopped("some-url", &FieldValue{"d.custom1", "my-label"})
//
// Or:
//
//	AddStopped("some-url", DLabel.SetValue("my-label"))
//
// Adds the Torrent by URL (stopped) and  sets the label and base path
//
//	AddStopped("some-url", &FieldValue{"d.custom1", "my-label"}, &FiedValue{"d.base_path", "/some/valid/path"})
//
// Or:
//
//	AddStopped("some-url", DLabel.SetValue("my-label"), DBasePath.SetValue("/some/valid/path"))
func (r *Client) AddStopped(ctx context.Context, url string, extraArgs ...*FieldValue) error {
	return r.add(ctx, "load.normal", []byte(url), extraArgs...)
}

// Add adds a new torrent by URL and starts the torrent
//
// extraArgs can be any valid rTorrent rpc command. For instance:
//
// Adds the Torrent by URL and sets the label on the torrent
//
//	Add("some-url", "d.custom1.set=\"my-label\"")
//
// Or:
//
//	Add("some-url", DLabel.SetValue("my-label"))
//
// Adds the Torrent by URL and  sets the label as well as base path
//
//	Add("some-url", "d.custom1.set=\"my-label\"", "d.base_path=\"/some/valid/path\"")
//
// Or:
//
//	Add("some-url", DLabel.SetValue("my-label"), DBasePath.SetValue("/some/valid/path"))
func (r *Client) Add(ctx context.Context, url string, extraArgs ...*FieldValue) error {
	return r.add(ctx, "load.start", []byte(url), extraArgs...)
}

// AddTorrentStopped adds a new torrent by the torrent files data but does not start the torrent
//
// extraArgs can be any valid rTorrent rpc command. For instance:
//
// Adds the Torrent file (stopped) and sets the label on the torrent
//
//	AddTorrentStopped(fileData, "d.custom1.set=\"my-label\"")
//
// Or:
//
//	AddTorrentStopped(fileData, DLabel.SetValue("my-label"))
//
// Adds the Torrent file and (stopped) sets the label and base path
//
//	AddTorrentStopped(fileData, "d.custom1.set=\"my-label\"", "d.base_path=\"/some/valid/path\"")
//
// Or:
//
//	AddTorrentStopped(fileData, DLabel.SetValue("my-label"), DBasePath.SetValue("/some/valid/path"))
func (r *Client) AddTorrentStopped(ctx context.Context, data []byte, extraArgs ...*FieldValue) error {
	return r.add(ctx, "load.raw", data, extraArgs...)
}

// AddTorrent adds a new torrent by the torrent files data and starts the torrent
//
// extraArgs can be any valid rTorrent rpc command. For instance:
//
// Adds the Torrent file and sets the label on the torrent
//
//	Add(fileData, "d.custom1.set=\"my-label\"")
//
// Or:
//
//	AddTorrent(fileData, DLabel.SetValue("my-label"))
//
// Adds the Torrent file and  sets the label and base path
//
//	Add(fileData, "d.custom1.set=\"my-label\"", "d.base_path=\"/some/valid/path\"")
//
// Or:
//
//	AddTorrent(fileData, DLabel.SetValue("my-label"), DBasePath.SetValue("/some/valid/path"))
func (r *Client) AddTorrent(ctx context.Context, data []byte, extraArgs ...*FieldValue) error {
	return r.add(ctx, "load.raw_start", data, extraArgs...)
}

func (r *Client) add(ctx context.Context, cmd string, data []byte, extraArgs ...*FieldValue) error {
	args := []interface{}{"", data}
	for _, v := range extraArgs {
		args = append(args, v.String())
	}

	_, err := r.xmlrpcClient.Call(ctx, cmd, args...)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("%s XMLRPC call failed", cmd))
	}
	return nil
}

// IP returns the IP reported by this Client instance
func (r *Client) IP(ctx context.Context) (string, error) {
	result, err := r.xmlrpcClient.Call(ctx, "network.bind_address")
	if err != nil {
		return "", errors.Wrap(err, "network.bind_address XMLRPC call failed")
	}
	if ips, ok := result.([]interface{}); ok {
		result = ips[0]
	}
	if ip, ok := result.(string); ok {
		return ip, nil
	}
	return "", errors.Errorf("result isn't string: %v", result)
}

// Name returns the name reported by this Client instance
func (r *Client) Name(ctx context.Context) (string, error) {
	result, err := r.xmlrpcClient.Call(ctx, "system.hostname")
	if err != nil {
		return "", errors.Wrap(err, "system.hostname XMLRPC call failed")
	}
	if names, ok := result.([]interface{}); ok {
		result = names[0]
	}
	if name, ok := result.(string); ok {
		return name, nil
	}
	return "", errors.Errorf("result isn't string: %v", result)
}

// DownTotal returns the total downloaded metric reported by this Client instance (bytes)
func (r *Client) DownTotal(ctx context.Context) (int, error) {
	result, err := r.xmlrpcClient.Call(ctx, "throttle.global_down.total")
	if err != nil {
		return 0, errors.Wrap(err, "throttle.global_down.total XMLRPC call failed")
	}
	if totals, ok := result.([]interface{}); ok {
		result = totals[0]
	}
	if total, ok := result.(int); ok {
		return total, nil
	}
	return 0, errors.Errorf("result isn't int: %v", result)
}

// DownRate returns the current download rate reported by this Client instance (bytes/s)
func (r *Client) DownRate(ctx context.Context) (int, error) {
	result, err := r.xmlrpcClient.Call(ctx, "throttle.global_down.rate")
	if err != nil {
		return 0, errors.Wrap(err, "throttle.global_down.rate XMLRPC call failed")
	}
	if totals, ok := result.([]interface{}); ok {
		result = totals[0]
	}
	if total, ok := result.(int); ok {
		return total, nil
	}
	return 0, errors.Errorf("result isn't int: %v", result)
}

// UpTotal returns the total uploaded metric reported by this Client instance (bytes)
func (r *Client) UpTotal(ctx context.Context) (int, error) {
	result, err := r.xmlrpcClient.Call(ctx, "throttle.global_up.total")
	if err != nil {
		return 0, errors.Wrap(err, "throttle.global_up.total XMLRPC call failed")
	}
	if totals, ok := result.([]interface{}); ok {
		result = totals[0]
	}
	if total, ok := result.(int); ok {
		return total, nil
	}
	return 0, errors.Errorf("result isn't int: %v", result)
}

// UpRate returns the current upload rate reported by this Client instance (bytes/s)
func (r *Client) UpRate(ctx context.Context) (int, error) {
	result, err := r.xmlrpcClient.Call(ctx, "throttle.global_up.rate")
	if err != nil {
		return 0, errors.Wrap(err, "throttle.global_up.rate XMLRPC call failed")
	}
	if totals, ok := result.([]interface{}); ok {
		result = totals[0]
	}
	if total, ok := result.(int); ok {
		return total, nil
	}
	return 0, errors.Errorf("result isn't int: %v", result)
}

// GetTorrents returns all the torrents reported by this Client instance
func (r *Client) GetTorrents(ctx context.Context, view View) ([]Torrent, error) {
	args := []interface{}{"", string(view), DName.Query(), DSizeInBytes.Query(), DHash.Query(), DLabel.Query(), DDirectory.Query(), DIsActive.Query(), DComplete.Query(), DRatio.Query(), DCreationTime.Query(), DFinishedTime.Query(), DStartedTime.Query()}
	results, err := r.xmlrpcClient.Call(ctx, "d.multicall2", args...)
	var torrents []Torrent
	if err != nil {
		return torrents, errors.Wrap(err, "d.multicall2 XMLRPC call failed")
	}
	for _, outerResult := range results.([]interface{}) {
		for _, innerResult := range outerResult.([]interface{}) {
			torrentData := innerResult.([]interface{})
			torrents = append(torrents, Torrent{
				Hash:      torrentData[2].(string),
				Name:      torrentData[0].(string),
				Path:      torrentData[4].(string),
				Size:      torrentData[1].(int),
				Label:     torrentData[3].(string),
				Completed: torrentData[6].(int) > 0,
				Ratio:     float64(torrentData[7].(int)) / float64(1000),
				Created:   time.Unix(int64(torrentData[8].(int)), 0),
				Finished:  time.Unix(int64(torrentData[9].(int)), 0),
				Started:   time.Unix(int64(torrentData[10].(int)), 0),
			})
		}
	}
	return torrents, nil
}

// GetTorrent returns the torrent identified by the given hash
func (r *Client) GetTorrent(ctx context.Context, hash string) (Torrent, error) {
	var t Torrent
	t.Hash = hash
	// Name
	results, err := r.xmlrpcClient.Call(ctx, "d.name", t.Hash)
	if err != nil {
		return t, errors.Wrap(err, "d.name XMLRPC call failed")
	}
	t.Name = results.([]interface{})[0].(string)
	// Size
	results, err = r.xmlrpcClient.Call(ctx, "d.size_bytes", t.Hash)
	if err != nil {
		return t, errors.Wrap(err, "d.size_bytes XMLRPC call failed")
	}
	t.Size = results.([]interface{})[0].(int)
	// Label
	results, err = r.xmlrpcClient.Call(ctx, "d.custom1", t.Hash)
	if err != nil {
		return t, errors.Wrap(err, "d.custom1 XMLRPC call failed")
	}
	t.Label = results.([]interface{})[0].(string)
	// Path
	results, err = r.xmlrpcClient.Call(ctx, "d.directory", t.Hash)
	if err != nil {
		return t, errors.Wrap(err, "d.directory XMLRPC call failed")
	}
	t.Path = results.([]interface{})[0].(string)
	// Completed
	results, err = r.xmlrpcClient.Call(ctx, "d.complete", t.Hash)
	if err != nil {
		return t, errors.Wrap(err, "d.complete XMLRPC call failed")
	}
	t.Completed = results.([]interface{})[0].(int) > 0
	// Ratio
	results, err = r.xmlrpcClient.Call(ctx, "d.ratio", t.Hash)
	if err != nil {
		return t, errors.Wrap(err, "d.ratio XMLRPC call failed")
	}
	t.Ratio = float64(results.([]interface{})[0].(int)) / float64(1000)
	// Created
	results, err = r.xmlrpcClient.Call(ctx, string(DCreationTime), t.Hash)
	if err != nil {
		return t, errors.Wrap(err, fmt.Sprintf("%s XMLRPC call failed", string(DCreationTime)))
	}
	t.Created = time.Unix(int64(results.([]interface{})[0].(int)), 0)
	// Finished
	results, err = r.xmlrpcClient.Call(ctx, string(DFinishedTime), t.Hash)
	if err != nil {
		return t, errors.Wrap(err, fmt.Sprintf("%s XMLRPC call failed", string(DFinishedTime)))
	}
	t.Finished = time.Unix(int64(results.([]interface{})[0].(int)), 0)
	// Started
	results, err = r.xmlrpcClient.Call(ctx, string(DStartedTime), t.Hash)
	if err != nil {
		return t, errors.Wrap(err, fmt.Sprintf("%s XMLRPC call failed", string(DStartedTime)))
	}
	t.Created = time.Unix(int64(results.([]interface{})[0].(int)), 0)

	return t, nil
}

// Delete removes the torrent
func (r *Client) Delete(ctx context.Context, t Torrent) error {
	_, err := r.xmlrpcClient.Call(ctx, "d.erase", t.Hash)
	if err != nil {
		return errors.Wrap(err, "d.erase XMLRPC call failed")
	}
	return nil
}

// GetFiles returns all the files for a given `Torrent`
func (r *Client) GetFiles(ctx context.Context, t Torrent) ([]File, error) {
	args := []interface{}{t.Hash, 0, FPath.Query(), FSizeInBytes.Query()}
	results, err := r.xmlrpcClient.Call(ctx, "f.multicall", args...)
	var files []File
	if err != nil {
		return files, errors.Wrap(err, "f.multicall XMLRPC call failed")
	}
	for _, outerResult := range results.([]interface{}) {
		for _, innerResult := range outerResult.([]interface{}) {
			fileData := innerResult.([]interface{})
			files = append(files, File{
				Path: fileData[0].(string),
				Size: fileData[1].(int),
			})
		}
	}
	return files, nil
}

// SetLabel sets the label on the given Torrent
func (r *Client) SetLabel(ctx context.Context, t Torrent, newLabel string) error {
	t.Label = newLabel
	args := []interface{}{t.Hash, newLabel}
	if _, err := r.xmlrpcClient.Call(ctx, "d.custom1.set", args...); err != nil {
		return errors.Wrap(err, "d.custom1.set XMLRPC call failed")
	}
	return nil
}

// GetStatus returns the Status for a given Torrent
func (r *Client) GetStatus(ctx context.Context, t Torrent) (Status, error) {
	var s Status
	// Completed
	results, err := r.xmlrpcClient.Call(ctx, "d.complete", t.Hash)
	if err != nil {
		return s, errors.Wrap(err, "d.complete XMLRPC call failed")
	}
	s.Completed = results.([]interface{})[0].(int) > 0
	// CompletedBytes
	results, err = r.xmlrpcClient.Call(ctx, "d.completed_bytes", t.Hash)
	if err != nil {
		return s, errors.Wrap(err, "d.completed_bytes XMLRPC call failed")
	}
	s.CompletedBytes = results.([]interface{})[0].(int)
	// DownRate
	results, err = r.xmlrpcClient.Call(ctx, "d.down.rate", t.Hash)
	if err != nil {
		return s, errors.Wrap(err, "d.down.rate XMLRPC call failed")
	}
	s.DownRate = results.([]interface{})[0].(int)
	// UpRate
	results, err = r.xmlrpcClient.Call(ctx, "d.up.rate", t.Hash)
	if err != nil {
		return s, errors.Wrap(err, "d.up.rate XMLRPC call failed")
	}
	s.UpRate = results.([]interface{})[0].(int)
	// Ratio
	results, err = r.xmlrpcClient.Call(ctx, "d.ratio", t.Hash)
	if err != nil {
		return s, errors.Wrap(err, "d.ratio XMLRPC call failed")
	}
	s.Ratio = float64(results.([]interface{})[0].(int)) / float64(1000)
	// Size
	results, err = r.xmlrpcClient.Call(ctx, "d.size_bytes", t.Hash)
	if err != nil {
		return s, errors.Wrap(err, "d.size_bytes XMLRPC call failed")
	}
	s.Size = results.([]interface{})[0].(int)
	return s, nil
}

// StartTorrent starts the torrent
func (r *Client) StartTorrent(ctx context.Context, t Torrent) error {
	_, err := r.xmlrpcClient.Call(ctx, "d.start", t.Hash)
	if err != nil {
		return errors.Wrap(err, "d.start XMLRPC call failed")
	}
	return nil
}

// StopTorrent stops the torrent
func (r *Client) StopTorrent(ctx context.Context, t Torrent) error {
	_, err := r.xmlrpcClient.Call(ctx, "d.stop", t.Hash)
	if err != nil {
		return errors.Wrap(err, "d.stop XMLRPC call failed")
	}
	return nil
}

// CloseTorrent closes the torrent
func (r *Client) CloseTorrent(ctx context.Context, t Torrent) error {
	_, err := r.xmlrpcClient.Call(ctx, "d.close", t.Hash)
	if err != nil {
		return errors.Wrap(err, "d.close XMLRPC call failed")
	}
	return nil
}

// OpenTorrent opens the torrent
func (r *Client) OpenTorrent(ctx context.Context, t Torrent) error {
	_, err := r.xmlrpcClient.Call(ctx, "d.open", t.Hash)
	if err != nil {
		return errors.Wrap(err, "d.open XMLRPC call failed")
	}
	return nil
}

// PauseTorrent pauses the torrent
func (r *Client) PauseTorrent(ctx context.Context, t Torrent) error {
	_, err := r.xmlrpcClient.Call(ctx, "d.pause", t.Hash)
	if err != nil {
		return errors.Wrap(err, "d.pause XMLRPC call failed")
	}
	return nil
}

// ResumeTorrent resumes the torrent
func (r *Client) ResumeTorrent(ctx context.Context, t Torrent) error {
	_, err := r.xmlrpcClient.Call(ctx, "d.resume", t.Hash)
	if err != nil {
		return errors.Wrap(err, "d.resume XMLRPC call failed")
	}
	return nil
}

// IsActive checks if the torrent is active
func (r *Client) IsActive(ctx context.Context, t Torrent) (bool, error) {
	results, err := r.xmlrpcClient.Call(ctx, "d.is_active", t.Hash)
	if err != nil {
		return false, errors.Wrap(err, "d.is_active XMLRPC call failed")
	}
	// active = 1; inactive = 0
	return results.([]interface{})[0].(int) == 1, nil
}

// IsOpen checks if the torrent is open
func (r *Client) IsOpen(ctx context.Context, t Torrent) (bool, error) {
	results, err := r.xmlrpcClient.Call(ctx, "d.is_open", t.Hash)
	if err != nil {
		return false, errors.Wrap(err, "d.is_open XMLRPC call failed")
	}
	// open = 1; closed = 0
	return results.([]interface{})[0].(int) == 1, nil
}

// State returns the state that the torrent is into
// It returns: 0 for stopped, 1 for started/paused
func (r *Client) State(ctx context.Context, t Torrent) (int, error) {
	results, err := r.xmlrpcClient.Call(ctx, "d.state", t.Hash)
	if err != nil {
		return 0, errors.Wrap(err, "d.state XMLRPC call failed")
	}
	return results.([]interface{})[0].(int), nil
}
