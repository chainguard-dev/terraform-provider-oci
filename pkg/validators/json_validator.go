package validators
import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// JSONValidator is a string validator that checks that the string is valid JSON.
type JSONValidator struct{}

var _ validator.String = JSONValidator{}

func (v JSONValidator) Description(context.Context) string             { return "value must be valid json" }
func (v JSONValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (v JSONValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	val := req.ConfigValue.ValueString()

	var untyped interface{}
	if err := json.Unmarshal([]byte(val), &untyped); err != nil {
		resp.Diagnostics.AddError("Invalid json", err.Error())
	}
}
