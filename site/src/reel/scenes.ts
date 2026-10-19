/*
 * CueSeek — the motion reel. Twelve bars at 120 BPM; four beats a bar.
 * Every frame is drawn by this file, live, in your browser.
 * `s.at(beat)` is 0 until that beat, then rises to 1: that is the whole timeline.
 */
import { HANDLE_CENTRE, LONG, TAIL, HANDLE } from '../arrival/geometry.ts'
import { type Stage, bounce, clamp, inCubic, inOut, lerp, mix, out, outBack, outExpo, typed } from './kit.ts'

// bar 1 — boot
function boot(s: Stage) {
  s.label('AGENT v0.1.4 · LISTENING')
  const r = Math.min(s.w, s.h) * 0.27
  const sweep = inOut(s.at(0.3, 3.6)) * 270
  for (let a = 0; a <= sweep; a += 3.2) {
    const th = ((135 + a) * Math.PI) / 180
    s.dot(s.cx + r * Math.cos(th), s.cy + r * Math.sin(th), 2.4, s.c.line)
  }
  for (const beat of [1, 2, 3]) {
    const p = s.at(beat, 1.5)
    if (p > 0 && p < 1) s.ring(s.cx, s.cy, 14 + 110 * out(p), s.c.ok, 0.45 * (1 - p))
  }
  const pop = outBack(s.at(0.25, 0.6))
  s.dot(s.cx, s.cy, 12 * pop * (1 + 0.14 * Math.sin(s.b * Math.PI * 2)), s.c.hi, 1, 24)
  s.text(typed('live', s.at(1.1, 0.5) * 1.4), s.cx + 32, s.cy + 10, { size: 28, mono: true, color: s.c.muted })
}

// bar 2 — the question
function question(s: Stage) {
  s.label('ONE QUESTION')
  const size1 = s.fit('IS EVERYTHING', 170, s.w * 0.84, { weight: 500 })
  const size2 = size1 * 1.39
  const left = s.cx - s.textW('IS EVERYTHING', { size: size1, weight: 500 }) / 2
  const leave = inCubic(s.at(3.8, 0.2))
  const word = (w: string, x: number, y: number, size: number, p: number) =>
    s.text(w, x, y + (1 - outExpo(p)) * 150 - 70 * leave, { size, weight: 500, alpha: clamp(p * 4) * (1 - leave * 0.6) })
  word('IS', left, s.cy - size1 * 0.35, size1, s.at(0, 0.5))
  word('EVERYTHING', left + s.textW('IS ', { size: size1, weight: 500 }), s.cy - size1 * 0.35, size1, s.at(1, 0.5))
  const fineW = s.textW('FINE?', { size: size2, weight: 500 })
  word('FINE?', s.cx - fineW / 2 - size2 * 0.12, s.cy + size2 * 0.72, size2, s.at(2, 0.5))
  // the dot falls into place as the full stop on beat three — and swells into the next bar
  const fall = s.at(3, 0.7)
  if (fall > 0) {
    const x = s.cx + fineW / 2 + size2 * 0.06
    const y = lerp(s.cy - s.h * 0.3, s.cy + size2 * 0.6, bounce(fall))
    s.dot(x, y, size2 * 0.11 * (1 + 2.2 * inCubic(s.at(3.64, 0.36))), s.c.hi, 1, 30)
  }
}

// bar 3 — the pile: every service, its own app
const PILE = [
  ['Jellyfin', ':8096'], ['qBittorrent', ':8080'], ['Plex', ':32400'], ['Sonarr', ':8989'],
  ['Immich', ':2283'], ['Vaultwarden', ':8000'], ['Syncthing', ':8384'], ['Radarr', ':7878'],
]
function pile(s: Stage) {
  s.label('ONE APP EACH')
  const gather = inCubic(s.at(3.1, 0.9))
  PILE.forEach(([name, port], i) => {
    const p = outBack(s.at(i * 0.32, 0.4))
    const col = i % (s.p ? 2 : 4)
    const row = Math.floor(i / (s.p ? 2 : 4))
    const x0 = s.p ? s.w * (0.12 + col * 0.42) : s.w * (0.1 + col * 0.21)
    const y0 = s.h * (s.p ? 0.24 + row * 0.13 : 0.3 + row * 0.2) + Math.sin(i * 2.1) * 30
    const x = lerp(x0, s.cx - 150, gather)
    const y = lerp(y0, s.cy, gather)
    const flicker = 0.5 + 0.5 * Math.sin(s.t * 9 + i * 1.7) // each one, asking to be looked at
    s.ctx.save()
    s.ctx.translate(x + 150, y)
    s.ctx.rotate(Math.sin(i * 1.3) * 0.05)
    s.ctx.scale(p * (1 - gather), p * (1 - gather))
    s.box(-150, -34, 300, 68, 12, s.c.card, mix(s.c.line, s.c.warn, flicker * s.at(1.5, 1)))
    s.text(`${name}  ${port}`, 0, 9, { size: 24, mono: true, align: 'center' })
    s.ctx.restore()
  })
  s.text(typed('every one, its own app and address', s.at(1.6, 1)), s.cx, s.h * 0.86, {
    size: 26, mono: true, align: 'center', color: s.c.muted, alpha: 1 - gather,
  })
  s.dot(s.cx, s.cy, 40 * outBack(s.at(3.5, 0.5)), s.c.hi, 1, 30)
}

// bar 4 — the answer: one word
function answer(s: Stage) {
  s.label('HOMESERVER · 2/2 HEALTHY')
  const green = mix(s.c.bg, s.c.glow, 0.45)
  const flood = out(s.at(0, 0.5)) * Math.hypot(s.w, s.h)
  const shrink = inOut(s.at(3.2, 0.8)) // into the phone, next bar
  const g = s.ctx
  g.save()
  g.beginPath()
  if (shrink > 0) {
    const ph = phoneRect(s)
    g.roundRect(lerp(0, ph.x, shrink), lerp(0, ph.y, shrink), lerp(s.w, ph.w, shrink), lerp(s.h, ph.h, shrink), 34 * shrink)
  } else g.arc(s.cx, s.cy, flood, 0, Math.PI * 2)
  g.fillStyle = green
  g.fill()
  g.restore()
  const fade = 1 - s.at(3, 0.4)
  const x = s.w * 0.08
  const size = s.fit('Operational.', 230, s.w * 0.84, { weight: 500 })
  s.text('Operational.', x + (1 - outExpo(s.at(0.4, 0.6))) * -80, s.cy + size * 0.2, {
    size, weight: 500, color: s.c.hi, alpha: s.at(0.4, 0.3) * fade,
  })
  s.box(x, s.cy + size * 0.45, (s.w * 0.84) * out(s.at(1, 1)), 10, 5, s.c.hi, undefined, fade)
  s.text(typed('✓ 2  ·  live ●', s.at(2, 0.8)), x, s.cy + size * 0.45 + 70, {
    size: 30, mono: true, color: s.c.hi, alpha: fade,
  })
}

/** Where the phone stands in bars 4 and 5. */
function phoneRect(s: Stage) {
  const h = s.h * (s.p ? 0.62 : 0.78)
  const w = (h * 1272) / 1460
  return { x: s.cx - w / 2 + (s.p ? 0 : s.w * 0.12), y: s.h * (s.p ? 0.1 : 0.11), w, h }
}

// bar 5 — the phone: the real app
const PARTS = [
  // what it says, and where it is on the capture: [top, bottom] as fractions of its height
  ['the verdict', 0.135, 0.385],
  ['the server: processor · memory · disk · temperature', 0.405, 0.61],
  ['each service, and a menu to act on it', 0.635, 0.98],
] as const
function phone(s: Stage) {
  s.label('THE REAL APP')
  const r = phoneRect(s)
  s.shot(s.img.dashboard, r.x, r.y, r.w, { alpha: s.at(0, 0.4) })
  PARTS.forEach(([says, top, bottom], i) => {
    const p = s.at(0.8 + i, 0.4) * (1 - s.at(1.8 + i, 0.3) * (i < 2 ? 1 : 0))
    const y0 = r.y + r.h * top
    const y1 = r.y + r.h * bottom
    s.box(r.x - 14, y0, r.w + 28, y1 - y0, 18, undefined, s.c.accent, p)
    if (s.p) s.text(says, s.cx, s.h * 0.86, { size: 30, align: 'center', color: s.c.text, alpha: p })
    else s.text(says, r.x - 80, (y0 + y1) / 2 + 10, { size: 28, align: 'right', alpha: p })
  })
  s.text('every 10 s, live', s.p ? s.cx : r.x - 80, s.p ? s.h * 0.91 : s.h * 0.92, {
    size: 22, mono: true, align: s.p ? 'center' : 'right', color: s.c.muted, alpha: s.at(2, 0.5),
  })
}

// bar 6 — inside: a service's own page, and what CueSeek knows of the same moment
function inside(s: Stage) {
  s.label('HEALTH FROM ITS OWN API')
  const second = s.b >= 2 // Jellyfin for two beats, then qBittorrent
  const page = second ? s.img.qbPage : s.img.jfPage
  const ours = second ? s.img.qbNow : s.img.jfNow
  const k = second ? 2 : 0
  const pw = s.w * (s.p ? 0.84 : 0.6)
  const px = s.w * (s.p ? 0.08 : 0.07) - (1 - outExpo(s.at(k, 0.5))) * 400
  const py = s.h * (s.p ? 0.16 : 0.15)
  s.shot(page, px, py, pw, { alpha: s.at(k, 0.3), bezel: 6 })
  const ow = s.w * (s.p ? 0.66 : 0.38)
  const ox = s.w * (s.p ? 0.28 : 0.56) + (1 - outExpo(s.at(k + 0.8, 0.5))) * 400
  const oy = s.h * (s.p ? 0.5 : 0.47)
  s.shot(ours, ox, oy, ow, { alpha: s.at(k + 0.8, 0.3), bezel: 6 })
  s.text('its own page', px, py - 26, { size: 22, mono: true, color: s.c.muted, alpha: s.at(k, 0.4) })
  s.text('what CueSeek knows', ox, oy - 26, { size: 22, mono: true, color: s.c.accent, alpha: s.at(k + 0.8, 0.4) })
  if (!second) {
    s.text('Sintel · Tears of Steel © Blender Foundation, CC BY 3.0', s.w - 80, s.h - 110, {
      size: 16, mono: true, align: 'right', color: s.c.muted, alpha: 0.8,
    })
  }
}

// bar 7 — attention: one service needs you
function attention(s: Stage) {
  s.label('1 NEEDS ATTENTION')
  const x = s.p ? s.cx : s.w * 0.28
  const y = s.p ? s.h * 0.34 : s.cy
  s.ring(x, y, 130 * outBack(s.at(0, 0.5)), s.c.warn, 1, 4)
  s.ring(x, y, 130 + 160 * out(s.at(0, 2)), s.c.warn, 0.35 * (1 - s.at(0, 2)), 2)
  s.dot(x, y, 34 * outBack(s.at(0.2, 0.4)), s.c.warn)
  const tx = s.p ? s.cx : s.w * 0.42
  const ty = s.p ? s.h * 0.6 : s.cy - 40
  const align = s.p ? 'center' : 'left'
  s.text('qBittorrent', tx, ty, { size: 64, align, alpha: s.at(0.5, 0.3) })
  s.text('Degraded', tx, ty + 70, { size: 46, align, color: s.c.warn, alpha: s.at(1, 0.3) })
  s.text(typed('running but not connected to any peers', s.at(1.5, 1.5)), tx, ty + 140, {
    size: s.p ? 24 : 28, mono: true, align, color: s.c.muted,
  })
}

// bar 8 — control: asked, not done
function control(s: Stage) {
  s.label('SERVICE.CONTROL')
  const w = s.p ? 820 : 760
  const y = s.cy - 120
  const press = (beat: number) => Math.sin(Math.PI * s.at(beat, 0.25)) // a tap
  const stage = s.b < 0.94 ? '' : s.b < 1.94 ? ' — confirm' : ' — asked'
  s.box(s.cx - w / 2, y - 50, w, 100, 50, mix(s.c.card, s.c.accent, 0.25 + 0.3 * (press(0.94) + press(1.94))))
  s.text(`Restart qBittorrent${typed(stage, s.b < 1.94 ? s.at(0.94, 0.3) : s.at(2, 0.5))}`, s.cx, y + 14, {
    size: 40, weight: 500, align: 'center', alpha: s.at(0, 0.3),
  })
  const fixed = s.at(3, 0.3) // the agent reports back: only then is it green
  s.dot(s.cx - 200, y + 160, 12, mix(s.c.warn, s.c.ok, fixed))
  s.text('qBittorrent', s.cx - 172, y + 172, { size: 34, alpha: s.at(0.2, 0.3) })
  s.text(fixed ? 'Running' : 'Degraded', s.cx + 90, y + 172, { size: 34, color: fixed ? s.c.ok : s.c.warn, alpha: s.at(0.2, 0.3) })
  s.text('asked, not done — the agent confirms the outcome', s.cx, y + 260, {
    size: s.p ? 22 : 26, mono: true, align: 'center', color: s.c.muted, alpha: s.at(2.4, 0.5),
  })
}

// bar 9 — the hold: it only acts when you mean it
function hold(s: Stage) {
  s.label('HOST.POWER · 1200 ms')
  const size = s.fit('HOLD TO MEAN IT.', 150, s.w * 0.86, { weight: 500 })
  s.text('HOLD TO MEAN IT.', s.cx, s.cy - 90, { size, weight: 500, align: 'center', alpha: s.at(0, 0.3) })
  const w = s.p ? 860 : 900
  const y = s.cy + 40
  // held from beat 0.6 for 1200 ms (2.4 beats) — let go a breath before the end
  const held = s.b < 2.9 ? s.at(0.6, 2.4) : (1 - s.at(2.9, 0.5)) * 0.96
  s.box(s.cx - w / 2, y, w, 100, 50, mix(s.c.bg, s.c.bad, 0.35))
  s.ctx.save()
  s.ctx.beginPath()
  s.ctx.roundRect(s.cx - w / 2, y, w, 100, 50)
  s.ctx.clip()
  s.box(s.cx - w / 2, y, w * held, 100, 0, s.c.bad)
  s.ctx.restore()
  s.text('Shut down machine — hold', s.cx, y + 64, { size: 40, weight: 500, align: 'center', color: s.c.text })
  s.text(s.b < 2.9 ? 'turning it back on needs someone physically there' : 'let go — nothing happens', s.cx, y + 180, {
    size: s.p ? 22 : 26, mono: true, align: 'center', color: s.c.muted, alpha: s.at(0.6, 0.5),
  })
}

// bar 10 — everywhere: phone, watch and complication, the same answer
function everywhere(s: Stage) {
  s.label('PHONE · WATCH · TILE · COMPLICATION')
  const ph = { x: s.w * (s.p ? 0.08 : 0.14), y: s.h * (s.p ? 0.1 : 0.12), w: s.w * (s.p ? 0.4 : 0.2) }
  s.shot(s.img.dashboard, ph.x, ph.y - (1 - out(s.at(0, 0.5))) * 60, ph.w, { alpha: s.at(0, 0.3), h: ph.w * 1.15 })
  const ww = s.w * (s.p ? 0.44 : 0.24)
  const wx = s.w * (s.p ? 0.5 : 0.52)
  const wy = s.h * (s.p ? 0.5 : 0.26)
  s.shot(s.img.watch, wx, wy - (1 - out(s.at(0.5, 0.5))) * 60, ww, { alpha: s.at(0.5, 0.3), round: true, bezel: 16 })
  const cw = s.w * (s.p ? 0.16 : 0.08)
  s.shot(s.img.complication, s.w * (s.p ? 0.76 : 0.84), s.h * (s.p ? 0.12 : 0.14), cw, { alpha: s.at(1, 0.3), round: true, bezel: 4 })
  // the handoff: one packet along a dotted line, phone to watch
  const a = { x: ph.x + ph.w, y: ph.y + ph.w * 0.6 }
  const b = { x: wx, y: wy + ww / 2 }
  const line = s.at(1.2, 0.4)
  for (let i = 0; i <= 24; i++) s.dot(lerp(a.x, b.x, i / 24), lerp(a.y, b.y, i / 24) + Math.sin(i / 3) * 10, 2.5, s.c.line, line)
  const p = inOut(s.at(1.44, 0.9))
  if (p > 0 && p < 1) s.dot(lerp(a.x, b.x, p), lerp(a.y, b.y, p) + Math.sin(p * 8) * 10, 9, s.c.hi, 1, 20)
  s.text('the same answer, wherever you look', s.cx, s.h * (s.p ? 0.93 : 0.88), {
    size: 28, align: 'center', color: s.c.muted, alpha: s.at(2.5, 0.5),
  })
}

// bar 11 — works with: three tiers, honestly labelled
const TIERS = [
  ['FULL SUPPORT', 'Jellyfin · qBittorrent', 'knows what is going on inside'],
  ['WORKS TODAY', 'Plex · Sonarr · Immich · …', 'anything run as a service'],
  ['PLANNED', 'Plex, in full', 'coming later'],
] as const
function worksWith(s: Stage) {
  s.label('WORKS WITH')
  const gather = inCubic(s.at(3.2, 0.8))
  const n = TIERS.length
  TIERS.forEach(([tier, names, says], i) => {
    const flip = out(s.at(i * 0.3, 0.5))
    const w = s.p ? s.w * 0.8 : s.w * 0.26
    const h = s.p ? 220 : 330
    const x = s.p ? s.cx - w / 2 : s.cx + (i - (n - 1) / 2) * (w + 40) - w / 2
    const y = s.p ? s.h * 0.15 + i * (h + 40) : s.cy - h / 2 - 40
    s.ctx.save()
    s.ctx.translate(lerp(x + w / 2, s.cx, gather), lerp(y + h / 2, s.cy, gather))
    s.ctx.scale(1 - gather, flip * (1 - gather))
    const planned = tier === 'PLANNED'
    s.box(-w / 2, -h / 2, w, h, 22, planned ? undefined : s.c.card, s.c.line)
    s.text(tier, -w / 2 + 36, -h / 2 + 60, { size: 20, mono: true, color: s.c.muted, spacing: 0.14 })
    s.text(flaps(names, s.at(0.6 + i * 0.3, 1)), -w / 2 + 36, -h / 2 + (s.p ? 120 : 150), { size: s.p ? 34 : 25, mono: true })
    s.text(says, -w / 2 + 36, -h / 2 + (s.p ? 170 : 210), { size: 24, color: s.c.muted })
    s.ctx.restore()
  })
  s.text(typed('self-hosted · open source · over your private network', s.at(2, 1)), s.cx, s.h * (s.p ? 0.9 : 0.84), {
    size: s.p ? 22 : 26, mono: true, align: 'center', color: s.c.muted, alpha: 1 - gather,
  })
  s.dot(s.cx, s.cy, 16 * outBack(s.at(3.6, 0.4)), s.c.hi, 1, 24)
}

/** A departure board: each letter ticks through others before it settles. */
function flaps(text: string, p: number) {
  return [...text]
    .map((ch, i) => (ch === ' ' || p * (text.length + 6) > i + 6 ? ch : 'ABCDEFGHJKMNPRSTUVWXYZ'[(i * 7 + Math.floor(p * 40)) % 22]))
    .join('')
}

// bar 12 — the end card: the Swell draws itself, and the dot lands in its gap
function endCard(s: Stage) {
  s.label('APACHE-2.0 · v0.1.4')
  const scale = s.p ? 7 : 7.4
  const mx = s.p ? s.cx - 55 * scale : s.w * 0.36 - 55 * scale
  const my = s.p ? s.h * 0.36 - 57.5 * scale : s.cy - 57.5 * scale
  const g = s.ctx
  const reveal = (from: number, sweep: number) => {
    g.moveTo(55, 57.5)
    for (let i = 0; i <= 40; i++) {
      const a = ((from + (sweep * i) / 40) * Math.PI) / 180
      g.lineTo(55 + 60 * Math.cos(a), 57.5 + 60 * Math.sin(a))
    }
    g.closePath()
  }
  const fade = 1 - s.at(3.6, 0.4)
  g.save()
  g.globalAlpha = fade
  g.translate(mx, my)
  g.scale(scale, scale)
  g.save()
  g.beginPath()
  reveal(134, inOut(s.at(0.05, 0.9)) * 150) // the long arc, up to the gap
  reveal(346, out(s.at(1.1, 0.8)) * 62) // the tail, after the dot
  g.clip()
  g.fillStyle = s.c.hi
  g.fill(new Path2D(LONG))
  g.fill(new Path2D(TAIL))
  g.restore()
  const pop = outBack(s.at(0.95, 0.5)) // the dot lands, on beat one
  if (pop > 0) {
    g.fillStyle = s.c.hi
    g.translate(HANDLE_CENTRE.x, HANDLE_CENTRE.y)
    g.scale(pop, pop)
    g.translate(-HANDLE_CENTRE.x, -HANDLE_CENTRE.y)
    g.fill(new Path2D(HANDLE))
  }
  g.restore()
  const tx = s.p ? s.cx : s.w * 0.36 + 34 * scale
  const ty = s.p ? s.h * 0.66 : s.cy + 30
  const align = s.p ? 'center' : 'left'
  const name = 'CueSeek'
  s.text(typed(name, s.at(1.2, 0.7)), tx, ty, { size: 150, weight: 500, align, alpha: fade })
  s.text('An operations console for', tx, ty + 80, { size: 38, align, color: s.c.muted, alpha: s.at(1.9, 0.4) * fade })
  s.text('self-hosted home servers.', tx, ty + 130, { size: 38, align, color: s.c.muted, alpha: s.at(1.9, 0.4) * fade })
  s.text(typed('cueseek.vercel.app', s.at(2.3, 0.9)), tx, ty + 210, { size: 28, mono: true, align, color: s.c.accent, alpha: fade })
}

export const SCENES = [boot, question, pile, answer, phone, inside, attention, control, hold, everywhere, worksWith, endCard]
