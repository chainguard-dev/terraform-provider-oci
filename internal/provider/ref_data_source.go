package provider

import (
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
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
	Ref    types.String `tfsdk:"ref"`
	Id     types.String `tfsdk:"id"`
	Digest types.String `tfsdk:"digest"`
	Tag    types.String `tfsdk:"tag"`

	Manifest *Manifest `tfsdk:"manifest"`

	Images map[string]Image `tfsdk:"images"`
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

			"manifest": manifestAttribute,

			"images": schema.MapAttribute{
				MarkdownDescription: "Map of image platforms to manifests.",
				Computed:            true,
				ElementType:         imageType,
			},
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
	mf, err := ManifestFromDescriptor(desc)
	if err != nil {
		resp.Diagnostics.AddError("Unable to parse manifest", fmt.Sprintf("Unable to parse manifest for ref %s, got error: %s", data.Ref.String(), err))
		return
	}
	data.Manifest = mf

	if desc.MediaType.IsIndex() {
		idx, err := desc.ImageIndex()
		if err != nil {
			resp.Diagnostics.AddError("Unable to parse index", fmt.Sprintf("Unable to parse index for ref %s, got error: %s", data.Ref.String(), err))
			return
		}
		imf, err := idx.IndexManifest()
		if err != nil {
			resp.Diagnostics.AddError("Unable to parse index manifest", fmt.Sprintf("Unable to parse index manifest for ref %s, got error: %s", data.Ref.String(), err))
			return
		}
		data.Images = make(map[string]Image, len(imf.Manifests))
		for _, m := range imf.Manifests {
			if m.Platform == nil {
				continue
			}
			data.Images[m.Platform.String()] = Image{
				Digest:   types.StringValue(m.Digest.String()),
				ImageRef: types.StringValue(ref.Context().Digest(m.Digest.String()).String()),
			}
		}
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "read a data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

var imageType = basetypes.ObjectType{
	AttrTypes: map[string]attr.Type{
		"digest":    basetypes.StringType{},
		"image_ref": basetypes.StringType{},
	},
}

var manifestAttribute = schema.ObjectAttribute{
	MarkdownDescription: "Manifest of the image or index.",
	Computed:            true,
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
