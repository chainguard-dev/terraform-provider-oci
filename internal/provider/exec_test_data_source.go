package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/chainguard-dev/terraform-provider-oci/pkg/validators"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &ExecTestDataSource{}

func NewExecTestDataSource() datasource.DataSource {
	return &ExecTestDataSource{}
}

// ExecTestDataSource defines the data source implementation.
type ExecTestDataSource struct {
	popts ProviderOpts
}

// ExecTestDataSourceModel describes the data source data model.
type ExecTestDataSourceModel struct {
	Digest         types.String `tfsdk:"digest"`
	Script         types.String `tfsdk:"script"`
	TimeoutSeconds types.Int64  `tfsdk:"timeout_seconds"`

	ExitCode  types.Int64  `tfsdk:"exit_code"`
	Output    types.String `tfsdk:"output"`
	Id        types.String `tfsdk:"id"`
	TestedRef types.String `tfsdk:"tested_ref"`
}

func (d *ExecTestDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_exec_test"
}

func (d *ExecTestDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Exec test data source",

		Attributes: map[string]schema.Attribute{
			"digest": schema.StringAttribute{
				MarkdownDescription: "Image digest to test",
				Optional:            false,
				Required:            true,
				Validators:          []validator.String{validators.DigestValidator{}},
			},
			"script": schema.StringAttribute{
				MarkdownDescription: "Script to run against the image",
				Required:            true,
			},
			"timeout_seconds": schema.Int64Attribute{
				MarkdownDescription: "Timeout for the test in seconds (default is 5 minutes)",
				Optional:            true,
				Validators:          []validator.Int64{positiveIntValidator{}},
			},

			// TODO: platform?

			"exit_code": schema.Int64Attribute{
				MarkdownDescription: "Exit code of the test",
				Computed:            true,
			},
			"output": schema.StringAttribute{
				MarkdownDescription: "Output of the test",
				Computed:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Fully qualified image digest of the image.",
				Computed:            true,
			},
			"tested_ref": schema.StringAttribute{
				MarkdownDescription: "Tested image ref by digest.",
				Computed:            true,
			},
		},
	}
}

func (d *ExecTestDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ExecTestDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ExecTestDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ref, err := name.NewDigest(data.Digest.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid ref", fmt.Sprintf("Unable to parse ref %s, got error: %s", data.Digest.ValueString(), err))
		return
	}
	// Check we can get the image before running the test.
	if _, err := remote.Image(ref, d.popts.withContext(ctx)...); err != nil {
		resp.Diagnostics.AddError("Unable to fetch image", fmt.Sprintf("Unable to fetch image for ref %s, got error: %s", data.Digest.ValueString(), err))
		return
	}

	timeout := data.TimeoutSeconds.ValueInt64()
	if timeout == 0 {
		timeout = 300
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	repo := ref.Context().RepositoryStr()
	registry := ref.Context().RegistryStr()
	cmd := exec.CommandContext(ctx, "sh", "-c", data.Script.ValueString())
	cmd.Env = append(os.Environ(),
		"IMAGE_NAME="+data.Digest.ValueString(),
		"IMAGE_REPOSITORY="+repo,
		"IMAGE_REGISTRY="+registry,
	)
	out, err := cmd.CombinedOutput()
	if len(out) > 1024 {
		out = out[len(out)-1024:] // trim output to the last 1KB
	}

	data.TestedRef = data.Digest
	data.Id = data.Digest
	data.ExitCode = types.Int64Value(int64(cmd.ProcessState.ExitCode()))
	data.Output = types.StringValue(string(out))

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		resp.Diagnostics.AddError("Test timed out", fmt.Sprintf("Test for ref %s timed out after %d seconds", data.Digest.ValueString(), timeout))
		return
	} else if err != nil {
		resp.Diagnostics.AddError("Test failed", fmt.Sprintf("Test failed for ref %s, got error: %s\n%s", data.Digest.ValueString(), err, string(out)))
		return
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "read a data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

type positiveIntValidator struct{}

func (positiveIntValidator) MarkdownDescription(context.Context) string { return "positive integer" }
func (positiveIntValidator) Description(context.Context) string         { return "positive integer" }
func (positiveIntValidator) ValidateInt64(ctx context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if i := req.ConfigValue.ValueInt64(); i < 0 {
		resp.Diagnostics.AddAttributeError(req.Path, fmt.Sprintf("value %d must be a positive integer", i), "")
	}
}
