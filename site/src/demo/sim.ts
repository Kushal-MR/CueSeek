/**
 * A pretend HomeServer, running in this tab (M6.11). Nothing here reaches a real machine.
 *
 * It behaves like the real agent where a visitor can see it: the same verdict words, the
 * same action labels and descriptions (agent/internal/adapters/lifecycle.go, host.go), the
 * same risk tiers, and the same order of events — an action is *asked*, and the dashboard
 * changes only when the server reports back.
 */

export type Status = 'healthy' | 'degraded' | 'unreachable'
export type Risk = 'safe' | 'disruptive' | 'destructive'
export type ServiceId = 'jellyfin' | 'qbittorrent'

export interface Action {
  id: string
  label: string
  description: string
  risk: Risk
}

export interface Service {
  id: ServiceId
  name: string
  port: number
  running: boolean
  status: Status
  reason?: string
  /** What it is doing: the line under its name. */
  activity: string
  playing?: { title: string; detail: string; at: number; length: number }
  transfers?: { name: string; percent: number; rate: string }[]
  /** Seconds since the agent last heard from it. */
  age: number
}

export interface Note {
  id: number
  title: string
  text: string
  cite?: { label: string; href: string }
}

export interface State {
  power: 'on' | 'restarting' | 'off'
  services: Record<ServiceId, Service>
  vitals: { cpu: number; mem: number; disk: number; temp: number; up: number }
  /** An action the server has accepted and not yet reported on. */
  asked: { service?: ServiceId; label: string } | null
  notes: Note[]
}

const ADR = 'https://github.com/Kushal-MR/CueSeek/blob/main/docs/adr/'
const INTERRUPTION: Record<ServiceId, string> = {
  jellyfin: 'Anyone currently watching will be interrupted and will need to resume playback.',
  qbittorrent: 'Active torrents will be paused and will resume when it starts again.',
}

/** The agent's own actions for a unit in a given state (lifecycle.go). */
export function actionsFor(s: Service): Action[] {
  if (!s.running) {
    return [{ id: 'start', label: `Start ${s.name}`, description: `Starts the ${s.name} service.`, risk: 'safe' }]
  }
  return [
    {
      id: 'restart',
      label: `Restart ${s.name}`,
      description: `Restarts the ${s.name} service. ${INTERRUPTION[s.id]} It comes back on its own.`,
      risk: 'disruptive',
    },
    {
      id: 'stop',
      label: `Stop ${s.name}`,
      description: `Stops the ${s.name} service. ${INTERRUPTION[s.id]} It stays stopped until you start it again or the host reboots.`,
      risk: 'destructive',
    },
  ]
}

/** The machine's own actions (host.go). Both destructive: both are held. */
export const POWER: Action[] = [
  { id: 'reboot', label: 'Restart machine', description: 'Everything on this machine stops and comes back in a minute or two.', risk: 'destructive' },
  {
    id: 'poweroff',
    label: 'Shut down machine',
    description: 'The machine turns off and stays off. Turning it back on needs someone physically there.',
    risk: 'destructive',
  },
]

/** One word for everything (core/model/Verdict.kt). */
export function verdict(st: State) {
  const attention = Object.values(st.services).filter((s) => s.status !== 'healthy').length
  if (attention === 1) return '1 needs attention'
  if (attention > 1) return `${attention} need attention`
  return 'Operational'
}

/** "Right now: …" — what an action on the whole machine would interrupt. */
export function busy(st: State) {
  const streams = st.services.jellyfin.playing ? 1 : 0
  const transfers = st.services.qbittorrent.running ? (st.services.qbittorrent.transfers?.length ?? 0) : 0
  const parts = [
    streams && `${streams} stream playing`,
    transfers && `${transfers} transfer${transfers > 1 ? 's' : ''} running`,
  ].filter(Boolean)
  return parts.length ? `Right now: ${parts.join(' and ')}.` : ''
}

function fresh(): State {
  return {
    power: 'on',
    services: {
      jellyfin: { id: 'jellyfin', name: 'Jellyfin', port: 8096, running: true, status: 'healthy', activity: 'Running', age: 4 },
      qbittorrent: {
        id: 'qbittorrent',
        name: 'qBittorrent',
        port: 8080,
        running: true,
        status: 'healthy',
        activity: '1 active · ↓ 10.5 MB/s',
        transfers: [{ name: 'ubuntu-24.04.4-desktop-amd64.iso', percent: 20, rate: '10.5 MB/s' }],
        age: 4,
      },
    },
    vitals: { cpu: 9, mem: 21, disk: 81, temp: 52, up: 3 * 3600 + 12 * 60 },
    asked: null,
    notes: [],
  }
}

// ---- a tiny store

let state = fresh()
let noteId = 0
const listeners = new Set<() => void>()
const timers = new Set<number>()

export const store = {
  get: () => state,
  subscribe(f: () => void) {
    listeners.add(f)
    return () => listeners.delete(f)
  },
}

function set(change: (s: State) => State) {
  state = change(state)
  listeners.forEach((f) => f())
}

function later(ms: number, f: () => void) {
  const t = window.setTimeout(() => {
    timers.delete(t)
    f()
  }, ms)
  timers.add(t)
}

function note(n: Omit<Note, 'id'>) {
  set((s) => ({ ...s, notes: [{ ...n, id: ++noteId }, ...s.notes].slice(0, 6) }))
}

function patch(id: ServiceId, p: Partial<Service>) {
  set((s) => ({ ...s, services: { ...s.services, [id]: { ...s.services[id], ...p, age: 0 } } }))
}

// The live part: vitals drift, ages tick, the film plays, the transfer moves.
let clock = 0
export function start() {
  if (clock) return
  clock = window.setInterval(() => {
    if (state.power !== 'on') return
    set((s) => {
      const j = s.services.jellyfin
      const q = s.services.qbittorrent
      const moving = q.running && q.status === 'healthy' && q.transfers?.length
      const transfers = moving ? q.transfers!.map((t) => ({ ...t, percent: Math.min(100, t.percent + 1) })) : q.transfers
      return {
        ...s,
        vitals: {
          cpu: clamp(Math.round(s.vitals.cpu + (Math.random() - 0.5) * 6 + (j.playing ? 1 : -1)), 4, j.playing ? 34 : 18),
          mem: clamp(Math.round(s.vitals.mem + (Math.random() - 0.5) * 2), 18, 30),
          disk: s.vitals.disk,
          temp: clamp(Math.round(s.vitals.temp + (Math.random() - 0.5) * 2 + (j.playing ? 0.6 : -0.4)), 48, 61),
          up: s.vitals.up + 2,
        },
        services: {
          jellyfin: { ...j, age: j.age + 2, playing: j.playing && { ...j.playing, at: Math.min(j.playing.length, j.playing.at + 2) } },
          qbittorrent: { ...q, age: q.age + 2, transfers },
        },
      }
    })
  }, 2000)
}

// ---- what the visitor does through the apps

/** Invoke an action, as the app would: the server accepts it, and reports back later. */
export function invoke(id: ServiceId | 'host', action: string, from: 'phone' | 'watch') {
  if (state.power !== 'on' || state.asked) return
  if (id === 'host') return power(action, from)
  const svc = state.services[id]
  const label = actionsFor(svc).find((a) => a.id === action)?.label ?? action
  set((s) => ({ ...s, asked: { service: id, label } }))
  if (action === 'restart') {
    note({
      title: 'It says "asked", not "done".',
      text: `The ${from} sent the request and the agent accepted it — that is all the app knows yet. The row changes only when the server reports what actually happened.`,
      cite: { label: 'ADR-0004', href: `${ADR}0004-contract-openapi-sse.md` },
    })
  }
  if (action === 'stop') {
    note({
      title: 'Stopping takes a hold.',
      text: 'Restart asks you to confirm; Stop — which does not undo itself — needs a 1.2-second hold. The risk comes from the agent with the action, so the app never has to guess.',
      cite: { label: 'ADR-0005', href: `${ADR}0005-capability-based-adapters.md` },
    })
  }
  later(1600, () => {
    set((s) => ({ ...s, asked: null }))
    if (action === 'stop') return down(id, 'stopped')
    patch(id, {
      running: true,
      status: 'healthy',
      reason: undefined,
      activity: id === 'jellyfin' ? 'Running' : '1 active · ↓ 10.5 MB/s',
      playing: undefined,
    })
    if (id === 'qbittorrent') patch(id, { transfers: [{ name: 'ubuntu-24.04.4-desktop-amd64.iso', percent: state.services.qbittorrent.transfers?.[0]?.percent ?? 20, rate: '10.5 MB/s' }] })
    note({
      title: action === 'start' ? `${svc.name} is back.` : `${svc.name} is running again.`,
      text: 'The server confirmed it — so now, and only now, the row is green.',
    })
  })
}

function down(id: ServiceId, why: 'stopped' | 'crashed') {
  const name = state.services[id].name
  patch(id, {
    running: false,
    status: 'unreachable',
    reason: `Could not reach ${name}: connection refused.`,
    activity: why === 'stopped' ? 'Stopped' : 'Not responding',
    playing: undefined,
  })
}

function power(action: string, from: 'phone' | 'watch') {
  if (action === 'poweroff') {
    set((s) => ({ ...s, asked: { label: 'Shut down machine' } }))
    later(1400, () => {
      set((s) => ({ ...s, asked: null, power: 'off' }))
      note({
        title: 'HomeServer is off.',
        text: `Asked from the ${from}, and done. In real life, you would now walk over and press its power button — which is why this one needs a hold, and why the watch cannot do it unless you allow it.`,
        cite: { label: 'ADR-0002', href: `${ADR}0002-host-privilege-dbus-polkit.md` },
      })
    })
    return
  }
  set((s) => ({ ...s, asked: { label: 'Restart machine' } }))
  later(1400, () => {
    set((s) => ({ ...s, asked: null, power: 'restarting' }))
    note({ title: 'Restarting.', text: 'The phone simply waits: no agent, no answer — and it says so rather than guessing.' })
    later(5000, () => set((s) => ({ ...fresh(), notes: s.notes, vitals: { ...fresh().vitals, up: 0 } })))
  })
}

// ---- the scenarios: things that happen to a server, not things you do to it

export const scenarios = {
  disconnect() {
    if (state.power !== 'on' || !state.services.qbittorrent.running) return
    patch('qbittorrent', {
      status: 'degraded',
      reason: 'qBittorrent is running but not connected to any peers. Check the host’s network and qBittorrent’s connection settings.',
      activity: 'Not connected to any peers',
    })
    note({
      title: 'Why "Degraded", not "Down"?',
      text: 'qBittorrent is running — it just cannot reach anyone. CueSeek asks qBittorrent itself, so "running" and "working" are different answers. (Firewalled alone would not turn it amber: that is often a deliberate choice.)',
      cite: { label: 'ADR-0008', href: `${ADR}0008-multi-host-and-computed-health.md` },
    })
  },
  crash() {
    if (state.power !== 'on' || !state.services.jellyfin.running) return
    down('jellyfin', 'crashed')
    note({
      title: 'One answer, worked out once.',
      text: 'The verdict now reads "1 needs attention" — on the phone and on the watch at the same moment, because the server works it out and both just show it.',
      cite: { label: 'ADR-0008', href: `${ADR}0008-multi-host-and-computed-health.md` },
    })
  },
  film() {
    const j = state.services.jellyfin
    if (state.power !== 'on' || !j.running || j.playing) return
    patch('jellyfin', {
      activity: '1 playing',
      playing: { title: 'Sintel', detail: '2010 · Demo · Chrome', at: 0, length: 888 },
    })
    note({
      title: 'Knows what is going on inside.',
      text: 'Jellyfin is a fully supported service: CueSeek reads who is watching what from Jellyfin’s own API. Try restarting the machine now — it will tell you a stream is playing first.',
      cite: { label: 'ADR-0005', href: `${ADR}0005-capability-based-adapters.md` },
    })
  },
  reset() {
    timers.forEach(clearTimeout)
    timers.clear()
    set(() => fresh())
  },
}

export function watchHandoff() {
  note({
    title: 'Same answer, on the wrist.',
    text: 'The watch asks the server directly — its own token, its own permissions. The phone only told it where the server is.',
    cite: { label: 'ADR-0014', href: `${ADR}0014-watch-pairing-address-handoff.md` },
  })
}

const clamp = (v: number, a: number, b: number) => Math.max(a, Math.min(b, v))
