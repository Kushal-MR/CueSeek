import { useEffect, useSyncExternalStore } from 'react'
import { Mark } from '../Mark.tsx'
import { Knob } from '../theme/Knob.tsx'
import { Phone } from './Phone.tsx'
import { Watch } from './Watch.tsx'
import { scenarios, start, store } from './sim.ts'
import './demo.css'

const TRY = [
  { icon: '⚡', label: 'qBittorrent loses its connection', run: scenarios.disconnect },
  { icon: '✕', label: 'Jellyfin crashes', run: scenarios.crash },
  { icon: '▶', label: 'Someone starts a film', run: scenarios.film },
  { icon: '↺', label: 'Reset everything', run: scenarios.reset },
]

/**
 * The demo page: a pretend HomeServer, the phone and the watch side by side, things to make
 * happen to it, and a note for each thing that happens saying why CueSeek does what it does.
 */
export function Demo() {
  const st = useSyncExternalStore(store.subscribe, store.get)
  useEffect(start, [])

  return (
    <>
      <header className="nav">
        <a className="brand" href="./">
          <Mark size={20} />
          CueSeek
        </a>
        <nav aria-label="Demo">
          <a href="./">← Back to the site</a>
        </nav>
        <Knob />
      </header>

      <main className="demo">
        <p className="demo-banner">
          <b>Demo</b> — a pretend server called HomeServer, running in this tab. Nothing here reaches a real machine.
        </p>

        <div className="demo-grid">
          <aside className="demo-try" aria-label="Things to try">
            <h1>Break a server. Nothing will happen.</h1>
            <h2>Make something happen</h2>
            <ul>
              {TRY.map((t) => (
                <li key={t.label}>
                  <button type="button" onClick={t.run}>
                    <span aria-hidden="true">{t.icon}</span> {t.label}
                  </button>
                </li>
              ))}
            </ul>
            <h2>Then fix it</h2>
            <p>
              Everything on the phone and the watch can be pressed. Fix a service from its <b>⋮</b> menu, or tap it on
              the watch. The machine’s own <b>⋮</b> can restart it — or shut it down, with a hold.
            </p>
          </aside>

          <div className="demo-devices">
            <Phone st={st} />
            <Watch st={st} />
          </div>

          <section className="demo-notes" aria-label="Why" aria-live="polite">
            <h2>Why</h2>
            {st.notes.length === 0 && <p className="demo-empty">Make something happen, and the reason CueSeek reacts the way it does appears here.</p>}
            <ol>
              {st.notes.map((n) => (
                <li key={n.id}>
                  <h3>{n.title}</h3>
                  <p>{n.text}</p>
                  {n.cite && <a href={n.cite.href}>{n.cite.label} →</a>}
                </li>
              ))}
            </ol>
          </section>
        </div>

        {st.power === 'off' && (
          <div className="demo-off" role="alert">
            <p className="demo-off-big">HomeServer is off.</p>
            <p>In real life, you would now walk over and press its power button.</p>
            <button type="button" onClick={scenarios.reset}>
              Press the power button
            </button>
          </div>
        )}
      </main>
    </>
  )
}
