package provider

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	ggcrtypes "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &AppendResource{}
var _ resource.ResourceWithImportState = &AppendResource{}

func NewAppendResource() resource.Resource {
	return &AppendResource{}
}

// AppendResource defines the resource implementation.
type AppendResource struct{}

// AppendResourceModel describes the resource data model.
type AppendResourceModel struct {
	// Id is the output image digest.
	Id types.String `tfsdk:"id"`

	BaseImage types.String `tfsdk:"base_image"`
	Layers    types.List   `tfsdk:"layers"`
}

func (r *AppendResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_append"
}

func (r *AppendResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Image append resource",

		Attributes: map[string]schema.Attribute{
			"base_image": schema.StringAttribute{
				MarkdownDescription: "Base image to append layers to.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("cgr.dev/chainguard/static:latest"),
			},
			"layers": schema.ListNestedAttribute{
				MarkdownDescription: "Layers to append to the base image.",
				Optional:            false,
				Required:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"files": schema.MapNestedAttribute{
							MarkdownDescription: "Files to add to the layer.",
							Optional:            false,
							Required:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"contents": schema.StringAttribute{
										MarkdownDescription: "Content of the file.",
										Optional:            false,
										Required:            true,
									},
									// TODO: Add support for file mode.
									// TODO: Add support for symlinks.
									// TODO: Add support for deletion / whiteouts.
								},
							},
						},
					},
				},
			},

			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Fully qualified image digest of the mutated image.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *AppendResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}
}

func (r *AppendResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *AppendResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, diag := doAppend(ctx, data)
	if diag.HasError() {
		resp.Diagnostics.Append(diag...)
		return
	}
	data.Id = types.StringValue(id)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func doAppend(ctx context.Context, data *AppendResourceModel) (string, diag.Diagnostics) {
	baseref, err := name.ParseReference(data.BaseImage.ValueString())
	if err != nil {
		return "", []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to parse base image", fmt.Sprintf("Unable to parse base image %s, got error: %s", data.BaseImage.ValueString(), err))}
	}
	img, err := remote.Image(baseref,
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
	)
	if err != nil {
		return "", []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to fetch base image", fmt.Sprintf("Unable to fetch base image %s, got error: %s", data.BaseImage.ValueString(), err))}
	}

	var ls []struct {
		Files map[string]struct {
			Contents types.String `tfsdk:"contents"`
			Mode     types.String `tfsdk:"mode"`
		} `tfsdk:"files"`
	}
	if diag := data.Layers.ElementsAs(ctx, &ls, false); diag.HasError() {
		return "", diag.Errors()
	}

	adds := []mutate.Addendum{}
	for _, l := range ls {
		var b bytes.Buffer
		tw := tar.NewWriter(&b)
		for name, f := range l.Files {
			if err := tw.WriteHeader(&tar.Header{
				Name: name,
				//	Mode: f.Mode.ValueString(),
				Size: int64(len(f.Contents.ValueString())),
			}); err != nil {
				return "", []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to write tar header", fmt.Sprintf("Unable to write tar header for %s, got error: %s", name, err))}
			}
			if _, err := tw.Write([]byte(f.Contents.ValueString())); err != nil {
				return "", []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to write tar contents", fmt.Sprintf("Unable to write tar contents for %s, got error: %s", name, err))}
			}
		}
		if err := tw.Close(); err != nil {
			return "", []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to close tar writer", fmt.Sprintf("Unable to close tar writer, got error: %s", err))}
		}

		adds = append(adds, mutate.Addendum{
			Layer:     static.NewLayer(b.Bytes(), ggcrtypes.OCILayer),
			History:   v1.History{CreatedBy: "terraform-provider-crane: crane_append"},
			MediaType: ggcrtypes.OCILayer,
		})
	}

	img, err = mutate.Append(img, adds...)
	if err != nil {
		return "", []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to append layers", fmt.Sprintf("Unable to append layers, got error: %s", err))}
	}
	if err := remote.Write(baseref, img,
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
		return "", []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to push image", fmt.Sprintf("Unable to push image, got error: %s", err))}
	}
	dig, err := img.Digest()
	if err != nil {
		return "", []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to get image digest", fmt.Sprintf("Unable to get image digest, got error: %s", err))}
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "created a resource")

	return baseref.Context().Digest(dig.String()).String(), []diag.Diagnostic{}
}

func (r *AppendResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *AppendResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AppendResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *AppendResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, diag := doAppend(ctx, data)
	if diag.HasError() {
		resp.Diagnostics.Append(diag...)
		return
	}
	data.Id = types.StringValue(id)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AppendResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *AppendResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// TODO: optionally delete the previous image when the resource is deleted.
}

func (r *AppendResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
