# Licensing (proposal)

This document describes a **proposed** licensing and sustainability model for Clarity.

Clarity is a local-first tool. The goal is to keep the core experience useful without hosting anything for users, and without requiring ongoing network access.

## Goals

- **Local-first by default**: your workspace lives on disk.
- **No SaaS requirement**: no hosted server needed to use Clarity.
- **Optional Git sync**: Git-backed workflows are supported, but not required.
- **Trust and portability**: you should be able to keep working and move your data without vendor lock-in.

## Proposed model: free for individuals, paid for organizations

- **Individuals**: free to use forever.
  - The TUI may display an occasional reminder to support development with a paid personal license.
- **Organizations**: paid license required for commercial/organizational use.
  - Licenses should allow internal redistribution of the Clarity binary within the organization.

This is a “dual-track” model intended to keep adoption friction low for individuals while establishing a sustainable path for teams.

## Personal licenses (supporter tier)

Personal licenses are intended primarily as a way to support development. They
may also come with “convenience” benefits that don’t compromise the local-first
core (for example: signed builds, auto-update channel, and/or priority support).

## Org licenses

Org licenses are intended for teams and companies that want clarity on compliance and a supported path for internal distribution.

## License verification (offline-first)

The target behavior is:

- Clarity should work **offline** indefinitely after a license is installed.
- Any online component (if used) should be limited to **initial issuance** (e.g. checkout → receive a signed license file).

One practical approach is a **signed license file** that Clarity verifies locally using an embedded public key. This avoids ongoing “phone home” checks while still allowing a paid license to be portable and easy to install.

## Data portability commitments

Clarity is designed so your data stays portable:

- Canonical history is stored as append-only JSONL event logs under `events/`.
- Derived state is local and rebuildable (e.g. `.clarity/index.sqlite`).
- Backups are supported via `clarity workspace export` / `clarity workspace import`.
- Reading/sharing exports are supported via `clarity publish ...` (Markdown).

## Source availability (trust)

Clarity is intended to be auditable, especially around:
- storage format and migration safety
- network behavior

Whether Clarity is released under an OSI-approved open source license or a source-available license is a separate decision. If Clarity is not OSI-open, the recommended mitigation is to keep the repository source-visible and to document the network-IO behavior clearly (see `clarity docs network-contract`).

