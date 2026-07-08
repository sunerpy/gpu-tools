# Repository Metadata Configuration

This document records the exact `gh` commands to configure repository metadata for `sunerpy/gpu-tools`. These commands MUST be run manually at release time (they mutate the remote and are NOT executed during CI setup).

## Repository Description

Set the repository description to clearly communicate the project's purpose:

```bash
gh repo edit sunerpy/gpu-tools --description "Pure-Go NVIDIA GPU infrastructure CLI: detect, report, tuning advice, benchmark — single self-contained binary, no cgo"
```

## Repository Topics

Add searchable topics to improve discoverability on GitHub:

```bash
gh repo edit sunerpy/gpu-tools --add-topic go --add-topic cobra --add-topic cli --add-topic nvidia --add-topic gpu --add-topic nvml --add-topic purego --add-topic monitoring --add-topic benchmark
```

## Squash-Merge Policy (REQUIRED for release-please)

Configure squash-merge to use the PR title, ensuring release-please correctly detects conventional commits when PRs are squash-merged. This is **CRITICAL** for automated versioning:

```bash
gh repo edit sunerpy/gpu-tools --enable-squash-merge --squash-merge-commit-title PR_TITLE --squash-merge-commit-message COMMIT_MESSAGES
```

### Why this policy matters

- **release-please tracks commits** to determine version bumps and generate changelogs.
- If squash-merge collapses multiple commits (e.g., a PR with `feat:` and `fix:` commits) into a single commit with a non-conventional subject (e.g., `refactor:`), release-please silently stops bumping.
- Setting `PR_TITLE` forces the squash commit message to use the PR title, which **must** follow Conventional Commits format (e.g., `feat: add new feature` or `fix: resolve issue`).
- CI enforces conventional PR titles via a check in `.github/workflows/pr-title.yml`, ensuring all releases are semantic and traceable.

## Execution Checklist

- [ ] Run the description command
- [ ] Run the topics command
- [ ] Run the squash-merge policy command
- [ ] Verify in GitHub: repo "About" section shows description, topics listed, and squash-merge settings are correct
- [ ] Confirm that `gh repo view sunerpy/gpu-tools --json squashMergeCommitTitle` returns `PR_TITLE`

## Reference

- GitHub CLI documentation: `gh repo edit --help`
- release-please & squash-merge behavior: https://github.com/googleapis/release-please/discussions (search "squash-merge")
- Conventional Commits: https://www.conventionalcommits.org/
