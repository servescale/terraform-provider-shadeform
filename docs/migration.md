# Migrate upstream state to the servescale provider

Use this procedure **after the reviewed fork release is published and installable**.
The proposed first release, v0.2.0, is still pending; see the
[distribution status](releases.md). The procedure changes provider ownership in
state without recreating the instance. It is separate from enabling date management.

## Identify the current provider

Use the existing working directory, backend, and workspace. Pause concurrent
applies for that workspace. Keep the original configuration and lock file available.

Pull a backup and inspect the full provider references, including resources in
child modules. The inspection commands below require `jq`:

```sh
tofu workspace show
tofu state pull > state-before-provider-migration.json
jq -r '.resources[] | [.module // "(root)", .type, .name, .provider] | @tsv' \
  state-before-provider-migration.json
tofu providers
```

For example, raw state may contain either
`provider["registry.terraform.io/shadeform/shadeform"]` or
`provider["registry.opentofu.org/shadeform/shadeform"]`, possibly with an alias
suffix. Use the full source inside the brackets as the old provider address.
Do not infer the hostname from a shorthand source or from the CLI now in use.
If both upstream addresses occur, migrate each one. Keep provider aliases and
local names in the configuration intact.

Before changing provider ownership, make the old source explicit in each affected
module if needed, matching the full address in state and retaining the existing
version constraint. This is particularly relevant when moving a configuration
from Terraform to OpenTofu: the same shorthand can resolve to a different registry.
Run `tofu init` with that original selection, then confirm a clean baseline with
`tofu plan -detailed-exitcode` (exit code 0). Resolve unrelated changes separately.

State backups can contain credentials and other sensitive values; keep them
outside version control. If `.terraform.lock.hcl` exists, save a copy alongside
the backup rather than deleting it.

## Replace the address and install the fork

Set `old_provider` to the address actually found above. This example uses the
Terraform registry hostname; substitute the OpenTofu hostname when that is what
the state records:

```sh
old_provider='registry.terraform.io/shadeform/shadeform'
tofu state replace-provider "$old_provider" registry.opentofu.org/servescale/shadeform
```

Review the proposed affected resources before confirming. The
[`state replace-provider` command](https://opentofu.org/docs/cli/commands/state/replace-provider/)
updates every resource using that source and writes a mandatory state backup.
Keep state locking enabled. The command changes state references; it does not
install the fork, change HCL, or transfer the old lock-file checksums to the fork.

Update `required_providers` in the root and any child modules that declare the
upstream source. For the reviewed v0.2.0 migration, pin the exact version:

```hcl
terraform {
  required_providers {
    shadeform = {
      source  = "registry.opentofu.org/servescale/shadeform"
      version = "0.2.0"
    }
  }
}
```

Update any constraints in child modules that would reject this version. Keep
existing resource addresses and configuration unchanged, and initially leave
`auto_delete` omitted. Do not add a clearing object as part of the address change.
Changing only the HCL source does not migrate existing state.

```sh
tofu init
tofu providers
tofu version -json
tofu init -lockfile=readonly
```

Normal `init` selects the fork as a new provider identity and updates
`.terraform.lock.hcl`. Both configuration and state in `tofu providers` must refer
to the fork, and `provider_selections` in the JSON version output must select
`registry.opentofu.org/servescale/shadeform` at `0.2.0`. Review the new lock block
and the release signing-key fingerprint. The old source's block is removed when
it is no longer required; unrelated providers retain their selections. Do not
rename an old lock block or copy its hashes: the fork is a different package.
The second, readonly initialization must succeed without changing the lock file.
See [OpenTofu's lock-file behavior](https://opentofu.org/docs/language/files/dependency-lock/).

Avoid `init -upgrade` or removing the whole lock file during this migration;
neither is needed to select the new address, and either can change unrelated
provider selections. Discard saved plans made before the migration.

## Verify preservation before managing dates

```sh
tofu plan -refresh=false -detailed-exitcode
tofu apply -refresh-only
tofu plan -detailed-exitcode
tofu state pull > state-after-provider-migration.json
```

Both plans should exit 0 with no resource changes. Review the refresh-only plan
before confirming; it may populate the new computed observation of a remote date.
Compare the backups: the instance ID, resource addresses, state lineage, and
existing configuration must remain unchanged; the provider reference must now
use the fork. Existing remote date and spend thresholds must remain unchanged.
An omitted `auto_delete` stays unmanaged even when the remote instance has a date.
Investigate any replacement, deletion, or unexpected update before applying it.

After confirming the exact instance ID is in durable state, opt into date
management separately, for example:

```hcl
auto_delete = {
  date_threshold = "2030-01-03T15:04:05Z"
}
```

The next plan should show an in-place update of the same instance. Applying it
preserves any spend threshold. To explicitly clear only the date, change the
configured date to `null` and apply again. Removing the entire object instead
stops management and leaves the remote date unchanged. An unchanged plan/apply
after either operation must be a no-op. See the
[date-management contract](resources/instance.md#automatic-deletion).

Commit the updated provider requirements and lock file together after successful
verification. Repeat the procedure for each backend/workspace that uses upstream
state; a state command changes only the selected state.

## Validation and limits

The [migration test](../tests/lifecycle/provider_migration_test.go) uses state
created by the pinned pre-feature provider and covers both upstream registry
hostnames. It installs local packages using an offline filesystem mirror, checks
provider selection and lock-file checksums, and exercises the same-instance date
updates and clearing with spend preservation. Package versions in that fixture
represent 0.1.3 and 0.2.0; they are not published release artifacts.

The test does not validate registry registration, signed release installation,
remote backends, every historical provider version, or downgrades. Published
distribution verification remains on the [release checklist](releases.md).
