# Releases and distribution

servescale independently maintains this Shadeform provider for OpenTofu at
`registry.opentofu.org/servescale/shadeform`. The upstream baseline is
`shadeform/terraform-provider-shadeform` v0.1.3, commit
`c5e5295741be0c0859dee5e3a51dd14b6ea61717`.

Available versions and publication dates are recorded in
[GitHub Releases](https://github.com/servescale/terraform-provider-shadeform/releases)
and the [OpenTofu registry](https://search.opentofu.org/provider/servescale/shadeform/latest).
Track the first release's publication, registration, and installation evidence in
[issue #3](https://github.com/servescale/terraform-provider-shadeform/issues/3).
Record execution status and verification evidence in the release's tracking issue.

## Version policy

Fork releases use independent normal SemVer, starting with `v0.2.0`. Normal OpenTofu
version ranges do not select prereleases, so do not append a vendor prerelease
suffix. Fork and upstream version numbers have separate meanings; matching numbers
promise neither identical code nor compatibility. The provider addresses are
distinct identities.

Upstream contribution is optional, with no upstream contribution required for this
release. Record any future submission and its status in the relevant issue or
release notes. There is no planned retirement or automatic removal of downstream
changes; retirement requires an explicit compatibility and consumer migration
decision.

## Prepare a release

1. Merge the reviewed changes intended for the release. Update the version's
   section in [CHANGELOG.md](../CHANGELOG.md), including compatibility and known
   limitations. Its heading must match the tag exactly, for example `## v0.2.0`;
   the GitHub Release records the actual publication date. The release workflow
   uses that changelog section as the release notes and appends the signing-key
   fingerprint. A tag without a matching section fails before publication.
2. Run the [Checks workflow](../.github/workflows/checks.yml) and review its results.
   It runs on pull requests and pushes to `main`, and is reused by the release
   workflow. It checks dependency files without modifying them, runs `go vet`,
   explicitly enables the race-enabled OpenTofu lifecycle/schema/migration suite,
   and validates the GoReleaser configuration. Full Git history is fetched for the
   pinned legacy-provider tests. OpenTofu is pinned to 1.13.5 and GoReleaser to
   v2.20.2; Go is selected from `go.mod`.
3. Verify packaging in an isolated clean checkout using GoReleaser v2.20.2. A
   snapshot build exercises packaging without signing or publishing:

   ```sh
   goreleaser check
   goreleaser release --snapshot --clean --skip=publish,sign
   git status --short
   ```

   The generated `/dist/` directory is ignored. The build must leave the tracked
   source and dependency files unchanged. Inspect the platform archives, binary
   names, checksums, and manifest. For an exact-version rehearsal, create the
   version tag only in a disposable local clone and omit `--snapshot`; never push
   that rehearsal tag.
4. Configure a company-controlled release-signing key and the repository Actions
   secrets `GPG_PRIVATE_KEY` (ASCII-armored private key) and `PASSPHRASE`. Keep a
   secure company backup of the key, passphrase, and revocation certificate. The
   workflow imports the key and derives `GPG_FINGERPRINT`; GitHub supplies
   `GITHUB_TOKEN` automatically. Repository secrets are configured outside a PR.
5. Review the exact commit to tag. Record the completed local and CI checks in the
   preparation PR. Passing fixture tests does not establish registry installation
   or release-signature verification.

## Publish the signed GitHub release

Fetch `main` and inspect the commit before creating the tag. For v0.2.0:

```sh
git fetch origin main --tags
git log -1 origin/main
# Confirm this is the reviewed and tested release commit before continuing.
git tag -a v0.2.0 origin/main -m "Release v0.2.0"
git push origin refs/tags/v0.2.0
```

Pushing a `v*` tag starts the [Release workflow](../.github/workflows/release.yml).
The reusable checks must pass before the publishing job imports the GPG key.
GoReleaser then builds, signs, and publishes the GitHub Release. Only the publishing
job receives `contents: write`; the validation workflow has read access and does
not receive the signing secrets.

For `v0.2.0`, GoReleaser injects `main.version=0.2.0`. Keep the tag, changelog, binary,
and asset versions consistent:

```text
terraform-provider-shadeform_v0.2.0[.exe]
terraform-provider-shadeform_0.2.0_<os>_<arch>.zip
terraform-provider-shadeform_0.2.0_manifest.json
terraform-provider-shadeform_0.2.0_SHA256SUMS
terraform-provider-shadeform_0.2.0_SHA256SUMS.sig
```

The package matrix covers Linux and FreeBSD on amd64, 386, arm, and arm64; macOS on
amd64 and arm64; and Windows on amd64, 386, and arm64. Packaging is distinct from
runtime validation: the lifecycle suite is tested on Linux amd64.

Download the published assets and verify the detached checksum signature using
the expected public key, then check archive hashes against the signed checksum
file. For v0.2.0, after importing and checking the public key's fingerprint:

```sh
gh release download v0.2.0 --repo servescale/terraform-provider-shadeform --dir release-v0.2.0
cd release-v0.2.0
gpg --verify terraform-provider-shadeform_0.2.0_SHA256SUMS.sig terraform-provider-shadeform_0.2.0_SHA256SUMS
sha256sum --check terraform-provider-shadeform_0.2.0_SHA256SUMS
```

Review the release notes, signing-key fingerprint, supported platforms, and any
recovery or compatibility limitations. Record signature verification and the
release URL in the tracking issue. Publish a new version to correct an indexed
release; do not overwrite its artifacts or checksums.

## First-time OpenTofu registration

Submit `servescale/terraform-provider-shadeform` and the release key's armored
public key through the OpenTofu registry's
[provider and signing-key forms](https://github.com/opentofu/registry#adding-providers-modules-or-gpg-keys-to-the-opentofu-registry).
Use namespace `servescale` and provider name `shadeform` for the key submission.
These submissions require the GitHub issue form UI. The key submitter must have
public organization membership. Await approval and confirm indexing of the signed
GitHub release. No HashiCorp registry publication is required.

Record submission links and verification evidence in issue #3. Keep that issue
open until the reviewed distribution, checksums, compatibility notes, migration
instructions, and published-installation checks are delivered. For later releases,
reuse the registration and signing key unless a new key needs to be registered.

## Verify registry installation and migration

1. In a clean OpenTofu working directory without development overrides or mirrors,
   use the [installation example](../README.md#installation), pinning
   `version = "0.2.0"` for this release's verification. Run:

   ```sh
   OPENTOFU_ENFORCE_GPG_VALIDATION=true tofu init
   tofu providers
   tofu version -json
   tofu init -lockfile=readonly
   ```

   Confirm selection of `registry.opentofu.org/servescale/shadeform` v0.2.0 and
   signature verification with the expected key. Review and commit
   `.terraform.lock.hcl` in consumer projects.
2. Repeat the [upstream provider-address migration](migration.md) with the
   published signed distribution, using the actual full provider address in the
   existing state. Verify the same instance ID, clean plans, unchanged date/spend
   thresholds, and supported in-place date management.
3. Record the CLI version, selected provider version, signing fingerprint, and
   migration results in the release's tracking issue. The
   [local fixture tests](../tests/lifecycle/README.md) cover provider selection,
   schema compatibility, and lock-file behavior, but cannot replace these checks
   of registry installation and signatures.

## Compatibility and support

OpenTofu defaults `servescale/shadeform` to `registry.opentofu.org/servescale/shadeform`.
Use the fully qualified source in configurations shared with Terraform. See
[OpenTofu provider source addresses](https://opentofu.org/docs/language/providers/requirements/#source-addresses).

OpenTofu 1.13.1 and 1.13.5 have passed all 24 lifecycle/schema/migration scenarios
on Linux amd64. These versions do not establish a minimum supported range.
Terraform results before the fork identity change are historical; Terraform has
not been validated with this fork identity.

v0.2.0 includes automatic deletion date management. Failed-create recovery
([issue #4](https://github.com/servescale/terraform-provider-shadeform/issues/4)) and
replacement planning for creation-only fields
([issue #5](https://github.com/servescale/terraform-provider-shadeform/issues/5)) are
separate limitations. Release notes must accurately describe which fixes are
included.

Report fork bugs and distribution problems to
[servescale's issue tracker](https://github.com/servescale/terraform-provider-shadeform/issues).
