# servescale Shadeform Provider

[![GitHub release](https://img.shields.io/github/v/release/servescale/terraform-provider-shadeform)](https://github.com/servescale/terraform-provider-shadeform/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A Terraform/OpenTofu provider for managing GPU instances and storage volumes on Shadeform, a unified platform for deploying and managing cloud GPUs across multiple cloud providers. For more information about Shadeform, visit the [Shadeform public docs](https://docs.shadeform.ai/getting-started/introduction).

This is an independently maintained servescale fork for OpenTofu, based on upstream [v0.1.3](https://github.com/shadeform/terraform-provider-shadeform/tree/v0.1.3) (`c5e5295741be0c0859dee5e3a51dd14b6ea61717`). Its provider address is `registry.opentofu.org/servescale/shadeform`. Fork versions follow independent SemVer; matching an upstream version does not imply identical code or compatibility. Upstream contribution remains an option; this fork has no planned retirement date.

Available versions are listed in the [OpenTofu registry](https://search.opentofu.org/provider/servescale/shadeform/latest) and [GitHub Releases](https://github.com/servescale/terraform-provider-shadeform/releases). Maintainers can follow the [release procedure](docs/releases.md).

## Quick Start

### Installation

Add the provider to your OpenTofu configuration:

```hcl
terraform {
  required_providers {
    shadeform = {
      source  = "servescale/shadeform"
      version = "~> 0.2.0"
    }
  }
}
```

OpenTofu resolves this shorthand to `registry.opentofu.org/servescale/shadeform`. Use the full address in configurations shared with Terraform, whose default registry differs. HashiCorp registry publication is not part of this fork's release contract. Existing upstream state requires [provider-address migration](docs/migration.md); changing only this block is insufficient.

### Configuration

Configure the provider with your Shadeform API key:

```hcl
provider "shadeform" {
  api_key = "YOUR_API_KEY"
}
```

You can also set the API key via environment variable:

```bash
export SHADEFORM_API_KEY="YOUR_API_KEY"
```

If you do that, make sure to set the `api_key` to `""`

#### Commands

```bash
# Initialize OpenTofu
tofu init

# Plan the changes
tofu plan

# Apply the changes
tofu apply

# Check the state
tofu show

# Destroy the resources
tofu destroy
```

### Basic Usage

Create a GPU instance:

```hcl
resource "shadeform_instance" "test-instance" {
  cloud              = "scaleway"
  region             = "paris-france-1"
  shade_instance_type = "H100"
  name               = "terraform-test-instance"
} 
```

Create a persistent storage volume:

```hcl
resource "shadeform_volume" "test-volume" {
  cloud      = "datacrunch"
  region     = "helsinki-finland-2"
  name       = "terraform-test-volume"
  size_in_gb = 101
}
```

> **_NOTE:_** Instances can take anywhere from 1 - 15 minutes on average to spin up with some evening taking upwards of 30-40 minutes.
The `terraform apply` command won't finish until the instances are active (or errored out). There is a default timeout of 60 minutes.

## Automatic deletion

Use the optional `auto_delete.date_threshold` setting on `shadeform_instance` to manage its automatic deletion date. A configured date is included in the original create request and can be updated in place. A present object with `date_threshold = null` explicitly clears only the date; omitting the entire object leaves the remote date unchanged. Spend thresholds are preserved on updates.

Use `effective_auto_delete_date_threshold` to inspect the current remote date, including dates inherited from templates or found on import. See the [instance documentation](docs/resources/instance.md#automatic-deletion) for examples, timestamp requirements, and the durable-state check callers must perform before clearing provisioning protection.

At creation, supplying `auto_delete` replaces the template's whole auto-delete object: setting or clearing its date also prevents inheritance of the template's spend threshold. Terraform/OpenTofu warns when creating an instance with both `template_id` and `auto_delete`. Omitting the object inherits both thresholds. This differs from date updates on an existing instance, which preserve its spend threshold.

## Requirements

| Name | Version |
|------|---------|
| OpenTofu | Tested with 1.13.1 and 1.13.9 on Linux amd64 |
| go (building from source) | >= 1.25 |

Use a tested OpenTofu version for this release. These runs do not establish a minimum supported version or compatibility with untested versions. All 24 lifecycle/schema/migration scenarios passed with the fork identity on OpenTofu 1.13.1 and 1.13.9; see the [validation details](tests/lifecycle/README.md). Terraform 1.14.4 results predate the identity change and are historical; Terraform has not been validated with the fork identity.

## Contributing

Pull requests run dependency checks, `go vet`, the race-enabled OpenTofu lifecycle
suite, and GoReleaser validation in [GitHub Actions](.github/workflows/checks.yml).
The release workflow runs the same checks before signing and publishing.

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Submit a pull request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Support

- **Documentation**: [Shadeform API Documentation](https://docs.shadeform.ai)
- **Fork bugs, releases, and support**: [servescale GitHub Issues](https://github.com/servescale/terraform-provider-shadeform/issues)
- **Upstream provider**: [Shadeform repository](https://github.com/shadeform/terraform-provider-shadeform)

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for a list of changes and version history.
