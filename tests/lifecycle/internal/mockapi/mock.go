// Package mockapi supplies a file-backed API fixture for real CLI lifecycle tests.
// No request is ever forwarded to a network transport.
package mockapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type State struct {
	Instance map[string]interface{} `json:"instance"`
	Creates  int                    `json:"creates"`
	Updates  int                    `json:"updates"`
	Deletes  int                    `json:"deletes"`
	NotFound bool                   `json:"not_found"`
	// Failure injection and the original payload survive provider restarts.
	Failure            string                 `json:"failure,omitempty"`
	Reads              int                    `json:"reads"`
	CreatedAutoDelete  map[string]interface{} `json:"created_auto_delete,omitempty"`
	TemplateAutoDelete map[string]interface{} `json:"template_auto_delete,omitempty"`
}

func Load(file string) (State, error) {
	var state State
	b, err := os.ReadFile(file)
	if err != nil {
		return state, err
	}
	err = json.Unmarshal(b, &state)
	return state, err
}

func Save(file string, state State) error {
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(file, b, 0600)
}

type Transport struct{ File string }

func (t Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" || req.URL.Host != "api.shadeform.ai" || req.Header.Get("X-API-KEY") != "lifecycle-test-key" {
		return nil, fmt.Errorf("unexpected fixture request: %s %s", req.Method, req.URL)
	}
	state, err := Load(t.File)
	if err != nil {
		return nil, err
	}
	body := map[string]interface{}{}
	if req.Body != nil {
		defer req.Body.Close()
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			return nil, err
		}
	}
	var result interface{}
	code := http.StatusOK
	switch {
	case req.Method == http.MethodPost && req.URL.Path == "/v1/instances/create":
		if state.Instance != nil {
			return nil, fmt.Errorf("unexpected replacement of an existing fixture instance")
		}
		state.Creates++
		state.Reads = 0
		if autoDelete, ok := body["auto_delete"].(map[string]interface{}); ok {
			state.CreatedAutoDelete = make(map[string]interface{}, len(autoDelete))
			for key, value := range autoDelete {
				state.CreatedAutoDelete[key] = value
			}
		}
		state.NotFound = false
		state.Instance = NewInstance(fmt.Sprintf("allocated-%d", state.Creates), body["name"].(string), nil)
		for key, value := range body {
			state.Instance[key] = value
		}
		// Live template creation inherits the whole object only when omitted.
		// A supplied date/null object replaces it, unlike update's field merge.
		if _, supplied := body["auto_delete"]; !supplied && body["template_id"] == "template-test" && state.TemplateAutoDelete != nil {
			inherited := make(map[string]interface{}, len(state.TemplateAutoDelete))
			for key, value := range state.TemplateAutoDelete {
				inherited[key] = value
			}
			state.Instance["auto_delete"] = inherited
		}
		if autoDelete, ok := state.Instance["auto_delete"].(map[string]interface{}); ok {
			for key, value := range autoDelete {
				if value == nil {
					delete(autoDelete, key)
				}
			}
			if len(autoDelete) == 0 {
				delete(state.Instance, "auto_delete")
			}
		}
		result = map[string]interface{}{"id": state.Instance["id"], "status": "creating"}
	case req.Method == http.MethodGet && state.Instance != nil && req.URL.Path == "/v1/instances/"+state.Instance["id"].(string)+"/info":
		state.Reads++
		switch {
		case state.NotFound:
			code = http.StatusNotFound
		case state.Failure == "poll_http", state.Failure == "final_http" && state.Reads == 2:
			code = http.StatusServiceUnavailable
			result = map[string]interface{}{"error": "fixture read failure"}
		case state.Failure == "poll_error":
			result = map[string]interface{}{"status": "error"}
		default:
			result = state.Instance
		}
	case req.Method == http.MethodPost && state.Instance != nil && req.URL.Path == "/v1/instances/"+state.Instance["id"].(string)+"/update":
		state.Updates++
		for key, value := range body {
			if key == "auto_delete" {
				if value == nil {
					delete(state.Instance, key)
					continue
				}
				current, _ := state.Instance[key].(map[string]interface{})
				if current == nil {
					current = map[string]interface{}{}
				}
				for field, threshold := range value.(map[string]interface{}) {
					if threshold == nil {
						delete(current, field)
					} else {
						current[field] = threshold
					}
				}
				if len(current) == 0 {
					delete(state.Instance, key)
				} else {
					state.Instance[key] = current
				}
			} else {
				state.Instance[key] = value
			}
		}
	case req.Method == http.MethodPost && state.Instance != nil && req.URL.Path == "/v1/instances/"+state.Instance["id"].(string)+"/delete":
		state.Deletes++
		if state.Failure == "delete_http" {
			code = http.StatusServiceUnavailable
			result = map[string]interface{}{"error": "fixture cleanup failure"}
		} else {
			state.Instance = nil
		}
	case req.Method == http.MethodGet:
		code = http.StatusNotFound
	default:
		return nil, fmt.Errorf("unexpected fixture operation: %s %s", req.Method, req.URL.Path)
	}
	if err := Save(t.File, state); err != nil {
		return nil, err
	}
	b, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(b))), Request: req}, nil
}

func NewInstance(id, name string, autoDelete map[string]interface{}) map[string]interface{} {
	instance := map[string]interface{}{
		"id": id, "name": name, "cloud": "verda", "region": "helsinki-finland-1", "shade_instance_type": "cpu_4",
		"shade_cloud": true, "status": "active", "ssh_key_id": "test-key", "ssh_port": 22,
		"ssh_user": "root", "ip": "192.0.2.1", "cloud_instance_type": "CPU.4V.16G", "cloud_assigned_id": "cloud-test",
		"hourly_price": 12, "cost_estimate": "0", "created_at": "2030-01-01T00:00:00Z",
	}
	if autoDelete != nil {
		instance["auto_delete"] = autoDelete
	}
	return instance
}
