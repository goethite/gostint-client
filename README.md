# gostint-client
GoStint API client and commandline tool

## Testing agains GoStint Vagrant dev instance
```
go run main.go -vault-token=root \
  -url=https://127.0.0.1:3232 \
  -vault-url=http://127.0.0.1:8300 \
  -job-json=@../gostint/tests/job1.json
```

## Examples

### Debugging with -debug option
```
$ gostint-client -vault-token=@.vault_token \
  -url=https://127.0.0.1:13232 \
  -vault-url=http://127.0.0.1:18200 \
  -image=alpine \
  -run='["cat", "/etc/os-release"]' \
  -debug
2018-08-28T13:11:23+01:00 Validating command line arguments
2018-08-28T13:11:23+01:00 Resolving file argument @.vault_token
2018-08-28T13:11:23+01:00 Building Job Request
2018-08-28T13:11:23+01:00 Getting Vault api connection http://127.0.0.1:18200
2018-08-28T13:11:23+01:00 Authenticating with Vault
2018-08-28T13:11:23+01:00 Getting minimal token to authenticate with GoStint API
2018-08-28T13:11:24+01:00 Getting Wrapped Secret_ID for the AppRole
2018-08-28T13:11:24+01:00 Encrypting the job payload
2018-08-28T13:11:24+01:00 Getting minimal limited use / ttl token for the cubbyhole
2018-08-28T13:11:24+01:00 Putting encrypted payload in a vault cubbyhole
2018-08-28T13:11:24+01:00 Creating job request wrapper to submit
2018-08-28T13:11:24+01:00 Submitting job
2018-08-28T13:11:24+01:00 Response status: 200 OK
...
2018-08-28T13:11:29+01:00 Elapsed time: 5.327 seconds
```

### Run a command in a container
```
$ gostint-client -vault-token=@.vault_token \
  -url=https://127.0.0.1:13232 \
  -vault-url=http://127.0.0.1:18200 \
  -image=alpine \
  -run='["cat", "/etc/os-release"]'
NAME="Alpine Linux"
ID=alpine
VERSION_ID=3.8.0
PRETTY_NAME="Alpine Linux v3.8"
HOME_URL="http://alpinelinux.org"
BUG_REPORT_URL="http://bugs.alpinelinux.org"
```
### Running Ansible containers
```
$ gostint-client -vault-token=@.vault_token \
  -url=https://127.0.0.1:13232 \
  -vault-url=http://127.0.0.1:18200 \
  -image="jmal98/ansiblecm:2.5.5" \
  -entrypoint='["ansible"]' \
  -run='["--version"]'
ansible 2.5.5
  config file = None
  configured module search path = [u'/tmp/.ansible/plugins/modules', u'/usr/share/ansible/plugins/modules']
  ansible python module location = /usr/lib/python2.7/site-packages/ansible
  executable location = /usr/bin/ansible
  python version = 2.7.14 (default, Dec 14 2017, 15:51:29) [GCC 6.4.0]
```

```
$ gostint-client -vault-token=@.vault_token \
  -url=https://127.0.0.1:13232 \
  -vault-url=http://127.0.0.1:18200 \
  -image="jmal98/ansiblecm:2.5.5" \
  -entrypoint='["ansible"]' \
  -run='["-i", "127.0.0.1 ansible_connection=local,", "-m", "ping", "127.0.0.1"]'
127.0.0.1 | SUCCESS => {
    "changed": false,
    "ping": "pong"
}
```

```
$ ./gostint-client -vault-token=@.vault_token -url=https://127.0.0.1:13232 -vault-url=http://127.0.0.1:18200 -image="jmal98/ansiblecm:2.5.5" -content=../gostint/tests/content_ansible_play -run='["-i", "hosts", "play1.yml"]'

PLAY [all] *********************************************************************

TASK [Gathering Facts] *********************************************************
ok: [127.0.0.1]

TASK [include_vars] ************************************************************
ok: [127.0.0.1]

TASK [debug] *******************************************************************
ok: [127.0.0.1] => {
    "gostint": {
        "TOKEN": "secret-injected-by-gostint"
    }
}

PLAY RECAP *********************************************************************
127.0.0.1                  : ok=3    changed=0    unreachable=0    failed=0   
```
