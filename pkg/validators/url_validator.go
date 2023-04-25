package provider

import (
	"context"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// URLValidator is a string validator that checks that the string is a valid URL.
type URLValidator struct{}

var _ validator.String = URLValidator{}

func (v URLValidator) Description(context.Context) string             { return "value must be a valid URL" }
func (v URLValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v URLValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()
	if _, err := url.Parse(val); err != nil {
		resp.Diagnostics.AddError("Invalid url", err.Error())
	}
}
