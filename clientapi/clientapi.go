/*
Copyright 2018 Graham Lee Bevan <graham.bevan@ntlworld.com>

This file is part of gostint.

gostint is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

gostint is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with gostint.  If not, see <https://www.gnu.org/licenses/>.
*/

package clientapi

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/hashicorp/vault/api"
)

var enableDebug bool
var token string

func debug(format string, a ...interface{}) {
	if !enableDebug {
		return
	}
	Debug(format, a...)
}

// Debug in color...
func Debug(format string, a ...interface{}) {
	t := time.Now()
	d := color.New(color.FgBlue).Add(color.Bold)
	d.Printf(t.Format(time.RFC3339)+" "+format, a...)
	fmt.Println()
}

// APIRequest structure the job request passed to the client api
type APIRequest struct {
	AppRoleID       *string
	AppSecretID     *string // AppRole auth or Token
	Token           *string
	GoStintRole     *string
	JobJSON         *string // request can be whole JSON:
	QName           *string // or can be passed as parameters:
	ContainerImage  *string
	ImagePullPolicy *string
	Content         *string
	EntryPoint      *string
	Run             *string
	WorkingDir      *string
	EnvVars         *string
	SecretRefs      *string
	SecretFileType  *string
	ContOnWarnings  *bool
	URL             *string
	VaultURL        *string
}

type job struct {
	QName           string   `json:"qname"`
	ContainerImage  string   `json:"container_image"`
	ImagePullPolicy string   `json:"image_pull_policy"`
	Content         string   `json:"content"`
	EntryPoint      []string `json:"entrypoint"`
	Run             []string `json:"run"`
	WorkingDir      string   `json:"working_directory"`
	EnvVars         []string `json:"env_vars"`
	SecretRefs      []string `json:"secret_refs"`
	SecretFileType  string   `json:"secret_file_type"`
	ContOnWarnings  bool     `json:"cont_on_warnings"`
}

// func buildJob(c APIRequest) (*[]byte, error) {
func buildJob(c APIRequest) (*job, error) {
	debug("Building Job Request")
	j := job{}

	if *c.JobJSON != "" {
		err := json.Unmarshal([]byte(*c.JobJSON), &j)
		if err != nil {
			return nil, err
		}
	}
	if *c.QName != "" {
		j.QName = *c.QName
	}
	if *c.ContainerImage != "" {
		j.ContainerImage = *c.ContainerImage
	}
	if *c.ImagePullPolicy != "" {
		j.ImagePullPolicy = *c.ImagePullPolicy
	}
	if *c.Content != "" {
		j.Content = *c.Content
	}
	if *c.EntryPoint != "" {
		// j.EntryPoint = *c.EntryPoint
		eps := make([]string, 0)
		err := json.Unmarshal([]byte(*c.EntryPoint), &eps)
		if err != nil {
			return nil, err
		}
		j.EntryPoint = eps
	}
	if *c.Run != "" {
		// j.Run = *c.Run
		eps := make([]string, 0)
		err := json.Unmarshal([]byte(*c.Run), &eps)
		if err != nil {
			return nil, err
		}
		j.Run = eps
	}
	if *c.WorkingDir != "" {
		j.WorkingDir = *c.WorkingDir
	}
	if *c.EnvVars != "" {
		eps := make([]string, 0)
		err := json.Unmarshal([]byte(*c.EnvVars), &eps)
		if err != nil {
			return nil, err
		}
		j.EnvVars = eps
	}
	if *c.SecretRefs != "" {
		// j.SecretRefs = *c.SecretRefs
		eps := make([]string, 0)
		err := json.Unmarshal([]byte(*c.SecretRefs), &eps)
		if err != nil {
			return nil, err
		}
		j.SecretRefs = eps
	}
	if *c.SecretFileType != "" {
		j.SecretFileType = *c.SecretFileType
	}
	if *c.ContOnWarnings {
		j.ContOnWarnings = *c.ContOnWarnings
	}
	return &j, nil
}

func getVaultClient(url string, c *APIRequest) (*api.Client, error) {
	debug("Getting Vault api connection %s", url)

	cfg := api.DefaultConfig()
	cfg.Address = url

	client, err := api.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	if *c.AppRoleID != "" && *c.AppSecretID != "" {
		debug("Using AppRole authentication")
		data := map[string]interface{}{
			"role_id":   *c.AppRoleID,
			"secret_id": *c.AppSecretID,
		}
		sec, err2 := client.Logical().Write("/auth/approle/login", data)
		if err2 != nil {
			return nil, err2
		}
		debug("policies %v", sec.Auth.Policies)
		*c.Token = sec.Auth.ClientToken
	}

	debug("Authenticating with Vault")
	client.SetToken(*c.Token)

	// Verify the token is good
	_, err = client.Logical().Read("auth/token/lookup-self")
	if err != nil {
		return nil, err
	}
	debug("Vault token authenticated ok")
	return client, nil
}

func submitJob(c *APIRequest, jsonBytes *[]byte, token string) (*submitResponse, error) {
	debug("Submitting job")
	req, err := http.NewRequest(
		"POST",
		fmt.Sprintf("%s/v1/api/job", *c.URL),
		bytes.NewBuffer(*jsonBytes),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Auth-Token", token)
	req.Header.Set("Content-Type", "application/json")

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			// TODO: parameterise this
			InsecureSkipVerify: true,
			// TODO: more Cert/CA options for trust
		},
	}

	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	debug("Response status: %s", resp.Status)
	debug("Response headers: %s", resp.Header)
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	debug("Response body:\n%s", string(body))

	subResp := submitResponse{}
	err = json.Unmarshal(body, &subResp)
	if err != nil {
		return nil, err
	}

	return &subResp, nil
}

type submitResponse struct {
	ID     string `json:"_id"`
	Status string `json:"status"`
	QName  string `json:"qname"`
}

// GetJob returns a job status from gostint
func GetJob(c *APIRequest, token string, ID string) (*GetResponse, error) {
	debug("Getting job state")
	req, err := http.NewRequest(
		"GET",
		fmt.Sprintf("%s/v1/api/job/%s", *c.URL, ID),
		nil,
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Auth-Token", token)
	req.Header.Set("Content-Type", "application/json")

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	debug("Response status: %s", resp.Status)
	debug("Response headers: %s", resp.Header)
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	debug("Response body:\n%s", string(body))

	getResp := GetResponse{}
	err = json.Unmarshal(body, &getResp)
	if err != nil {
		return nil, err
	}

	return &getResp, nil
}

// GetResponse structure holds response from gostint job query
type GetResponse struct {
	ID             string `json:"_id"`
	Status         string `json:"status"`
	NodeUUID       string `json:"node_uuid"`
	QName          string `json:"qname"`
	ContainerImage string `json:"container_image"`
	Submitted      string `json:"submitted"`
	Started        string `json:"started"`
	Ended          string `json:"ended"`
	Output         string `json:"output"`
	ReturnCode     int    `json:"return_code"`
}

func (r *GetResponse) String() string {
	return fmt.Sprintf(
		"Queue: %s, ID: %s, Status: %s, ReturnCode: %d",
		r.QName,
		r.ID,
		r.Status,
		r.ReturnCode,
	)
}

// RunJob to submit a job request to gostint api
func RunJob(c *APIRequest, debugLogging bool, pollSecs int, waitFor bool) (*GetResponse, error) {
	start := time.Now()

	enableDebug = debugLogging
	pollIntervalSecs := pollSecs

	job, err := buildJob(*c)
	if err != nil {
		return nil, err
	}

	if *c.VaultURL == "" {
		*c.VaultURL = os.Getenv("VAULT_ADDR")
	}

	vc, err := getVaultClient(*c.VaultURL, c)
	if err != nil {
		return nil, err
	}

	debug("Getting minimal token to authenticate with GoStint API")
	data := map[string]interface{}{
		"policies": []string{"default"},
	}
	sec, err := vc.Logical().Write("auth/token/create", data)
	if err != nil {
		return nil, err
	}
	apiToken := sec.Auth.ClientToken

	defer func() {
		debug("Revoking the minimal authentication token after use")
		_, err = vc.Logical().Write("auth/token/revoke-self", nil)
		if err != nil {
			log.Printf("Error: revoking token after job completed: %s", err)
		}
	}()

	debug("Getting Wrapped Secret_ID for the AppRole")
	vc.SetWrappingLookupFunc(func(op, path string) string { return "1h" })
	sec, err = vc.Logical().Write(
		fmt.Sprintf("auth/approle/role/%s/secret-id", *c.GoStintRole),
		nil,
	)
	if err != nil {
		return nil, err
	}
	wrapSecretID := sec.WrapInfo.Token
	vc.SetWrappingLookupFunc(nil)

	jsonBytes, err := json.Marshal(*job)
	if err != nil {
		return nil, err
	}

	debug("Encrypting the job payload")
	data = map[string]interface{}{
		"plaintext": base64.StdEncoding.EncodeToString(jsonBytes),
	}
	sec, err = vc.Logical().Write(
		fmt.Sprintf("transit/encrypt/%s", *c.GoStintRole),
		data,
	)
	if err != nil {
		return nil, err
	}
	encryptedPayload := sec.Data["ciphertext"]

	debug("Getting minimal limited use / ttl token for the cubbyhole")
	data = map[string]interface{}{
		"policies":  []string{"default"},
		"ttl":       "60m",
		"use_limit": 2,
	}
	sec, err = vc.Logical().Write("auth/token/create", data)
	if err != nil {
		return nil, err
	}
	cubbyToken := sec.Auth.ClientToken

	debug("Putting encrypted payload in a vault cubbyhole")
	vc.SetToken(cubbyToken)
	data = map[string]interface{}{
		"payload": encryptedPayload,
	}
	sec, err = vc.Logical().Write("cubbyhole/job", data)
	if err != nil {
		return nil, err
	}

	debug("Creating job request wrapper to submit")
	jWrap := jobWrapper{
		QName:        job.QName,
		CubbyToken:   cubbyToken,
		CubbyPath:    "cubbyhole/job",
		WrapSecretID: wrapSecretID,
	}
	jWrapBytes, err := json.Marshal(jWrap)
	if err != nil {
		return nil, err
	}

	subResp, err := submitJob(c, &jWrapBytes, apiToken)
	if err != nil {
		return nil, err
	}

	// loop until status != queued or running
	var getResp *GetResponse
	for {
		getResp, err = GetJob(c, apiToken, subResp.ID)
		if err != nil {
			return nil, err
		}
		if !waitFor {
			break
		}
		if getResp.Status != "queued" && getResp.Status != "running" {
			break
		}
		time.Sleep(time.Duration(pollIntervalSecs) * time.Second)
	}

	t := time.Now()
	elapsed := t.Sub(start)
	debug("Elapsed time: %.3f seconds", float64(elapsed/time.Millisecond)/1000.0)

	return getResp, nil
}

type jobWrapper struct {
	QName        string `json:"qname"`
	CubbyToken   string `json:"cubby_token"`
	CubbyPath    string `json:"cubby_path"`
	WrapSecretID string `json:"wrap_secret_id"`
}

func getContent(content *string) (*bytes.Buffer, error) {
	debug("Packing content from %s", *content)
	var buf bytes.Buffer
	if *content == "." {
		cwd, err := os.Getwd()
		if err != nil {
			return bytes.NewBuffer([]byte{}), err
		}
		*content = cwd
	}
	fi, err := os.Stat(*content)
	if err != nil {
		return bytes.NewBuffer([]byte{}), err
	}

	if fi.Mode().IsDir() { // if content points to a folder
		// from blog: https://medium.com/@skdomino/taring-untaring-files-in-go-6b07cf56bc07
		gzw := gzip.NewWriter(&buf)
		defer gzw.Close()
		tw := tar.NewWriter(gzw)
		defer tw.Close()

		// walk path
		err = filepath.Walk(*content, func(file string, fi os.FileInfo, err error) error {

			// return on any error
			if err != nil {
				return err
			}

			// create a new dir/file header
			header, err := tar.FileInfoHeader(fi, fi.Name())
			if err != nil {
				return err
			}

			// update the name to correctly reflect the desired destination when untaring
			header.Name = strings.TrimPrefix(strings.TrimPrefix(file, *content), string(filepath.Separator))

			// force uid/gid of injected content to 2001(gostint)
			header.Uid = 2001
			header.Gid = 2001
			header.Uname = "gostint"
			header.Gname = "gostint"

			// write the header
			if err := tw.WriteHeader(header); err != nil {
				return err
			}

			// return on non-regular files (thanks to [kumo](https://medium.com/@komuw/just-like-you-did-fbdd7df829d3) for this suggested update)
			if !fi.Mode().IsRegular() {
				return nil
			}

			// open file for taring
			f, err := os.Open(file)
			if err != nil {
				return err
			}

			// copy file data into tar writer
			if _, err := io.Copy(tw, f); err != nil {
				return err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			f.Close()

			return nil
		})

		if err != nil {
			return bytes.NewBuffer([]byte{}), err
		}
		return &buf, nil

	} else if fi.Mode().IsRegular() { // Test if content points to a tar.gz file
		var b []byte
		b, err = ioutil.ReadFile(*content)
		if err != nil {
			return &buf, err
		}
		return bytes.NewBuffer(b), nil
	}

	return bytes.NewBuffer([]byte{}), fmt.Errorf("Unsupported file mode for content")
}

// EncodeContent utility function to encode a folder's content as a tar.gz
// archive and return base64 encoded to be submitted as -content parameter
func EncodeContent(content *string) error {
	if *content == "" {
		return nil
	}
	debug("Encoding content from %s", *content)
	buf, err := getContent(content)
	if err != nil {
		return err
	}

	// Base64 encode targz into a string and store back in content
	*content = fmt.Sprintf("targz,%s", base64.StdEncoding.EncodeToString(buf.Bytes()))
	// fmt.Printf("content: %s\n", *content)
	return nil
}
