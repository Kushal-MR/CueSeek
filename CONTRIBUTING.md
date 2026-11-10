# Contributing

Bug reports, questions and pull requests are all welcome. For anything security-related, use
private reporting instead; see [SECURITY.md](SECURITY.md).

## Before you start

- **Read the decision records** in [docs/adr/](docs/adr/). Most "why is it like this?"
  questions are answered there, along with what each choice cost.
- **For anything bigger than a small fix, open an issue first** so we can agree on the shape
  before you spend time on it.

## Building and testing

The same checks run in CI on every pull request.

**Agent** (Go 1.25+, from `agent/`):

```bash
go vet ./...
go test -race ./...
go build ./...
```

**Phone and watch apps** (JDK 21+, from `clients/android/`):

```bash
./gradlew build
```

**Website** (Node, from `site/`):

```bash
npm ci
npm run lint
npm run build
```

## A few rules the codebase holds to

- **The API spec comes first.** Change `api/openapi.yaml`, then regenerate. CI fails if the
  generated Go or Kotlin code differs from what is committed
  ([ADR-0009](docs/adr/0009-monorepo-contract-drift-gate.md)).
- **No client code branches on which service it is.** Apps draw capabilities, not services
  ([ADR-0007](docs/adr/0007-client-capability-registry.md)).
- **The agent never gains privileges.** Anything that needs more than the polkit rule allows
  is a design discussion, not a patch
  ([ADR-0002](docs/adr/0002-host-privilege-dbus-polkit.md)).

## Adding support for a service

A new service is an adapter: a package under `agent/internal/adapters/` and one line in
`adapters/builtin/builtin.go`. It implements the small `Service` interface and opts into
the capabilities it has, such as control, now playing or transfers
([ADR-0005](docs/adr/0005-capability-based-adapters.md)). The Jellyfin and qBittorrent
adapters are the examples to copy. If your adapter needs changes outside its own package
beyond that registry line, say so in the pull request; that is worth discussing.

## Licence

By contributing, you agree that your contribution is licensed under the
[Apache License 2.0](LICENSE), like the rest of the project.
