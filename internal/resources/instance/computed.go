package instance

import (
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Apply plans can mark every computed field unknown when a date changes. Fill
// them from readback on Update as well as Create and Read before saving state.
func refreshComputedFields(model *InstanceResourceModel, info map[string]interface{}) {
	fields := map[string]*types.String{
		"cloud_instance_type": &model.CloudInstanceType,
		"cloud_assigned_id":   &model.CloudAssignedID,
		"ip":                  &model.IP,
		"ssh_user":            &model.SshUser,
		"status":              &model.Status,
		"cost_estimate":       &model.CostEstimate,
		"created_at":          &model.CreatedAt,
	}
	for key, field := range fields {
		if value, ok := info[key].(string); ok {
			*field = types.StringValue(value)
		} else {
			*field = types.StringNull()
		}
	}
	switch value := info["ssh_port"].(type) {
	case float64: // encoding/json decodes JSON numbers as float64.
		model.SshPort = types.Int64Value(int64(value))
	case int64:
		model.SshPort = types.Int64Value(value)
	default:
		model.SshPort = types.Int64Null()
	}
	switch value := info["hourly_price"].(type) {
	case string:
		model.HourlyPrice = types.StringValue(value)
	case float64:
		model.HourlyPrice = types.StringValue(strconv.FormatFloat(value, 'f', -1, 64))
	default:
		model.HourlyPrice = types.StringNull()
	}
}
