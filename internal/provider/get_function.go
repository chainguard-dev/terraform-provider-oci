package provider

import (
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ function.Function = &GetFunction{}

const getFuncMarkdownDesc = `Fetches an OCI image or index from a registry and returns detailed information about it, including the manifest, configuration, and platform-specific images.

The function accepts either a tag or digest reference and returns an object with the following properties:

- ` + "`full_ref`" + ` - The complete reference with digest (e.g., ` + "`cgr.dev/chainguard/wolfi-base@sha256:abcd1234...`" + `)
- ` + "`digest`" + ` - The digest of the image or index (e.g., ` + "`sha256:abcd1234...`" + `)
- ` + "`tag`" + ` - The tag if one was provided in the input (e.g., ` + "`latest`" + `)
- ` + "`manifest`" + ` - The OCI manifest object containing:
  - ` + "`schema_version`" + ` - The manifest schema version
  - ` + "`media_type`" + ` - The media type of the manifest
  - ` + "`config`" + ` - The config descriptor (for images)
  - ` + "`layers`" + ` - List of layer descriptors (for images)
  - ` + "`annotations`" + ` - Key-value annotations
  - ` + "`manifests`" + ` - List of manifest descriptors (for indexes)
  - ` + "`subject`" + ` - Subject descriptor (for referrers)
- ` + "`images`" + ` - Map of platform strings to image objects (populated for multi-arch indexes). Each image contains:
  - ` + "`digest`" + ` - The platform-specific image digest
  - ` + "`image_ref`" + ` - The full reference for the platform-specific image
- ` + "`config`" + ` - The image configuration (populated for single-arch images only). Contains:
  - ` + "`env`" + ` - List of environment variables
  - ` + "`user`" + ` - The user to run as
  - ` + "`working_dir`" + ` - The working directory
  - ` + "`entrypoint`" + ` - The entrypoint command
  - ` + "`cmd`" + ` - The default command arguments
  - ` + "`created_at`" + ` - Timestamp when the image was created

**Note:** This function makes a network request to the registry to fetch the image metadata. Authentication is handled via the default keychain (Docker config, credential helpers, etc.).

Terraform calls this function again during apply and fails the run if a tag moved after plan and the result changed, so use the ` + "`oci_ref`" + ` data source for a tag that can move during a run.

## Example: Single-arch image

` + "```" + `terraform
output "image_info" {
  value = provider::oci::get("cgr.dev/chainguard/wolfi-base:latest")
}
` + "```" + `

## Example: Multi-arch index

` + "```" + `terraform
locals {
  multi_arch = provider::oci::get("cgr.dev/chainguard/static:latest")
}

output "linux_amd64" {
  value = local.multi_arch.images["linux/amd64"]
}
` + "```" + `

## Example: Using the config

` + "```" + `terraform
locals {
  image = provider::oci::get("cgr.dev/chainguard/wolfi-base:latest")
}

output "entrypoint" {
  value = local.image.config.entrypoint
}

output "environment" {
  value = local.image.config.env
}
` + "```"

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
		Summary:             "Fetches an OCI image or index from a registry and returns detailed metadata.",
		MarkdownDescription: getFuncMarkdownDesc,
		Parameters: []function.Parameter{
			function.StringParameter{
				Name:        "input",
				Description: "The OCI reference string to get.",
			},
		},
		Return: function.ObjectReturn{
			AttributeTypes: map[string]attr.Type{
				"full_ref": basetypes.StringType{},
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

	info, err := getRef(ref,
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithUserAgent("terraform-provider-oci"),
		remote.WithContext(ctx))
	if err != nil {
		resp.Error = function.NewFuncError("Failed to " + err.Error())
		return
	}

	resp.Error = function.ConcatFuncErrors(resp.Error, resp.Result.Set(ctx, info))
}

// refInfo is the object that provider::oci::get returns and the oci_ref data
// source stores. It depends only on the reference and the digest the
// reference resolves to, so two reads of one reference differ exactly when
// the digest does.
type refInfo struct {
	FullRef  string           `tfsdk:"full_ref"`
	Digest   string           `tfsdk:"digest"`
	Tag      string           `tfsdk:"tag"`
	Manifest *Manifest        `tfsdk:"manifest"`
	Images   map[string]Image `tfsdk:"images"`
	Config   *Config          `tfsdk:"config"`
}

// getRef fetches ref from its registry and describes what it resolves to.
// Every error begins with the step that failed as a verb phrase, such as
// "get image: ...", which callers prefix with "Failed to ".
func getRef(ref name.Reference, opts ...remote.Option) (*refInfo, error) {
	desc, err := remote.Get(ref, opts...)
	if err != nil {
		return nil, fmt.Errorf("get image: %w", err)
	}
	return describe(ref, desc)
}

// describe builds the object for ref from desc, the descriptor ref resolved to.
func describe(ref name.Reference, desc *remote.Descriptor) (*refInfo, error) {
	info := &refInfo{
		FullRef:  ref.Context().Digest(desc.Digest.String()).String(),
		Digest:   desc.Digest.String(),
		Manifest: &Manifest{},
	}
	if t, ok := ref.(name.Tag); ok {
		info.Tag = t.TagStr()
	}
	if err := info.Manifest.FromDescriptor(desc); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	var err error
	switch {
	case desc.MediaType.IsIndex():
		info.Images, err = platformImages(ref.Context(), desc)
	case desc.MediaType.IsImage():
		info.Config, err = imageConfig(desc)
	}
	if err != nil {
		return nil, err
	}
	return info, nil
}

// platformImages maps each platform in the index desc describes to that
// platform's image in repo.
func platformImages(repo name.Repository, desc *remote.Descriptor) (map[string]Image, error) {
	idx, err := desc.ImageIndex()
	if err != nil {
		return nil, fmt.Errorf("parse index: %w", err)
	}
	imf, err := idx.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("parse index manifest: %w", err)
	}
	images := make(map[string]Image, len(imf.Manifests))
	for _, m := range imf.Manifests {
		if m.Platform == nil {
			continue
		}
		images[m.Platform.String()] = Image{
			Digest:   m.Digest.String(),
			ImageRef: repo.Digest(m.Digest.String()).String(),
		}
	}
	return images, nil
}

// imageConfig reads the config of the image desc describes.
func imageConfig(desc *remote.Descriptor) (*Config, error) {
	img, err := desc.Image()
	if err != nil {
		return nil, fmt.Errorf("parse image: %w", err)
	}
	cf, err := img.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg := &Config{}
	cfg.FromConfigFile(cf)
	return cfg, nil
}

var imageType = basetypes.ObjectType{
	AttrTypes: map[string]attr.Type{
		"digest":    basetypes.StringType{},
		"image_ref": basetypes.StringType{},
	},
}

var manifestAttribute = schema.ObjectAttribute{
	MarkdownDescription: "Manifest of the image or index.",
	AttributeTypes: map[string]attr.Type{
		"schema_version": basetypes.NumberType{},
		"media_type":     basetypes.StringType{},
		"config":         descriptorType,
		"layers": basetypes.ListType{
			ElemType: descriptorType,
		},
		"annotations": basetypes.MapType{
			ElemType: basetypes.StringType{},
		},
		"manifests": basetypes.ListType{
			ElemType: descriptorType,
		},
		"subject": descriptorType,
	},
}

var descriptorType = basetypes.ObjectType{
	AttrTypes: map[string]attr.Type{
		"media_type": basetypes.StringType{},
		"size":       basetypes.NumberType{},
		"digest":     basetypes.StringType{},
		"platform": basetypes.ObjectType{
			AttrTypes: map[string]attr.Type{
				"architecture": basetypes.StringType{},
				"os":           basetypes.StringType{},
				"variant":      basetypes.StringType{},
				"os_version":   basetypes.StringType{},
			},
		},
	},
}

var configAttribute = schema.ObjectAttribute{
	MarkdownDescription: "Config of an image.",
	AttributeTypes: map[string]attr.Type{
		"env": basetypes.ListType{
			ElemType: basetypes.StringType{},
		},
		"user":        basetypes.StringType{},
		"working_dir": basetypes.StringType{},
		"entrypoint": basetypes.ListType{
			ElemType: basetypes.StringType{},
		},
		"cmd": basetypes.ListType{
			ElemType: basetypes.StringType{},
		},
		"created_at": basetypes.StringType{},
	},
}
