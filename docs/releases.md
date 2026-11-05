# Fork releases and distribution

ServeScale independently maintains this Shadeform provider for OpenTofu at
`registry.opentofu.org/servescale/shadeform`. The upstream baseline is
`shadeform/terraform-provider-shadeform` v0.1.3, commit
`c5e5295741be0c0859dee5e3a51dd14b6ea61717`.

## Status and version policy

The first proposed fork release is **v0.2.0, pending publication and registry
registration**. The README's installation example describes the intended release;
it does not identify an available registry package. Merging PR #6 alone does not
complete the distribution requirements of
[issue #3](https://github.com/servescale/terraform-provider-shadeform/issues/3).

Use independent normal SemVer releases. Do not append a vendor prerelease suffix:
normal OpenTofu version ranges do not select prereleases. Fork and upstream
version numbers have separate meanings; matching numbers promise neither identical
code nor compatibility. The provider addresses are distinct identities.

Upstream contribution is optional, with no upstream contribution required for this
release. Record any future submission and its status here. There is no planned
retirement or automatic removal of the downstream changes; any future retirement
requires an explicit compatibility and consumer migration decision.

## First release checklist

All items below remain outstanding until verified by the release maintainer:

- [ ] Complete provider-address migration documentation and coverage (PR #6's
  second review finding), keeping the existing schema compatibility tests.
- [ ] Review and test the release commit; convert the Unreleased changelog section
  to v0.2.0 with its release date. Tag that exact commit `v0.2.0`.
- [ ] Configure the fork's release signing key and the workflow secrets
  `GPG_PRIVATE_KEY` and `PASSPHRASE`. The release workflow obtains its fingerprint
  from the imported key.
- [ ] Publish the GitHub release from the tag using the existing
  [release workflow](../.github/workflows/release.yml) and
  [GoReleaser configuration](../goreleaser.yml). For `v0.2.0`, GoReleaser injects
  `main.version=0.2.0` into the binary. Keep this version consistent with the tag,
  changelog, `terraform-provider-shadeform_v0.2.0` binaries (with `.exe` on Windows),
  `terraform-provider-shadeform_0.2.0_<os>_<arch>.zip` archives,
  `terraform-provider-shadeform_0.2.0_manifest.json`, and
  `terraform-provider-shadeform_0.2.0_SHA256SUMS` plus its detached `.sig` signature.
- [ ] Verify the checksum signature with the published signing key and verify
  archive SHA256 hashes against the signed checksum file. Record the release
  URL, signing-key fingerprint, and supported platforms in the release notes.
- [ ] Register `servescale/shadeform` and its signing key using the OpenTofu
  registry's [provider and signing-key submission forms](https://github.com/opentofu/registry#adding-providers-modules-or-gpg-keys-to-the-opentofu-registry).
  These require the GitHub issue form UI. Await approval and confirm that the
  registry indexes the fork's GitHub release. No HashiCorp registry publication
  is required.
- [ ] From a clean OpenTofu working directory, use the README's source and version
  constraint and run `tofu init`. Verify that it selects
  `registry.opentofu.org/servescale/shadeform` v0.2.0 and verifies the signed
  checksums. Commit the resulting `.terraform.lock.hcl` in consumer projects;
  use `version = "0.2.0"` if an exact version is required.
- [ ] Validate the documented migration using the actual full provider address
  recorded in existing state, including provider selection and lock-file
  behavior. Local development overrides do not verify registry installation.
- [ ] Update the distribution status and installation documentation only after
  the package is installable. Keep issue #3's release requirements outstanding
  until the reviewed distribution, checksums, compatibility notes, and migration
  instructions are delivered.

## Installation and compatibility

After publication, OpenTofu users can follow the
[quick start](../README.md#installation): `source = "servescale/shadeform"` and
`version = "~> 0.2.0"`. OpenTofu defaults the registry hostname to
`registry.opentofu.org`; use the fully qualified source in configurations shared
with Terraform. See [OpenTofu provider source addresses](https://opentofu.org/docs/language/providers/requirements/#source-addresses).

The lifecycle suite exercises the fork identity with a local fixture, alongside
separate legacy schema checks. It does not prove registry installation or address
migration. See [test coverage and limits](../tests/lifecycle/README.md).

Report fork bugs and distribution problems to
[ServeScale's issue tracker](https://github.com/servescale/terraform-provider-shadeform/issues).
