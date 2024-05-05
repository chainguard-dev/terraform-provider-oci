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
		Summary: "Parses a pinned OCI string into its constituent parts.",
		Parameters: []function.Parameter{
			function.StringParameter{
				Name:        "input",
				Description: `The OCI reference string to parse. This supports any valid OCI reference string, including those with a tag, digest, or both. For example: 'cgr.dev/my-project/my-image:latest' or 'cgr.dev/my-project/my-image@sha256:...'. Note that when tags are provided, they will be replaced in favor of the digest.`,
			},
		},
		Return: function.ObjectReturn{
			AttributeTypes: map[string]attr.Type{
				"registry":      basetypes.StringType{},
				"repo":          basetypes.StringType{},
				"registry_repo": basetypes.StringType{},
				"digest":        basetypes.StringType{},
				"pseudo_tag":    basetypes.StringType{},
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
	}{
		Registry:     ref.Context().RegistryStr(),
		Repo:         ref.Context().RepositoryStr(),
		RegistryRepo: ref.Context().RegistryStr() + "/" + ref.Context().RepositoryStr(),
		Digest:       ref.Identifier(),
		PseudoTag:    fmt.Sprintf("unused@%s", ref.Identifier()),
	}

	resp.Error = function.ConcatFuncErrors(resp.Error, resp.Result.Set(ctx, &result))
}
