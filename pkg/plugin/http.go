package starlarklib

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func makeHTTPModule() *starlarkstruct.Module {
	return &starlarkstruct.Module{
		Name: "http",
		Members: starlark.StringDict{
			"get":     starlark.NewBuiltin("http.get", httpGet),
			"post":    starlark.NewBuiltin("http.post", httpPost),
			"put":     starlark.NewBuiltin("http.put", httpPut),
			"delete":  starlark.NewBuiltin("http.delete", httpDelete),
			"request": starlark.NewBuiltin("http.request", httpRequest),
			"client":  starlark.NewBuiltin("http.client", httpNewClient),
		},
	}
}

type httpClient struct {
	client *http.Client
}

func (h *httpClient) String() string        { return "http.Client" }
func (h *httpClient) Type() string          { return "http.Client" }
func (h *httpClient) Freeze()               {}
func (h *httpClient) Truth() starlark.Bool  { return starlark.True }
func (h *httpClient) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable type: http.Client") }

func httpNewClient(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var timeout int64 = 30
	var insecureSkipVerify bool

	if err := starlark.UnpackArgs(
		"http.client",
		args,
		kwargs,
		"timeout?", &timeout,
		"insecure_skip_verify?", &insecureSkipVerify,
	); err != nil {
		return nil, err
	}

	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecureSkipVerify,
			},
		},
	}

	return &httpClient{client: client}, nil
}

func httpGet(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var url string
	var headers *starlark.Dict

	if err := starlark.UnpackArgs(
		"http.get",
		args,
		kwargs,
		"url", &url,
		"headers?", &headers,
	); err != nil {
		return nil, err
	}

	return doHTTPRequest("GET", url, nil, headers, nil)
}

func httpPost(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var url string
	var data starlark.Value
	var headers *starlark.Dict

	if err := starlark.UnpackArgs(
		"http.post",
		args,
		kwargs,
		"url", &url,
		"data?", &data,
		"headers?", &headers,
	); err != nil {
		return nil, err
	}

	return doHTTPRequest("POST", url, data, headers, nil)
}

func httpPut(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var url string
	var data starlark.Value
	var headers *starlark.Dict

	if err := starlark.UnpackArgs(
		"http.put",
		args,
		kwargs,
		"url", &url,
		"data?", &data,
		"headers?", &headers,
	); err != nil {
		return nil, err
	}

	return doHTTPRequest("PUT", url, data, headers, nil)
}

func httpDelete(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var url string
	var headers *starlark.Dict

	if err := starlark.UnpackArgs(
		"http.delete",
		args,
		kwargs,
		"url", &url,
		"headers?", &headers,
	); err != nil {
		return nil, err
	}

	return doHTTPRequest("DELETE", url, nil, headers, nil)
}

func httpRequest(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var method, url string
	var data starlark.Value
	var headers *starlark.Dict
	var client starlark.Value

	if err := starlark.UnpackArgs(
		"http.request",
		args,
		kwargs,
		"method", &method,
		"url", &url,
		"data?", &data,
		"headers?", &headers,
		"client?", &client,
	); err != nil {
		return nil, err
	}

	var httpCli *http.Client
	if c, ok := client.(*httpClient); ok {
		httpCli = c.client
	}

	return doHTTPRequest(method, url, data, headers, httpCli)
}

func doHTTPRequest(
	method, url string,
	data starlark.Value,
	headers *starlark.Dict,
	client *http.Client,
) (starlark.Value, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	var body io.Reader
	if data != nil && data != starlark.None {
		bodyBytes, err := starlarkValueToBytes(data)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	if headers != nil {
		for _, item := range headers.Items() {
			key, ok := item[0].(starlark.String)
			if !ok {
				continue
			}
			value, ok := item[1].(starlark.String)
			if !ok {
				continue
			}
			req.Header.Set(string(key), string(value))
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Build response dict
	result := starlark.NewDict(5)
	result.SetKey(starlark.String("status_code"), starlark.MakeInt(resp.StatusCode))
	result.SetKey(starlark.String("body"), starlark.String(respBody))
	result.SetKey(starlark.String("ok"), starlark.Bool(resp.StatusCode >= 200 && resp.StatusCode < 300))

	// Convert headers
	headersDict := starlark.NewDict(len(resp.Header))
	for k, v := range resp.Header {
		if len(v) > 0 {
			headersDict.SetKey(starlark.String(k), starlark.String(v[0]))
		}
	}
	result.SetKey(starlark.String("headers"), headersDict)

	return result, nil
}

func starlarkValueToBytes(v starlark.Value) ([]byte, error) {
	switch val := v.(type) {
	case starlark.String:
		return []byte(val), nil
	case *starlark.Dict:
		goVal := starlarkToGoValue(val)
		return json.Marshal(goVal)
	default:
		return []byte(v.String()), nil
	}
}
