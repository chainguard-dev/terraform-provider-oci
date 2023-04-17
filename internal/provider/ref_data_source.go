package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &RefDataSource{}

func NewRefDataSource() datasource.DataSource {
	return &RefDataSource{}
}

// RefDataSource defines the data source implementation.
type RefDataSource struct {
	client *http.Client
}

// ExampleDataSourceModel describes the data source data model.
type ExampleDataSourceModel struct {
	Ref    types.String `tfsdk:"ref"`
	Id     types.String `tfsdk:"id"`
	Digest types.String `tfsdk:"digest"`
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
		},
	}
}

func (d *RefDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*http.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *http.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.client = client
}

func (d *RefDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ExampleDataSourceModel

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
	desc, err := remote.Get(ref, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		resp.Diagnostics.AddError("Unable to read ref", fmt.Sprintf("Unable to read ref %s, got error: %s", data.Ref.String(), err))
		return
	}

	data.Id = types.StringValue(ref.Context().Digest(desc.Digest.String()).String())
	data.Digest = types.StringValue(desc.Digest.String())

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "read a data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
