import { useState } from 'react'
import { Hold } from './Hold.tsx'
import { type Action, type Service, type ServiceId, type State, POWER, actionsFor, busy, invoke, verdict } from './sim.ts'

type Sheet =
  | { kind: 'detail'; id: ServiceId }
  | { kind: 'menu'; id: ServiceId }
  | { kind: 'host' }
  | { kind: 'confirm'; target: ServiceId | 'host'; action: Action }
  | null

/**
 * The phone app, redrawn for the demo: the dashboard, a service's sheet, its ⋮ menu, and
 * the confirmation each action asks for — tap-to-confirm for a restart, a hold for anything
 * that takes something down.
 */
export function Phone({ st }: { st: State }) {
  const [sheet, setSheet] = useState<Sheet>(null)
  const v = st.power === 'on' ? verdict(st) : 'Unverified'
  const tone = st.power !== 'on' ? 'off' : v === 'Operational' ? 'ok' : 'warn'
  const services = Object.values(st.services)
  const healthy = services.filter((s) => s.status === 'healthy').length

  const choose = (target: ServiceId | 'host', a: Action) => {
    if (a.risk === 'safe') {
      invoke(target, a.id, 'phone')
      setSheet(null)
    } else setSheet({ kind: 'confirm', target, action: a })
  }

  return (
    <div className="phone" aria-label="The CueSeek phone app">
      <div className="ph-status" aria-hidden="true">
        <span>11:11</span>
        <span className="ph-icons">▾ ▮▮▮ 73%</span>
      </div>

      <div className="ph-top">
        <div>
          <div className="ph-host">HomeServer</div>
          <div className="ph-verdict" data-tone={tone}>
            {v}
          </div>
        </div>
        <button type="button" className="ph-dots" aria-label="Machine and appearance" onClick={() => setSheet({ kind: 'host' })} disabled={st.power !== 'on'}>
          ⋮
        </button>
      </div>
      <div className="ph-bar" data-tone={tone} />
      <div className="ph-tally">
        <span className="ph-count">{st.power === 'on' ? (<><Check /> {healthy}</>) : '—'}</span>
        <span data-live={st.power === 'on' || undefined}>{st.power === 'on' ? '● live' : st.power === 'restarting' ? 'restarting…' : 'can’t reach HomeServer'}</span>
      </div>

      <div className="ph-vitals" data-dim={st.power !== 'on' || undefined}>
        {[
          ['CPU', st.vitals.cpu, '4 cores', `up ${uptime(st.vitals.up)}`],
          ['MEM', st.vitals.mem, `${(4 * (1 - st.vitals.mem / 100)).toFixed(1)} GB free`, '0.4 queued'],
          ['/', st.vitals.disk, '94.4 GB free', `${st.vitals.temp}°C coretemp`],
        ].map(([k, pct, a, b]) => (
          <div key={k as string}>
            <div className="ph-v-head">
              <span>{k}</span>
              <b>{pct}%</b>
            </div>
            <div className="ph-v-bar">
              <i style={{ width: `${pct}%` }} />
            </div>
            <div className="ph-v-sub">{a}</div>
            <div className="ph-v-sub">{b}</div>
          </div>
        ))}
      </div>

      <ul className="ph-services" data-dim={st.power !== 'on' || undefined}>
        {services.map((s) => (
          <li key={s.id}>
            <button type="button" className="ph-row" onClick={() => setSheet({ kind: 'detail', id: s.id })} disabled={st.power !== 'on'}>
              <Badge s={s} asked={st.asked?.service === s.id} />
              <span className="ph-name">{s.name}</span>
              <span className="ph-activity" data-tone={s.status}>
                {st.asked?.service === s.id ? `${st.asked.label} — asked` : s.activity}
              </span>
              <span className="ph-age">{s.age}s</span>
            </button>
            <button type="button" className="ph-dots ph-row-dots" aria-label={`Actions for ${s.name}`} onClick={() => setSheet({ kind: 'menu', id: s.id })} disabled={st.power !== 'on' || !!st.asked}>
              ⋮
            </button>
          </li>
        ))}
      </ul>

      {sheet && <div className="ph-scrim" onClick={() => setSheet(null)} />}

      {sheet?.kind === 'menu' && (
        <div className="ph-menu" role="menu">
          {actionsFor(st.services[sheet.id]).map((a) => (
            <button key={a.id} type="button" role="menuitem" data-risk={a.risk} onClick={() => choose(sheet.id, a)}>
              {a.label}
            </button>
          ))}
          <button type="button" role="menuitem" disabled title="Not in the demo: there is no real web page behind it">
            Open web interface
          </button>
        </div>
      )}

      {sheet?.kind === 'host' && (
        <div className="ph-menu ph-menu-host" role="menu">
          <div className="ph-menu-label">Machine</div>
          {POWER.map((a) => (
            <button key={a.id} type="button" role="menuitem" onClick={() => choose('host', a)}>
              {a.label} <span aria-hidden="true">⏻</span>
            </button>
          ))}
        </div>
      )}

      {sheet?.kind === 'detail' && <Detail s={st.services[sheet.id]} />}

      {sheet?.kind === 'confirm' && (
        <div className="ph-dialog" role="dialog" aria-modal="true" aria-label={`${sheet.action.label}?`}>
          <h3>{sheet.action.label}?</h3>
          <p>{sheet.action.description}</p>
          {sheet.target === 'host' && busy(st) && <p className="ph-busy">{busy(st)}</p>}
          {sheet.action.risk === 'destructive' ? (
            <>
              <Hold
                label={sheet.action.label}
                onConfirmed={() => {
                  invoke(sheet.target, sheet.action.id, 'phone')
                  setSheet(null)
                }}
              />
              <button type="button" className="ph-text" onClick={() => setSheet(null)}>
                Cancel
              </button>
            </>
          ) : (
            <div className="ph-buttons">
              <button type="button" className="ph-text" onClick={() => setSheet(null)}>
                Cancel
              </button>
              <button
                type="button"
                className="ph-text ph-confirm"
                onClick={() => {
                  invoke(sheet.target, sheet.action.id, 'phone')
                  setSheet(null)
                }}
              >
                {sheet.action.label}
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function Badge({ s, asked }: { s: Service; asked: boolean }) {
  return (
    <span className="ph-badge" data-tone={s.status} data-asked={asked || undefined} aria-hidden="true">
      {s.status === 'healthy' ? <Check /> : s.status === 'degraded' ? '!' : '✕'}
    </span>
  )
}

/** The apps' check mark, drawn: the text fonts do not carry one. */
function Check() {
  return (
    <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true">
      <path d="M3 8.5l3.2 3.2L13 4.8" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function Detail({ s }: { s: Service }) {
  const word = s.status === 'healthy' ? 'Healthy' : s.status === 'degraded' ? 'Degraded' : 'Unreachable'
  return (
    <div className="ph-sheet" role="dialog" aria-label={s.name}>
      <div className="ph-grab" />
      <h3>{s.name}</h3>
      <div className="ph-label">Health</div>
      <div className="ph-health">
        <span className="ph-badge" data-tone={s.status}>
          {s.status === 'healthy' ? <Check /> : s.status === 'degraded' ? '!' : '✕'}
        </span>
        <div>
          <div className="ph-health-word" data-tone={s.status}>
            {word}
          </div>
          <div className="ph-age">{s.age}s ago</div>
        </div>
      </div>
      <p className="ph-reason">{s.reason ?? 'The agent reached it at that moment.'}</p>
      <div className="ph-label">Controls</div>
      <p>{actionsFor(s).map((a) => a.label).join(', ')} — from the row’s ⋮ menu.</p>
      {s.playing && (
        <>
          <div className="ph-label">Now Playing</div>
          <p className="ph-mono">1 session · direct play</p>
          <p className="ph-title">{s.playing.title}</p>
          <p className="ph-sub">{s.playing.detail}</p>
          <div className="ph-progress">
            <i style={{ width: `${(s.playing.at / s.playing.length) * 100}%` }} />
          </div>
          <p className="ph-mono">
            {clock(s.playing.at)} / {clock(s.playing.length)}
          </p>
        </>
      )}
      {s.transfers && s.running && (
        <>
          <div className="ph-label">Transfers</div>
          {s.transfers.map((t) => (
            <div key={t.name}>
              <p className="ph-title">{t.name}</p>
              <div className="ph-progress">
                <i style={{ width: `${t.percent}%` }} />
              </div>
              <p className="ph-mono">
                {t.percent}% · {s.status === 'healthy' ? `downloading · ${t.rate}` : 'stalled'}
              </p>
            </div>
          ))}
        </>
      )}
    </div>
  )
}

const clock = (sec: number) => `${Math.floor(sec / 60)}:${String(Math.floor(sec % 60)).padStart(2, '0')}`
function uptime(sec: number) {
  if (sec < 60) return `${sec}s`
  if (sec < 3600) return `${Math.floor(sec / 60)}m`
  return `${Math.floor(sec / 3600)}h`
}
