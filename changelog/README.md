# Changelog

This `changelog/` directory is the single source of truth for the project's
changelog. Per-major-version files live here, one Markdown file per major series:

- `CHANGELOG-v1.x.md` — the `1.x` series (active)
- …

**How updates work:**

- The active changelog file (`CHANGELOG-v1.x.md`) is maintained automatically
  by release-please in its release PR. The
  [`release-please-config.json`](../release-please-config.json) sets
  `"changelog-path": "changelog/CHANGELOG-v1.x.md"`. When a new major series
  begins, create a new `CHANGELOG-vN.x.md` and bump `changelog-path`.
- GitHub Release notes (shown on the Releases page for each tag) are rendered
  separately by [git-cliff](https://git-cliff.org) (config:
  [`cliff.toml`](../cliff.toml)) in the release workflow. That is distinct from
  this repo changelog.

**Do not hand-edit:** These files are auto-generated and excluded from `oxfmt`
(see `.oxfmtignore`). Reformatting them causes spurious diffs on every PR.
