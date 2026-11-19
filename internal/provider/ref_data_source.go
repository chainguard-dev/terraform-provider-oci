package provider

import (
	"context"
	"fmt"

	"github.com/chainguard-dev/terraform-provider-oci/pkg/validators"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSourceWithConfigure = (*RefDataSource)(nil)

const refDataSourceMarkdownDesc = `Reads an OCI image or index from a registry. It exposes the same ` + "`full_ref`" + `, ` + "`digest`" + `, ` + "`tag`" + `, ` + "`manifest`" + `, ` + "`images`" + `, and ` + "`config`" + ` values that the ` + "`provider::oci::get`" + ` function returns for the same reference.

Use this data source instead of ` + "`provider::oci::get`" + ` for a tag that can move during a run. Terraform reads a data source once per run, during plan or during apply when ` + "`ref`" + ` is not known until then, and keeps the value it read for the rest of the run. Terraform calls a provider function again during apply and fails the run if the result differs from the one it planned with, which happens when a tag moves between plan and apply. The ` + "`digest`" + ` this data source reports is the one the tag pointed to when Terraform read it.

The read fails if the reference does not resolve, for example when the tag does not exist.`

func NewRefDataSource() datasource.DataSource {
	return &RefDataSource{}
}

// RefDataSource defines the data source implementation.
type RefDataSource struct {
	popts ProviderOpts
}

// RefDataSourceModel describes the data source data model.
type RefDataSourceModel struct {
	Ref types.String `tfsdk:"ref"`
	refInfo
}

func (d *RefDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ref"
}

func (d *RefDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: refDataSourceMarkdownDesc,
		Attributes: map[string]schema.Attribute{
			"ref": schema.StringAttribute{
				MarkdownDescription: "The OCI reference to read, by tag or by digest.",
				Required:            true,
				Validators:          []validator.String{validators.RefValidator{}},
			},
			"full_ref": schema.StringAttribute{
				MarkdownDescription: "The repository of `ref` at `digest` (e.g., `example.com/test@sha256:abcd1234...`).",
				Computed:            true,
			},
			"digest": schema.StringAttribute{
				MarkdownDescription: "The digest `ref` resolved to when Terraform read it.",
				Computed:            true,
			},
			"tag": schema.StringAttribute{
				MarkdownDescription: "The tag in `ref`, or empty when `ref` is a digest.",
				Computed:            true,
			},
			"manifest": schema.ObjectAttribute{
				MarkdownDescription: manifestAttribute.MarkdownDescription,
				AttributeTypes:      manifestAttribute.AttributeTypes,
				Computed:            true,
			},
			"images": schema.MapAttribute{
				MarkdownDescription: "Map of platform strings to the `digest` and `image_ref` of each platform-specific image. Populated for indexes only.",
				ElementType:         imageType,
				Computed:            true,
			},
			"config": schema.ObjectAttribute{
				MarkdownDescription: configAttribute.MarkdownDescription + " Populated for single-platform images only.",
				AttributeTypes:      configAttribute.AttributeTypes,
				Computed:            true,
			},
		},
	}
}

func (d *RefDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	popts, ok := req.ProviderData.(*ProviderOpts)
	if !ok || popts == nil {
		resp.Diagnostics.AddError("Client Error", "invalid provider data")
		return
	}
	d.popts = *popts
}

func (d *RefDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	// Only ref is set in config; the computed attributes are null there.
	var ref types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("ref"), &ref)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r, err := name.ParseReference(ref.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("ref"), "Invalid image reference", err.Error())
		return
	}

	info, err := getRef(r, d.popts.withContext(ctx)...)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("ref"), "Unable to fetch image", fmt.Sprintf("Unable to read %q: failed to %v", ref.ValueString(), err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &RefDataSourceModel{Ref: ref, refInfo: *info})...)
}
