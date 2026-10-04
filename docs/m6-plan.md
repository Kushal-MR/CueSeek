# M6 — the website

**Not started.** This file collects what has already been decided or asked for, so the
milestone opens with its inputs written down rather than remembered. The phase plan itself is
written when M6 begins, in the shape of `m5-plan.md`.

The page shipped in M4.9 (`docs/index.html`, live at
[kushal-mr.github.io/CueSeek](https://kushal-mr.github.io/CueSeek/)) is a deliberate
placeholder: plain, no framework, no analytics, no third-party requests. M6 replaces it.

## Asked for so far

### A motion reel, as the site's video preview

Decided 2026-10-04, after a proof of concept built at the end of M5.

**What the proof showed.** A 20-second reel made **entirely from code** — every frame drawn by
a program, the soundtrack synthesised by another, no editing software, no stock assets, no
services. It used only CueSeek's own identity: the near-black page, the greens, the status
palette, IBM Plex, and the Swell mark regenerated from `docs/brand/icon.py`'s geometry. Ten
scenes at 120 BPM: the question ("is everything fine?"), the verdict, vitals, a service
needing attention, a restart that says "asked", the hold-to-confirm, phone and watch agreeing,
and the end card. Kushal's verdict: *"this is exactly what I'm looking for"* — it needs polish,
not a new direction.

**What M6's version adds:**

- **Real screenshots of the services it works with** — the Jellyfin and qBittorrent web UIs,
  shown alongside CueSeek's view of them, so the reel shows *how it works with them* and not
  only what CueSeek looks like.
- **More polished text** and **more imagery** — the phone and watch shown with real captures
  rather than drawn stand-ins.
- **Audio, decided in M6** — the proof's synthesised track was a placeholder for this.
- The proof's renderer and synth are the starting point; they are code, so every scene,
  colour, timing and note is editable.

**One constraint to settle first: Plex.** Kushal asked for Plex screenshots. **CueSeek has no
Plex adapter** — Jellyfin and qBittorrent are supported in full, and any systemd unit for
health and lifecycle. A reel that showed Plex would advertise support that does not exist.
Either an adapter lands before the reel is made, or Plex stays out of it. Showing a Plex
server as a plain `systemd` unit — health, restart, nothing more — is honest, and is an option.

**Practical notes from the proof:** frames render in under a minute on the laptop with the
JDK Android Studio already bundles; the last step, muxing picture and sound into MP4, needs an
encoder (ffmpeg), which is not installed and should be chosen deliberately when M6 starts.

### The site itself

- **Ambition:** animation, a 3D logo — "professional, looks like a complete project."
- **Hosting:** Vercel if the site becomes React/Next (preview deploys per PR match how this
  project verifies everything before merge); GitHub Pages if it stays hand-written.
- **A question to answer before choosing references:** the placeholder promises *"This page
  loads nothing but itself"*, and its security section leans on the same idea. A WebGL hero is
  several hundred KB of script and breaks that promise; a video is heavier still. Decide which
  story the site tells — and whether a self-hosted video counts as "itself" — before design
  starts.
- **The download page** carries both signed APKs and the agent, with checksums and attestation
  instructions, and states plainly that installing the Wear app means ADB over Wi-Fi
  (see `m5-plan.md`, "Distribution").
