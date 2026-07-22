# Creating a new release

1. Update the `Version` const in `version.go`.
2. Add a new entry to `CHANGELOG.md`.
3. Open a PR with the above changes.
4. Once the PR is merged, tag `main` with the new version, e.g. `v0.0.2`. Push the tag. This
   kicks off a CI workflow which runs the test suite and publishes a draft GitHub release with
   auto-generated notes.
5. Review the draft release notes against the CHANGELOG entry, edit as needed, and publish.
