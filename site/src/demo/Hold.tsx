import { useEffect, useRef, useState } from 'react'

/** The apps' threshold (ServiceActionMenu.kt, ActionControls.kt): measured on a real wrist. */
export const HOLD_MS = 1200

/**
 * A button that must be held, as in the apps. Timed by the clock, not by the fill — the fill
 * is only a picture of the wait — so letting go early can never confirm. Keyboard: hold
 * Space or Enter.
 */
export function Hold({ label, onConfirmed, className = '' }: { label: string; onConfirmed: () => void; className?: string }) {
  const [holding, setHolding] = useState(false)
  const timer = useRef(0)
  useEffect(() => () => clearTimeout(timer.current), [])

  const begin = () => {
    if (holding) return
    setHolding(true)
    timer.current = window.setTimeout(() => {
      setHolding(false)
      onConfirmed()
    }, HOLD_MS)
  }
  const end = () => {
    clearTimeout(timer.current)
    setHolding(false)
  }

  return (
    <button
      type="button"
      className={`hold ${className}`}
      data-holding={holding || undefined}
      style={{ '--hold': `${HOLD_MS}ms` } as React.CSSProperties}
      onPointerDown={(e) => {
        e.currentTarget.setPointerCapture(e.pointerId)
        begin()
      }}
      onPointerUp={end}
      onPointerCancel={end}
      onKeyDown={(e) => {
        if ((e.key === ' ' || e.key === 'Enter') && !e.repeat) {
          e.preventDefault()
          begin()
        }
      }}
      onKeyUp={(e) => (e.key === ' ' || e.key === 'Enter') && end()}
      onContextMenu={(e) => e.preventDefault()}
    >
      <span className="hold-fill" aria-hidden="true" />
      <span className="hold-label">{holding ? 'Hold…' : `${label} — hold`}</span>
    </button>
  )
}
