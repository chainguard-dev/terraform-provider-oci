package validators

import (
	"context"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// RefValidator is a string validator that checks that the string is a valid OCI reference.
type RefValidator struct{}

var _ validator.String = RefValidator{}

func (v RefValidator) Description(context.Context) string {
	return `value must be a valid OCI reference (e.g., "example.com/image:tag" or "example.com/image@sha256:abcdef...")`
}
func (v RefValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v RefValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	if _, err := name.ParseReference(val); err != nil {
		resp.Diagnostics.AddError("Invalid image reference", err.Error())
	}
}
