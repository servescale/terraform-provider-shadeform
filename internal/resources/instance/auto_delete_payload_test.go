package instance

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/shadeform/terraform-provider-shadeform/internal/provider/provider_shadeform"
)

// These tests invoke resource methods through the real HTTP/JSON client. The
// transport is entirely in memory: no real credentials, sockets, or allocations.
// Terraform planning and successful provisioning are outside this suite's scope.
func TestCreateAutoDeleteAPIPayload(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		autoDelete types.Object
		template   types.String
		wantJSON   string
	}{
		{
			name:       "deadline is in the original create request",
			autoDelete: payloadAutoDelete(types.StringValue("2030-01-02T15:04:05Z")),
			wantJSON:   `{"cloud":"verda","region":"helsinki-finland-1","shade_instance_type":"cpu_4","shade_cloud":true,"name":"payload-test","auto_delete":{"date_threshold":"2030-01-02T15:04:05Z"}}`,
		},
		{
			name:       "timezone offset is preserved on the wire",
			autoDelete: payloadAutoDelete(types.StringValue("2030-01-02T17:04:05+02:00")),
			wantJSON:   `{"cloud":"verda","region":"helsinki-finland-1","shade_instance_type":"cpu_4","shade_cloud":true,"name":"payload-test","auto_delete":{"date_threshold":"2030-01-02T17:04:05+02:00"}}`,
		},
		{
			name:       "omitted or null object is absent from JSON",
			autoDelete: payloadUnmanaged(),
			wantJSON:   `{"cloud":"verda","region":"helsinki-finland-1","shade_instance_type":"cpu_4","shade_cloud":true,"name":"payload-test"}`,
		},
		{
			name:       "explicit null date is not an omitted field or a null object",
			autoDelete: payloadAutoDelete(types.StringNull()),
			wantJSON:   `{"cloud":"verda","region":"helsinki-finland-1","shade_instance_type":"cpu_4","shade_cloud":true,"name":"payload-test","auto_delete":{"date_threshold":null}}`,
		},
		{
			name:       "template request leaves auto-delete omitted when unmanaged",
			autoDelete: payloadUnmanaged(), template: types.StringValue("template-test"),
			wantJSON: `{"cloud":"verda","region":"helsinki-finland-1","shade_instance_type":"cpu_4","shade_cloud":true,"name":"payload-test","template_id":"template-test"}`,
		},
		{
			name:       "template date override is sent in the original request",
			autoDelete: payloadAutoDelete(types.StringValue("2030-01-02T15:04:05Z")), template: types.StringValue("template-test"),
			wantJSON: `{"cloud":"verda","region":"helsinki-finland-1","shade_instance_type":"cpu_4","shade_cloud":true,"name":"payload-test","template_id":"template-test","auto_delete":{"date_threshold":"2030-01-02T15:04:05Z"}}`,
		},
		{
			name:       "explicit template override is sent in the original request",
			autoDelete: payloadAutoDelete(types.StringNull()), template: types.StringValue("template-test"),
			wantJSON: `{"cloud":"verda","region":"helsinki-finland-1","shade_instance_type":"cpu_4","shade_cloud":true,"name":"payload-test","template_id":"template-test","auto_delete":{"date_threshold":null}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Stop after the request is encoded, before allocation/readiness polling.
			// This proves the initial payload without testing provisioning failures.
			r := payloadResource(t, []payloadExchange{{
				method: http.MethodPost, path: "/v1/instances/create", wantJSON: tc.wantJSON,
				status: http.StatusServiceUnavailable, response: "payload captured; creation intentionally rejected",
			}})
			model := payloadModel(tc.autoDelete)
			model.TemplateId = tc.template
			plan := payloadPlan(t, r, model)
			resp := resource.CreateResponse{State: tfsdk.State{Schema: plan.Schema, Raw: plan.Raw}}
			r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &resp)
			errors := resp.Diagnostics.Errors()
			if len(errors) != 1 || errors[0].Summary() != "Error creating instance" ||
				!strings.Contains(errors[0].Detail(), "payload captured; creation intentionally rejected") {
				t.Fatalf("expected the deliberate API rejection, got %v", resp.Diagnostics)
			}
			wantWarnings := 0
			if !tc.template.IsNull() && !tc.autoDelete.IsNull() {
				wantWarnings = 1
			}
			if len(resp.Diagnostics.Warnings()) != wantWarnings {
				t.Fatalf("wanted %d template warnings, got %v", wantWarnings, resp.Diagnostics)
			}
		})
	}
}

func TestUpdateAutoDeleteAPIPayload(t *testing.T) {
	t.Parallel()
	oldDate := types.StringValue("2030-01-02T15:04:05Z")
	newDate := types.StringValue("2030-01-03T15:04:05Z")
	for _, tc := range []struct {
		name     string
		prior    types.Object
		planned  types.Object
		observed types.String
		readback types.String
		rename   bool
		wantJSON string // Empty means no POST at all, not an empty JSON object.
	}{
		{
			name:  "set a date on a previously unmanaged instance",
			prior: payloadUnmanaged(), planned: payloadAutoDelete(newDate), readback: newDate,
			wantJSON: `{"auto_delete":{"date_threshold":"2030-01-03T15:04:05Z"}}`,
		},
		{
			name:  "change only the date without sending the spend threshold",
			prior: payloadAutoDelete(oldDate), planned: payloadAutoDelete(newDate), observed: oldDate, readback: newDate,
			wantJSON: `{"auto_delete":{"date_threshold":"2030-01-03T15:04:05Z"}}`,
		},
		{
			name:  "clear only the date with explicit nested null",
			prior: payloadAutoDelete(oldDate), planned: payloadAutoDelete(types.StringNull()), observed: oldDate,
			wantJSON: `{"auto_delete":{"date_threshold":null}}`,
		},
		{
			name:  "explicit clearing also removes an unmanaged remote date",
			prior: payloadUnmanaged(), planned: payloadAutoDelete(types.StringNull()), observed: oldDate,
			wantJSON: `{"auto_delete":{"date_threshold":null}}`,
		},
		{
			name:  "omission during rename leaves the remote date alone",
			prior: payloadUnmanaged(), planned: payloadUnmanaged(), observed: oldDate, readback: oldDate, rename: true,
			wantJSON: `{"name":"renamed"}`,
		},
		{
			name:  "removing the object stops management without a POST",
			prior: payloadAutoDelete(oldDate), planned: payloadUnmanaged(), observed: oldDate, readback: oldDate,
		},
		{
			name:  "removing the object during rename does not clear",
			prior: payloadAutoDelete(oldDate), planned: payloadUnmanaged(), observed: oldDate, readback: oldDate, rename: true,
			wantJSON: `{"name":"renamed"}`,
		},
		{
			name:  "unchanged managed date is omitted during rename",
			prior: payloadAutoDelete(oldDate), planned: payloadAutoDelete(oldDate), observed: oldDate, readback: oldDate, rename: true,
			wantJSON: `{"name":"renamed"}`,
		},
		{
			name:  "equivalent offset does not resend the date",
			prior: payloadAutoDelete(oldDate), planned: payloadAutoDelete(types.StringValue("2030-01-02T17:04:05+02:00")), observed: oldDate, readback: oldDate, rename: true,
			wantJSON: `{"name":"renamed"}`,
		},
		{
			name:  "repeated clear produces no POST",
			prior: payloadAutoDelete(types.StringNull()), planned: payloadAutoDelete(types.StringNull()),
		},
		{
			name:  "rename after clearing does not rearm the deadline",
			prior: payloadAutoDelete(types.StringNull()), planned: payloadAutoDelete(types.StringNull()), rename: true,
			wantJSON: `{"name":"renamed"}`,
		},
		{
			name:  "send date and name together when both change",
			prior: payloadAutoDelete(oldDate), planned: payloadAutoDelete(newDate), observed: oldDate, readback: newDate, rename: true,
			wantJSON: `{"name":"renamed","auto_delete":{"date_threshold":"2030-01-03T15:04:05Z"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			prior := payloadModel(tc.prior)
			prior.Id = types.StringValue("instance-test")
			prior.EffectiveAutoDeleteDateThreshold = tc.observed
			planned := prior
			planned.AutoDelete = tc.planned
			planned.EffectiveAutoDeleteDateThreshold = types.StringUnknown()
			if tc.rename {
				planned.Name = types.StringValue("renamed")
			}
			var exchanges []payloadExchange
			if tc.wantJSON != "" {
				exchanges = append(exchanges, payloadExchange{
					method: http.MethodPost, path: "/v1/instances/instance-test/update", wantJSON: tc.wantJSON, status: http.StatusOK,
				})
			}
			exchanges = append(exchanges, payloadExchange{
				method: http.MethodGet, path: "/v1/instances/instance-test/info", status: http.StatusOK,
				response: payloadReadback(t, planned.Name.ValueString(), tc.readback),
			})
			r := payloadResource(t, exchanges)
			plan := payloadPlan(t, r, planned)
			statePlan := payloadPlan(t, r, prior)
			state := tfsdk.State{Schema: statePlan.Schema, Raw: statePlan.Raw}
			resp := resource.UpdateResponse{State: state}
			r.Update(context.Background(), resource.UpdateRequest{Plan: plan, State: state}, &resp)
			payloadNoErrors(t, resp.Diagnostics)
		})
	}
}

func TestAutoDeleteInvalidPayloadMakesNoRequest(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		value     types.Object
		wantError string
	}{
		{"unknown object", types.ObjectUnknown(map[string]attr.Type{"date_threshold": types.StringType}), "Unknown auto-delete configuration"},
		{"unknown date", payloadAutoDelete(types.StringUnknown()), "Unknown auto-delete date"},
		{"fractional seconds", payloadAutoDelete(types.StringValue("2030-01-02T15:04:05.123Z")), "Invalid auto-delete date"},
		{"invalid timestamp", payloadAutoDelete(types.StringValue("tomorrow")), "Invalid auto-delete date"},
	} {
		for _, operation := range []string{"create", "update"} {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				r := payloadResource(t, nil)
				model := payloadModel(tc.value)
				model.Name = types.StringValue("renamed") // Update must validate before sending even a name change.
				plan := payloadPlan(t, r, model)
				var diagnostics diag.Diagnostics
				if operation == "create" {
					resp := resource.CreateResponse{State: tfsdk.State{Schema: plan.Schema, Raw: plan.Raw}}
					r.Create(context.Background(), resource.CreateRequest{Plan: plan}, &resp)
					diagnostics = resp.Diagnostics
				} else {
					prior := payloadModel(payloadAutoDelete(types.StringValue("2030-01-02T15:04:05Z")))
					prior.Id = types.StringValue("instance-test")
					prior.EffectiveAutoDeleteDateThreshold = types.StringValue("2030-01-02T15:04:05Z")
					priorPlan := payloadPlan(t, r, prior)
					state := tfsdk.State{Schema: priorPlan.Schema, Raw: priorPlan.Raw}
					resp := resource.UpdateResponse{State: state}
					r.Update(context.Background(), resource.UpdateRequest{Plan: plan, State: state}, &resp)
					diagnostics = resp.Diagnostics
				}
				if !diagnostics.HasError() || len(diagnostics) != 1 || diagnostics[0].Summary() != tc.wantError {
					t.Fatalf("wanted %q, got %v", tc.wantError, diagnostics)
				}
			})
		}
	}
}

type payloadExchange struct {
	method   string
	path     string
	wantJSON string
	status   int
	response string
}

type payloadTransport func(*http.Request) (*http.Response, error)

func (f payloadTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func payloadResource(t *testing.T, exchanges []payloadExchange) *InstanceResource {
	t.Helper()
	next := 0
	transport := payloadTransport(func(req *http.Request) (*http.Response, error) {
		if next >= len(exchanges) {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
		want := exchanges[next]
		next++
		if req.Method != want.method || req.URL.Path != want.path {
			t.Fatalf("request %d: got %s %s, want %s %s", next, req.Method, req.URL.Path, want.method, want.path)
		}
		if req.URL.Scheme != "https" || req.URL.Host != "api.shadeform.ai" || req.URL.RawQuery != "" {
			t.Fatalf("unexpected request URL: %s", req.URL)
		}
		if req.Header.Get("X-API-KEY") != "payload-test-key" {
			t.Fatal("request did not use the injected test credential")
		}
		if want.wantJSON == "" {
			if req.Body != nil && req.Body != http.NoBody {
				t.Fatal("GET should not have a request body")
			}
		} else {
			if req.Header.Get("Content-Type") != "application/json" {
				t.Fatal("request content type is not application/json")
			}
			if req.Body == nil {
				t.Fatal("missing JSON request body")
			}
			body, err := io.ReadAll(req.Body)
			req.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			var got, expected interface{}
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("request is not valid JSON: %v", err)
			}
			if err := json.Unmarshal([]byte(want.wantJSON), &expected); err != nil {
				t.Fatalf("invalid expected JSON: %v", err)
			}
			// Full-object comparison catches extra keys (notably spend_threshold),
			// omitted nulls, whole-object nulls, and non-string date encodings.
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("request JSON:\n got: %s\nwant: %s", body, want.wantJSON)
			}
		}
		return &http.Response{StatusCode: want.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(want.response)), Request: req}, nil
	})
	t.Cleanup(func() {
		if next != len(exchanges) {
			t.Errorf("got %d requests, want %d", next, len(exchanges))
		}
	})
	return &InstanceResource{client: provider_shadeform.NewClientWithHTTPClient("payload-test-key", &http.Client{Transport: transport})}
}

func payloadAutoDelete(date types.String) types.Object {
	return types.ObjectValueMust(map[string]attr.Type{"date_threshold": types.StringType}, map[string]attr.Value{"date_threshold": date})
}

func payloadUnmanaged() types.Object {
	return types.ObjectNull(map[string]attr.Type{"date_threshold": types.StringType})
}

func payloadModel(autoDelete types.Object) InstanceResourceModel {
	return InstanceResourceModel{
		Cloud: types.StringValue("verda"), Region: types.StringValue("helsinki-finland-1"),
		ShadeInstanceType: types.StringValue("cpu_4"), ShadeCloud: types.BoolValue(true), Name: types.StringValue("payload-test"),
		AutoDelete: autoDelete, VolumeIds: types.ListNull(types.StringType),
		Timeouts: timeouts.Value{Object: types.ObjectNull(map[string]attr.Type{"create": types.StringType})},
	}
}

func payloadPlan(t *testing.T, r *InstanceResource, model InstanceResourceModel) tfsdk.Plan {
	t.Helper()
	ctx := context.Background()
	var resp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &resp)
	payloadNoErrors(t, resp.Diagnostics)
	plan := tfsdk.Plan{Schema: resp.Schema}
	payloadNoErrors(t, plan.Set(ctx, model))
	return plan
}

func payloadNoErrors(t *testing.T, diagnostics diag.Diagnostics) {
	t.Helper()
	if diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
}

func payloadReadback(t *testing.T, name string, date types.String) string {
	t.Helper()
	autoDelete := map[string]interface{}{"spend_threshold": "0.50"}
	if !date.IsNull() {
		autoDelete["date_threshold"] = date.ValueString()
	}
	data, err := json.Marshal(map[string]interface{}{
		"id": "instance-test", "name": name, "cloud": "verda", "region": "helsinki-finland-1",
		"shade_instance_type": "cpu_4", "shade_cloud": true, "status": "active", "auto_delete": autoDelete,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
