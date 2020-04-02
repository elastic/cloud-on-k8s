# Release notes generator

This tool generates release notes from all PRs labeled with a specific version. If an issue is linked, it will use that as well.

PRs labeled with the following are not included:
- non-issue
- refactoring
- docs
- test
- ci
- backport

## Usage

```
go run . $VERSION > outfile
```

e.g.

```
go run . v1.1.0 > ../../docs/release-notes/1.1.0.asciidoc
```

You will then likely also want to update `docs/release-notes/highlights.asciidoc` and `docs/release-notes.asciidoc` to include the new release notes.
