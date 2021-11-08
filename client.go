// Copyright 2021 The Libsacloud-v86 Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v86

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sacloud/libsacloud/v2/sacloud"
)

type Client struct {
	httpRequestTimeout time.Duration

	requestStream io.Writer
	responseDir   string

	mu sync.Mutex
}

func NewClient(requestStream io.Writer, responsePath string) (sacloud.APICaller, error) {
	return &Client{
		httpRequestTimeout: 30 * time.Second,
		requestStream:      requestStream,
		responseDir:        responsePath,
	}, nil
}

func (c *Client) Do(ctx context.Context, method, url string, body interface{}) ([]byte, error) {
	var bodyString string

	if body != nil {
		var bodyJSON []byte
		bodyJSON, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyString = bytes.NewBuffer(bodyJSON).String()
		if method == "GET" {
			url = fmt.Sprintf("%s?%s", url, bodyJSON)
		}
	}

	return c.do(ctx, &Request{
		UUID:   uuid.NewString(),
		Method: method,
		URL:    url,
		Body:   bodyString,
	})
}

func (c *Client) do(ctx context.Context, req *Request) ([]byte, error) {
	reqCtx, cancel := context.WithTimeout(ctx, c.httpRequestTimeout)
	defer cancel()

	if err := c.postRequestMessage(reqCtx, req); err != nil {
		return nil, err
	}
	return c.handleResponse(reqCtx, req)
}

func (c *Client) handleResponse(ctx context.Context, req *Request) ([]byte, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_, err := os.Stat(c.responseJobFileName(req))
			if err != nil {
				continue
			}
			data, err := os.ReadFile(c.responseDataFileName(req))
			if err != nil {
				return nil, err
			}
			if err := os.RemoveAll(c.responseJobFileName(req)); err != nil {
				return nil, err
			}
			if err := os.RemoveAll(c.responseDataFileName(req)); err != nil {
				return nil, err
			}

			return c.parseResponse(ctx, req, data)
		case <-ctx.Done():
			return nil, fmt.Errorf("request canceled: %s", ctx.Err())
		}
	}
}

func (c *Client) parseResponse(ctx context.Context, req *Request, data []byte) ([]byte, error) {
	var res Response
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, err
	}
	if res.Error != "" {
		parsedURL, _ := url.Parse(req.URL)
		var errRes *sacloud.APIErrorResponse
		if err := json.Unmarshal([]byte(res.Error), errRes); err != nil {
			return nil, sacloud.NewAPIError(req.Method, parsedURL, req.Body, res.StatusCode, errRes)
		}
		return nil, fmt.Errorf("unknown error: %s", res.Error)
	}

	return []byte(res.Result), nil
}

func (c *Client) postRequestMessage(ctx context.Context, req *Request) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := req.toJSON()
	if err != nil {
		return err
	}

	_, err = c.requestStream.Write(append(data, []byte("\n")...))
	return err
}

func (c *Client) responseJobFileName(req *Request) string {
	return filepath.Join(c.responseDir, req.UUID+".done")
}

func (c *Client) responseDataFileName(req *Request) string {
	return filepath.Join(c.responseDir, req.UUID)
}
