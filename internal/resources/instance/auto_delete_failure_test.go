package instance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/shadeform/terraform-provider-shadeform/internal/provider/provider_shadeform"
)

// Faults are injected only after the simulated service has allocated the instance
// and saved the original create payload. Production readiness polling is used.
func TestCreateFailurePreservesAutoDelete(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		fault         string
		timeout       string
		wantError     string
		wantDetail    string
		wantReads     int
		wantDeletes   int
		wantAllocated bool
	}{
		{name: "missing ID after allocation", fault: "missing_id", wantError: "Error creating instance", wantDetail: "Could not extract instance ID", wantAllocated: true},
		{name: "malformed create response after allocation", fault: "create_json", wantError: "Error creating instance", wantDetail: "failed to unmarshal", wantAllocated: true},
		{name: "lost create response after allocation", fault: "create_network", wantError: "Error creating instance", wantDetail: "fixture lost create response", wantAllocated: true},
		{name: "invalid timeout after allocation", timeout: "invalid-duration", wantError: "Timeout Cannot Be Parsed", wantAllocated: true},
		{name: "cancellation after allocation", fault: "cancel", wantError: "Instance not ready", wantDetail: "context canceled", wantAllocated: true},
		{name: "polling HTTP error", fault: "poll_http", wantError: "Instance not ready", wantDetail: "status 503", wantReads: 1, wantAllocated: true},
		{name: "polling transport error", fault: "poll_network", wantError: "Instance not ready", wantDetail: "fixture polling transport failure", wantReads: 1, wantAllocated: true},
		{name: "polling malformed JSON", fault: "poll_json", wantError: "Instance not ready", wantDetail: "failed to unmarshal", wantReads: 1, wantAllocated: true},
		{name: "instance enters error state", fault: "poll_error", wantError: "Instance not ready", wantDetail: "is in error state", wantReads: 1, wantAllocated: true},
		{name: "final read HTTP error", fault: "final_http", wantError: "Error reading instance after create", wantDetail: "status 503", wantReads: 2, wantAllocated: true},
		{name: "final read malformed JSON", fault: "final_json", wantError: "Error reading instance after create", wantDetail: "failed to unmarshal", wantReads: 2, wantAllocated: true},
		{name: "final read malformed auto-delete object", fault: "final_object", wantError: "Invalid auto-delete response", wantReads: 2, wantAllocated: true},
		{name: "final read omits the requested deadline", fault: "final_missing_date", wantError: "Auto-delete date did not match the request", wantDetail: "allocated-failure-test", wantReads: 2, wantAllocated: true},
		{name: "timeout cleanup fails", fault: "delete_http", timeout: "1ms", wantError: "Instance creation failed and cleanup failed", wantDetail: "allocated-failure-test", wantDeletes: 1, wantAllocated: true},
		{name: "timeout cleanup succeeds", timeout: "1ms", wantError: "Instance creation timed out", wantDetail: "automatically deleted", wantDeletes: 1, wantAllocated: false},
		{name: "successful create also keeps its deadline", wantReads: 2, wantAllocated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			const deadline = "2030-01-02T15:04:05Z"
			var allocated bool
			var autoDelete map[string]interface{}
			var requests []string
			creates, reads, updates, deletes := 0, 0, 0, 0
			transport := payloadTransport(func(req *http.Request) (*http.Response, error) {
				requests = append(requests, req.Method+" "+req.URL.Path)
				response := func(code int, body string) (*http.Response, error) {
					return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
				}
				decode := func() map[string]interface{} {
					t.Helper()
					defer req.Body.Close()
					var body map[string]interface{}
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					return body
				}
				switch req.Method + " " + req.URL.Path {
				case "POST /v1/instances/create":
					creates++
					body := decode()
					var ok bool
					autoDelete, ok = body["auto_delete"].(map[string]interface{})
					if !ok || !reflect.DeepEqual(autoDelete, map[string]interface{}{"date_threshold": deadline}) {
						t.Fatalf("original create payload has no exact deadline: %#v", body)
					}
					allocated = true
					switch tc.fault {
					case "missing_id":
						return response(200, `{"status":"creating"}`)
					case "create_json":
						return response(200, `{`)
					case "create_network":
						return nil, fmt.Errorf("fixture lost create response")
					case "cancel":
						cancel()
					}
					return response(200, `{"id":"allocated-failure-test"}`)
				case "GET /v1/instances/allocated-failure-test/info":
					reads++
					if !allocated {
						t.Fatal("read after instance was deleted")
					}
					if reads == 1 {
						switch tc.fault {
						case "poll_http":
							return response(503, "fixture polling failure")
						case "poll_network":
							return nil, fmt.Errorf("fixture polling transport failure")
						case "poll_json":
							return response(200, `{`)
						case "poll_error":
							return response(200, `{"status":"error"}`)
						}
					}
					info := map[string]interface{}{"status": "active", "auto_delete": autoDelete, "shade_cloud": true, "ssh_key_id": "fixture-key"}
					if reads == 2 {
						switch tc.fault {
						case "final_http":
							return response(503, "fixture final read failure")
						case "final_json":
							return response(200, `{`)
						case "final_object":
							info["auto_delete"] = "invalid-object"
						case "final_missing_date":
							delete(info, "auto_delete")
						}
					}
					body, err := json.Marshal(info)
					if err != nil {
						t.Fatal(err)
					}
					return response(200, string(body))
				case "POST /v1/instances/allocated-failure-test/update":
					updates++
					body := decode()
					if value, present := body["auto_delete"]; present {
						if value == nil {
							autoDelete = nil
						} else {
							if autoDelete == nil {
								autoDelete = map[string]interface{}{}
							}
							for key, value := range value.(map[string]interface{}) {
								if value == nil {
									delete(autoDelete, key)
								} else {
									autoDelete[key] = value
								}
							}
						}
					}
					return response(200, "")
				case "POST /v1/instances/allocated-failure-test/delete":
					deletes++
					if autoDelete["date_threshold"] != deadline {
						t.Fatal("deadline was removed before cleanup")
					}
					if tc.fault == "delete_http" {
						return response(503, "fixture cleanup failure")
					}
					allocated = false
					return response(200, "")
				default:
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
					return nil, fmt.Errorf("unexpected request")
				}
			})
			r := &InstanceResource{client: provider_shadeform.NewClientWithHTTPClient("failure-test-key", &http.Client{Transport: transport})}
			model := payloadModel(payloadAutoDelete(types.StringValue(deadline)))
			if tc.timeout != "" {
				model.Timeouts = timeouts.Value{Object: types.ObjectValueMust(map[string]attr.Type{"create": types.StringType}, map[string]attr.Value{"create": types.StringValue(tc.timeout)})}
			}
			plan := payloadPlan(t, r, model)
			resp := resource.CreateResponse{State: tfsdk.State{Schema: plan.Schema, Raw: plan.Raw}}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
			if tc.wantError == "" {
				payloadNoErrors(t, resp.Diagnostics)
			} else if !resp.Diagnostics.HasError() || len(resp.Diagnostics) != 1 || resp.Diagnostics[0].Summary() != tc.wantError || !strings.Contains(resp.Diagnostics[0].Detail(), tc.wantDetail) {
				t.Fatalf("expected %q (%s), got %v", tc.wantError, tc.wantDetail, resp.Diagnostics)
			}
			if creates != 1 || reads != tc.wantReads || updates != 0 || deletes != tc.wantDeletes {
				t.Fatalf("unexpected operations: creates=%d reads=%d updates=%d deletes=%d; %v", creates, reads, updates, deletes, requests)
			}
			if allocated != tc.wantAllocated {
				t.Fatalf("allocated=%v, want %v", allocated, tc.wantAllocated)
			}
			if !reflect.DeepEqual(autoDelete, map[string]interface{}{"date_threshold": deadline}) {
				t.Fatalf("original protection changed: %#v", autoDelete)
			}
			// No assertion about early ID persistence: that recovery contract belongs
			// to issue #4. Here the invariant is the surviving allocation's deadline.
		})
	}
}
