package dev.cueseek.core.api

import dev.cueseek.core.model.AgentAddress
import dev.cueseek.core.model.DeviceToken
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer

/** Builds a client pointed at [server], optionally holding a token. */
internal fun MockWebServer.api(token: String? = null): CueSeekApi =
    CueSeekApiFactory.create(
        address = AgentAddress(hostName, port),
        tokens = token?.let { t -> TokenProvider { DeviceToken(t) } } ?: TokenProvider.None,
    )

internal fun jsonResponse(code: Int, body: String): MockResponse =
    MockResponse.Builder()
        .code(code)
        .setHeader("Content-Type", "application/json")
        .body(body)
        .build()

internal fun problemResponse(code: Int, body: String): MockResponse =
    MockResponse.Builder()
        .code(code)
        .setHeader("Content-Type", "application/problem+json")
        .body(body)
        .build()

internal fun problem(type: String, title: String, status: Int, detail: String? = null): String =
    buildString {
        append("""{"type":"https://cueseek.dev/problems/$type",""")
        append(""""title":"$title","status":$status""")
        if (detail != null) append(""","detail":"$detail"""")
        append(""","instance":"/v1/test"}""")
    }

/** A payload copied from a live agent, not invented. See docs/m2-android-api.md §5. */
internal const val JELLYFIN_SERVICE_JSON = """
{
  "id": "jellyfin",
  "name": "Jellyfin",
  "capabilities": [
    { "id": "health",  "label": "Health" },
    { "id": "control", "label": "Controls" }
  ],
  "actions": [
    {
      "id": "restart",
      "label": "Restart Jellyfin",
      "description": "Restarts the Jellyfin service.",
      "risk": "disruptive"
    }
  ],
  "health": {
    "status": "healthy",
    "reachable": true,
    "reasons": [],
    "observed_at": "2026-08-08T14:56:51.7940666Z"
  }
}
"""
