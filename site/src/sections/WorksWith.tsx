import { useEffect, useRef, useState } from 'react'
import jellyfinPage from '../assets/screens/web-jellyfin.webp'
import qbittorrentPage from '../assets/screens/web-qbittorrent.webp'
import jellyfinNow from '../assets/screens/cueseek-jellyfin-now.webp'
import qbittorrentTransfers from '../assets/screens/cueseek-qbittorrent-transfers.webp'
import { useCue } from '../traveller/useCue.ts'
import { LAND_EVENT, arrivedGoing, dotTo } from '../traveller/traveller.ts'
import './WorksWith.css'

/** Full support: health from the service's own API, and what it is doing (docs/configure.md). */
const FULL = [
  {
    id: 'jellyfin',
    name: 'Jellyfin',
    what: 'who is watching what',
    page: jellyfinPage,
    pageAlt: "Jellyfin's own web page: the Demo user's home, with Tears of Steel and Sintel.",
    ours: jellyfinNow,
    oursAlt: 'CueSeek, on the same moment: Now Playing — Tears of Steel, 2012, Demo, Chrome, 0:54 of 12:14, direct play.',
  },
  {
    id: 'qbittorrent',
    name: 'qBittorrent',
    what: 'what is moving, how fast',
    page: qbittorrentPage,
    pageAlt: "qBittorrent's own web page: the Ubuntu and Debian installer images, one downloading and one seeding.",
    ours: qbittorrentTransfers,
    oursAlt: 'CueSeek, on the same moment: Transfers — the Ubuntu image 20% at 10.5 MB/s, the Debian image complete.',
  },
] as const
type Full = (typeof FULL)[number]['id']

/** The basic tier: anything run as a systemd unit. Examples from docs/configure.md. */
const BASIC = ['Plex', 'Sonarr · Radarr', 'Immich', 'Syncthing · Vaultwarden', 'Samba · Postgres · …']

/**
 * Section 03. What CueSeek works with, in three honestly labelled tiers.
 *
 * The cards rise in left to right; the basic tier's names tick into place once, like a
 * departure board. The full-support card shows a service's own web page beside what CueSeek
 * says about the same moment — the staged captures from M6.6.
 */
export function WorksWith() {
  const [ref, seen] = useCue<HTMLElement>('works-with', 0.25)
  const [shown, setShown] = useState<Full>('jellyfin')
  const card = useRef<HTMLDivElement>(null)

  // The dot's job here: it is the selection light. It lands on a service's status light and
  // that service's page turns up; it hops to the other and the page flips. Afterwards it
  // follows whichever service the reader points at.
  useEffect(() => {
    const light = (id: Full) => card.current?.querySelector(`[data-svc="${id}"] .light`) ?? null
    let timer = 0
    let route: Full[] = []
    const onLand = (e: Event) => {
      if ((e as CustomEvent<string>).detail !== 'works-with') return
      route = arrivedGoing === 'down' ? ['jellyfin', 'qbittorrent'] : ['qbittorrent', 'jellyfin']
      dotTo(light(route[0]), { scale: 0.36, dur: 220 })
    }
    const onArrive = (e: Event) => {
      const el = (e as CustomEvent<Element>).detail
      const id = el instanceof Element ? (el.closest('[data-svc]') as HTMLElement | null)?.dataset.svc : undefined
      if (id !== 'jellyfin' && id !== 'qbittorrent') return
      setShown(id)
      if (route.length && id === route[0]) {
        const second = route[1]
        route = []
        clearTimeout(timer)
        timer = window.setTimeout(() => dotTo(light(second), { scale: 0.36, dur: 320 }), 2600)
      }
    }
    addEventListener(LAND_EVENT, onLand)
    addEventListener('cueseek:dot-arrived', onArrive)
    return () => {
      removeEventListener(LAND_EVENT, onLand)
      removeEventListener('cueseek:dot-arrived', onArrive)
      clearTimeout(timer)
    }
  }, [])

  // Pointing at a service shows it; if the dot is here, it moves there too. (It ignores the
  // request if it is in another section — dotTo is section-scoped.)
  const pick = (id: Full) => {
    setShown(id)
    dotTo(card.current?.querySelector(`[data-svc="${id}"] .light`) ?? null, { scale: 0.36, dur: 260 })
  }

  return (
    <section
      ref={ref}
      id="works-with"
      className="works"
      data-in={seen || undefined}
      aria-labelledby="works-with-title"
    >
      <span className="hud">Section 03 — Works with</span>
      <h2 id="works-with-title">Works with what you already run.</h2>

      <div className="tiers">
        <article ref={card} className="tier full" style={{ '--i': 0 } as React.CSSProperties}>
          <h3 className="tier-name">Full support</h3>
          <p className="tier-says">Knows what's going on inside</p>
          <ul className="svcs">
            {FULL.map((s, i) => (
              <li key={s.id} data-svc={s.id}>
                <button
                  type="button"
                  className="svc"
                  aria-pressed={shown === s.id}
                  onMouseEnter={() => pick(s.id)}
                  onFocus={() => pick(s.id)}
                  onClick={() => pick(s.id)}
                >
                  <span
                    className="light"
                    aria-hidden="true"
                    data-station={i === 0 ? 'works-with' : undefined}
                    data-entry-up={i === 0 ? '[data-svc="qbittorrent"] .light' : undefined}
                  />
                  <span className="name">{s.name}</span>
                  <span className="what">{s.what}</span>
                  <span className="go" aria-hidden="true">›</span>
                </button>
              </li>
            ))}
          </ul>
          <p className="svcs-hint" aria-hidden="true">
            <span className="fine">Point at</span>
            <span className="coarse">Tap</span> a service to see its own page
          </p>

          <figure className="peek" data-show={shown}>
            <div className="peek-turn">
              {FULL.map((s) => (
                <div key={s.id} className={`face ${s.id}`} aria-hidden={shown !== s.id}>
                  <span className="tag theirs">its own page</span>
                  <img className="page" src={s.page} alt={s.pageAlt} loading="lazy" />
                  <div className="ours">
                    <span className="tag">in CueSeek</span>
                    <img src={s.ours} alt={s.oursAlt} loading="lazy" />
                  </div>
                </div>
              ))}
            </div>
            <figcaption>
              Health comes from the service's own API, so "running" and "working" are different
              answers.
            </figcaption>
          </figure>
        </article>

        <article className="tier today" style={{ '--i': 1 } as React.CSSProperties}>
          <h3 className="tier-name">Works today</h3>
          <p className="tier-says">Anything your server runs as a service</p>
          <p className="board-head">
            <span>each one</span>
            <span>up · start · stop · restart</span>
          </p>
          <ul className="board">
            {BASIC.map((name, i) => (
              <li key={name}>
                <Flaps text={name} go={seen} delay={700 + i * 260} />
              </li>
            ))}
          </ul>
          <p className="tier-note">Is it running, and a button to fix it when it isn't.</p>
        </article>

        <article className="tier planned" style={{ '--i': 2 } as React.CSSProperties}>
          <h3 className="tier-name">Planned</h3>
          <p className="tier-says">Coming later</p>
          <ul className="board">
            <li>
              <span className="flaps">Plex, in full</span>
              <span className="pill">planned</span>
            </li>
          </ul>
          <p className="tier-note">
            Moving a service up a tier is a server-side change — the app on your phone doesn't
            need an update.
          </p>
        </article>
      </div>

      <p className="machine">
        And the machine itself: processor, memory, disks, temperature — and reboot or shut down,
        if you allow it.
      </p>
      <p className="credit">
        Films shown: <i>Sintel</i> and <i>Tears of Steel</i>, © Blender Foundation,{' '}
        <a href="https://creativecommons.org/licenses/by/3.0/">CC BY 3.0</a>.
      </p>
    </section>
  )
}

const FLAP = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz'

/**
 * A name that ticks into place once, letter by letter, like a departure board, then stops.
 * Readers (and reduced motion) get the plain name; the flicker is decoration only.
 */
function Flaps({ text, go, delay }: { text: string; go: boolean; delay: number }) {
  const [still] = useState(() => matchMedia('(prefers-reduced-motion: reduce)').matches)
  const [shown, setShown] = useState(() => (still ? text : text.replace(/[^ ·…]/g, ' ')))

  useEffect(() => {
    if (!go || still) return
    let tick = 0
    let iv = 0
    const start = window.setTimeout(() => {
      iv = window.setInterval(() => {
        tick += 1
        let done = true
        setShown(
          [...text]
            .map((ch, i) => {
              if (/[ ·…]/.test(ch) || tick > 4 + i * 1.5) return ch
              done = false
              return FLAP[Math.floor(Math.random() * FLAP.length)]
            })
            .join(''),
        )
        if (done) clearInterval(iv)
      }, 45)
    }, delay)
    return () => {
      clearTimeout(start)
      clearInterval(iv)
    }
  }, [go, still, text, delay])

  return (
    <span className="flaps">
      <span className="sr">{text}</span>
      <span aria-hidden="true">{shown}</span>
    </span>
  )
}
