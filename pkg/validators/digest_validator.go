package provider

import (
	"context"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// DigestValidator is a string validator that checks that the string is valid OCI reference by digest.
type DigestValidator struct{}

var _ validator.String = DigestValidator{}

func (v DigestValidator) Description(context.Context) string {
	return "value must be a valid OCI digest"
}
func (v DigestValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v DigestValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	if _, err := name.NewDigest(val); err != nil {
		resp.Diagnostics.AddError("Invalid OCI digest", err.Error())
	}
}
