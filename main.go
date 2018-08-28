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

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/hashicorp/vault/api"
)

var enableDebug = false

func debug(format string, a ...interface{}) {
	if !enableDebug {
		return
	}
	t := time.Now()
	d := color.New(color.FgBlue).Add(color.Bold)
	d.Printf(t.Format(time.RFC3339)+" "+format, a...)
	fmt.Println()
}

type cliRequest struct {
	AppRoleID      *string
	AppSecretID    *string // AppRole auth or Token
	Token          *string
	JobJSON        *string // request can be whole JSON:
	QName          *string // or can be passed as parameters:
	ContainerImage *string
	Content        *string
	EntryPoint     *string
	Run            *string
	WorkingDir     *string
	SecretRefs     *string
	SecretFileType *string
	ContOnWarnings *bool
	URL            *string
	VaultURL       *string
}

func validate(c cliRequest) error {
	debug("Validating command line arguments")
	if *c.URL == "" {
		return fmt.Errorf("url must be specified")
	}
	if *c.VaultURL == "" {
		return fmt.Errorf("vault-url must be specified")
	}
	if *c.Token == "" && *c.AppRoleID == "" {
		return fmt.Errorf("One of vault-roleid OR token must be specified")
	}
	if *c.AppRoleID != "" && *c.AppSecretID == "" {
		return fmt.Errorf("vault-roleid must also have vault-secretid specified")
	}
	if *c.AppRoleID == "" && *c.AppSecretID != "" {
		return fmt.Errorf("vault-secretid must also have vault-roleid specified")
	}

	if *c.Token != "" && *c.AppRoleID != "" {
		return fmt.Errorf("vault-token cannot be used with vault-roleid")
	}

	return nil
}

func tryResolveFile(p *string) error {
	if strings.HasPrefix(*p, "@") {
		debug("Resolving file argument %s", *p)
		b, err := ioutil.ReadFile(strings.TrimPrefix(*p, "@"))
		if err != nil {
			return err
		}
		*p = strings.Trim(string(b), " \t\n\r")
		// debug("file contents:\n%s", *p)
	}
	return nil
}

type job struct {
	QName          string   `json:"qname"`
	ContainerImage string   `json:"container_image"`
	Content        string   `json:"content"`
	EntryPoint     []string `json:"entrypoint"`
	Run            []string `json:"run"`
	WorkingDir     string   `json:"working_directory"`
	SecretRefs     []string `json:"secret_refs"`
	SecretFileType string   `json:"secret_file_type"`
	ContOnWarnings bool     `json:"cont_on_warnings"`
}

// func buildJob(c cliRequest) (*[]byte, error) {
func buildJob(c cliRequest) (*job, error) {
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

func chkError(err error) {
	if err != nil {
		// color.HiRed(fmt.Sprintf("Error: %s", err.Error()))
		var red = color.New(color.FgRed).Add(color.Bold).SprintfFunc()
		fmt.Fprintln(color.Error, red("Error: %s", err.Error()))
		// panic(err)
		os.Exit(1)
	}
}

func getVaultClient(url string, c *cliRequest) (*api.Client, error) {
	debug("Getting Vault api connection %s", url)
	client, err := api.NewClient(&api.Config{
		Address: url,
	})
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

func submitJob(c *cliRequest, jsonBytes *[]byte, token string) (*submitResponse, error) {
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

func getJob(c *cliRequest, token string, ID string) (*getResponse, error) {
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

	getResp := getResponse{}
	err = json.Unmarshal(body, &getResp)
	if err != nil {
		return nil, err
	}

	return &getResp, nil
}

type getResponse struct {
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

func encodeContent(content *string) error {
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

func main() {
	start := time.Now()
	c := cliRequest{}
	c.AppRoleID = flag.String("vault-roleid", "", "Vault App Role ID (can read file e.g. '@role_id.txt')")
	c.AppSecretID = flag.String("vault-secretid", "", "Vault App Secret ID (can read file e.g. '@secret_id.txt')")
	c.Token = flag.String("vault-token", "", "Vault token - used instead of App Role (can read file e.g. '@token.txt')")

	c.JobJSON = flag.String("job-json", "", "JSON Job request")

	c.QName = flag.String("qname", "", "Job Queue to submit to, overrides value in job-json")
	c.ContainerImage = flag.String("image", "", "Docker image to run job within, overrides value in job-json")
	c.Content = flag.String("content", "", "Folder or targz to inject into the container relative to root '/' folder, overrides value in job-json")
	c.EntryPoint = flag.String("entrypoint", "", "JSON array of string parts defining the container's entrypoint, e.g.: '[\"ansible\"]', overrides value in job-json")
	c.Run = flag.String("run", "", "JSON array of string parts defining the command to run in the container - aka the job, e.g.: '[\"-m\", \"ping\", \"127.0.0.1\"]', overrides value in job-json")
	c.WorkingDir = flag.String("run-dir", "", "Working directory within the container to run the job")
	c.SecretRefs = flag.String("secret-refs", "", "JSON array of strings providing paths to secrets in the Vault to be injected into the job's container, e.g.: '[\"mysecret@secret/data/my-secret.my-value\", ...]', overrides value in job-json")
	c.SecretFileType = flag.String("secret-filetype", "yaml", "Injected secret file type, can be either 'yaml' (default) or 'json', overrides value in job-json")
	c.ContOnWarnings = flag.Bool("cont-on-warnings", false, "Continue to run job even if vault reported warnings when looking up secret refs, overrides value in job-json")

	c.URL = flag.String("url", "", "GoStint API URL, e.g. https://somewhere:3232")
	c.VaultURL = flag.String("vault-url", "", "Vault API URL, e.g. https://your-vault:8200 - defaults to env var VAULT_ADDR")

	deb := flag.Bool("debug", false, "Enable debugging")
	pollIntervalSecs := flag.Int("poll-interval", 1, "Overide default poll interval for results (in seconds)")

	flag.Parse()
	enableDebug = *deb

	err := validate(c)
	chkError(err)

	err = tryResolveFile(c.AppRoleID)
	chkError(err)
	err = tryResolveFile(c.AppSecretID)
	chkError(err)
	err = tryResolveFile(c.Token)
	chkError(err)
	err = tryResolveFile(c.JobJSON)
	chkError(err)

	err = encodeContent(c.Content)
	chkError(err)

	job, err := buildJob(c)
	chkError(err)

	if *c.VaultURL == "" {
		*c.VaultURL = os.Getenv("VAULT_ADDR")
	}

	vc, err := getVaultClient(*c.VaultURL, &c)
	chkError(err)

	debug("Getting minimal token to authenticate with GoStint API")
	data := map[string]interface{}{
		"policies": []string{"default"},
	}
	sec, err := vc.Logical().Write("auth/token/create", data)
	chkError(err)
	apiToken := sec.Auth.ClientToken

	debug("Getting Wrapped Secret_ID for the AppRole")
	vc.SetWrappingLookupFunc(func(op, path string) string { return "1h" })
	sec, err = vc.Logical().Write("auth/approle/role/gostint-role/secret-id", nil)
	chkError(err)
	wrapSecretID := sec.WrapInfo.Token
	vc.SetWrappingLookupFunc(nil)

	jsonBytes, err := json.Marshal(*job)
	chkError(err)

	debug("Encrypting the job payload")
	data = map[string]interface{}{
		"plaintext": base64.StdEncoding.EncodeToString(jsonBytes),
	}
	sec, err = vc.Logical().Write("transit/encrypt/gostint", data)
	chkError(err)
	encryptedPayload := sec.Data["ciphertext"]

	debug("Getting minimal limited use / ttl token for the cubbyhole")
	data = map[string]interface{}{
		"policies":  []string{"default"},
		"ttl":       "60m",
		"use_limit": 2,
	}
	sec, err = vc.Logical().Write("auth/token/create", data)
	chkError(err)
	cubbyToken := sec.Auth.ClientToken

	debug("Putting encrypted payload in a vault cubbyhole")
	vc.SetToken(cubbyToken)
	data = map[string]interface{}{
		"payload": encryptedPayload,
	}
	sec, err = vc.Logical().Write("cubbyhole/job", data)
	chkError(err)

	debug("Creating job request wrapper to submit")
	jWrap := jobWrapper{
		QName:        job.QName,
		CubbyToken:   cubbyToken,
		CubbyPath:    "cubbyhole/job",
		WrapSecretID: wrapSecretID,
	}
	jWrapBytes, err := json.Marshal(jWrap)
	chkError(err)

	subResp, err := submitJob(&c, &jWrapBytes, apiToken)
	chkError(err)

	// loop until status != queued or running
	var getResp *getResponse
	for {
		getResp, err = getJob(&c, apiToken, subResp.ID)
		chkError(err)
		// fmt.Printf("getResp: %t\n", getResp)
		if getResp.Status != "queued" && getResp.Status != "running" {
			break
		}
		time.Sleep(time.Duration(*pollIntervalSecs) * time.Second)
	}
	debug("Final job state: %v", getResp)
	if getResp.Status == "success" {
		fmt.Printf(getResp.Output)
	} else {
		color.HiRed(fmt.Sprintf("[%s] %s", getResp.Status, getResp.Output))
	}

	t := time.Now()
	elapsed := t.Sub(start)
	debug("Elapsed time: %.3f seconds", float64(elapsed/time.Millisecond)/1000.0)
}

type jobWrapper struct {
	QName        string `json:"qname"`
	CubbyToken   string `json:"cubby_token"`
	CubbyPath    string `json:"cubby_path"`
	WrapSecretID string `json:"wrap_secret_id"`
}
