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

// WithCustomClient uses the given http.Client for requests. Basic auth from the Config is kept, but TLSSkipVerify
// is not applied, configure TLS on the given client instead.
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
	return NewClientWithOpts(cfg)
}

// WithHTTPClient allows you to a provide a custom http.Client.
// Basic auth from the Config is kept. TLSSkipVerify is not applied, configure TLS on the given client instead.
func (r *Client) WithHTTPClient(client *http.Client) *Client {
	WithCustomClient(client)(r)
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

	// command marks Field as a complete command name that is called with
	// Value, rather than a field whose ".set" command is called.
	command bool
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
	// DPriority represents the bandwidth priority of a "Downloading Item": 0 off, 1 low, 2 normal, 3 high
	DPriority Field = "d.priority"

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
	return &FieldValue{Field: f, Value: value}
}

// Cmd returns the representation of the field which allows it to be used a command with Client
func (f Field) Cmd() string {
	return string(f)
}

// Command returns a FieldValue which calls cmd with value on a newly added torrent, for commands that have no
// ".set" form, such as "view.set_visible" which puts the torrent in a view or ratio group:
//
//	AddTorrent(fileData, Command("view.set_visible", "rat_0"))
func Command(cmd string, value string) *FieldValue {
	return &FieldValue{Field: Field(cmd), Value: value, command: true}
}

func (f *FieldValue) String() string {
	if f.command {
		return fmt.Sprintf("%s=\"%s\"", f.Field, f.Value)
	}
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
//	AddStopped("some-url", &FieldValue{Field: "d.custom1", Value: "my-label"})
//
// Or:
//
//	AddStopped("some-url", DLabel.SetValue("my-label"))
//
// Adds the Torrent by URL (stopped) and  sets the label and base path
//
//	AddStopped("some-url", &FieldValue{Field: "d.custom1", Value: "my-label"}, &FieldValue{Field: "d.base_path", Value: "/some/valid/path"})
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
	return r.callString(ctx, "network.bind_address")
}

// Name returns the name reported by this Client instance
func (r *Client) Name(ctx context.Context) (string, error) {
	return r.callString(ctx, "system.hostname")
}

// DownTotal returns the total downloaded metric reported by this Client instance (bytes)
func (r *Client) DownTotal(ctx context.Context) (int, error) {
	return r.callInt(ctx, "throttle.global_down.total")
}

// DownRate returns the current download rate reported by this Client instance (bytes/s)
func (r *Client) DownRate(ctx context.Context) (int, error) {
	return r.callInt(ctx, "throttle.global_down.rate")
}

// UpTotal returns the total uploaded metric reported by this Client instance (bytes)
func (r *Client) UpTotal(ctx context.Context) (int, error) {
	return r.callInt(ctx, "throttle.global_up.total")
}

// UpRate returns the current upload rate reported by this Client instance (bytes/s)
func (r *Client) UpRate(ctx context.Context) (int, error) {
	return r.callInt(ctx, "throttle.global_up.rate")
}

// Views returns the names of all views, including persistent views used as ratio groups
func (r *Client) Views(ctx context.Context) ([]View, error) {
	result, err := r.xmlrpcClient.Call(ctx, "view.list")
	if err != nil {
		return nil, errors.Wrap(err, "view.list XMLRPC call failed")
	}
	if outer, ok := result.([]interface{}); ok && len(outer) == 1 {
		if inner, ok := outer[0].([]interface{}); ok {
			result = inner
		}
	}
	names, ok := result.([]interface{})
	if !ok {
		return nil, errors.Errorf("result isn't list: %v", result)
	}
	views := make([]View, 0, len(names))
	for _, name := range names {
		s, ok := name.(string)
		if !ok {
			return nil, errors.Errorf("view name isn't string: %v", name)
		}
		views = append(views, View(s))
	}
	return views, nil
}

// GetTorrents returns all the torrents reported by this Client instance
func (r *Client) GetTorrents(ctx context.Context, view View) ([]Torrent, error) {
	fields := []Field{DName, DSizeInBytes, DHash, DLabel, DDirectory, DIsActive, DComplete, DRatio, DCreationTime, DFinishedTime, DStartedTime}
	args := []interface{}{"", string(view)}
	for _, f := range fields {
		args = append(args, f.Query())
	}
	results, err := r.xmlrpcClient.Call(ctx, "d.multicall2", args...)
	if err != nil {
		return nil, errors.Wrap(err, "d.multicall2 XMLRPC call failed")
	}
	rows, err := singleList(results)
	if err != nil {
		return nil, errors.Wrap(err, "d.multicall2 XMLRPC call failed")
	}
	var torrents []Torrent
	for _, row := range rows {
		values, err := toList(row)
		if err != nil {
			return nil, errors.Wrap(err, "d.multicall2 XMLRPC call failed")
		}
		fv := &fieldValues{fields: fields, values: values}
		t := Torrent{
			Hash:      fv.stringValue(DHash),
			Name:      fv.stringValue(DName),
			Path:      fv.stringValue(DDirectory),
			Size:      fv.intValue(DSizeInBytes),
			Label:     fv.stringValue(DLabel),
			Completed: fv.intValue(DComplete) > 0,
			Ratio:     float64(fv.intValue(DRatio)) / float64(1000),
			Created:   fv.timeValue(DCreationTime),
			Finished:  fv.timeValue(DFinishedTime),
			Started:   fv.timeValue(DStartedTime),
		}
		if fv.err != nil {
			return nil, errors.Wrap(fv.err, "d.multicall2 XMLRPC call failed")
		}
		torrents = append(torrents, t)
	}
	return torrents, nil
}

// GetTorrent returns the torrent identified by the given hash
func (r *Client) GetTorrent(ctx context.Context, hash string) (Torrent, error) {
	t := Torrent{Hash: hash}
	fv, err := r.multicall(ctx, []Field{DName, DSizeInBytes, DLabel, DDirectory, DComplete, DRatio, DCreationTime, DFinishedTime, DStartedTime}, hash)
	if err != nil {
		return t, err
	}
	t.Name = fv.stringValue(DName)
	t.Size = fv.intValue(DSizeInBytes)
	t.Label = fv.stringValue(DLabel)
	t.Path = fv.stringValue(DDirectory)
	t.Completed = fv.intValue(DComplete) > 0
	t.Ratio = float64(fv.intValue(DRatio)) / float64(1000)
	t.Created = fv.timeValue(DCreationTime)
	t.Finished = fv.timeValue(DFinishedTime)
	t.Started = fv.timeValue(DStartedTime)
	if fv.err != nil {
		return Torrent{Hash: hash}, errors.Wrap(fv.err, "system.multicall XMLRPC call failed")
	}
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
	fields := []Field{FPath, FSizeInBytes}
	args := []interface{}{t.Hash, 0}
	for _, f := range fields {
		args = append(args, f.Query())
	}
	results, err := r.xmlrpcClient.Call(ctx, "f.multicall", args...)
	if err != nil {
		return nil, errors.Wrap(err, "f.multicall XMLRPC call failed")
	}
	rows, err := singleList(results)
	if err != nil {
		return nil, errors.Wrap(err, "f.multicall XMLRPC call failed")
	}
	var files []File
	for _, row := range rows {
		values, err := toList(row)
		if err != nil {
			return nil, errors.Wrap(err, "f.multicall XMLRPC call failed")
		}
		fv := &fieldValues{fields: fields, values: values}
		f := File{
			Path: fv.stringValue(FPath),
			Size: fv.intValue(FSizeInBytes),
		}
		if fv.err != nil {
			return nil, errors.Wrap(fv.err, "f.multicall XMLRPC call failed")
		}
		files = append(files, f)
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
	fv, err := r.multicall(ctx, []Field{DComplete, DCompletedBytes, DDownRate, DUpRate, DRatio, DSizeInBytes}, t.Hash)
	if err != nil {
		return Status{}, err
	}
	s := Status{
		Completed:      fv.intValue(DComplete) > 0,
		CompletedBytes: fv.intValue(DCompletedBytes),
		DownRate:       fv.intValue(DDownRate),
		UpRate:         fv.intValue(DUpRate),
		Ratio:          float64(fv.intValue(DRatio)) / float64(1000),
		Size:           fv.intValue(DSizeInBytes),
	}
	if fv.err != nil {
		return Status{}, errors.Wrap(fv.err, "system.multicall XMLRPC call failed")
	}
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
	active, err := r.callInt(ctx, "d.is_active", t.Hash)
	if err != nil {
		return false, err
	}
	// active = 1; inactive = 0
	return active == 1, nil
}

// IsOpen checks if the torrent is open
func (r *Client) IsOpen(ctx context.Context, t Torrent) (bool, error) {
	open, err := r.callInt(ctx, "d.is_open", t.Hash)
	if err != nil {
		return false, err
	}
	// open = 1; closed = 0
	return open == 1, nil
}

// State returns the state that the torrent is into
// It returns: 0 for stopped, 1 for started/paused
func (r *Client) State(ctx context.Context, t Torrent) (int, error) {
	return r.callInt(ctx, "d.state", t.Hash)
}

// callSingle calls method and returns the single value of its response.
func (r *Client) callSingle(ctx context.Context, method string, args ...interface{}) (interface{}, error) {
	result, err := r.xmlrpcClient.Call(ctx, method, args...)
	if err == nil {
		result, err = single(result)
	}
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("%s XMLRPC call failed", method))
	}
	return result, nil
}

// callString calls method and returns its single string value.
func (r *Client) callString(ctx context.Context, method string, args ...interface{}) (string, error) {
	v, err := r.callSingle(ctx, method, args...)
	if err != nil {
		return "", err
	}
	s, err := toString(v)
	if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("%s XMLRPC call failed", method))
	}
	return s, nil
}

// callInt calls method and returns its single int value.
func (r *Client) callInt(ctx context.Context, method string, args ...interface{}) (int, error) {
	v, err := r.callSingle(ctx, method, args...)
	if err != nil {
		return 0, err
	}
	i, err := toInt(v)
	if err != nil {
		return 0, errors.Wrap(err, fmt.Sprintf("%s XMLRPC call failed", method))
	}
	return i, nil
}

// multicallEntry is one call in a system.multicall request. It is a struct rather than a map so methodName is
// always encoded before params, which tinyxml2 builds of rTorrent require.
type multicallEntry struct {
	MethodName string        `xml:"methodName"`
	Params     []interface{} `xml:"params"`
}

// multicall calls the command of every field with params in a single system.multicall request.
func (r *Client) multicall(ctx context.Context, fields []Field, params ...interface{}) (*fieldValues, error) {
	calls := make([]interface{}, 0, len(fields))
	for _, f := range fields {
		calls = append(calls, multicallEntry{MethodName: f.Cmd(), Params: params})
	}
	result, err := r.xmlrpcClient.Call(ctx, "system.multicall", calls)
	if err != nil {
		return nil, errors.Wrap(err, "system.multicall XMLRPC call failed")
	}
	results, err := singleList(result)
	if err != nil {
		return nil, errors.Wrap(err, "system.multicall XMLRPC call failed")
	}
	if len(results) != len(fields) {
		return nil, errors.Errorf("system.multicall XMLRPC call failed: got %d results for %d calls", len(results), len(fields))
	}

	values := make([]interface{}, len(fields))
	for i, entry := range results {
		if fault, ok := entry.(map[string]interface{}); ok {
			code, _ := fault["faultCode"].(int)
			msg, _ := fault["faultString"].(string)
			return nil, errors.Wrap(&xmlrpc.Fault{Code: code, Message: msg}, fmt.Sprintf("%s XMLRPC call failed", fields[i]))
		}
		v, err := single(entry)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("%s XMLRPC call failed", fields[i]))
		}
		values[i] = v
	}
	return &fieldValues{fields: fields, values: values}, nil
}

// single returns the only value of a response, rTorrent returns every result as a list of params.
func single(result interface{}) (interface{}, error) {
	values, err := toList(result)
	if err != nil {
		return nil, err
	}
	if len(values) != 1 {
		return nil, errors.Errorf("result has %d values, want 1: %v", len(values), values)
	}
	return values[0], nil
}

// singleList returns the only value of a response as a list.
func singleList(result interface{}) ([]interface{}, error) {
	v, err := single(result)
	if err != nil {
		return nil, err
	}
	return toList(v)
}

func toList(v interface{}) ([]interface{}, error) {
	list, ok := v.([]interface{})
	if !ok {
		return nil, errors.Errorf("result isn't list: %v", v)
	}
	return list, nil
}

func toString(v interface{}) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", errors.Errorf("result isn't string: %v", v)
	}
	return s, nil
}

func toInt(v interface{}) (int, error) {
	i, ok := v.(int)
	if !ok {
		return 0, errors.Errorf("result isn't int: %v", v)
	}
	return i, nil
}

// fieldValues reads the values rTorrent returned for fields, in the same order, and keeps the first error.
type fieldValues struct {
	fields []Field
	values []interface{}
	err    error
}

func (fv *fieldValues) value(f Field) (interface{}, bool) {
	if fv.err != nil {
		return nil, false
	}
	for i, field := range fv.fields {
		if field != f {
			continue
		}
		if i >= len(fv.values) {
			fv.err = errors.Errorf("%s: missing value", f)
			return nil, false
		}
		return fv.values[i], true
	}
	fv.err = errors.Errorf("%s: not requested", f)
	return nil, false
}

func (fv *fieldValues) stringValue(f Field) string {
	v, ok := fv.value(f)
	if !ok {
		return ""
	}
	s, err := toString(v)
	if err != nil {
		fv.err = errors.Wrap(err, string(f))
	}
	return s
}

func (fv *fieldValues) intValue(f Field) int {
	v, ok := fv.value(f)
	if !ok {
		return 0
	}
	i, err := toInt(v)
	if err != nil {
		fv.err = errors.Wrap(err, string(f))
	}
	return i
}

func (fv *fieldValues) timeValue(f Field) time.Time {
	return time.Unix(int64(fv.intValue(f)), 0)
}
