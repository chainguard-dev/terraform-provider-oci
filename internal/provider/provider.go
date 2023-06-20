package provider

import (
	"context"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/v1/google"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var _ provider.Provider = &OCIProvider{}

// OCIProvider defines the provider implementation.
type OCIProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string

	defaultExecTimeoutSeconds int64
}

// OCIProviderModel describes the provider data model.
type OCIProviderModel struct {
	DefaultExecTimeoutSeconds *int64 `tfsdk:"default_exec_timeout_seconds"`
}

type ProviderOpts struct {
	ropts                     []remote.Option
	defaultExecTimeoutSeconds int64
}

func (p *ProviderOpts) withContext(ctx context.Context) []remote.Option {
	return append([]remote.Option{remote.WithContext(ctx)}, p.ropts...)
}

func (p *OCIProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "oci"
	resp.Version = p.version
}

func (p *OCIProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"default_exec_timeout_seconds": schema.Int64Attribute{
				MarkdownDescription: "Default timeout for exec tests",
				Optional:            true,
			},
		},
	}
}

func (p *OCIProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data OCIProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	kc := authn.NewMultiKeychain(google.Keychain, authn.DefaultKeychain)
	ropts := []remote.Option{remote.WithAuthFromKeychain(kc)}

	// These errors are impossible in current impl, but we can't return an err, so panic.
	puller, err := remote.NewPuller(ropts...)
	if err != nil {
		resp.Diagnostics.AddError("NewPuller", err.Error())
		return
	}

	pusher, err := remote.NewPusher(ropts...)
	if err != nil {
		resp.Diagnostics.AddError("NewPusher", err.Error())
		return
	}

	ropts = append(ropts, remote.Reuse(puller), remote.Reuse(pusher))

	opts := &ProviderOpts{
		ropts: ropts,
	}
	if p.defaultExecTimeoutSeconds != 0 {
		// This is only for testing, so we can inject provider config
		opts.defaultExecTimeoutSeconds = p.defaultExecTimeoutSeconds
	} else if data.DefaultExecTimeoutSeconds != nil {
		opts.defaultExecTimeoutSeconds = *data.DefaultExecTimeoutSeconds
	}

	resp.DataSourceData = opts
	resp.ResourceData = opts
}

func (p *OCIProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewAppendResource,
		NewTagResource,
	}
}

func (p *OCIProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewRefDataSource,
		NewStructureTestDataSource,
		NewExecTestDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &OCIProvider{
			version: version,
		}
	}
}
