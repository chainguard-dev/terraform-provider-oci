package provider

import (
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &StringDataSource{}

func NewStringDataSource() datasource.DataSource {
	return &StringDataSource{}
}

// StringDataSource defines the data source implementation.
type StringDataSource struct {
	popts ProviderOpts
}

type StringDataSourceModel struct {
	Input        types.String `tfsdk:"input"`
	Id           types.String `tfsdk:"id"`
	Registry     types.String `tfsdk:"registry"`
	Repo         types.String `tfsdk:"repo"`
	Digest       types.String `tfsdk:"digest"`
	PseudoTag    types.String `tfsdk:"pseudo_tag"`
	RegistryRepo types.String `tfsdk:"registry_repo"`
}

// Metadata implements datasource.DataSource.
func (*StringDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_string"
}

// Schema implements datasource.DataSource.
func (d *StringDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Data source for parsing a pinned oci string into its constituent parts. A pinned oci reference is one that includes a digest, and is in the format: '${registry}/${repo}@${digest}'. For example: 'cgr.dev/my-project/my-image@sha256:...'.`,
		Attributes: map[string]schema.Attribute{
			"input": schema.StringAttribute{
				MarkdownDescription: `The oci reference string to parse. This supports any valid oci reference string, including those with a tag, digest, or both. For example: 'cgr.dev/my-project/my-image:latest' or 'cgr.dev/my-project/my-image@sha256:...'. Note that when tags are provided, they will be replaced in favor of the digest.`,
				Required:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "The fully qualified oci reference string, in the format: '${registry}/${repo}@${digest}'. For example: `cgr.dev/my-project/my-image@sha256:...`",
				Computed:            true,
			},
			"registry": schema.StringAttribute{
				MarkdownDescription: "The registry of the oci reference. For example: `cgr.dev`",
				Computed:            true,
			},
			"repo": schema.StringAttribute{
				MarkdownDescription: "The repository of the oci reference. For example: `my-project/my-image`",
				Computed:            true,
			},
			"registry_repo": schema.StringAttribute{
				MarkdownDescription: "Helper attribute equivalent to '${registry}/${repo}'. For example: `cgr.dev/my-project/my-image`",
				Computed:            true,
			},
			"digest": schema.StringAttribute{
				MarkdownDescription: "The digest of the oci reference. For example: `sha256:...`",
				Computed:            true,
			},
			"pseudo_tag": schema.StringAttribute{
				MarkdownDescription: "A pseudo tag pinned to a digest that can be used in place of a real tag. This is useful for cases where a tag is not provided, but is required for compatibility reasons. For example: `unused@sha256:...`. The tag always has the value `unused`, and the digest is the same as the input digest.",
				Computed:            true,
			},
		},
	}
}

// Read implements datasource.DataSource.
func (d *StringDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data StringDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ref, err := name.ParseReference(data.Input.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid reference string", fmt.Sprintf("Unable to parse ref %s, got error: %v", data.Input.ValueString(), err))
	}
	data.Id = types.StringValue(ref.Name())

	data.Registry = types.StringValue(ref.Context().RegistryStr())
	data.Repo = types.StringValue(ref.Context().RepositoryStr())
	data.RegistryRepo = types.StringValue(ref.Context().String())

	if _, ok := ref.(name.Tag); ok {
		resp.Diagnostics.AddError("Invalid reference string", fmt.Sprintf("Reference %s contains only a tag, but a digest is required", data.Input.ValueString()))
		return
	}

	if d, ok := ref.(name.Digest); ok {
		data.Digest = types.StringValue(d.DigestStr())
	}

	if data.PseudoTag == types.StringNull() && data.Digest != types.StringNull() {
		// Create a cursed tag that we can use in place of a "real" tag
		data.PseudoTag = types.StringValue(fmt.Sprintf("unused@%s", data.Digest.ValueString()))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
