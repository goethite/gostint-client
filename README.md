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
* see https://github.com/goethite/gostint/tree/master/tests for referenced content below.
* Examples below are using a vault root token for demo purposes.  In production
  you would use AppRole Authentication - see AppRole examples further down, which
  includes the required minimal vault policy definition.

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

### Using Vault AppRole Authentication

Create a vault policy for the gostint-client's approle
```
vault policy write gostint-client - <<EOF
path "auth/token/create" {
  capabilities = ["create", "read", "update", "delete", "list"]
}
path "auth/approle/role/gostint-role/secret-id" {
  capabilities = ["update"]
}
path "transit/encrypt/gostint" {
  capabilities = ["update"]
}
EOF
```

Create an AppRole (PUSH mode for this example) for the gostint-client:
```
vault write auth/approle/role/gostint-client-role \
  token_ttl=20m \
  token_max_ttl=30m \
  policies="gostint-client"
```
Get the Role_Id for the AppRole:
```
vault read /auth/approle/role/gostint-client-role/role-id
```
For this example we will use PUSH mode on the AppRole (not the secret_id was a
random uuid) - you would probably prefer to use PULL mode in production:
```
vault write auth/approle/role/gostint-client-role/custom-secret-id \
  secret_id=7a32c590-aacc-11e8-a59c-8b71f9a0c1a4
```

Run gostint-client using the AppRole:
```
$ gostint-client -vault-roleid=43a03f77-7461-d4d2-c14d-76b39ea400d5 \
  -vault-secretid=7a32c590-aacc-11e8-a59c-8b71f9a0c1a4 \
  -url=https://127.0.0.1:13232 \
  -vault-url=http://127.0.0.1:18200 \
  -image=alpine \
  -run='["cat", "/etc/os-release"]'
```
