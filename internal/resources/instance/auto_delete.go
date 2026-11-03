package instance

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var autoDeleteAttributeTypes = map[string]attr.Type{
	"date_threshold": types.StringType,
}

var wholeSecondTimestamp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(Z|[+-]\d{2}:\d{2})$`)

type autoDeleteModel struct {
	DateThreshold types.String `tfsdk:"date_threshold"`
}

type autoDeleteDateValidator struct{}

var _ validator.String = autoDeleteDateValidator{}

func (autoDeleteDateValidator) Description(context.Context) string {
	return "An RFC3339 timestamp with a timezone and no fractional seconds."
}

func (v autoDeleteDateValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (autoDeleteDateValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := parseAutoDeleteDate(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid auto-delete date", err.Error())
	}
}

func parseAutoDeleteDate(value string) (time.Time, error) {
	timestamp, err := time.Parse(time.RFC3339, value)
	if err != nil || !wholeSecondTimestamp.MatchString(value) {
		return time.Time{}, fmt.Errorf("Use an RFC3339 timestamp with a timezone and no fractional seconds, such as 2030-01-02T15:04:05Z. Shadeform does not preserve fractional seconds")
	}
	// time.Parse accepts some out-of-range timezone offsets; require RFC3339's ranges.
	if value[len(value)-1] != 'Z' {
		offset := value[len(value)-5:]
		if offset[:2] > "23" || offset[3:] > "59" {
			return time.Time{}, fmt.Errorf("The RFC3339 timezone offset must have hours from 00 to 23 and minutes from 00 to 59")
		}
	}
	return timestamp, nil
}

// A null outer object is unmanaged. A known object with a null date means clear.
// Unknown values are never interpreted as either of these instructions.
func autoDeleteDate(ctx context.Context, value types.Object) (types.String, diag.Diagnostics) {
	var diags diag.Diagnostics
	if value.IsUnknown() {
		diags.AddAttributeError(path.Root("auto_delete"), "Unknown auto-delete configuration",
			"The auto_delete object must be known before creating or updating an instance.")
		return types.StringUnknown(), diags
	}
	if value.IsNull() {
		return types.StringNull(), diags
	}
	var model autoDeleteModel
	diags.Append(value.As(ctx, &model, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return types.StringUnknown(), diags
	}
	if model.DateThreshold.IsUnknown() {
		diags.AddAttributeError(path.Root("auto_delete").AtName("date_threshold"), "Unknown auto-delete date",
			"The date_threshold must be known before creating or updating an instance. Unknown does not mean clear.")
	} else if !model.DateThreshold.IsNull() {
		if _, err := parseAutoDeleteDate(model.DateThreshold.ValueString()); err != nil {
			diags.AddAttributeError(path.Root("auto_delete").AtName("date_threshold"), "Invalid auto-delete date", err.Error())
		}
	}
	return model.DateThreshold, diags
}

// This payload deliberately omits spend_threshold, including when clearing.
func autoDeletePayload(date types.String) map[string]interface{} {
	var value interface{}
	if !date.IsNull() {
		value = date.ValueString() // Validated before building the request.
	}
	return map[string]interface{}{"date_threshold": value}
}

func sameAutoDeleteDate(left, right types.String) bool {
	if left.IsUnknown() || right.IsUnknown() {
		return false
	}
	if left.IsNull() || right.IsNull() {
		return left.IsNull() && right.IsNull()
	}
	l, le := time.Parse(time.RFC3339Nano, left.ValueString())
	r, re := time.Parse(time.RFC3339Nano, right.ValueString())
	return le == nil && re == nil && l.Equal(r)
}

func remoteAutoDeleteDate(info map[string]interface{}) (types.String, error) {
	raw, present := info["auto_delete"]
	if !present || raw == nil {
		return types.StringNull(), nil
	}
	object, ok := raw.(map[string]interface{})
	if !ok {
		return types.StringNull(), fmt.Errorf("auto_delete must be an object or null")
	}
	rawDate, present := object["date_threshold"]
	if !present || rawDate == nil {
		return types.StringNull(), nil
	}
	value, ok := rawDate.(string)
	if !ok {
		return types.StringNull(), fmt.Errorf("auto_delete.date_threshold must be a timestamp string or null")
	}
	timestamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return types.StringNull(), fmt.Errorf("auto_delete.date_threshold is not a valid RFC3339 timestamp: %w", err)
	}
	return types.StringValue(timestamp.UTC().Format(time.RFC3339Nano)), nil
}

// Refresh always exposes the observed date. It only refreshes the managed object
// when that object already exists, so import and omission never adopt a deadline.
func refreshAutoDelete(ctx context.Context, model *InstanceResourceModel, info map[string]interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	remote, err := remoteAutoDeleteDate(info)
	if err != nil {
		diags.AddError("Invalid auto-delete response", err.Error())
		return diags
	}
	model.EffectiveAutoDeleteDateThreshold = remote
	if model.AutoDelete.IsNull() {
		return diags
	}
	// Prior state can contain drift observed from the service. Do not apply the
	// configuration's precision validator to that historical remote value.
	var prior autoDeleteModel
	diags.Append(model.AutoDelete.As(ctx, &prior, basetypes.ObjectAsOptions{})...)
	if diags.HasError() {
		return diags
	}
	// Preserve the user's spelling of an equivalent timestamp to avoid drift
	// caused only by the service converting a timezone offset to UTC.
	if sameAutoDeleteDate(prior.DateThreshold, remote) {
		return diags
	}
	model.AutoDelete = types.ObjectValueMust(autoDeleteAttributeTypes, map[string]attr.Value{"date_threshold": remote})
	return diags
}

// Apply must keep the configured value (including its null and timezone spelling)
// and report mismatches instead of accepting an ineffective set/clear operation.
func confirmAutoDelete(ctx context.Context, model *InstanceResourceModel, info map[string]interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	remote, err := remoteAutoDeleteDate(info)
	if err != nil {
		diags.AddError("Invalid auto-delete response", err.Error())
		return diags
	}
	model.EffectiveAutoDeleteDateThreshold = remote
	if model.AutoDelete.IsNull() {
		return diags
	}
	desired, valueDiags := autoDeleteDate(ctx, model.AutoDelete)
	diags.Append(valueDiags...)
	if !diags.HasError() && !sameAutoDeleteDate(desired, remote) {
		diags.AddAttributeError(path.Root("auto_delete"), "Auto-delete date did not match the request",
			fmt.Sprintf("Shadeform instance %s did not return the requested deletion date (or its requested absence). Inspect the instance and any template-provided deadline. The provider has not sent a follow-up request to clear it.", model.Id.ValueString()))
	}
	return diags
}
