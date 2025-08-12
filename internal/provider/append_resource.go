package provider

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	ggcrtypes "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &AppendResource{}
	_ resource.ResourceWithImportState = &AppendResource{}
)

func NewAppendResource() resource.Resource {
	return &AppendResource{}
}

// AppendResource defines the resource implementation.
type AppendResource struct {
	popts ProviderOpts
}

// AppendResourceModel describes the resource data model.
type AppendResourceModel struct {
	Id       types.String `tfsdk:"id"`
	ImageRef types.String `tfsdk:"image_ref"`

	BaseImage types.String `tfsdk:"base_image"`
	Layers    types.List   `tfsdk:"layers"`
}

func (r *AppendResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_append"
}

func (r *AppendResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Append layers to an existing image.",
		Attributes: map[string]schema.Attribute{
			"base_image": schema.StringAttribute{
				MarkdownDescription: "Base image to append layers to.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("cgr.dev/chainguard/static:latest"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"layers": schema.ListNestedAttribute{
				MarkdownDescription: "Layers to append to the base image.",
				Optional:            false,
				Required:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"files": schema.MapNestedAttribute{
							MarkdownDescription: "Files to add to the layer.",
							Required:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"contents": schema.StringAttribute{
										MarkdownDescription: "Content of the file.",
										Optional:            true,
									},
									"path": schema.StringAttribute{
										MarkdownDescription: "Path to a file.",
										Optional:            true,
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
			"image_ref": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The resulting fully-qualified digest (e.g. {repo}@sha256:deadbeef).",
			},
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The resulting fully-qualified digest (e.g. {repo}@sha256:deadbeef).",
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

	popts, ok := req.ProviderData.(*ProviderOpts)
	if !ok || popts == nil {
		resp.Diagnostics.AddError("Client Error", "invalid provider data")
		return
	}
	r.popts = *popts
}

func (r *AppendResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *AppendResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	digest, diag := r.doAppend(ctx, data)
	if diag.HasError() {
		resp.Diagnostics.Append(diag...)
		return
	}
	data.Id = types.StringValue(digest.String())
	data.ImageRef = types.StringValue(digest.String())

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AppendResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *AppendResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	digest, diag := r.doAppend(ctx, data)
	if diag.HasError() {
		resp.Diagnostics.Append(diag...)
		return
	}

	data.Id = types.StringValue(digest.String())
	data.ImageRef = types.StringValue(digest.String())

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AppendResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *AppendResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	digest, diag := r.doAppend(ctx, data)
	if diag.HasError() {
		resp.Diagnostics.Append(diag...)
		return
	}

	data.Id = types.StringValue(digest.String())
	data.ImageRef = types.StringValue(digest.String())

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AppendResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *AppendResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// TODO: optionally delete the previous image when the resource is deleted.
}

func (r *AppendResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *AppendResource) doAppend(ctx context.Context, data *AppendResourceModel) (*name.Digest, diag.Diagnostics) {
	baseref, err := name.ParseReference(data.BaseImage.ValueString())
	if err != nil {
		return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to parse base image", fmt.Sprintf("Unable to parse base image %q, got error: %s", data.BaseImage.ValueString(), err))}
	}

	ropts := r.popts.withContext(ctx)

	desc, err := remote.Get(baseref, ropts...)
	if err != nil {
		return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to fetch base image", fmt.Sprintf("Unable to fetch base image %q, got error: %s", data.BaseImage.ValueString(), err))}
	}

	var ls []struct {
		Files map[string]struct {
			Contents types.String `tfsdk:"contents"`
			Path     types.String `tfsdk:"path"`
		} `tfsdk:"files"`
	}
	if diag := data.Layers.ElementsAs(ctx, &ls, false); diag.HasError() {
		return nil, diag.Errors()
	}

	adds := []mutate.Addendum{}
	for _, l := range ls {
		var b bytes.Buffer
		zw := gzip.NewWriter(&b)
		tw := tar.NewWriter(zw)
		for name, f := range l.Files {
			var (
				size   int64
				mode   int64
				datarc io.ReadCloser
			)

			write := func(rc io.ReadCloser) error {
				defer rc.Close()
				if err := tw.WriteHeader(&tar.Header{
					Name: name,
					Size: size,
					Mode: mode,
				}); err != nil {
					return fmt.Errorf("unable to write tar header: %w", err)
				}

				if _, err := io.CopyN(tw, rc, size); err != nil {
					return fmt.Errorf("unable to write tar contents: %w", err)
				}
				return nil
			}

			if f.Contents.ValueString() != "" {
				size = int64(len(f.Contents.ValueString()))
				mode = 0o644
				datarc = io.NopCloser(strings.NewReader(f.Contents.ValueString()))

			} else if f.Path.ValueString() != "" {
				fi, err := os.Stat(f.Path.ValueString())
				if err != nil {
					return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to stat file", fmt.Sprintf("Unable to stat file %q, got error: %s", f.Path.ValueString(), err))}
				}

				// skip any directories or symlinks
				if fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
					continue
				}

				size = fi.Size()
				mode = int64(fi.Mode())

				fr, err := os.Open(f.Path.ValueString())
				if err != nil {
					return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to open file", fmt.Sprintf("Unable to open file %q, got error: %s", f.Path.ValueString(), err))}
				}
				datarc = fr

			} else {
				return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("No file contents or path specified", fmt.Sprintf("No file contents or path specified for %q", name))}
			}

			if err := write(datarc); err != nil {
				return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to write tar contents", fmt.Sprintf("Unable to write tar contents for %q, got error: %s", name, err))}
			}
		}
		if err := tw.Close(); err != nil {
			return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to close tar writer", fmt.Sprintf("Unable to close tar writer, got error: %s", err))}
		}
		if err := zw.Close(); err != nil {
			return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to close gzip writer", fmt.Sprintf("Unable to close gzip writer, got error: %s", err))}
		}

		l, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewBuffer(b.Bytes())), nil
		})
		if err != nil {
			return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to create layer", fmt.Sprintf("Unable to create layer, got error: %s", err))}
		}

		adds = append(adds, mutate.Addendum{
			Layer:     l,
			History:   v1.History{CreatedBy: "terraform-provider-oci: oci_append"},
			MediaType: ggcrtypes.OCILayer,
		})
	}

	var d name.Digest

	if desc.MediaType.IsIndex() {
		baseidx, err := desc.ImageIndex()
		if err != nil {
			return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to read image index", fmt.Sprintf("Unable to read image index for ref %q, got error: %s", data.BaseImage.ValueString(), err))}
		}

		baseimf, err := baseidx.IndexManifest()
		if err != nil {
			return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to read image index manifest", fmt.Sprintf("Unable to read image index manifest for ref %q, got error: %s", data.BaseImage.ValueString(), err))}
		}

		var idx v1.ImageIndex = empty.Index

		// append to each manifest in the index
		for _, manifest := range baseimf.Manifests {
			baseimg, err := baseidx.Image(manifest.Digest)
			if err != nil {
				return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to load image", fmt.Sprintf("Unable to load image for ref %q, got error: %s", data.BaseImage.ValueString(), err))}
			}

			img, err := mutate.Append(baseimg, adds...)
			if err != nil {
				return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to append layers", fmt.Sprintf("Unable to append layers, got error: %s", err))}
			}

			imgdig, err := img.Digest()
			if err != nil {
				return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to get image digest", fmt.Sprintf("Unable to get image digest, got error: %s", err))}
			}

			if err := remote.Write(baseref.Context().Digest(imgdig.String()), img, ropts...); err != nil {
				return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to push image", fmt.Sprintf("Unable to push image, got error: %s", err))}
			}

			// Update the index with the new image
			idx = mutate.AppendManifests(idx, mutate.IndexAddendum{
				Add: img,
				Descriptor: v1.Descriptor{
					MediaType:    manifest.MediaType,
					URLs:         manifest.URLs,
					Annotations:  manifest.Annotations,
					Platform:     manifest.Platform,
					ArtifactType: manifest.ArtifactType,
				},
			})
		}

		dig, err := idx.Digest()
		if err != nil {
			return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to get index digest", fmt.Sprintf("Unable to get index digest, got error: %s", err))}
		}

		d = baseref.Context().Digest(dig.String())
		if err := remote.WriteIndex(d, idx, ropts...); err != nil {
			return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to push index", fmt.Sprintf("Unable to push index, got error: %s", err))}
		}

	} else if desc.MediaType.IsImage() {
		baseimg, err := remote.Image(baseref, ropts...)
		if err != nil {
			return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to fetch base image", fmt.Sprintf("Unable to fetch base image %q, got error: %s", data.BaseImage.ValueString(), err))}
		}

		img, err := mutate.Append(baseimg, adds...)
		if err != nil {
			return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to append layers", fmt.Sprintf("Unable to append layers, got error: %s", err))}
		}

		dig, err := img.Digest()
		if err != nil {
			return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to get image digest", fmt.Sprintf("Unable to get image digest, got error: %s", err))}
		}

		d = baseref.Context().Digest(dig.String())
		if err := remote.Write(d, img, r.popts.withContext(ctx)...); err != nil {
			return nil, []diag.Diagnostic{diag.NewErrorDiagnostic("Unable to push image", fmt.Sprintf("Unable to push image, got error: %s", err))}
		}
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "created a resource")

	return &d, []diag.Diagnostic{}
}
