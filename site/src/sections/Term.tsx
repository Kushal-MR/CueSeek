import { useId, useState, type ReactNode } from 'react'
import './Term.css'

/**
 * A word a newcomer might not know, with its one-line meaning a tap away.
 *
 * Only three words on the page get this — home server, agent, private network — because
 * those are the three a stranger needs to follow the story. Everything else is plain words.
 */
export function Term({ children, means }: { children: ReactNode; means: string }) {
  const [open, setOpen] = useState(false)
  const id = useId()
  return (
    <span className="term" onMouseLeave={() => setOpen(false)}>
      <button
        type="button"
        aria-expanded={open}
        aria-describedby={open ? id : undefined}
        onClick={() => setOpen((o) => !o)}
        onMouseEnter={() => setOpen(true)}
        onBlur={() => setOpen(false)}
      >
        {children}
      </button>
      {open && (
        <span role="tooltip" id={id} className="term-tip">
          <b>{children}</b> — {means}
        </span>
      )}
    </span>
  )
}
