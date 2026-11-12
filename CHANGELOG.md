# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## v0.2.0

This is the first independent servescale release, using `registry.opentofu.org/servescale/shadeform`. The upstream baseline is v0.1.3 (`c5e5295741be0c0859dee5e3a51dd14b6ea61717`); the v0.1.x entries below describe upstream history. See [GitHub Releases](https://github.com/servescale/terraform-provider-shadeform/releases) for publication dates and signed artifacts, and the [release procedure](docs/releases.md) for maintainer instructions.

### Added

- Independent servescale provider identity `registry.opentofu.org/servescale/shadeform`, with installation instructions and a repeatable release procedure.
- Upstream-to-fork state migration instructions and OpenTofu tests for both upstream registry hostnames, including local package installation, lock-file verification, and same-instance date updates with spend preservation.
- Optional instance `auto_delete.date_threshold` management: set a date at creation, update it in place, or explicitly clear it. Date updates on an existing instance preserve spend thresholds. At creation, supplying the object overrides the template's whole auto-delete object, including its inherited spend threshold. Omitting the object leaves remote dates unchanged and inherits template thresholds at creation.
- Read-only `effective_auto_delete_date_threshold` for inspecting managed and unmanaged dates.
- Terraform/OpenTofu creation warnings when `template_id` and `auto_delete` are supplied together, explaining that the template's spend threshold is not inherited.

### Fixed

- Refresh recognizes instances deleted automatically or externally through HTTP 404 or `status: deleted`.
- Instance updates populate computed fields from readback instead of leaving them unknown. Numeric SSH ports and hourly prices are decoded from JSON responses.

### Release tooling

- Validate dependency files, Go packages, the race-enabled OpenTofu lifecycle suite, and release configuration on pull requests and before publishing tags.
- Pin GitHub Actions by commit SHA and GoReleaser to v2.20.2; keep release builds from modifying dependency files and ignore generated distribution output.

### Compatibility and known limitations

- Tested with OpenTofu 1.13.1 and 1.13.9 on Linux amd64. These results do not establish a minimum supported version. Terraform has not been validated with the fork provider identity.
- Existing upstream state requires the documented [provider-address migration](docs/migration.md).
- Failed creation can leave an allocated instance outside state; automatic deletion dates limit its lifetime when configured, but do not implement recovery ([issue #4](https://github.com/servescale/terraform-provider-shadeform/issues/4)).
- Creation-only instance settings do not yet consistently require replacement when changed ([issue #5](https://github.com/servescale/terraform-provider-shadeform/issues/5)).

## v0.1.0

### Added
- Initial release of the Shadeform Terraform provider
- Support for GPU instance management (create, read, update, delete)
- Support for storage volume management (create, read, delete)
- Data source for querying available instance types
- Volume attachment to instances

### Resources
- `shadeform_instance` - Manage GPU instances
- `shadeform_volume` - Manage storage volumes

### Data Sources
- `shadeform_instance_types` - Query available instance types and availability

### Features
- Full CRUD operations for instances and volumes
- Availability checking before deployment
- Volume attachment and management

## v0.1.1

### Added
- Updated instance resource documentation
- Updated instance_types data source documentation
- Updated README

## v0.1.2

### Added
- Updated instance resource documentation
- Updated volume resource documentation
- Updated README

## v0.1.3

### Updated

- If the default timeout of 60 minutes is reached and an instance hasn't become active yet,
we will now attempt to delete the instance before returning
