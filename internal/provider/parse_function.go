package provider

import (
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ function.Function = &ParseFunction{}

const parseFuncMarkdownDesc = `Converts a fully qualified OCI image reference with a digest into an object representation with the following properties:

- ` + "`registry`" + ` - The registry hostname (e.g., ` + "`cgr.dev`" + `)
- ` + "`repo`" + ` - The repository path without the registry (e.g., ` + "`chainguard/wolfi-base`" + `)
- ` + "`registry_repo`" + ` - The full registry and repository path (e.g., ` + "`cgr.dev/chainguard/wolfi-base`" + `)
- ` + "`digest`" + ` - The digest identifier (e.g., ` + "`sha256:abcd1234...`" + `)
- ` + "`pseudo_tag`" + ` - A pseudo tag format combining unused with the digest (e.g., ` + "`unused@sha256:abcd1234...`" + `)
- ` + "`ref`" + ` - The complete reference string as provided

**Note:** The input must include a digest. References with only a tag (without a digest) will result in an error.

## Example

` + "```" + `terraform
output "parsed" {
  value = provider::oci::parse("cgr.dev/chainguard/wolfi-base@sha256:abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab")
}
` + "```" + `

This returns:
` + "```" + `json
{
  "registry": "cgr.dev",
  "repo": "chainguard/wolfi-base",
  "registry_repo": "cgr.dev/chainguard/wolfi-base",
  "digest": "sha256:abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
  "pseudo_tag": "unused@sha256:abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
  "ref": "cgr.dev/chainguard/wolfi-base@sha256:abcd1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab"
}
` + "```"

func NewParseFunction() function.Function {
	return &ParseFunction{}
}

// ParseFunction defines the function implementation.
type ParseFunction struct{}

// Metadata should return the name of the function, such as parse_xyz.
func (s *ParseFunction) Metadata(_ context.Context, _ function.MetadataRequest, resp *function.MetadataResponse) {
	resp.Name = "parse"
}

// Definition should return the definition for the function.
func (s *ParseFunction) Definition(_ context.Context, _ function.DefinitionRequest, resp *function.DefinitionResponse) {
	resp.Definition = function.Definition{
		Summary:             "Parses a pinned OCI string into its constituent parts.",
		MarkdownDescription: parseFuncMarkdownDesc,
		Parameters: []function.Parameter{
			function.StringParameter{
				Name:        "input",
				Description: "The OCI reference string to parse.",
			},
		},
		Return: function.ObjectReturn{
			AttributeTypes: map[string]attr.Type{
				"registry":      basetypes.StringType{},
				"repo":          basetypes.StringType{},
				"registry_repo": basetypes.StringType{},
				"digest":        basetypes.StringType{},
				"pseudo_tag":    basetypes.StringType{},
				"ref":           basetypes.StringType{},
			},
		},
	}
}

// Run should return the result of the function logic. It is called when
// Terraform reaches a function call in the configuration. Argument data
// values should be read from the [RunRequest] and the result value set in
// the [RunResponse].
func (s *ParseFunction) Run(ctx context.Context, req function.RunRequest, resp *function.RunResponse) {
	var input string
	if ferr := req.Arguments.GetArgument(ctx, 0, &input); ferr != nil {
		resp.Error = ferr
		return
	}

	// Parse the input string into its constituent parts.
	ref, err := name.ParseReference(input)
	if err != nil {
		resp.Error = function.NewFuncError(fmt.Sprintf("Failed to parse OCI reference: %v", err))
		return
	}

	if _, ok := ref.(name.Tag); ok {
		resp.Error = function.NewFuncError(fmt.Sprintf("Reference %s contains only a tag, but a digest is required", input))
		return
	}

	result := struct {
		Registry     string `tfsdk:"registry"`
		Repo         string `tfsdk:"repo"`
		RegistryRepo string `tfsdk:"registry_repo"`
		Digest       string `tfsdk:"digest"`
		PseudoTag    string `tfsdk:"pseudo_tag"`
		Ref          string `tfsdk:"ref"`
	}{
		Registry:     ref.Context().RegistryStr(),
		Repo:         ref.Context().RepositoryStr(),
		RegistryRepo: ref.Context().RegistryStr() + "/" + ref.Context().RepositoryStr(),
		Digest:       ref.Identifier(),
		PseudoTag:    fmt.Sprintf("unused@%s", ref.Identifier()),
		Ref:          ref.String(),
	}

	resp.Error = function.ConcatFuncErrors(resp.Error, resp.Result.Set(ctx, &result))
}
