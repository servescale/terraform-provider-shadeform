package instance

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestTemplateAutoDeletePlanWarning(t *testing.T) {
	t.Parallel()
	date := payloadAutoDelete(types.StringValue("2030-01-02T15:04:05Z"))
	template := types.StringValue("external-template")
	for _, tc := range []struct {
		name       string
		template   types.String
		autoDelete types.Object
		existing   bool
		destroy    bool
		warn       bool
	}{
		{name: "create with date", template: template, autoDelete: date, warn: true},
		{name: "create with explicit clear or empty object", template: template, autoDelete: payloadAutoDelete(types.StringNull()), warn: true},
		{name: "create with unknown date", template: template, autoDelete: payloadAutoDelete(types.StringUnknown()), warn: true},
		{name: "create with unknown object defers warning", template: template, autoDelete: types.ObjectUnknown(autoDeleteAttributeTypes)},
		{name: "create with unknown template defers warning", template: types.StringUnknown(), autoDelete: date},
		{name: "create without template", autoDelete: date},
		{name: "create without override", template: template, autoDelete: payloadUnmanaged()},
		{name: "existing instance", template: template, autoDelete: date, existing: true},
		{name: "destroy", template: template, autoDelete: date, existing: true, destroy: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := &InstanceResource{}
			model := payloadModel(tc.autoDelete)
			model.TemplateId = tc.template
			plan := payloadPlan(t, r, model)
			state := tfsdk.State{Schema: plan.Schema, Raw: tftypes.NewValue(plan.Raw.Type(), nil)}
			if tc.existing {
				state.Raw = plan.Raw
			}
			if tc.destroy {
				plan.Raw = tftypes.NewValue(plan.Raw.Type(), nil)
			}
			resp := resource.ModifyPlanResponse{Plan: plan}
			r.ModifyPlan(context.Background(), resource.ModifyPlanRequest{Plan: plan, State: state}, &resp)
			payloadNoErrors(t, resp.Diagnostics)
			warnings := resp.Diagnostics.Warnings()
			if (len(warnings) > 0) != tc.warn || len(warnings) > 1 {
				t.Fatalf("warn=%v, got %v", tc.warn, resp.Diagnostics)
			}
			if tc.warn && warnings[0].Summary() != "Template spend threshold will not be inherited" {
				t.Fatalf("unexpected warning: %v", warnings)
			}
			if !resp.Plan.Raw.Equal(plan.Raw) {
				t.Fatal("warning changed the plan")
			}
		})
	}
}
