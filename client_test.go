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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/sacloud/libsacloud/v2/sacloud"
)

func TestClient(t *testing.T) {
	caller, cleanup, errCh := setup(t)
	defer cleanup()

	go func() {
		for err := range errCh {
			t.Logf("error: %s", err)
		}
	}()

	_, err := sacloud.NewAuthStatusOp(caller).Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

func setup(t *testing.T) (sacloud.APICaller, func(), <-chan error) {
	inr, inw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = inr

	outDir, err := ioutil.TempDir("", "libsacloud-v86-test")
	if err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error)
	go watchInput(os.Stdin, outDir, errCh)

	caller, err := NewClient(inw, outDir)
	if err != nil {
		t.Fatal(err)
	}

	return caller, func() {
		os.RemoveAll(outDir)
		inw.Close()
	}, errCh
}

func watchInput(reader io.Reader, outDir string, errCh chan<- error) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		if err := handleInput(outDir, scanner.Text()); err != nil {
			errCh <- err
			return
		}
	}
	errCh <- scanner.Err()
}

func handleInput(outDir, line string) error {
	fmt.Printf("[DEBUG] %s\n", line)
	var req Request
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		return err
	}

	actualClient, err := sacloud.NewClientFromEnv()
	if err != nil {
		return err
	}

	res, err := actualClient.Do(context.Background(), req.Method, req.URL, req.Body)

	response := &Response{}
	if err != nil {
		response.Error = err.Error()
	}
	if res != nil {
		response.Result = string(res)
	}

	resData, err := json.Marshal(response)
	if err != nil {
		return err
	}

	outPath := filepath.Join(outDir, req.UUID)
	if err := os.WriteFile(outPath, resData, 0700); err != nil {
		return err
	}

	outDonePath := filepath.Join(outDir, req.UUID+".done")
	if err := os.WriteFile(outDonePath, []byte{}, 0700); err != nil {
		return err
	}
	return nil
}
