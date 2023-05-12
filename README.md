# Terraform Provider for OCI operations

[![Tests](https://github.com/chainguard-dev/terraform-provider-oci/actions/workflows/test.yml/badge.svg)](https://github.com/chainguard-dev/terraform-provider-oci/actions/workflows/test.yml)

This provider is intended to provide some behavior similar to [`crane`](https://github.com/google/go-containerregistry/blob/main/cmd/crane/README.md).

## Developing the Provider

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `go generate`.

In order to run the full suite of Acceptance tests, run:

```shell
TF_ACC=1 go test ./internal/provider/...
```
