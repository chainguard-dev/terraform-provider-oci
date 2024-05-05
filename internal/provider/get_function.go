package provider

import (
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ function.Function = &GetFunction{}

func NewGetFunction() function.Function {
	return &GetFunction{}
}

// GetFunction defines the function implementation.
type GetFunction struct{}

// Metadata should return the name of the function, such as parse_xyz.
func (s *GetFunction) Metadata(_ context.Context, _ function.MetadataRequest, resp *function.MetadataResponse) {
	resp.Name = "get"
}

// Definition should return the definition for the function.
func (s *GetFunction) Definition(_ context.Context, _ function.DefinitionRequest, resp *function.DefinitionResponse) {
	resp.Definition = function.Definition{
		Summary: "Parses a pinned OCI string into its constituent parts.",
		Parameters: []function.Parameter{
			function.StringParameter{
				Name:        "input",
				Description: "The OCI reference string to get.",
			},
		},
		Return: function.ObjectReturn{
			AttributeTypes: map[string]attr.Type{
				"digest":   basetypes.StringType{},
				"tag":      basetypes.StringType{},
				"manifest": basetypes.ObjectType{AttrTypes: manifestAttribute.AttributeTypes},
				"images":   basetypes.MapType{ElemType: imageType},
				"config":   basetypes.ObjectType{AttrTypes: configAttribute.AttributeTypes},
			},
		},
	}
}

// Run should return the result of the function logic. It is called when
// Terraform reaches a function call in the configuration. Argument data
// values should be read from the [RunRequest] and the result value set in
// the [RunResponse].
func (s *GetFunction) Run(ctx context.Context, req function.RunRequest, resp *function.RunResponse) {
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

	result := struct {
		Digest   string           `tfsdk:"digest"`
		Tag      string           `tfsdk:"tag"`
		Manifest *Manifest        `tfsdk:"manifest"`
		Images   map[string]Image `tfsdk:"images"`
		Config   *Config          `tfsdk:"config"`
	}{}

	if t, ok := ref.(name.Tag); ok {
		result.Tag = t.TagStr()
	}

	desc, err := remote.Get(ref,
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithUserAgent("terraform-provider-oci"),
		remote.WithContext(ctx))
	if err != nil {
		resp.Error = function.NewFuncError(fmt.Sprintf("Failed to get image: %v", err))
		return
	}

	result.Digest = desc.Digest.String()

	mf := &Manifest{}
	if err := mf.FromDescriptor(desc); err != nil {
		resp.Error = function.NewFuncError(fmt.Sprintf("Failed to parse manifest: %v", err))
		return
	}
	result.Manifest = mf

	if desc.MediaType.IsIndex() {
		idx, err := desc.ImageIndex()
		if err != nil {
			resp.Error = function.NewFuncError(fmt.Sprintf("Failed to parse index: %v", err))
			return
		}
		imf, err := idx.IndexManifest()
		if err != nil {
			resp.Error = function.NewFuncError(fmt.Sprintf("Failed to parse index manifest: %v", err))
			return
		}
		result.Images = make(map[string]Image, len(imf.Manifests))
		for _, m := range imf.Manifests {
			if m.Platform == nil {
				continue
			}
			result.Images[m.Platform.String()] = Image{
				Digest:   m.Digest.String(),
				ImageRef: ref.Context().Digest(m.Digest.String()).String(),
			}
		}
	} else if desc.MediaType.IsImage() {
		img, err := desc.Image()
		if err != nil {
			resp.Error = function.NewFuncError(fmt.Sprintf("Failed to parse image: %v", err))
			return
		}
		cf, err := img.ConfigFile()
		if err != nil {
			resp.Error = function.NewFuncError(fmt.Sprintf("Failed to parse config: %v", err))
			return
		}
		cfg := &Config{}
		cfg.FromConfigFile(cf)
		result.Config = cfg
	}

	resp.Error = function.ConcatFuncErrors(resp.Error, resp.Result.Set(ctx, &result))
}
