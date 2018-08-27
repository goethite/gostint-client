# gostint-client
GoStint API client and commandline tool

## Testing agains GoStint Vagrant dev instance
```
go run main.go -vault-token=root \
  -url=https://127.0.0.1:3232 \
  -vault-url=http://127.0.0.1:8300 \
  -job-json=@../gostint/tests/job1.json
```
