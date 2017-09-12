# caddy-tlsclouddatastore

[Google Cloud Datastore](https://cloud.google.com/datastore/docs/concepts/overview) storage for [Caddy](https://github.com/mholt/caddy) TLS data. 

Caddy usually uses a local filesystem to store TLS data when it auto-generates certificates from a CA like Lets Encrypt.
With this plugin it is possible to use multiple Caddy instances with the same HTTPS domain, for instance with DNS round-robin or behind a load balancer, 
with centralized storage (Google Cloud Datastore) for auto-generated certificates. Using a caddy DNS challenge plugin is required.

It works with recent versions of Caddy 0.10.x
All data that is stored is encrypted using AES.

## Installation

You need to compile Caddy by yourself to use this plugin.

- Set up a working Go installation, see https://golang.org/doc/install
- Checkout Caddy source code from https://github.com/mholt/caddy
- Get latest caddy-tlsclouddatastore with `go get -u github.com/j0hnsmith/caddy-tlsclouddatastore`
- Add this line to `caddy/caddymain/run.go` in the `import` region:
```go
import (
  ...
  _ "github.com/j0hnsmith/caddy-tlsclouddatastore"
)
```
- Change dir into `caddy/caddymain` and compile Caddy with `go run build.go`

## Configuration

In order to use Cloud Datastore you have to change the storage provider in your Caddyfile like so:

```
    tls my@email.com {
        storage cloud-datastore
        dns ... # dns challenge provider
    }
```

## Env Vars

- `DATASTORE_PROJECT_ID` GCP project id (not name), required.
- `CADDY_CLOUDDATASTORETLS_SERVICE_ACCOUNT_FILE` the full path to service account json key file  ([create service account](https://console.developers.google.com/permissions/serviceaccounts) with Datastore -> Cloud Datastore User role), required. 
- `CADDY_CLOUDDATASTORETLS_B64_AESKEY` defines your personal AES key to use when encrypting data, generate with `openssl rand -base64 32` or similar (don't use a string), defaults to an insecure key. 
- `CADDY_CLOUDDATASTORETLS_PREFIX` defines the prefix for the keys, default is `caddytls`.

## Credits

[caddy-tlsconsul](https://github.com/pteich/caddy-tlsconsul) provided inspiration, thanks also to Matt Holt for [Caddy](https://github.com/mholt/caddy).

