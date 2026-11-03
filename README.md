# Terraform Provider for Shadeform

[![Terraform Registry](https://img.shields.io/badge/terraform-registry-blue.svg)](https://registry.terraform.io/providers/shadeform/shadeform)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A Terraform provider for managing GPU instances and storage volumes on Shadeform, a unified platform for deploying and managing cloud GPUs across multiple cloud providers. For more information about what Shadeform is and how to use it, visit our [public docs](https://docs.shadeform.ai/getting-started/introduction).

## Quick Start

### Installation

Add the provider to your Terraform configuration:

```hcl
terraform {
  required_providers {
    shadeform = {
      source  = "shadeform/shadeform"
      version = "~> 0.1.0"
    }
  }
}
```

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
# Initialize Terraform
terraform init

# Plan the changes
terraform plan

# Apply the changes
terraform apply

# Check the state
terraform show

# Destroy the resources
terraform destroy
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
| terraform | >= 1.0 |
| go | >= 1.21 |

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Submit a pull request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Support

- **Documentation**: [Shadeform API Documentation](https://docs.shadeform.ai)
- **Issues**: [GitHub Issues](https://github.com/shadeform/terraform-provider-shadeform/issues)
- **Email**: support@shadeform.ai

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for a list of changes and version history.
