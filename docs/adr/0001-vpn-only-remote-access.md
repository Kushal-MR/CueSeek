# ADR-0001: VPN-only remote access

- **Status:** Accepted
- **Date:** 2026-08-08

## Context

CueSeek clients must reach the agent from outside the home network. The options were:
expose the agent publicly behind a reverse proxy with TLS; build a cloud relay that the
agent dials out to; or require the client to already have network access to the host via
a VPN.

The agent can reboot and power off the machine. That single fact changes the risk
calculus: a public endpoint would need TLS, certificate renewal, rate limiting,
brute-force protection and ongoing hardening — a large and permanent surface guarding an
irreversible capability, maintained by one person. A relay would need infrastructure,
presence tracking, multi-tenancy and end-to-end encryption, plus a trust story explaining
why a third-party server sits in the path.

Meanwhile, the realistic user of a self-hosted operations console almost certainly
already runs Tailscale or WireGuard. Home Assistant and Immich both recommend exactly
that for their own remote access.

## Decision

CueSeek assumes the client can already route to the host. It implements no NAT
traversal, no cloud relay, no TLS termination and no certificate management, and binds to
the private-network interface rather than `0.0.0.0`.

## Consequences

- The largest attack surface does not exist. There is nothing to expose.
- Substantial scope removed from every milestone; no infrastructure to run or pay for.
- **Network reachability is not authorisation.** A tailnet can include a work laptop, a
  shared node, or a device that gets lost. Application-level authentication with scopes
  remains mandatory — see ADR-0006. Delegating transport security must not become an
  excuse for skipping authorisation.
- Cost: a prerequisite for users. "Install Tailscale first" is a real barrier, and it
  rules out users who want a URL and nothing else.
- Cost, deferred: **push notifications become hard.** Alerting normally needs a cloud
  intermediary, which is precisely what was declined here. See ADR-0012 — this is a known
  future decision point, not an oversight.
- A relay tier remains additive rather than a rewrite: clients speak to a single agent
  endpoint either way.
