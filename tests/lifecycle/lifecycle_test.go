package lifecycle_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shadeform/terraform-provider-shadeform/tests/lifecycle/internal/mockapi"
)

const address = "shadeform_instance.test"
const dateA = "2030-01-02T15:04:05Z"
const dateB = "2030-01-03T15:04:05Z"
const dateOffset = "2030-01-03T17:04:05+02:00"

func TestTerraformLifecycle(t *testing.T) {
	if os.Getenv("SHADEFORM_LIFECYCLE") != "1" {
		t.Skip("set SHADEFORM_LIFECYCLE=1 to run local CLI lifecycle tests")
	}
	type engine struct{ name, binary string }
	var engines []engine
	for _, name := range []string{"terraform", "tofu"} {
		binary := os.Getenv("SHADEFORM_LIFECYCLE_" + strings.ToUpper(name))
		if binary == "" {
			binary, _ = exec.LookPath(name)
		}
		if binary == "" {
			continue
		}
		absolute, err := filepath.Abs(binary)
		if err != nil {
			t.Fatal(err)
		}
		engines = append(engines, engine{name, absolute})
	}
	if len(engines) == 0 {
		t.Fatal("provide Terraform or OpenTofu on PATH or through SHADEFORM_LIFECYCLE_TERRAFORM / SHADEFORM_LIFECYCLE_TOFU")
	}
	fixtureDir := t.TempDir()
	fixture := filepath.Join(fixtureDir, "terraform-provider-shadeform")
	command(t, ".", nil, "go", "build", "-buildvcs=false", "-o", fixture, "./fixture")
	for _, e := range engines {
		t.Run(e.name, func(t *testing.T) {
			t.Parallel()
			t.Log(strings.TrimSpace(command(t, ".", cleanEnv(), e.binary, "version")))
			for name, exercise := range map[string]func(*scenario){
				"manage_clear_drift_and_stop": manageLifecycle,
				"import_unmanaged":            importLifecycle,
				"create_omitted":              func(s *scenario) { createWithoutDate(s, "") },
				"create_explicit_null":        func(s *scenario) { createWithoutDate(s, `auto_delete = { date_threshold = null }`) },
				"unknown_date":                func(s *scenario) { unknownLifecycle(s, false) },
				"unknown_object":              func(s *scenario) { unknownLifecycle(s, true) },
				"validation":                  validationLifecycle,
				"deleted_status":              func(s *scenario) { deletedLifecycle(s, false) },
				"not_found":                   func(s *scenario) { deletedLifecycle(s, true) },
				"failure_poll_http":           func(s *scenario) { failureLifecycle(s, "poll_http", false, "Instance not ready", 1, false) },
				"failure_poll_error":          func(s *scenario) { failureLifecycle(s, "poll_error", false, "Instance not ready", 1, false) },
				"failure_final_read": func(s *scenario) {
					failureLifecycle(s, "final_http", false, "Error reading instance after create", 2, false)
				},
				"failure_timeout_cleanup": func(s *scenario) {
					failureLifecycle(s, "delete_http", true, "Instance creation failed and cleanup failed", 0, false)
				},
				"timeout_cleanup_succeeds": func(s *scenario) { failureLifecycle(s, "", true, "Instance creation timed out", 0, true) },
			} {
				t.Run(name, func(t *testing.T) { t.Parallel(); exercise(newScenario(t, e.binary, fixtureDir)) })
			}
		})
	}
}

// Check the service-side allocation after a real failed CLI apply. Early state
// persistence and recovery belong to issue #4, not this deadline invariant.
func failureLifecycle(s *scenario, fault string, timeout bool, diagnostic string, wantReads int, deleted bool) {
	s.edit(func(remote *mockapi.State) { remote.Failure = fault })
	setting := managed(dateA)
	wantDeletes := 0
	if timeout {
		setting += "\ntimeouts { create = \"1ms\" }"
		wantDeletes = 1
	}
	s.config(setting, "")
	s.plan("create")
	out, err := output(s.dir, s.env, s.engine, "apply", "-input=false", "-no-color", "plan.out")
	if err == nil || !strings.Contains(out, "Error: "+diagnostic) {
		s.t.Fatalf("expected failed apply with %q: %v\n%s", diagnostic, err, out)
	}
	remote := s.remote()
	if remote.Creates != 1 || remote.Reads != wantReads || remote.Updates != 0 || remote.Deletes != wantDeletes {
		s.t.Fatalf("unexpected operations after failed apply: creates=%d reads=%d updates=%d deletes=%d", remote.Creates, remote.Reads, remote.Updates, remote.Deletes)
	}
	if !reflect.DeepEqual(remote.CreatedAutoDelete, map[string]interface{}{"date_threshold": dateA}) {
		s.t.Fatalf("original create did not contain the exact deadline: %#v", remote.CreatedAutoDelete)
	}
	if deleted {
		if remote.Instance != nil {
			s.t.Fatal("successful timeout cleanup left an allocation")
		}
	} else {
		if remote.Instance == nil || remote.Instance["id"] != "allocated-1" {
			s.t.Fatal("expected the original allocation to survive the failure")
		}
		s.assertRemote(dateA, nil)
	}
}

func manageLifecycle(s *scenario) {
	s.config(managed(dateA), "")
	s.apply("create")
	id := s.assertState(true, dateA, dateA)
	s.assertRemote(dateA, nil)
	s.noChanges()

	// External spend protection must survive every date-only operation.
	s.edit(func(remote *mockapi.State) {
		remote.Instance["auto_delete"].(map[string]interface{})["spend_threshold"] = "0.50"
	})
	s.config(managed(dateB), "")
	s.apply("update")
	s.assertID(id)
	s.assertState(true, dateB, dateB)
	s.assertRemote(dateB, "0.50")
	s.noChanges()

	// Offset-only configuration changes may update state, never the remote date.
	updates := s.remote().Updates
	s.config(managed(dateOffset), "")
	s.apply("update")
	s.assertState(true, dateOffset, dateB)
	if s.remote().Updates != updates {
		s.t.Fatal("equivalent timezone resent the date")
	}
	s.noChanges()

	// Managed date drift is corrected on the same instance.
	s.setRemoteDate(dateA)
	s.apply("update")
	s.assertState(true, dateOffset, dateB)
	s.assertRemote(dateOffset, "0.50")
	s.assertID(id)
	s.noChanges()

	s.config(`auto_delete = { date_threshold = null }`, "")
	s.apply("update")
	s.assertState(true, nil, nil)
	s.assertRemote(nil, "0.50")
	s.noChanges()

	// Empty HCL objects and explicit null dates have the same persistent intent.
	s.config(`auto_delete = {}`, "")
	s.noChanges()
	s.setRemoteDate(dateA)
	s.apply("update")
	s.assertState(true, nil, nil)
	s.assertRemote(nil, "0.50")
	s.noChanges()

	s.config(managed(dateA), "")
	s.apply("update")
	updates = s.remote().Updates
	s.config("", "")
	s.apply("update")
	s.assertState(false, nil, dateA)
	s.assertRemote(dateA, "0.50")
	if s.remote().Updates != updates {
		s.t.Fatal("stopping management changed the API")
	}
	s.setRemoteDate(dateB)
	s.noChanges() // Refresh observes unmanaged drift without proposing a correction.
	s.cli("apply", "-refresh-only", "-auto-approve", "-input=false", "-no-color")
	s.assertState(false, nil, dateB)
	s.config(`auto_delete = null`, "")
	s.noChanges()
	s.assertID(id)
	if s.remote().Creates != 1 {
		s.t.Fatal("date operations replaced the instance")
	}
	s.destroy()
}

func importLifecycle(s *scenario) {
	s.seed(dateA)
	s.config("", "")
	s.cli("import", "-input=false", "-no-color", address, "imported-1")
	s.assertState(false, nil, dateA)
	s.noChanges()
	s.assertRemote(dateA, "0.50")
	if s.remote().Creates != 0 || s.remote().Updates != 0 {
		s.t.Fatal("import mutated remote settings")
	}
	s.config(`auto_delete = { date_threshold = null }`, "")
	s.apply("update")
	s.assertState(true, nil, nil)
	s.assertRemote(nil, "0.50")
	s.assertID("imported-1")
	s.noChanges()
	s.destroy()
}

func createWithoutDate(s *scenario, setting string) {
	s.config(setting, "")
	s.apply("create")
	s.assertState(setting != "", nil, nil)
	s.noChanges()
	s.destroy()
}

func unknownLifecycle(s *scenario, wholeObject bool) {
	setting := `auto_delete = { date_threshold = terraform_data.deadline.output }`
	extra := `resource "terraform_data" "deadline" { input = "` + dateA + `" }`
	if wholeObject {
		setting = `auto_delete = terraform_data.deadline.output ? { date_threshold = "` + dateA + `" } : null`
		extra = `resource "terraform_data" "deadline" { input = true }`
	}
	s.config(setting, extra)
	change := s.plan("create")
	unknown := change.AfterUnknown["auto_delete"]
	if wholeObject {
		if unknown != true {
			s.t.Fatalf("expected unknown object, got %#v", unknown)
		}
	} else {
		object, ok := unknown.(map[string]interface{})
		if !ok || object["date_threshold"] != true {
			s.t.Fatalf("expected unknown date, got %#v", unknown)
		}
	}
	if s.remote().Creates != 0 {
		s.t.Fatal("planning unknown configuration made an API mutation")
	}
	s.cli("apply", "-input=false", "-no-color", "plan.out")
	s.assertState(true, dateA, dateA)
	s.assertRemote(dateA, nil)
	s.noChanges()
	s.destroy()
}

func validationLifecycle(s *scenario) {
	for _, value := range []string{"tomorrow", "2030-01-02T15:04:05.123Z", "2030-01-02T15:04:05+24:00"} {
		s.config(managed(value), "")
		out, err := output(s.dir, s.env, s.engine, "validate", "-no-color")
		if err == nil || !strings.Contains(out, "Invalid auto-delete date") {
			s.t.Fatalf("expected validation error for %q: %v\n%s", value, err, out)
		}
	}
	if remote := s.remote(); remote.Creates != 0 || remote.Updates != 0 {
		s.t.Fatal("validation mutated the API")
	}
}

func deletedLifecycle(s *scenario, notFound bool) {
	s.seed(dateA)
	s.config("", "")
	s.cli("import", "-input=false", "-no-color", address, "imported-1")
	s.edit(func(remote *mockapi.State) { remote.NotFound = notFound; remote.Instance["status"] = "deleted" })
	s.plan("create")
	s.cli("apply", "-refresh-only", "-auto-approve", "-input=false", "-no-color")
	if s.values() != nil {
		s.t.Fatal("deleted instance remains in persisted state")
	}
	if remote := s.remote(); remote.Creates != 0 || remote.Updates != 0 || remote.Deletes != 0 {
		s.t.Fatal("refresh of deleted instance performed mutations")
	}
}

type scenario struct {
	t                 *testing.T
	engine, dir, file string
	env               []string
}
type change struct {
	Actions      []string               `json:"actions"`
	AfterUnknown map[string]interface{} `json:"after_unknown"`
}

func newScenario(t *testing.T, engine, fixtureDir string) *scenario {
	t.Helper()
	dir := t.TempDir()
	s := &scenario{t: t, engine: engine, dir: dir, file: filepath.Join(dir, "api.json")}
	if err := mockapi.Save(s.file, mockapi.State{}); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf("provider_installation {\n dev_overrides {\n  \"registry.terraform.io/shadeform/shadeform\" = %q\n }\n direct { exclude = [\"registry.terraform.io/shadeform/shadeform\"] }\n}\n", fixtureDir)
	cliConfig := filepath.Join(dir, "test.tfrc")
	if err := os.WriteFile(cliConfig, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	s.env = append(cleanEnv(), "TF_CLI_CONFIG_FILE="+cliConfig, "SHADEFORM_LIFECYCLE_STATE="+s.file)
	return s
}

func managed(date string) string { return fmt.Sprintf("auto_delete = { date_threshold = %q }", date) }

func (s *scenario) config(setting, extra string) {
	s.t.Helper()
	config := `terraform {
  required_providers {
    shadeform = { source = "registry.terraform.io/shadeform/shadeform" }
  }
}
provider "shadeform" { api_key = "lifecycle-test-key" }
resource "shadeform_instance" "test" {
  cloud = "verda"
  region = "helsinki-finland-1"
  shade_instance_type = "cpu_4"
  shade_cloud = true
  name = "lifecycle-test"
  ` + setting + `
}
` + extra + "\n"
	if err := os.WriteFile(filepath.Join(s.dir, "main.tf"), []byte(config), 0600); err != nil {
		s.t.Fatal(err)
	}
}

func (s *scenario) cli(args ...string) string {
	s.t.Helper()
	return command(s.t, s.dir, s.env, s.engine, args...)
}
func (s *scenario) plan(actions ...string) change {
	s.t.Helper()
	s.cli("plan", "-input=false", "-no-color", "-out=plan.out")
	var document struct {
		ResourceChanges []struct {
			Address string `json:"address"`
			Change  change `json:"change"`
		} `json:"resource_changes"`
	}
	if err := json.Unmarshal([]byte(s.cli("show", "-json", "plan.out")), &document); err != nil {
		s.t.Fatal(err)
	}
	for _, resource := range document.ResourceChanges {
		if resource.Address == address {
			if !reflect.DeepEqual(resource.Change.Actions, actions) {
				s.t.Fatalf("plan actions = %v, want %v", resource.Change.Actions, actions)
			}
			return resource.Change
		}
	}
	s.t.Fatal("instance missing from plan resource_changes")
	return change{}
}
func (s *scenario) apply(actions ...string) {
	s.t.Helper()
	s.plan(actions...)
	s.cli("apply", "-input=false", "-no-color", "plan.out")
}
func (s *scenario) noChanges() {
	s.t.Helper()
	before := s.remote()
	s.apply("no-op")
	after := s.remote()
	if before.Creates != after.Creates || before.Updates != after.Updates || before.Deletes != after.Deletes {
		s.t.Fatal("unchanged apply mutated the remote instance")
	}
}
func (s *scenario) values() map[string]interface{} {
	s.t.Helper()
	var state struct {
		Values struct {
			RootModule struct {
				Resources []struct {
					Address string                 `json:"address"`
					Values  map[string]interface{} `json:"values"`
				} `json:"resources"`
			} `json:"root_module"`
		} `json:"values"`
	}
	if err := json.Unmarshal([]byte(s.cli("show", "-json")), &state); err != nil {
		s.t.Fatal(err)
	}
	for _, r := range state.Values.RootModule.Resources {
		if r.Address == address {
			return r.Values
		}
	}
	return nil
}
func (s *scenario) assertState(managed bool, date, effective interface{}) string {
	s.t.Helper()
	v := s.values()
	if v == nil {
		s.t.Fatal("missing persisted instance state")
	}
	if managed {
		object, ok := v["auto_delete"].(map[string]interface{})
		if !ok || object["date_threshold"] != date {
			s.t.Fatalf("managed date state = %#v, want %v", v["auto_delete"], date)
		}
	} else if v["auto_delete"] != nil {
		s.t.Fatalf("omitted object adopted remote state: %#v", v["auto_delete"])
	}
	if got := v["effective_auto_delete_date_threshold"]; got != effective {
		s.t.Fatalf("observed date = %#v, want %#v", got, effective)
	}
	id, ok := v["id"].(string)
	if !ok || id == "" {
		s.t.Fatal("missing instance ID")
	}
	return id
}
func (s *scenario) assertID(id string) {
	s.t.Helper()
	if got := s.values()["id"]; got != id {
		s.t.Fatalf("instance ID changed: %v -> %v", id, got)
	}
}
func (s *scenario) assertRemote(date, spend interface{}) {
	s.t.Helper()
	remote := s.remote()
	object, _ := remote.Instance["auto_delete"].(map[string]interface{})
	if object["date_threshold"] != date || object["spend_threshold"] != spend {
		s.t.Fatalf("remote thresholds = %#v, want date=%v spend=%v", object, date, spend)
	}
}
func (s *scenario) remote() mockapi.State {
	s.t.Helper()
	state, err := mockapi.Load(s.file)
	if err != nil {
		s.t.Fatal(err)
	}
	return state
}
func (s *scenario) edit(f func(*mockapi.State)) {
	s.t.Helper()
	state := s.remote()
	f(&state)
	if err := mockapi.Save(s.file, state); err != nil {
		s.t.Fatal(err)
	}
}
func (s *scenario) setRemoteDate(date string) {
	s.edit(func(state *mockapi.State) {
		object, _ := state.Instance["auto_delete"].(map[string]interface{})
		if object == nil {
			object = map[string]interface{}{}
		}
		object["date_threshold"] = date
		state.Instance["auto_delete"] = object
	})
}
func (s *scenario) seed(date string) {
	s.edit(func(state *mockapi.State) {
		state.Instance = mockapi.NewInstance("imported-1", "lifecycle-test", map[string]interface{}{"date_threshold": date, "spend_threshold": "0.50"})
	})
}
func (s *scenario) destroy() {
	s.t.Helper()
	s.cli("destroy", "-auto-approve", "-input=false", "-no-color")
	if s.remote().Instance != nil || s.values() != nil {
		s.t.Fatal("destroy left an instance in the API or persisted state")
	}
}

func cleanEnv() []string {
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(key, "TF_") && !strings.HasPrefix(key, "TOFU_") && !strings.HasPrefix(key, "SHADEFORM_") && key != "CHECKPOINT_DISABLE" {
			env = append(env, entry)
		}
	}
	return append(env, "TF_IN_AUTOMATION=1", "TF_INPUT=0", "TF_LOG=OFF", "CHECKPOINT_DISABLE=1")
}
func output(dir string, env []string, binary string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	b, err := cmd.CombinedOutput()
	return string(b), err
}
func command(t *testing.T, dir string, env []string, binary string, args ...string) string {
	t.Helper()
	out, err := output(dir, env, binary, args...)
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", binary, strings.Join(args, " "), err, out)
	}
	return out
}
