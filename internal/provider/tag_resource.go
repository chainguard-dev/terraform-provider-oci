package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/chainguard-dev/terraform-provider-oci/pkg/validators"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"golang.org/x/sync/errgroup"
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
	Tags      []string     `tfsdk:"tags"`
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
				Optional:            true,
				Validators:          []validator.String{validators.TagValidator{}},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				DeprecationMessage:  "The `tag` attribute is deprecated. Use `tags` instead.",
			},
			"tags": schema.ListAttribute{
				MarkdownDescription: "Tags to apply to the image.",
				// TODO: make this required after tag deprecation period.
				Optional:      true,
				ElementType:   basetypes.StringType{},
				Validators:    []validator.List{uniqueTagsValidator{}},
				PlanModifiers: []planmodifier.List{listplanmodifier.RequiresReplace()},
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

	// Don't actually tag, but check whether the digest is already tagged with all requested tags, so we get a useful diff.
	// If the digest is already tagged with all requested tags, we'll set the ID and tagged_ref to the correct output value.
	// Otherwise, we'll set them to empty strings so that the create will run when applied.

	d, err := name.NewDigest(data.DigestRef.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Tag Error", fmt.Sprintf("Error parsing digest ref: %s", err.Error()))
		return
	}

	tags := []string{}
	if data.Tag.ValueString() != "" {
		tags = append(tags, data.Tag.ValueString())
	} else if len(data.Tags) > 0 {
		tags = data.Tags
	} else {
		resp.Diagnostics.AddError("Tag Error", "either tag or tags must be set")
	}
	if data.Tag.ValueString() != "" && len(data.Tags) > 0 {
		resp.Diagnostics.AddError("Tag Error", "only one of tag or tags may be set")
	}
	for _, tag := range tags {
		t := d.Context().Tag(tag)
		desc, err := remote.Get(t, r.popts.withContext(ctx)...)
		if err != nil {
			// Failed to get the image by tag, so we need to create.
			return
		}

		// Some tag is wrong, so we need to create.
		if desc.Digest.String() != d.DigestStr() {
			data.Id = types.StringValue("")
			data.TaggedRef = types.StringValue("")
			break
		}
	}

	// All tags are correct so we can set the ID and tagged_ref to the correct output value.
	id := fmt.Sprintf("%s@%s", d.Context().Tag(tags[0]), d.DigestStr())
	data.Id = types.StringValue(id)
	data.TaggedRef = types.StringValue(id)

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
	var tags []string
	if data.Tag.ValueString() != "" {
		tags = append(tags, data.Tag.ValueString())
	} else if len(data.Tags) > 0 {
		tags = data.Tags
	} else {
		return "", errors.New("either tag or tags must be set")
	}
	if data.Tag.ValueString() != "" && len(data.Tags) > 0 {
		return "", errors.New("only one of tag or tags may be set")
	}

	d, err := name.NewDigest(data.DigestRef.ValueString())
	if err != nil {
		return "", fmt.Errorf("digest_ref must be a digest reference: %v", err)
	}

	desc, err := remote.Get(d, r.popts.withContext(ctx)...)
	if err != nil {
		return "", fmt.Errorf("error fetching digest: %v", err)
	}

	errg, ctx := errgroup.WithContext(ctx)
	for _, tag := range tags {
		tag := tag
		errg.Go(func() error {
			t := d.Context().Tag(tag)
			if err != nil {
				return fmt.Errorf("error parsing tag %q: %v", tag, err)
			}
			if err := remote.Tag(t, desc, r.popts.withContext(ctx)...); err != nil {
				return fmt.Errorf("error tagging digest with %q: %v", tag, err)
			}
			return nil
		})
	}
	if err := errg.Wait(); err != nil {
		return "", err
	}

	t := d.Context().Tag(tags[0])
	if err != nil {
		return "", fmt.Errorf("error parsing tag: %v", err)
	}
	digest := fmt.Sprintf("%s@%s", t.Name(), d.DigestStr())
	return digest, nil
}

type uniqueTagsValidator struct{}

var _ validator.List = uniqueTagsValidator{}

func (v uniqueTagsValidator) Description(context.Context) string {
	return `value must be valid OCI tag elements (e.g., "latest", "v1.2.3")`
}
func (v uniqueTagsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v uniqueTagsValidator) ValidateList(ctx context.Context, req validator.ListRequest, resp *validator.ListResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	var tags []string
	if diag := req.ConfigValue.ElementsAs(ctx, &tags, false); diag.HasError() {
		resp.Diagnostics.Append(diag...)
		return
	}

	seen := map[string]bool{}
	for _, t := range tags {
		if seen[t] {
			resp.Diagnostics.AddWarning("Duplicate tag", fmt.Sprintf("duplicate tag %q", t))
		}
		seen[t] = true
		if _, err := name.NewTag("example.com:" + t); err != nil {
			resp.Diagnostics.AddError("Invalid OCI tag name", fmt.Sprintf("parsing tag %q: %v", t, err))
		}
	}
}
