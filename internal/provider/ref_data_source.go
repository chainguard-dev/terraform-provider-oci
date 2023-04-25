package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &RefDataSource{}

func NewRefDataSource() datasource.DataSource {
	return &RefDataSource{}
}

// RefDataSource defines the data source implementation.
type RefDataSource struct{}

// RefDataSourceModel describes the data source data model.
type RefDataSourceModel struct {
	Ref      types.String `tfsdk:"ref"`
	Id       types.String `tfsdk:"id"`
	Digest   types.String `tfsdk:"digest"`
	Tag      types.String `tfsdk:"tag"`
	Manifest types.Object `tfsdk:"manifest"`
	Config   types.Object `tfsdk:"config"`
}

func (d *RefDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ref"
}

func (d *RefDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Image ref data source",

		Attributes: map[string]schema.Attribute{
			"ref": schema.StringAttribute{
				MarkdownDescription: "Image ref to lookup",
				Optional:            false,
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Fully qualified image digest of the image.",
				Computed:            true,
			},
			"digest": schema.StringAttribute{
				MarkdownDescription: "Image digest of the image.",
				Computed:            true,
			},
			"tag": schema.StringAttribute{
				MarkdownDescription: "Image tag of the image.",
				Computed:            true,
			},
			"manifest": schema.ObjectAttribute{
				MarkdownDescription: "Manifest of the image or index.",
				Computed:            true,
				AttributeTypes:      abstractManifestSchema,
			},
			"config": schema.ObjectAttribute{
				MarkdownDescription: "Config of the image, if it's an image.",
				Computed:            true,
				AttributeTypes:      configSchema,
			},

			// TODO:
			// - output attribute for digests by platform (for indexes)
			// - output attribute for layers and config (for images)
			// - output attribute for manifest information (annotations, etc)
		},
	}
}

func (d *RefDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}
}

func (d *RefDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RefDataSourceModel

	// Read Terraform configuration data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ref, err := name.ParseReference(data.Ref.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid ref", fmt.Sprintf("Unable to parse ref %s, got error: %s", data.Ref.ValueString(), err))
		return
	}
	if t, ok := ref.(name.Tag); ok {
		data.Tag = types.StringValue(t.TagStr())
	}
	desc, err := remote.Get(ref,
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
	)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read ref", fmt.Sprintf("Unable to read ref %s, got error: %s", data.Ref.String(), err))
		return
	}

	data.Id = types.StringValue(ref.Context().Digest(desc.Digest.String()).String())
	data.Digest = types.StringValue(desc.Digest.String())

	{
		var diag diag.Diagnostics
		if desc.MediaType.IsImage() {
			img, err := desc.Image()
			if err != nil {
				resp.Diagnostics.AddError("Unable to read image", fmt.Sprintf("Unable to read image %q, got error: %s", data.Ref.String(), err))
				return
			}

			/*
				rmf, err := img.RawManifest()
				if err != nil {
					resp.Diagnostics.AddError("Unable to read image manifest", fmt.Sprintf("Unable to read image manifest %q, got error: %s", data.Ref.String(), err))
					return
				}
				var mf imageManifest
				if err := json.Unmarshal(rmf, &mf); err != nil {
					resp.Diagnostics.AddError("Unable to read image manifest", fmt.Sprintf("Unable to read image manifest %q, got error: %s", data.Ref.String(), err))
					return
				}
				data.Manifest, diag = types.ObjectValueFrom(ctx, imageManifestSchema, mf)
				resp.Diagnostics.Append(diag...)
				if diag.HasError() {
					return
				}
			*/

			rcf, err := img.RawConfigFile()
			if err != nil {
				resp.Diagnostics.AddError("Unable to read image config", fmt.Sprintf("Unable to read image config %q, got error: %s", data.Ref.String(), err))
				return
			}
			var cf imageConfig
			if err := json.Unmarshal(rcf, &cf); err != nil {
				resp.Diagnostics.AddError("Unable to read image config", fmt.Sprintf("Unable to read image config %q, got error: %s", data.Ref.String(), err))
				return
			}

			data.Config, diag = types.ObjectValueFrom(ctx, configSchema, cf)
			resp.Diagnostics.Append(diag...)
			if diag.HasError() {
				return
			}

		} else if desc.MediaType.IsIndex() {
			idx, err := desc.ImageIndex()
			if err != nil {
				resp.Diagnostics.AddError("Unable to read index", fmt.Sprintf("Unable to read index %q, got error: %s", data.Ref.String(), err))
				return
			}

			rmf, err := idx.RawManifest()
			if err != nil {
				resp.Diagnostics.AddError("Unable to read index manifest", fmt.Sprintf("Unable to read index manifest %q, got error: %s", data.Ref.String(), err))
				return
			}
			var mf indexManifest
			if err := json.Unmarshal(rmf, &idx); err != nil {
				resp.Diagnostics.AddError("Unable to read index manifest", fmt.Sprintf("Unable to read index manifest %q, got error: %s", data.Ref.String(), err))
				return
			}
			data.Manifest, diag = types.ObjectValueFrom(ctx, indexManifestSchema, mf)
			resp.Diagnostics.Append(diag...)
			if diag.HasError() {
				return
			}
		} else {
			resp.Diagnostics.AddError("Unknown media type", fmt.Sprintf("Unknown media type %q", desc.MediaType))
			return
		}
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "read a data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

var configSchema = map[string]attr.Type{
	"architecture": basetypes.StringType{},
	"created":      basetypes.StringType{},
	"os":           basetypes.StringType{},
	"config": basetypes.ObjectType{
		AttrTypes: map[string]attr.Type{
			"env": basetypes.ListType{
				ElemType: basetypes.StringType{},
			},
			"user": basetypes.StringType{},
		},
	},
}

type imageConfig struct {
	Architecture string `tfsdk:"architecture"`
	Created      string `tfsdk:"created"`
	OS           string `tfsdk:"os"`
	Config       struct {
		Env  []string `tfsdk:"env"`
		User string   `tfsdk:"user"`
	} `tfsdk:"config"`
}

type imageManifest struct {
	SchemaVersion int64             `tfsdk:"schema_version"`
	MediaType     string            `tfsdk:"media_type"`
	Annotations   map[string]string `tfsdk:"annotations"`
	Subject       string            `tfsdk:"subject"`
	Config        descriptor        `tfsdk:"config"`
	Layers        []descriptor      `tfsdk:"layers"`
}

type indexManifest struct {
	SchemaVersion int64             `tfsdk:"schema_version"`
	MediaType     string            `tfsdk:"media_type"`
	Annotations   map[string]string `tfsdk:"annotations"`
	Subject       string            `tfsdk:"subject"`
	Manifests     []descriptor      `tfsdk:"manifests"`
}

type descriptor struct {
	MediaType string `tfsdk:"media_type"`
	Digest    string `tfsdk:"digest"`
	Size      int64  `tfsdk:"size"`
}

var abstractManifestSchema = map[string]attr.Type{
	"schemaVersion": basetypes.NumberType{},
	"mediaType":     basetypes.StringType{},
	"annotations": basetypes.MapType{
		ElemType: basetypes.StringType{},
	},
	"subject": basetypes.StringType{},
	"config": basetypes.ObjectType{
		AttrTypes: descriptorSchema,
	},
	"layers": basetypes.ListType{
		ElemType: basetypes.ObjectType{
			AttrTypes: descriptorSchema,
		},
	},
	"manifests": basetypes.ListType{
		ElemType: basetypes.ObjectType{
			AttrTypes: map[string]attr.Type{
				"digest":    basetypes.StringType{},
				"mediaType": basetypes.StringType{},
				"size":      basetypes.NumberType{},
				"platform": basetypes.ObjectType{
					AttrTypes: map[string]attr.Type{
						"architecture": basetypes.StringType{},
						"os":           basetypes.StringType{},
						"os.version":   basetypes.StringType{},
						"variant":      basetypes.StringType{},
					},
				},
			},
		},
	},
}

var imageManifestSchema = map[string]attr.Type{
	"schemaVersion": basetypes.NumberType{},
	"mediaType":     basetypes.StringType{},
	"annotations": basetypes.MapType{
		ElemType: basetypes.StringType{},
	},
	"subject": basetypes.StringType{},
	"config": basetypes.ObjectType{
		AttrTypes: descriptorSchema,
	},
	"layers": basetypes.ListType{
		ElemType: basetypes.ObjectType{
			AttrTypes: descriptorSchema,
		},
	},
}

var descriptorSchema = map[string]attr.Type{
	"digest":    basetypes.StringType{},
	"mediaType": basetypes.StringType{},
	"size":      basetypes.NumberType{},
}

var indexManifestSchema = map[string]attr.Type{
	"schemaVersion": basetypes.NumberType{},
	"mediaType":     basetypes.StringType{},
	"annotations": basetypes.MapType{
		ElemType: basetypes.StringType{},
	},
	"subject": basetypes.StringType{},
	"manifests": basetypes.ListType{
		ElemType: basetypes.ObjectType{
			AttrTypes: map[string]attr.Type{
				"digest":    basetypes.StringType{},
				"mediaType": basetypes.StringType{},
				"size":      basetypes.NumberType{},
				"platform": basetypes.ObjectType{
					AttrTypes: map[string]attr.Type{
						"architecture": basetypes.StringType{},
						"os":           basetypes.StringType{},
						"os.version":   basetypes.StringType{},
						"variant":      basetypes.StringType{},
					},
				},
			},
		},
	},
}
