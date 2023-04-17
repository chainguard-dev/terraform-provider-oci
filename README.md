# Terraform Provider for [`crane`](https://github.com/google/go-containerregistry/blob/main/cmd/crane/README.md)

[![Tests](https://github.com/imjasonh/terraform-provider-crane/actions/workflows/test.yml/badge.svg)](https://github.com/imjasonh/terraform-provider-crane/actions/workflows/test.yml)

## Developing the Provider

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `go generate`.

In order to run the full suite of Acceptance tests, run:

```shell
TF_ACC=1 go test ./internal/provider/...
```
