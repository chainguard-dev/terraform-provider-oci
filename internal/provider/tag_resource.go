package provider

import (
	"context"
	"fmt"

	"github.com/chainguard-dev/terraform-provider-oci/pkg/validators"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ resource.Resource = &TagResource{}
var _ resource.ResourceWithImportState = &TagResource{}

func NewTagResource() resource.Resource {
	return &TagResource{}
}

// TagResource defines the resource implementation.
type TagResource struct {
	popts ProviderOpts
}

// TagResourceModel describes the resource data model.
type TagResourceModel struct {
	Id        types.String `tfsdk:"id"`
	TaggedRef types.String `tfsdk:"tagged_ref"`

	DigestRef types.String `tfsdk:"digest_ref"`
	Tag       types.String `tfsdk:"tag"`
}

func (r *TagResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tag"
}

func (r *TagResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Tag an existing image by digest.",
		Attributes: map[string]schema.Attribute{
			"digest_ref": schema.StringAttribute{
				MarkdownDescription: "Image ref by digest to apply the tag to.",
				Required:            true,
				Validators:          []validator.String{validators.DigestValidator{}},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"tag": schema.StringAttribute{
				MarkdownDescription: "Tag to apply to the image.",
				Required:            true,
				Validators:          []validator.String{validators.TagValidator{}},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},

			"tagged_ref": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The resulting fully-qualified image ref by digest (e.g. {repo}:tag@sha256:deadbeef).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The resulting fully-qualified image ref by digest (e.g. {repo}:tag@sha256:deadbeef).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *TagResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	popts, ok := req.ProviderData.(*ProviderOpts)
	if !ok || popts == nil {
		resp.Diagnostics.AddError("Client Error", "invalid provider data")
		return
	}
	r.popts = *popts
}

func (r *TagResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *TagResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	digest, err := r.doTag(ctx, data)
	if err != nil {
		resp.Diagnostics.AddError("Tag Error", fmt.Sprintf("Error tagging image: %s", err.Error()))
		return
	}

	data.Id = types.StringValue(digest)
	data.TaggedRef = types.StringValue(digest)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TagResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *TagResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Don't actually tag, but check whether the digest is already tagged so we get a useful diff.
	// If the digest is already tagged, we'll set the ID and tagged_ref to the correct output value.
	// Otherwise, we'll set them to empty strings so that the create will run when applied.

	d, err := name.NewDigest(data.DigestRef.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Tag Error", fmt.Sprintf("Error parsing digest ref: %s", err.Error()))
		return
	}

	t := d.Context().Tag(data.Tag.ValueString())
	desc, err := remote.Get(t, r.popts.withContext(ctx)...)
	if err != nil {
		resp.Diagnostics.AddError("Tag Error", fmt.Sprintf("Error getting image: %s", err.Error()))
		return
	}

	if desc.Digest.String() != d.DigestStr() {
		data.Id = types.StringValue("")
		data.TaggedRef = types.StringValue("")
	} else {
		id := fmt.Sprintf("%s@%s", t.Name(), desc.Digest.String())
		data.Id = types.StringValue(id)
		data.TaggedRef = types.StringValue(id)
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TagResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *TagResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	digest, err := r.doTag(ctx, data)
	if err != nil {
		resp.Diagnostics.AddError("Tag Error", fmt.Sprintf("Error tagging image: %s", err.Error()))
		return
	}

	data.Id = types.StringValue(digest)
	data.TaggedRef = types.StringValue(digest)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TagResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.Append(req.State.Get(ctx, &TagResourceModel{})...)
}

func (r *TagResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *TagResource) doTag(ctx context.Context, data *TagResourceModel) (string, error) {
	d, err := name.NewDigest(data.DigestRef.ValueString())
	if err != nil {
		return "", fmt.Errorf("digest_ref must be a digest reference: %v", err)
	}
	t := d.Context().Tag(data.Tag.ValueString())
	if err != nil {
		return "", fmt.Errorf("error parsing tag: %v", err)
	}

	desc, err := remote.Get(d, r.popts.withContext(ctx)...)
	if err != nil {
		return "", fmt.Errorf("error fetching digest: %v", err)
	}
	if err := remote.Tag(t, desc, r.popts.withContext(ctx)...); err != nil {
		return "", fmt.Errorf("error tagging digest: %v", err)
	}
	digest := fmt.Sprintf("%s@%s", t.Name(), desc.Digest.String())
	return digest, nil
}
