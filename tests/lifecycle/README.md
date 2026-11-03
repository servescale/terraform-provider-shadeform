# Instance lifecycle tests

These tests run real Terraform/OpenTofu CLI commands against the production provider's schema and resource methods. A test-only provider wrapper changes only the API transport to a file-backed fixture. The fixture never forwards HTTP requests, and the test processes use a synthetic API key. No Shadeform account, cloud allocation, registry download, or live API connection is required.

## Running

Install Terraform and/or OpenTofu, then enable the suite explicitly:

```sh
SHADEFORM_LIFECYCLE=1 go test -buildvcs=false ./tests/lifecycle -v -count=1
```

The suite finds `terraform` and `tofu` on `PATH` and runs each available CLI. You can select binaries explicitly:

```sh
SHADEFORM_LIFECYCLE=1 \
SHADEFORM_LIFECYCLE_TERRAFORM=/absolute/path/to/terraform \
SHADEFORM_LIFECYCLE_TOFU=/absolute/path/to/tofu \
go test -buildvcs=false ./tests/lifecycle -v -count=1
```

Without `SHADEFORM_LIFECYCLE=1`, the suite is skipped. When enabled, having no available CLI is an error. Explicit executable paths must exist. Versions are logged; the suite has passed with Terraform 1.14.4 and OpenTofu 1.13.1 on Linux amd64.

The test builds a provider executable in a temporary directory and uses a private CLI configuration with a development override. It does not run `init` or change the user's CLI configuration. Local plugin IPC must be allowed; a restricted sandbox can block the plugin handshake even though the fixture makes no external API requests. The production readiness polling interval is unchanged, so creation scenarios take at least 15 seconds. Independent scenarios run in parallel.

## Coverage

Fourteen scenarios run per CLI:

1. Create with a date, change it in place, change timezone spelling, reconcile managed drift, explicitly clear, repeat apply, use an empty object, stop management, observe unmanaged drift, and destroy.
2. Import an instance with a date and spend threshold without adopting management, then explicitly clear only its date and destroy.
3. Create with `auto_delete` omitted and confirm a clean subsequent plan.
4. Create with an explicit null date and preserve the present clearing object in state.
5. Resolve an unknown date from a built-in `terraform_data` resource during apply.
6. Resolve an unknown outer object during apply.
7. Reject invalid dates, fractional seconds, and invalid timezone offsets before API mutations.
8. Remove an externally deleted instance from state after HTTP 200 with `status: deleted`.
9. Remove an instance from state after HTTP 404.
10. Preserve the original deadline after a readiness polling HTTP error.
11. Preserve the original deadline when readiness reports an instance error.
12. Preserve the original deadline when the final read fails after readiness succeeds.
13. Preserve the original deadline when creation times out and cleanup fails.
14. Remove the allocation when creation times out and cleanup succeeds.

Assertions inspect saved JSON plans, persisted state, remote fixture settings, instance IDs, and API mutation counts. Unchanged applies must have a `no-op` instance plan and make no API mutations; date changes must use `update`, with no replacement. Spend thresholds survive date updates and clearing.

Failure scenarios require a failed CLI apply with the expected diagnostic, the date in the original create payload, exactly one allocation attempt, no update requests, and the expected read/cleanup counts. Every surviving allocation must retain its deadline. The fixture stores its allocation and request counters independently of Terraform state so failed applies cannot hide a surviving instance.

The fixture models the merge/null behavior observed in the earlier live API verification. These tests establish provider/CLI behavior against that fixture; they do not establish real service template inheritance, deadline execution, or early identity persistence/recovery (issue #4). Those remain separate validation steps. The sibling API payload tests check exact serialized request bodies independently. `internal/resources/instance/auto_delete_failure_test.go` additionally covers lost/malformed create responses after allocation, cancellation, malformed responses and transport errors during polling/readback, mismatched readback, invalid timeout parsing, and a successful-create control.
