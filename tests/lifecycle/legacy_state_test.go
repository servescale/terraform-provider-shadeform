package lifecycle_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/shadeform/terraform-provider-shadeform/tests/lifecycle/internal/mockapi"
)

// Pin the actual main revision from which issue #3 branched. Never manufacture
// old state by deleting fields from state written by the new provider.
const legacyRevision = "3c30a04ad5774aa586bb9f76b82c64995ace86ff"

func TestTerraformLegacyState(t *testing.T) {
	engines := lifecycleEngines(t)
	legacyDir := buildLegacyProvider(t)
	currentDir := t.TempDir()
	command(t, ".", nil, "go", "build", "-buildvcs=false", "-o", filepath.Join(currentDir, "terraform-provider-shadeform"), "./fixture")
	for _, engine := range engines {
		t.Run(engine.name, func(t *testing.T) {
			t.Parallel()
			for _, tc := range []struct {
				name     string
				date     interface{}
				spend    interface{}
				imported bool
			}{
				{name: "no_thresholds"},
				{name: "date_only", date: dateA},
				{name: "spend_only", spend: "0.50"},
				{name: "date_and_spend", date: dateA, spend: "0.50"},
				{name: "imported_date_and_spend", date: dateA, spend: "0.50", imported: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					s := newScenario(t, engine.binary, legacyDir)
					s.config("", "")
					if tc.imported {
						s.seed(dateA)
						s.cli("import", "-input=false", "-no-color", address, "imported-1")
					} else {
						s.apply("create") // The legacy binary creates and persists state.
						s.edit(func(remote *mockapi.State) {
							thresholds := map[string]interface{}{}
							if tc.date != nil {
								thresholds["date_threshold"] = tc.date
							}
							if tc.spend != nil {
								thresholds["spend_threshold"] = tc.spend
							}
							if len(thresholds) > 0 {
								remote.Instance["auto_delete"] = thresholds
							}
						})
					}
					s.noChanges() // Old Read ignores thresholds even when present remotely.
					before := s.rawInstanceState()
					for _, field := range []string{"auto_delete", "effective_auto_delete_date_threshold"} {
						if _, exists := before.Attributes[field]; exists {
							t.Fatalf("legacy state unexpectedly contains %s", field)
						}
					}
					id := before.Attributes["id"].(string)
					baseline := s.remote()
					s.useProvider(currentDir) // Preserve HCL, state, address, and remote fixture.
					// Decode legacy state without Read supplying the new attributes first.
					s.cli("plan", "-refresh=false", "-detailed-exitcode", "-input=false", "-no-color")
					s.cli("apply", "-refresh-only", "-auto-approve", "-input=false", "-no-color")
					s.assertState(false, nil, tc.date)
					s.assertID(id)
					s.assertRemote(tc.date, tc.spend)
					s.noChanges()
					after := s.rawInstanceState()
					if before.Lineage != after.Lineage || before.SchemaVersion != after.SchemaVersion {
						t.Fatal("upgrade changed lineage or required a schema version migration")
					}
					for _, field := range []string{"id", "name", "cloud", "region", "shade_instance_type", "shade_cloud", "ssh_key_id", "volume_ids", "template_id"} {
						if !reflect.DeepEqual(before.Attributes[field], after.Attributes[field]) {
							t.Fatalf("upgrade changed %s", field)
						}
					}
					if value, present := after.Attributes["auto_delete"]; !present || value != nil {
						t.Fatalf("upgraded configuration intent = %#v, present=%v", value, present)
					}
					if value, present := after.Attributes["effective_auto_delete_date_threshold"]; !present || value != tc.date {
						t.Fatalf("upgraded observed date = %#v, present=%v", value, present)
					}
					remote := s.remote()
					if baseline.Creates != remote.Creates || baseline.Updates != remote.Updates || baseline.Deletes != remote.Deletes {
						t.Fatal("upgrade mutated the remote instance")
					}

					// Explicitly opt into clearing after upgrade, then manage a new date.
					s.config(`auto_delete = { date_threshold = null }`, "")
					s.apply("update")
					s.assertState(true, nil, nil)
					s.assertRemote(nil, tc.spend)
					s.noChanges()
					s.config(managed(dateB), "")
					s.apply("update")
					s.assertState(true, dateB, dateB)
					s.assertRemote(dateB, tc.spend)
					s.assertID(id)
					s.noChanges()
					if s.remote().Creates != baseline.Creates {
						t.Fatal("management after upgrade replaced the instance")
					}
					s.destroy()
				})
			}
		})
	}
}

type rawInstanceState struct {
	Lineage       string
	SchemaVersion int
	Attributes    map[string]interface{}
}

func (s *scenario) rawInstanceState() rawInstanceState {
	s.t.Helper()
	var state struct {
		Lineage   string `json:"lineage"`
		Resources []struct {
			Type      string `json:"type"`
			Name      string `json:"name"`
			Instances []struct {
				SchemaVersion int                    `json:"schema_version"`
				Attributes    map[string]interface{} `json:"attributes"`
			} `json:"instances"`
		} `json:"resources"`
	}
	if err := json.Unmarshal([]byte(s.cli("state", "pull")), &state); err != nil {
		s.t.Fatal(err)
	}
	for _, r := range state.Resources {
		if r.Type == "shadeform_instance" && r.Name == "test" && len(r.Instances) == 1 {
			return rawInstanceState{state.Lineage, r.Instances[0].SchemaVersion, r.Instances[0].Attributes}
		}
	}
	s.t.Fatal("missing instance in raw state")
	return rawInstanceState{}
}

func buildLegacyProvider(t *testing.T) string {
	t.Helper()
	source := t.TempDir()
	archive := filepath.Join(t.TempDir(), "legacy.tar")
	command(t, "../..", nil, "git", "archive", "--format=tar", "--output", archive, legacyRevision, "go.mod", "go.sum", "internal")
	command(t, source, nil, "tar", "-xf", archive)
	// Copy only the test transport and entry point. Legacy production source,
	// dependencies, schema and CRUD implementations remain byte-for-byte intact.
	for _, name := range []string{"legacyfixture/main.go", "internal/mockapi/mock.go"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(source, "tests/lifecycle", name)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	command(t, source, nil, "go", "build", "-buildvcs=false", "-o", filepath.Join(dir, "terraform-provider-shadeform"), "./tests/lifecycle/legacyfixture")
	t.Logf("legacy provider built from %s with its original dependencies", legacyRevision)
	return dir
}
