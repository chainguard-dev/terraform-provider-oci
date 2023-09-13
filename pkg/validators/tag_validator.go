package validators

import (
	"context"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// TagValidator is a string validator that checks that the string is valid OCI reference by digest.
type TagValidator struct{}

var _ validator.String = TagValidator{}

func (v TagValidator) Description(context.Context) string {
	return `value must be a valid OCI tag element (e.g., "latest", "v1.2.3")`
}
func (v TagValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v TagValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := name.NewTag("example.com:" + req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddError("Invalid OCI tag name", err.Error())
	}
}
