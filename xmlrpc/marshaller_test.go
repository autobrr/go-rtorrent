package xmlrpc

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarshal_Call(t *testing.T) {
	var buf bytes.Buffer
	err := Marshal(&buf, "load.raw_start", "", []byte("data"), `d.custom1.set="a<b"`, 3)
	require.NoError(t, err)

	want := "<methodCall><methodName>load.raw_start</methodName>\n<params>\n" +
		"  <param><value><string></string></value></param>\n" +
		"  <param><value><base64>ZGF0YQ==</base64></value></param>\n" +
		"  <param><value><string>d.custom1.set=&quot;a&lt;b&quot;</string></value></param>\n" +
		"  <param><value><int>3</int></value></param>\n" +
		"</params></methodCall>"
	require.Equal(t, want, buf.String())
}

func TestMarshal_RoundTrip(t *testing.T) {
	args := []interface{}{
		"text & more",
		42,
		true,
		1.5,
		[]byte{0x00, 0xff},
		[]interface{}{"a", 1, []interface{}{"nested"}},
		map[string]interface{}{"key": "value"},
	}

	var buf bytes.Buffer
	require.NoError(t, Marshal(&buf, "some.method", args...))

	name, params, fault, err := Unmarshal(&buf)
	require.NoError(t, err)
	require.Nil(t, fault)
	require.Equal(t, "some.method", name)
	require.Equal(t, args, params)
}

func TestUnmarshal_Response(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<methodResponse>
<params>
<param><value><array><data>
<value><string>main</string></value>
<value><i8>5665497088</i8></value>
</data></array></value></param>
</params>
</methodResponse>`

	name, params, fault, err := Unmarshal(strings.NewReader(body))
	require.NoError(t, err)
	require.Nil(t, fault)
	require.Empty(t, name)
	require.Equal(t, []interface{}{[]interface{}{"main", 5665497088}}, params)
}

func TestUnmarshal_Fault(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, Marshal(&buf, "", Fault{Code: -506, Message: "method 'nope' not defined"}))

	_, params, fault, err := Unmarshal(&buf)
	require.NoError(t, err)
	require.Empty(t, params)
	require.Equal(t, &Fault{Code: -506, Message: "method 'nope' not defined"}, fault)
}
