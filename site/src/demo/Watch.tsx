import { useState } from 'react'
import { Hold } from './Hold.tsx'
import { type ServiceId, type State, actionsFor, invoke, verdict, watchHandoff } from './sim.ts'

/**
 * The watch app, redrawn for the demo: the verdict, the services, and a service's own
 * screen — restart with a tap and a confirm, stop with a hold. Like the real one it has no
 * power controls unless the operator grants them (clients/wear/README.md), so it has none.
 */
export function Watch({ st }: { st: State }) {
  const [open, setOpen] = useState<ServiceId | null>(null)
  const [confirming, setConfirming] = useState(false)
  const services = Object.values(st.services)
  const healthy = services.filter((s) => s.status === 'healthy').length
  const on = st.power === 'on'
  const v = on ? verdict(st) : 'Unverified'
  const s = open ? st.services[open] : null

  const act = (id: ServiceId, action: string) => {
    invoke(id, action, 'watch')
    watchHandoff()
    setConfirming(false)
  }

  return (
    <div className="watch-demo" aria-label="The CueSeek watch app">
      <div className="wd-face">
        <div className="wd-time">11:11</div>
        {!s || !on ? (
          <div className="wd-scroll">
            <div className="wd-host">HomeServer</div>
            <div className="wd-verdict" data-tone={!on ? 'off' : v === 'Operational' ? 'ok' : 'warn'}>
              {v}
            </div>
            <div className="wd-count">{on ? `${healthy}/${services.length} healthy` : 'can’t reach it'}</div>
            {on &&
              services.map((x) => (
                <button key={x.id} type="button" className="wd-row" onClick={() => setOpen(x.id)}>
                  <i data-tone={x.status} />
                  <span>
                    {x.name}
                    <small>{st.asked?.service === x.id ? 'asked…' : x.activity}</small>
                  </span>
                </button>
              ))}
          </div>
        ) : (
          <div className="wd-scroll">
            <button type="button" className="wd-back" onClick={() => (setOpen(null), setConfirming(false))} aria-label="Back">
              ‹
            </button>
            <div className="wd-title">{s.name}</div>
            <div className="wd-state" data-tone={s.status}>
              {st.asked?.service === s.id ? 'Asked…' : s.running ? (s.status === 'degraded' ? 'Degraded' : 'Running') : 'Stopped'}
            </div>
            {s.playing && (
              <div className="wd-playing">
                <small>Playing</small>
                {s.playing.title}
                <small>{s.playing.detail}</small>
              </div>
            )}
            {!st.asked &&
              actionsFor(s).map((a) =>
                a.risk === 'destructive' ? (
                  <Hold key={a.id} className="wd-hold" label={a.label} onConfirmed={() => act(s.id, a.id)} />
                ) : a.risk === 'disruptive' && confirming ? (
                  <div key={a.id} className="wd-confirm">
                    <button type="button" onClick={() => setConfirming(false)}>
                      Cancel
                    </button>
                    <button type="button" className="wd-yes" onClick={() => act(s.id, a.id)}>
                      Confirm
                    </button>
                  </div>
                ) : (
                  <button key={a.id} type="button" className="wd-btn" onClick={() => (a.risk === 'safe' ? act(s.id, a.id) : setConfirming(true))}>
                    {a.label}
                  </button>
                ),
              )}
          </div>
        )}
      </div>
    </div>
  )
}
