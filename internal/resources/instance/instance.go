package instance

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/shadeform/terraform-provider-shadeform/internal/provider/provider_shadeform"
)

var (
	_ resource.Resource                = &InstanceResource{}
	_ resource.ResourceWithConfigure   = &InstanceResource{}
	_ resource.ResourceWithImportState = &InstanceResource{}
	_ resource.ResourceWithModifyPlan  = &InstanceResource{}
)

type InstanceResource struct {
	client *provider_shadeform.Client
}

type InstanceResourceModel struct {
	Id                               types.String   `tfsdk:"id"`
	Cloud                            types.String   `tfsdk:"cloud"`
	Region                           types.String   `tfsdk:"region"`
	ShadeInstanceType                types.String   `tfsdk:"shade_instance_type"`
	ShadeCloud                       types.Bool     `tfsdk:"shade_cloud"`
	Name                             types.String   `tfsdk:"name"`
	Os                               types.String   `tfsdk:"os"`
	SshKeyId                         types.String   `tfsdk:"ssh_key_id"`
	TemplateId                       types.String   `tfsdk:"template_id"`
	AutoDelete                       types.Object   `tfsdk:"auto_delete"`
	EffectiveAutoDeleteDateThreshold types.String   `tfsdk:"effective_auto_delete_date_threshold"`
	VolumeIds                        types.List     `tfsdk:"volume_ids"`
	CloudInstanceType                types.String   `tfsdk:"cloud_instance_type"`
	CloudAssignedID                  types.String   `tfsdk:"cloud_assigned_id"`
	IP                               types.String   `tfsdk:"ip"`
	SshUser                          types.String   `tfsdk:"ssh_user"`
	SshPort                          types.Int64    `tfsdk:"ssh_port"`
	Status                           types.String   `tfsdk:"status"`
	CostEstimate                     types.String   `tfsdk:"cost_estimate"`
	HourlyPrice                      types.String   `tfsdk:"hourly_price"`
	CreatedAt                        types.String   `tfsdk:"created_at"`
	Timeouts                         timeouts.Value `tfsdk:"timeouts"`
}

func NewInstanceResource() resource.Resource {
	return &InstanceResource{}
}

func (r *InstanceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_instance"
}

func (r *InstanceResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Shadeform instance.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier for the instance.",
				Computed:    true,
			},
			"cloud": schema.StringAttribute{
				Description: "The cloud provider.",
				Required:    true,
			},
			"region": schema.StringAttribute{
				Description: "The region where the instance will be deployed.",
				Required:    true,
			},
			"shade_instance_type": schema.StringAttribute{
				Description: "The Shadeform standardized instance type.",
				Required:    true,
			},
			"shade_cloud": schema.BoolAttribute{
				Description: "Whether to use Shade Cloud or linked cloud account. This is usually true.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the instance.",
				Required:    true,
			},
			"os": schema.StringAttribute{
				Description: "The operating system of the instance.",
				Optional:    true,
			},
			"ssh_key_id": schema.StringAttribute{
				Description: "The ID of the SSH key to use for this instance.",
				Optional:    true,
				Computed:    true,
			},
			"template_id": schema.StringAttribute{
				Description: "The ID of the template to use for this instance.",
				Optional:    true,
			},
			"auto_delete": schema.SingleNestedAttribute{
				Description: "Manage only the automatic deletion date. Omit this object (or set it to null) to leave the remote date unchanged. A present object with a null or omitted date_threshold explicitly clears the date and maintains its absence. Removing this object stops management without clearing the date. Date updates on existing instances preserve spend thresholds. At creation, supplying this object overrides the template's entire auto_delete object, so its spend threshold is not inherited.",
				Optional:    true,
				Attributes: map[string]schema.Attribute{
					"date_threshold": schema.StringAttribute{
						Description: "Automatic deletion deadline as an RFC3339 timestamp with a timezone and no fractional seconds. On existing instances, null explicitly clears only the date. At creation, supplying auto_delete overrides the template's deletion settings. Changes do not replace the instance.",
						Optional:    true,
						Validators:  []validator.String{autoDeleteDateValidator{}},
					},
				},
			},
			"effective_auto_delete_date_threshold": schema.StringAttribute{
				Description: "The deletion date currently reported by Shadeform, in UTC, including unmanaged and template-provided dates. Null when no date is set. This read-only value does not opt into managing the date.",
				Computed:    true,
			},
			"volume_ids": schema.ListAttribute{
				ElementType: types.StringType,
				Description: "List of volume IDs to be mounted. Currently only supports 1 volume at a time.",
				Optional:    true,
			},
			"cloud_instance_type": schema.StringAttribute{
				Description: "The type of the instance in the cloud provider.",
				Computed:    true,
			},
			"cloud_assigned_id": schema.StringAttribute{
				Description: "The ID of the instance in the cloud provider.",
				Computed:    true,
			},
			"ip": schema.StringAttribute{
				Description: "The IP address of the instance.",
				Computed:    true,
			},
			"ssh_user": schema.StringAttribute{
				Description: "The user to use for SSH access to the instance.",
				Computed:    true,
			},
			"ssh_port": schema.Int64Attribute{
				Description: "The port to use for SSH access to the instance.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "The status of the instance.",
				Computed:    true,
			},
			"cost_estimate": schema.StringAttribute{
				Description: "The cost estimate so far for the instance.",
				Computed:    true,
			},
			"hourly_price": schema.StringAttribute{
				Description: "The hourly price of the instance.",
				Computed:    true,
			},
			"created_at": schema.StringAttribute{
				Description: "The date and time the instance was created.",
				Computed:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
			}),
		},
	}
}

// Warn before creation so callers can see the template override in their plan.
func (r *InstanceResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || !req.State.Raw.IsNull() {
		return
	}
	var plan InstanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(templateAutoDeleteWarning(plan.TemplateId, plan.AutoDelete)...)
}

// Configure adds the provider configured client to the resource.
func (r *InstanceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*provider_shadeform.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *provider_shadeform.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

// Create creates the resource and sets the initial Terraform state.
func (r *InstanceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan InstanceResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build request body
	requestBody := map[string]interface{}{
		"cloud":               plan.Cloud.ValueString(),
		"region":              plan.Region.ValueString(),
		"shade_instance_type": plan.ShadeInstanceType.ValueString(),
		"name":                plan.Name.ValueString(),
	}

	if !plan.Os.IsNull() && !plan.Os.IsUnknown() {
		requestBody["os"] = plan.Os.ValueString()
	}

	if !plan.TemplateId.IsNull() && !plan.TemplateId.IsUnknown() {
		requestBody["template_id"] = plan.TemplateId.ValueString()
	}

	if !plan.SshKeyId.IsNull() && !plan.SshKeyId.IsUnknown() {
		requestBody["ssh_key_id"] = plan.SshKeyId.ValueString()
	}

	if !plan.ShadeCloud.IsNull() && !plan.ShadeCloud.IsUnknown() {
		requestBody["shade_cloud"] = plan.ShadeCloud.ValueBool()
	}

	// Add volume_ids if specified
	if !plan.VolumeIds.IsNull() && !plan.VolumeIds.IsUnknown() {
		var volumeIds []string
		diags := plan.VolumeIds.ElementsAs(ctx, &volumeIds, false)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		requestBody["volume_ids"] = volumeIds
	}

	date, dateDiags := autoDeleteDate(ctx, plan.AutoDelete)
	resp.Diagnostics.Append(dateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !plan.AutoDelete.IsNull() {
		requestBody["auto_delete"] = autoDeletePayload(date)
	}
	// Also warn when applying a saved plan or values that resolved only at apply.
	resp.Diagnostics.Append(templateAutoDeleteWarning(plan.TemplateId, plan.AutoDelete)...)

	// Include auto_delete in this original request, never in a post-create update.
	// Create instance
	result, err := r.client.CreateInstance(requestBody)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating instance",
			"Could not create instance, unexpected error: "+err.Error(),
		)
		return
	}

	// Extract instance ID from response
	instanceID, ok := result["id"].(string)
	if !ok {
		resp.Diagnostics.AddError(
			"Error creating instance",
			"Could not extract instance ID from response",
		)
		return
	}

	const defaultCreateTimeout = 60 * time.Minute

	createTimeout, diags := plan.Timeouts.Create(ctx, defaultCreateTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	if err := pollInstanceStatus(ctx, r.client, plan.Name.ValueString(), instanceID, 15*time.Second); err != nil {
		// If polling failed due to timeout, attempt to clean up the instance
		if ctx.Err() == context.DeadlineExceeded {
			tflog.Warn(ctx, fmt.Sprintf("Instance creation timed out, attempting to clean up instance %s", instanceID))

			// Attempt to delete the instance
			deleteErr := r.client.DeleteInstance(instanceID)
			if deleteErr != nil {
				// If deletion fails, return a comprehensive error
				resp.Diagnostics.AddError(
					"Instance creation failed and cleanup failed",
					fmt.Sprintf("Instance %s creation timed out and could not be deleted: %s. Manual cleanup may be required.", instanceID, deleteErr.Error()),
				)
				return
			}

			// If deletion succeeds, return a timeout error
			resp.Diagnostics.AddError(
				"Instance creation timed out",
				fmt.Sprintf("Instance %s creation timed out and was automatically deleted. Please try again or check your configuration.", instanceID),
			)
			return
		}

		// For other errors (like instance in error state), just return the original error
		resp.Diagnostics.AddError(
			"Instance not ready",
			fmt.Sprintf("failed waiting for %s to become active: %s", instanceID, err),
		)
		return
	}

	// Now fetch the full instance info to populate all computed fields
	instanceInfo, err := r.client.GetInstance(instanceID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading instance after create",
			"Could not read instance, unexpected error: "+err.Error(),
		)
		return
	}

	// Set all fields from the API response
	plan.Id = types.StringValue(instanceID)
	resp.Diagnostics.Append(confirmAutoDelete(ctx, &plan, instanceInfo)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cloud, ok := instanceInfo["cloud"].(string); ok {
		plan.Cloud = types.StringValue(cloud)
	}
	if region, ok := instanceInfo["region"].(string); ok {
		plan.Region = types.StringValue(region)
	}
	if shadeInstanceType, ok := instanceInfo["shade_instance_type"].(string); ok {
		plan.ShadeInstanceType = types.StringValue(shadeInstanceType)
	}
	if shadeCloud, ok := instanceInfo["shade_cloud"].(bool); ok {
		plan.ShadeCloud = types.BoolValue(shadeCloud)
	}
	if name, ok := instanceInfo["name"].(string); ok {
		plan.Name = types.StringValue(name)
	}

	// Handle optional fields that might be returned by the API
	if os, ok := instanceInfo["os"].(string); ok {
		plan.Os = types.StringValue(os)
	} else {
		plan.Os = types.StringNull()
	}

	if templateId, ok := instanceInfo["template_id"].(string); ok {
		plan.TemplateId = types.StringValue(templateId)
	} else {
		plan.TemplateId = types.StringNull()
	}

	// For ssh_key_id, preserve the original plan value if it was set
	// Only override if it wasn't in the original plan
	if !plan.SshKeyId.IsNull() && !plan.SshKeyId.IsUnknown() {
		// Keep the original plan value
	} else if sshKeyId, ok := instanceInfo["ssh_key_id"].(string); ok {
		plan.SshKeyId = types.StringValue(sshKeyId)
	} else {
		plan.SshKeyId = types.StringNull()
	}
	refreshComputedFields(&plan, instanceInfo)

	// Handle volume_ids - it's a list in the API response
	if volumeIdsRaw, ok := instanceInfo["volume_ids"]; ok && volumeIdsRaw != nil {
		if volumeIdsArray, ok := volumeIdsRaw.([]interface{}); ok {
			var volumeIds []attr.Value
			for _, v := range volumeIdsArray {
				if vStr, ok := v.(string); ok {
					volumeIds = append(volumeIds, types.StringValue(vStr))
				}
			}
			if len(volumeIds) > 0 {
				plan.VolumeIds = types.ListValueMust(types.StringType, volumeIds)
			} else {
				plan.VolumeIds = types.ListNull(types.StringType)
			}
		} else {
			plan.VolumeIds = types.ListNull(types.StringType)
		}
	} else {
		plan.VolumeIds = types.ListNull(types.StringType)
	}

	// Set state
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state with the latest data.
func (r *InstanceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Get current state
	var state InstanceResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get instance from API
	result, err := r.client.GetInstance(state.Id.ValueString())
	if err != nil {
		if provider_shadeform.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error reading instance",
			"Could not read instance, unexpected error: "+err.Error(),
		)
		return
	}
	if status, _ := result["status"].(string); status == "deleted" {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(refreshAutoDelete(ctx, &state, result)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Update state with API response
	if cloud, ok := result["cloud"].(string); ok {
		state.Cloud = types.StringValue(cloud)
	}
	if region, ok := result["region"].(string); ok {
		state.Region = types.StringValue(region)
	}
	if shadeInstanceType, ok := result["shade_instance_type"].(string); ok {
		state.ShadeInstanceType = types.StringValue(shadeInstanceType)
	}
	if shadeCloud, ok := result["shade_cloud"].(bool); ok {
		state.ShadeCloud = types.BoolValue(shadeCloud)
	}
	if name, ok := result["name"].(string); ok {
		state.Name = types.StringValue(name)
	}

	// Handle optional fields that might be returned by the API
	if os, ok := result["os"].(string); ok {
		state.Os = types.StringValue(os)
	} else {
		state.Os = types.StringNull()
	}

	if templateId, ok := result["template_id"].(string); ok {
		state.TemplateId = types.StringValue(templateId)
	} else {
		state.TemplateId = types.StringNull()
	}

	if sshKeyId, ok := result["ssh_key_id"].(string); ok {
		state.SshKeyId = types.StringValue(sshKeyId)
	} else {
		state.SshKeyId = types.StringNull()
	}
	refreshComputedFields(&state, result)

	// Handle volume_ids - it's a list in the API response
	if volumeIdsRaw, ok := result["volume_ids"]; ok && volumeIdsRaw != nil {
		if volumeIdsArray, ok := volumeIdsRaw.([]interface{}); ok {
			var volumeIds []attr.Value
			for _, v := range volumeIdsArray {
				if vStr, ok := v.(string); ok {
					volumeIds = append(volumeIds, types.StringValue(vStr))
				}
			}
			if len(volumeIds) > 0 {
				state.VolumeIds = types.ListValueMust(types.StringType, volumeIds)
			} else {
				state.VolumeIds = types.ListNull(types.StringType)
			}
		} else {
			state.VolumeIds = types.ListNull(types.StringType)
		}
	} else {
		state.VolumeIds = types.ListNull(types.StringType)
	}

	// Set state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *InstanceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Get plan and state
	var plan InstanceResourceModel
	var state InstanceResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Build request body for update
	requestBody := make(map[string]interface{})

	if !plan.Name.Equal(state.Name) {
		requestBody["name"] = plan.Name.ValueString()
	}
	date, dateDiags := autoDeleteDate(ctx, plan.AutoDelete)
	resp.Diagnostics.Append(dateDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !plan.AutoDelete.IsNull() && !sameAutoDeleteDate(date, state.EffectiveAutoDeleteDateThreshold) {
		requestBody["auto_delete"] = autoDeletePayload(date)
	}

	// Removing the configuration object stops management; no remote clear is sent.
	if len(requestBody) > 0 {
		if err := r.client.UpdateInstance(state.Id.ValueString(), requestBody); err != nil {
			resp.Diagnostics.AddError(
				"Error updating instance",
				"Could not update instance, unexpected error: "+err.Error(),
			)
			return
		}
	}

	// Fetch the updated instance data to ensure all computed fields are set
	instanceInfo, err := r.client.GetInstance(state.Id.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading instance after update",
			"Could not read instance, unexpected error: "+err.Error(),
		)
		return
	}

	// Update the plan with the fetched data
	plan.Id = types.StringValue(state.Id.ValueString())
	refreshComputedFields(&plan, instanceInfo)
	if plan.SshKeyId.IsUnknown() || plan.SshKeyId.IsNull() {
		if sshKeyId, ok := instanceInfo["ssh_key_id"].(string); ok {
			plan.SshKeyId = types.StringValue(sshKeyId)
		} else {
			plan.SshKeyId = types.StringNull()
		}
	}
	resp.Diagnostics.Append(confirmAutoDelete(ctx, &plan, instanceInfo)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cloud, ok := instanceInfo["cloud"].(string); ok {
		plan.Cloud = types.StringValue(cloud)
	}
	if region, ok := instanceInfo["region"].(string); ok {
		plan.Region = types.StringValue(region)
	}
	if shadeInstanceType, ok := instanceInfo["shade_instance_type"].(string); ok {
		plan.ShadeInstanceType = types.StringValue(shadeInstanceType)
	}
	if shadeCloud, ok := instanceInfo["shade_cloud"].(bool); ok {
		plan.ShadeCloud = types.BoolValue(shadeCloud)
	}
	if name, ok := instanceInfo["name"].(string); ok {
		plan.Name = types.StringValue(name)
	}

	// Handle optional fields that might be returned by the API
	if os, ok := instanceInfo["os"].(string); ok {
		plan.Os = types.StringValue(os)
	} else {
		plan.Os = types.StringNull()
	}

	if templateId, ok := instanceInfo["template_id"].(string); ok {
		plan.TemplateId = types.StringValue(templateId)
	} else {
		plan.TemplateId = types.StringNull()
	}

	// Handle volume_ids - it's a list in the API response
	if volumeIdsRaw, ok := instanceInfo["volume_ids"]; ok && volumeIdsRaw != nil {
		if volumeIdsArray, ok := volumeIdsRaw.([]interface{}); ok {
			var volumeIds []attr.Value
			for _, v := range volumeIdsArray {
				if vStr, ok := v.(string); ok {
					volumeIds = append(volumeIds, types.StringValue(vStr))
				}
			}
			if len(volumeIds) > 0 {
				plan.VolumeIds = types.ListValueMust(types.StringType, volumeIds)
			} else {
				plan.VolumeIds = types.ListNull(types.StringType)
			}
		} else {
			plan.VolumeIds = types.ListNull(types.StringType)
		}
	} else {
		plan.VolumeIds = types.ListNull(types.StringType)
	}

	// Set state
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *InstanceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Get state
	var state InstanceResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Delete instance
	err := r.client.DeleteInstance(state.Id.ValueString())
	if err != nil && !provider_shadeform.IsNotFound(err) {
		resp.Diagnostics.AddError(
			"Error deleting instance",
			"Could not delete instance, unexpected error: "+err.Error(),
		)
		return
	}
}

// ImportState imports the resource into Terraform state.
func (r *InstanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// pollInstanceStatus blocks until the instance reaches the wanted status or
// the ctx deadline is hit.
func pollInstanceStatus(
	ctx context.Context,
	c *provider_shadeform.Client,
	name string,
	id string,
	interval time.Duration,
) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err() // timeout or user ^C
		case <-ticker.C:
			info, err := c.GetInstance(id)
			if err != nil {
				return err // API error – abort
			}

			status, _ := info["status"].(string)
			tflog.Debug(ctx, fmt.Sprintf("instance [name: %s, id: %s] status=%s", name, id, status))

			if status == "active" {
				return nil // success
			} else if status == "error" {
				return fmt.Errorf("instance %s is in error state", id)
			}
		}
	}
}
