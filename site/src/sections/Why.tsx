import { Fragment, useEffect, useRef, useState } from 'react'
import QUOTES from 'virtual:why-quotes'
import { useCue } from '../traveller/useCue.ts'
import { LAND_EVENT, arrivedGoing, dotTo } from '../traveller/traveller.ts'
import './Why.css'

const BLOB = 'https://github.com/Kushal-MR/CueSeek/blob/main/docs/'

/**
 * Each note: a plain claim, the reason, and the record that decided it. Every claim was
 * checked against its record; the excerpt shown on opening is the record's own text.
 */
const NOTES = [
  {
    claim: 'Never on the open internet.',
    why: 'The server answers only over your private network. No port forwarding, nothing for a stranger to find.',
    cite: 'ADR-0001',
    href: `${BLOB}adr/0001-vpn-only-remote-access.md`,
    quote: QUOTES[0],
  },
  {
    claim: 'Never root.',
    why: 'The agent runs as an ordinary user. It can restart exactly the services you list, and nothing else — the operating system enforces it, not the app.',
    cite: 'ADR-0002',
    href: `${BLOB}adr/0002-host-privilege-dbus-polkit.md`,
    quote: QUOTES[1],
  },
  {
    claim: 'The server decides what "fine" means.',
    why: 'Health is worked out once, on the server, so your phone and your watch can never disagree about it.',
    cite: 'ADR-0008',
    href: `${BLOB}adr/0008-multi-host-and-computed-health.md`,
    quote: QUOTES[2],
  },
  {
    claim: 'It says "asked", not "done".',
    why: 'Pressing Restart sends a request. The app reports what the server later confirms — never a success it didn’t see.',
    cite: 'ADR-0004',
    href: `${BLOB}adr/0004-contract-openapi-sse.md`,
    quote: QUOTES[3],
  },
  {
    claim: 'Shutting down takes a hold.',
    why: 'Every action carries its own risk level. A restart asks you to confirm; stopping a service or turning the machine off needs a deliberate press-and-hold.',
    cite: 'ADR-0005',
    href: `${BLOB}adr/0005-capability-based-adapters.md`,
    quote: QUOTES[4],
  },
  {
    claim: 'Tested on a real wrist.',
    why: 'A day on a real watch against a real server found what the desk couldn’t: a clipped button, and a hold that could be held twice.',
    cite: 'Verification record',
    href: `${BLOB}m5-verification.md`,
    quote: QUOTES[5],
  },
]

const ALL: ReadonlySet<number> = new Set(NOTES.map((_, i) => i))

/** Just enough Markdown for a quotation: `code`, **bold**, [links](…) as their text, list dashes. */
function Inline({ text }: { text: string }) {
  const clean = text.replace(/^- /, '').replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
  return (
    <>
      {clean.split(/(`[^`]+`|\*\*[^*]+\*\*)/).map((part, i) =>
        part.startsWith('`') ? (
          <code key={i}>{part.slice(1, -1)}</code>
        ) : part.startsWith('**') ? (
          <b key={i}>{part.slice(2, -2).replace(/`/g, '')}</b>
        ) : (
          <Fragment key={i}>{part}</Fragment>
        ),
      )}
    </>
  )
}

/**
 * Section 06. Why it's built this way: six notes, each a claim, its reason, and the record
 * that decided it. Opening one shows the record's own words in place.
 */
export function Why() {
  const [ref, seen] = useCue<HTMLElement>('why', 0.25)
  const [open, setOpen] = useState<number | null>(null)
  const list = useRef<HTMLOListElement>(null)
  const pager = useRef<HTMLDivElement>(null)
  // On a phone the notes are a row to swipe, one in view: there the dot is the page marker.
  const swiping = () => matchMedia('(max-width: 640px)').matches
  const mark = (i: number) =>
    (swiping() ? pager.current?.querySelectorAll('i')[i] : list.current?.querySelectorAll('.why-mark')[i]) ?? null

  /** The note in view, in the swiping row. */
  const current = () => {
    const el = list.current
    if (!el) return 0
    return Math.round(el.scrollLeft / Math.max(1, el.scrollWidth / NOTES.length))
  }

  // The dot's job here: it deals the notes — each one turns up as the dot lands on its mark —
  // and afterwards it marks the note that is open.
  const [dealt, setDealt] = useState<ReadonlySet<number>>(new Set())
  useEffect(() => {
    let next = -1
    let order: number[] = []
    let timer = 0
    const onLand = (e: Event) => {
      if ((e as CustomEvent<string>).detail !== 'why') return
      if (swiping()) {
        setDealt(ALL)
        return dotTo(mark(current()), { scale: 0.3, dur: 220 })
      }
      order = arrivedGoing === 'down' ? [0, 1, 2, 3, 4, 5] : [5, 4, 3, 2, 1, 0]
      next = 0
      dotTo(mark(order[0]), { scale: 0.36, dur: 380 })
    }
    const onArrive = (e: Event) => {
      const el = (e as CustomEvent<Element>).detail
      if (next < 0 || !(el instanceof Element) || el !== mark(order[next])) return
      const i = order[next]
      setDealt((d) => new Set(d).add(i))
      // Each note answers the dot: a small pop as it lands — on the first deal, and every visit.
      const note = el.closest('.why-note')
      if (note) {
        note.classList.remove('tapped')
        void (note as HTMLElement).offsetWidth
        note.classList.add('tapped')
      }
      next += 1
      // Unhurried: each note gets its moment before the dot moves on.
      if (next < order.length) timer = window.setTimeout(() => dotTo(mark(order[next]), { scale: 0.36, dur: 450 }), 380)
      else next = -1
    }
    addEventListener(LAND_EVENT, onLand)
    addEventListener('cueseek:dot-arrived', onArrive)
    return () => {
      removeEventListener(LAND_EVENT, onLand)
      removeEventListener('cueseek:dot-arrived', onArrive)
      clearTimeout(timer)
    }
  }, [])

  // Without the dot (reduced motion, or a reader who scrolled past), the notes are all there.
  useEffect(() => {
    if (!seen) return
    const t = setTimeout(() => setDealt(ALL), 4000)
    return () => clearTimeout(t)
  }, [seen])

  const [inView, setInView] = useState(0)
  const onSwipe = () => {
    const i = current()
    if (i === inView) return
    setInView(i)
    dotTo(mark(i), { scale: 0.3, dur: 200 })
  }

  const toggle = (i: number) => {
    const next = open === i ? null : i
    setOpen(next)
    setDealt(ALL)
    if (next !== null) dotTo(mark(next), { scale: 0.36, dur: 280, track: true })
  }

  return (
    <section ref={ref} id="why" className="why" data-in={seen || undefined} aria-labelledby="why-title">
      <span className="hud">Section 06 — Why</span>
      <h2 id="why-title">Why it’s built this way.</h2>
      <p className="why-sub">Six decisions, each with the record that made it. Open one to read the record’s own words.</p>

      <ol ref={list} className="why-notes" data-station="why" onScroll={onSwipe}>
        {NOTES.map((n, i) => (
          <li key={n.cite} className="why-note" data-open={open === i || undefined} data-dealt={dealt.has(i) || undefined} style={{ '--i': i } as React.CSSProperties}>
            <button type="button" className="why-head" aria-expanded={open === i} aria-controls={`why-${i}`} onClick={() => toggle(i)}>
              <span className="why-mark" aria-hidden="true" />
              <span className="why-n">{String(i + 1).padStart(2, '0')}</span>
              <span className="why-claim">{n.claim}</span>
              <span className="why-why">{n.why}</span>
              <span className="why-open" aria-hidden="true">
                {open === i ? `${n.cite} ↑` : `${n.cite} ↓`}
              </span>
            </button>
            <div id={`why-${i}`} className="why-record" hidden={open !== i}>
              <p className="why-from">From {n.cite}:</p>
              <blockquote>
                {n.quote.map((q, k) => (
                  <p key={k}>
                    <Inline text={q} />
                  </p>
                ))}
              </blockquote>
              <a href={n.href}>Read {n.cite} in full →</a>
            </div>
          </li>
        ))}
      </ol>
      <div ref={pager} className="why-pager" aria-hidden="true">
        {NOTES.map((n, i) => (
          <i key={n.cite} data-on={i === inView || undefined} />
        ))}
      </div>
    </section>
  )
}
