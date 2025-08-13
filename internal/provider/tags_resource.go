package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/chainguard-dev/terraform-provider-oci/pkg/validators"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var (
	_ resource.Resource                = &TagsResource{}
	_ resource.ResourceWithImportState = &TagsResource{}
)

func NewTagsResource() resource.Resource {
	return &TagsResource{}
}

// TagsResource defines the resource implementation.
type TagsResource struct {
	popts ProviderOpts
}

// TagsResourceModel describes the resource data model.
type TagsResourceModel struct {
	Id types.String `tfsdk:"id"`

	Repo string            `tfsdk:"repo"`
	Tags map[string]string `tfsdk:"tags"` // tag -> digest
}

func (r *TagsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tags"
}

func (r *TagsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Tag many digests with many tags.",
		Attributes: map[string]schema.Attribute{
			"repo": schema.StringAttribute{
				MarkdownDescription: "Repository for the tags.",
				Required:            true,
				Validators:          []validator.String{validators.RepoValidator{}},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"tags": schema.MapAttribute{
				MarkdownDescription: "Map of tag -> digest to apply.",
				Required:            true,
				ElementType:         basetypes.StringType{},
				// TODO: validator -- check that digests and tags are well formed.
				PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()},
			},

			// TODO: any outputs?

			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The resulting fully-qualified image ref by digest (e.g. {repo}:tag@sha256:deadbeef).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *TagsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TagsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *TagsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	digest, err := r.doTags(ctx, data)
	if err != nil {
		resp.Diagnostics.AddError("Tag Error", fmt.Sprintf("Error tagging image: %s", err.Error()))
		return
	}

	data.Id = types.StringValue(digest)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TagsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *TagsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Don't actually tag, but check whether the digests are already tagged with all requested tags, so we get a useful diff.
	// If the digests are already tagged with all requested tags, we'll set the ID to the correct output value.
	// Otherwise, we'll set them to empty strings so that the create will run when applied.
	// TODO: Can we get a better diff about what new updates will be applied?
	if id, err := r.checkTags(ctx, data); err != nil {
		data.Id = types.StringValue("")
	} else {
		data.Id = types.StringValue(id)
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TagsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *TagsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := r.doTags(ctx, data)
	if err != nil {
		resp.Diagnostics.AddError("Tag Error", fmt.Sprintf("Error tagging images: %s", err.Error()))
		return
	}

	data.Id = types.StringValue(id)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TagsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.Append(req.State.Get(ctx, &TagsResourceModel{})...)
}

func (r *TagsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *TagsResource) checkTags(ctx context.Context, data *TagsResourceModel) (string, error) {
	repo, err := name.NewRepository(data.Repo)
	if err != nil {
		return "", fmt.Errorf("error parsing repo ref: %w", err)
	}

	for tag, digest := range data.Tags {
		t := repo.Tag(tag)
		desc, err := remote.Head(t, r.popts.withContext(ctx)...)
		if err != nil {
			return "", fmt.Errorf("error getting tag %q: %w", t, err)
		}
		if desc.Digest.String() != digest {
			return "", fmt.Errorf("tag %q does not point to digest %q (got %q)", tag, digest, desc.Digest.String())
		}
	}
	// ID is the SHA256 of the JSONified map.
	b, err := json.Marshal(data.Tags)
	if err != nil {
		return "", fmt.Errorf("error marshaling tags: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(b)), nil
}

func (r *TagsResource) doTags(ctx context.Context, data *TagsResourceModel) (string, error) {
	repo, err := name.NewRepository(data.Repo)
	if err != nil {
		return "", fmt.Errorf("error parsing repo ref: %w", err)
	}

	for tag, digest := range data.Tags {
		t := repo.Tag(tag)
		d := repo.Digest(digest)
		desc, err := remote.Get(d, r.popts.withContext(ctx)...)
		if err != nil {
			return "", fmt.Errorf("error getting digest %q: %w", digest, err)
		}
		if err := remote.Tag(t, desc, r.popts.withContext(ctx)...); err != nil {
			return "", fmt.Errorf("error tagging %q with %q: %w", digest, tag, err)
		}
	}

	// ID is the SHA256 of the JSONified map.
	b, err := json.Marshal(data.Tags)
	if err != nil {
		return "", fmt.Errorf("error marshaling tags: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(b)), nil
}
