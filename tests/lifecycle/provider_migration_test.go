package lifecycle_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/shadeform/terraform-provider-shadeform/tests/lifecycle/internal/mockapi"
)

// Unlike the same-address schema tests, these scenarios install versioned local
// packages with init. No development overrides or registry connections are used.
func TestOpenTofuProviderMigration(t *testing.T) {
	var tofu string
	for _, engine := range lifecycleEngines(t) {
		if engine.name == "tofu" {
			tofu = engine.binary
		}
	}
	if tofu == "" {
		t.Skip("OpenTofu is required for provider-address migration tests")
	}
	legacyDir := buildLegacyProvider(t)
	currentDir := t.TempDir()
	command(t, ".", nil, "go", "build", "-buildvcs=false", "-o", filepath.Join(currentDir, "terraform-provider-shadeform"), "./fixture")
	for _, oldAddress := range []string{legacyProviderAddress, "registry.opentofu.org/shadeform/shadeform"} {
		t.Run(strings.Split(oldAddress, "/")[0], func(t *testing.T) {
			t.Parallel()
			s := newScenario(t, tofu, legacyDir, oldAddress)
			mirror := t.TempDir()
			mirrorPackage(t, mirror, oldAddress, "0.1.3", legacyDir)
			mirrorPackage(t, mirror, forkProviderAddress, "0.2.0", currentDir)
			cliConfig := fmt.Sprintf("provider_installation {\n filesystem_mirror {\n path = %q\n include = [\"*/*/*\"]\n }\n}\n", mirror)
			if err := os.WriteFile(filepath.Join(s.dir, "test.tfrc"), []byte(cliConfig), 0600); err != nil {
				t.Fatal(err)
			}
			migrationConfig(s, oldAddress, "0.1.3", "")
			s.cli("init", "-input=false", "-no-color")
			oldLock, oldHash := migrationLock(s, oldAddress, "0.1.3")
			assertProviderSelection(s, oldAddress, "0.1.3")
			s.apply("create") // State is written by the pinned old provider.
			s.edit(func(remote *mockapi.State) {
				remote.Instance["auto_delete"] = map[string]interface{}{"date_threshold": dateA, "spend_threshold": "0.50"}
			})
			s.cli("apply", "-refresh-only", "-auto-approve", "-input=false", "-no-color")
			before := s.rawInstanceState()
			for _, field := range []string{"auto_delete", "effective_auto_delete_date_threshold"} {
				if _, exists := before.Attributes[field]; exists {
					t.Fatalf("legacy state unexpectedly contains %s", field)
				}
			}
			id := before.Attributes["id"].(string)
			baseline := s.remote()
			// Discover the actual source in raw state, rather than migrate a shorthand
			// whose default hostname depends on the CLI that created the state.
			match := regexp.MustCompile(`provider\["([^"]+)"\]`).FindStringSubmatch(before.Provider)
			if len(match) != 2 || match[1] != oldAddress {
				t.Fatalf("unexpected old state provider: %q", before.Provider)
			}
			s.cli("state", "replace-provider", "-auto-approve", match[1], forkProviderAddress)
			migrated := s.rawInstanceState()
			if migrated.Provider != `provider["`+forkProviderAddress+`"]` || before.Lineage != migrated.Lineage ||
				before.ResourceCount != migrated.ResourceCount || before.SchemaVersion != migrated.SchemaVersion ||
				!reflect.DeepEqual(before.Attributes, migrated.Attributes) {
				t.Fatalf("replace-provider changed resources or failed to migrate the address: %#v", migrated)
			}
			unchangedLock, _ := migrationLock(s, oldAddress, "0.1.3")
			if !bytes.Equal(oldLock, unchangedLock) || !reflect.DeepEqual(baseline, s.remote()) {
				t.Fatal("state replace-provider changed the lock file or contacted the fixture API")
			}

			migrationConfig(s, forkProviderAddress, "0.2.0", "")
			s.cli("init", "-input=false", "-no-color")
			forkLock, forkHash := migrationLock(s, forkProviderAddress, "0.2.0")
			if oldHash == forkHash {
				t.Fatal("fork package reused the old binary checksum")
			}
			assertProviderSelection(s, forkProviderAddress, "0.2.0")
			providers := s.cli("providers")
			if strings.Contains(providers, oldAddress) || strings.Count(providers, forkProviderAddress) != 2 {
				t.Fatalf("configuration and state must both select the fork:\n%s", providers)
			}
			s.cli("init", "-input=false", "-no-color", "-lockfile=readonly")
			repeatLock, _ := migrationLock(s, forkProviderAddress, "0.2.0")
			if !bytes.Equal(forkLock, repeatLock) {
				t.Fatal("readonly init changed the fork lock file")
			}
			// An old binary under the fork address must fail the new package hash,
			// including when init would otherwise reuse the installed package.
			forkBinary := mirrorPackage(t, mirror, forkProviderAddress, "0.2.0", legacyDir)
			out, err := output(s.dir, s.env, s.engine, "init", "-input=false", "-no-color", "-lockfile=readonly")
			if err == nil || !strings.Contains(out, "checksum") {
				t.Fatalf("init accepted a different binary at %s: %v\n%s", forkBinary, err, out)
			}
			mirrorPackage(t, mirror, forkProviderAddress, "0.2.0", currentDir)
			s.cli("init", "-input=false", "-no-color", "-lockfile=readonly")

			s.cli("plan", "-refresh=false", "-detailed-exitcode", "-input=false", "-no-color")
			s.cli("apply", "-refresh-only", "-auto-approve", "-input=false", "-no-color")
			s.noChanges()
			s.assertState(false, nil, dateA)
			s.assertID(id)
			s.assertRemote(dateA, "0.50")
			after := s.rawInstanceState()
			if after.Lineage != before.Lineage || after.ResourceCount != before.ResourceCount || after.SchemaVersion != before.SchemaVersion {
				t.Fatal("migration changed state lineage, resource count, or schema version")
			}
			for _, field := range []string{"id", "name", "cloud", "region", "shade_instance_type", "shade_cloud", "ssh_key_id", "volume_ids", "template_id"} {
				if !reflect.DeepEqual(before.Attributes[field], after.Attributes[field]) {
					t.Fatalf("migration changed %s", field)
				}
			}
			assertMigrationMutations(s, baseline, 0)
			for i, step := range []struct {
				setting string
				date    interface{}
			}{
				{managed(dateB), dateB},
				{`auto_delete = { date_threshold = null }`, nil},
			} {
				migrationConfig(s, forkProviderAddress, "0.2.0", step.setting)
				s.apply("update")
				s.assertState(true, step.date, step.date)
				s.assertID(id)
				s.assertRemote(step.date, "0.50")
				s.noChanges()
				assertMigrationMutations(s, baseline, i+1)
			}
			s.destroy()
			remote := s.remote()
			if remote.Creates != baseline.Creates || remote.Updates != baseline.Updates+2 || remote.Deletes != baseline.Deletes+1 {
				t.Fatal("migration lifecycle performed unexpected API mutations")
			}
		})
	}
}

func migrationConfig(s *scenario, source, constraint, setting string) {
	s.t.Helper()
	config := fmt.Sprintf(`terraform {
  required_providers {
    shadeform = {
      source = %q
      version = %q
    }
  }
}
provider "shadeform" { api_key = "lifecycle-test-key" }
resource "shadeform_instance" "test" {
  cloud = "verda"
  region = "helsinki-finland-1"
  shade_instance_type = "cpu_4"
  shade_cloud = true
  name = "lifecycle-test"
  %s
}
`, source, constraint, setting)
	if err := os.WriteFile(filepath.Join(s.dir, "main.tf"), []byte(config), 0600); err != nil {
		s.t.Fatal(err)
	}
}

func mirrorPackage(t *testing.T, mirror, source, version, binaryDir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(binaryDir, "terraform-provider-shadeform"))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(mirror, filepath.FromSlash(source), version, runtime.GOOS+"_"+runtime.GOARCH)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "terraform-provider-shadeform_v"+version)
	// Replace the file atomically: an installed unpacked mirror package may be a
	// symlink, and overwriting an executable inode can fail with ETXTBSY.
	temporary := file + ".tmp"
	if err := os.WriteFile(temporary, data, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporary, file); err != nil {
		t.Fatal(err)
	}
	return file
}

func migrationLock(s *scenario, source, version string) ([]byte, string) {
	s.t.Helper()
	data, err := os.ReadFile(filepath.Join(s.dir, ".terraform.lock.hcl"))
	if err != nil {
		s.t.Fatal(err)
	}
	text := string(data)
	if strings.Count(text, "provider \"") != 1 || !strings.Contains(text, fmt.Sprintf("provider %q {", source)) ||
		!strings.Contains(text, fmt.Sprintf("version     = %q", version)) {
		s.t.Fatalf("lock file must select only %s v%s:\n%s", source, version, text)
	}
	hash := regexp.MustCompile(`"(h1:[^"]+)"`).FindStringSubmatch(text)
	if len(hash) != 2 {
		s.t.Fatalf("lock file has no package checksum:\n%s", text)
	}
	return data, hash[1]
}

func assertProviderSelection(s *scenario, source, version string) {
	s.t.Helper()
	var info struct {
		Selections map[string]string `json:"provider_selections"`
	}
	if err := json.Unmarshal([]byte(s.cli("version", "-json")), &info); err != nil {
		s.t.Fatal(err)
	}
	if len(info.Selections) != 1 || info.Selections[source] != version {
		s.t.Fatalf("provider selection = %#v, want %s v%s", info.Selections, source, version)
	}
}

func assertMigrationMutations(s *scenario, baseline mockapi.State, updates int) {
	s.t.Helper()
	remote := s.remote()
	if remote.Creates != baseline.Creates || remote.Updates != baseline.Updates+updates || remote.Deletes != baseline.Deletes {
		s.t.Fatalf("unexpected migration mutations: creates=%d updates=%d deletes=%d", remote.Creates, remote.Updates, remote.Deletes)
	}
}
