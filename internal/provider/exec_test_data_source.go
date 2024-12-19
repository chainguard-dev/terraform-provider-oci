package provider

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/chainguard-dev/terraform-provider-oci/pkg/validators"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
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
	WorkingDir     types.String `tfsdk:"working_dir"`
	Env            []EnvVar     `tfsdk:"env"`

	ExitCode  types.Int64  `tfsdk:"exit_code"`
	Output    types.String `tfsdk:"output"`
	Id        types.String `tfsdk:"id"`
	TestedRef types.String `tfsdk:"tested_ref"`
}

type EnvVar struct {
	Name  string `tfsdk:"name"`
	Value string `tfsdk:"value"`
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
			"working_dir": schema.StringAttribute{
				MarkdownDescription: "Working directory for the test",
				Optional:            true,
			},
			"env": schema.ListAttribute{
				ElementType: basetypes.ObjectType{
					AttrTypes: map[string]attr.Type{
						"name":  basetypes.StringType{},
						"value": basetypes.StringType{},
					},
				},
				MarkdownDescription: "Environment variables for the test",
				Optional:            true,
			},

			// TODO: platform?

			"exit_code": schema.Int64Attribute{
				MarkdownDescription: "Exit code of the test",
				Computed:            true,
			},
			"output": schema.StringAttribute{
				MarkdownDescription: "Output of the test",
				Computed:            true,
				DeprecationMessage:  "Not populated",
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
	if _, err := remote.Get(ref, d.popts.withContext(ctx)...); err != nil {
		resp.Diagnostics.AddError("Unable to fetch image", fmt.Sprintf("Unable to fetch image for ref %s, got error: %s", data.Digest.ValueString(), err))
		return
	}

	timeout := data.TimeoutSeconds.ValueInt64()
	if timeout == 0 {
		if d.popts.defaultExecTimeoutSeconds != 0 {
			timeout = d.popts.defaultExecTimeoutSeconds
		} else {
			timeout = 300
		}
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	// Prepopulate some environment variables:
	// - any environment variables defined on the host
	// - IMAGE_NAME: the fully qualified image name
	// - IMAGE_REPOSITORY: the repository part of the image name
	// - IMAGE_REGISTRY: the registry part of the image name
	// - FREE_PORT: a free port on the host
	// - any environment variables defined in the data source
	repo := ref.Context().RepositoryStr()
	registry := ref.Context().RegistryStr()
	env := append(os.Environ(),
		"IMAGE_NAME="+data.Digest.ValueString(),
		"IMAGE_REPOSITORY="+repo,
		"IMAGE_REGISTRY="+registry,
	)
	fp, err := freePort()
	if err != nil {
		resp.Diagnostics.AddError("Unable to find free port", fmt.Sprintf("Unable to find free port for ref %s, got error: %s", data.Digest.ValueString(), err))
		return
	}
	defer discardPort(fp)
	env = append(env, fmt.Sprintf("FREE_PORT=%d", fp))
	for _, e := range data.Env {
		env = append(env, fmt.Sprintf("%s=%s", e.Name, e.Value))
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", data.Script.ValueString())
	cmd.Env = env
	cmd.Dir = data.WorkingDir.ValueString()

	fullout, err := cmd.CombinedOutput()
	data.Output = types.StringValue("") // always empty.

	data.TestedRef = data.Digest
	data.Id = types.StringValue(md5str(data.Script.ValueString()) + data.Digest.ValueString())
	data.ExitCode = types.Int64Value(int64(cmd.ProcessState.ExitCode()))

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		resp.Diagnostics.AddError("Test timed out", fmt.Sprintf("Test for ref %s timed out after %d seconds:\n%s", data.Digest.ValueString(), timeout, string(fullout)))
		return
	} else if err != nil {
		resp.Diagnostics.AddError("Test failed", fmt.Sprintf("Test failed for ref %s, got error: %s\n%s", data.Digest.ValueString(), err, string(fullout)))
		return
	}

	// Write logs using the tflog package
	// Documentation: https://terraform.io/plugin/log
	tflog.Trace(ctx, "read a data source")

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func md5str(s string) string {
	h := md5.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

type positiveIntValidator struct{}

func (positiveIntValidator) MarkdownDescription(context.Context) string { return "positive integer" }
func (positiveIntValidator) Description(context.Context) string         { return "positive integer" }
func (positiveIntValidator) ValidateInt64(ctx context.Context, req validator.Int64Request, resp *validator.Int64Response) {
	if i := req.ConfigValue.ValueInt64(); i < 0 {
		resp.Diagnostics.AddAttributeError(req.Path, fmt.Sprintf("value %d must be a positive integer", i), "")
	}
}

var mu sync.Mutex
var freePorts = map[int]bool{}

func freePort() (int, error) {
	mu.Lock()
	defer mu.Unlock()

	for {
		addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
		if err != nil {
			return 0, err
		}

		l, err := net.ListenTCP("tcp", addr)
		if err != nil {
			return 0, err
		}
		defer l.Close()
		ta, ok := l.Addr().(*net.TCPAddr)
		if !ok {
			return 0, fmt.Errorf("failed to get port")
		}
		if freePorts[ta.Port] {
			tflog.Debug(context.Background(), "port already in use, trying again", map[string]interface{}{"port": ta.Port})
			continue
		}
		return ta.Port, nil
	}
}

func discardPort(port int) {
	mu.Lock()
	defer mu.Unlock()
	delete(freePorts, port)
}
