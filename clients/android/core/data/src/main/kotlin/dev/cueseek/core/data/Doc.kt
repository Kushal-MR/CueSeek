/**
 * Repositories, credential storage and the live-state machine that sits between
 * `:core:api` and the UI.
 *
 * Two constraints shape this module. Every repository is keyed by `host_id` from the
 * first commit, because retrofitting multi-host later is a navigation rewrite and
 * carrying it now is one field (ADR-0008). And the device token is sealed with an
 * Android Keystore key rather than stored in plain preferences (ADR-0013).
 *
 * The stream client's freshness watchdog also lives here: data is stale when nothing has
 * arrived for roughly 2x the 15s heartbeat, regardless of what the transport claims about
 * being connected. See `docs/m2-android-api.md` §8.
 *
 * Populated in M2 phases P2 and P3.
 */
package dev.cueseek.core.data
