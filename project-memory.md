# Project Memory

## Decisions

### Go module path default

- Context: `project.bootstrap` needed to initialize a Go module, but the spec does not define a public repository/import path and the git repo has no remote configured.
- Decision: use the local module path `battle-proxy-akira` for now.
- Rejected alternatives: a guessed hosted path such as `github.com/.../battle-proxy-akira`, because it would encode an unconfirmed remote location.
- Affected area: Go module/import paths for the initial server skeleton; can be changed later before publishing if a canonical remote path is chosen.
