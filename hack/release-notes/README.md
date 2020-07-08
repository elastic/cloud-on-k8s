Release notes generator
=======================

This tool generates release notes from all PRs labeled with a specific version. If any issues are linked to a PR, they will be included in the output as well.

PRs labeled with the following are not included:
- `>non-issue`
- `>refactoring`
- `>docs`
- `>test`
- `:ci`
- `backport`
- `exclude-from-release-notes`


Prerequisites
--------------

Create a GitHub token by going to https://github.com/settings/tokens. The token must have `repo:status` and `public_repo` scopes. Enable SSO for the token as well.


Usage
-----

```
GITHUB_TOKEN=<token> go run main.go <version>

Example:
GITHUB_TOKEN=xxxyyy go run main.go 1.2.0 > ../../docs/release-notes/1.2.0.asciidoc
```
