/*
This file is part of gostint.

MIT License

Copyright (c) 2018 Graham Lee Bevan <graham.bevan@ntlworld.com>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/goethite/gostint-client/clientapi"

	"github.com/fatih/color"
)

var enableDebug = false

func debug(format string, a ...interface{}) {
	if !enableDebug {
		return
	}
	clientapi.Debug(format, a...)
}

func validate(c clientapi.APIRequest) error {
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

	if *c.ImagePullPolicy != "IfNotPresent" && *c.ImagePullPolicy != "Always" {
		return fmt.Errorf("invalid image-pull-policy, must be 'IfNotPresetn' or 'Always'")
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

func chkError(err error) {
	if err != nil {
		// color.HiRed(fmt.Sprintf("Error: %s", err.Error()))
		var red = color.New(color.FgRed).Add(color.Bold).SprintfFunc()
		fmt.Fprintln(color.Error, red("Error: %s", err.Error()))
		// panic(err)
		os.Exit(1)
	}
}

func main() {
	c := clientapi.APIRequest{}
	c.AppRoleID = flag.String("vault-roleid", "", "Requestor's Vault App Role ID (can read file e.g. '@role_id.txt')")
	c.AppSecretID = flag.String("vault-secretid", "", "Requestor's Vault App Secret ID (can read file e.g. '@secret_id.txt')")
	c.Token = flag.String("vault-token", "", "Requestor's Vault token - used instead of App Role (can read file e.g. '@token.txt')")

	c.GoStintRole = flag.String("gostint-approle", "gostint-role", "Vault App Role Name of GoStint to run job on (can read file e.g. '@gostint_role.txt')")

	c.JobJSON = flag.String("job-json", "", "JSON Job request")

	c.QName = flag.String("qname", "", "Job Queue to submit to, overrides value in job-json")
	c.ContainerImage = flag.String("image", "", "Docker image to run job within, overrides value in job-json")
	c.ImagePullPolicy = flag.String("image-pull-policy", "IfNotPresent", "Docker image pull policy: IfNotPresent or Always")
	c.Content = flag.String("content", "", "Folder or targz to inject into the container relative to root '/' folder, overrides value in job-json")
	c.EntryPoint = flag.String("entrypoint", "", "JSON array of string parts defining the container's entrypoint, e.g.: '[\"ansible\"]', overrides value in job-json")
	c.Run = flag.String("run", "", "JSON array of string parts defining the command to run in the container - aka the job, e.g.: '[\"-m\", \"ping\", \"127.0.0.1\"]', overrides value in job-json")
	c.WorkingDir = flag.String("run-dir", "", "Working directory within the container to run the job")
	c.EnvVars = flag.String("env-vars", "", "JSON array of strings providing envronment variables to be passed to the job container, e.g.: '[\"MYVAR=value\"]'")
	c.SecretRefs = flag.String("secret-refs", "", "JSON array of strings providing paths to secrets in the Vault to be injected into the job's container, e.g.: '[\"mysecret@secret/data/my-secret.my-value\", ...]', overrides value in job-json")
	c.SecretFileType = flag.String("secret-filetype", "yaml", "Injected secret file type, can be either 'yaml' (default) or 'json', overrides value in job-json")
	c.ContOnWarnings = flag.Bool("cont-on-warnings", false, "Continue to run job even if vault reported warnings when looking up secret refs, overrides value in job-json")

	c.URL = flag.String("url", "", "GoStint API URL, e.g. https://somewhere:3232")
	c.VaultURL = flag.String("vault-url", "", "Vault API URL, e.g. https://your-vault:8200 - defaults to env var VAULT_ADDR")

	deb := flag.Bool("debug", false, "Enable debugging")
	pollIntervalSecs := flag.Int("poll-interval", 1, "Overide default poll interval for results (in seconds)")

	waitFor := flag.Bool("wait", true, "Wait for job to complete before returning final status")

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
	err = tryResolveFile(c.GoStintRole)
	chkError(err)
	err = tryResolveFile(c.JobJSON)
	chkError(err)

	err = clientapi.EncodeContent(c.Content)
	chkError(err)

	res, err := clientapi.RunJob(&c, *deb, *pollIntervalSecs, *waitFor)
	chkError(err)

	debug("Final job state: %v", res)
	if res.Status == "success" {
		fmt.Printf(res.Output)
	} else {
		color.HiRed(fmt.Sprintf("[%s] %s", res.Status, res.Output))
		if res.ReturnCode == 0 {
			// force non-zero rc - this can happen if executable not found in the container
			os.Exit(1)
		}
	}
	os.Exit(res.ReturnCode)
}
