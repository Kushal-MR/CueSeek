import { forwardRef } from 'react'
import mark from '@brand/cueseek-mark.svg?raw'
import { HANDLE_CENTRE } from './geometry.ts'

/**
 * The Swell taken apart into its three pieces, so each can perform on its own: the long arc,
 * the short tail, and the handle — the dot.
 *
 * The outlines come from the file docs/brand/icon.py generates, split here rather than
 * redrawn, so the assembled mark is still exactly the brand's. The two arcs are revealed by
 * masks: a wide stroke along the dial's track (centre 55, 57.5, radius 24 — DESIGN.md §13)
 * that draws on like a pen, uncovering the tapered shape beneath it.
 */

const d = mark.match(/ d="([^"]+)"/)![1]
// Three subpaths, each starting with its own `M`.
const [LONG, TAIL, HANDLE] = d.split(/ (?=M )/)

const C = { x: 55, y: 57.5, r: 24 }
const at = (deg: number) => {
  const a = (deg * Math.PI) / 180
  return `${(C.x + C.r * Math.cos(a)).toFixed(2)} ${(C.y + C.r * Math.sin(a)).toFixed(2)}`
}
/** Screen angles: 0° is three o'clock, increasing clockwise. Each runs a little past its
 *  piece's ends so the rounded caps are fully uncovered. */
const LONG_TRACK = `M ${at(128)} A ${C.r} ${C.r} 0 0 1 ${at(290)}` // 7:30, up the left, to the gap
const TAIL_TRACK = `M ${at(52)} A ${C.r} ${C.r} 0 0 0 ${at(-20)}` // 4:30, up the right


export const SwellParts = forwardRef<
  SVGSVGElement,
  { size: number; className?: string; onPointerDown?: () => void }
>(
  function SwellParts({ size, className, onPointerDown }, ref) {
    return (
      <svg
        ref={ref}
        className={className}
        viewBox="24 26 60 56"
        width={size}
        height={size * (56 / 60)}
        aria-hidden="true"
        overflow="visible"
        onPointerDown={onPointerDown}
      >
        <defs>
          <mask id="swell-long" maskUnits="userSpaceOnUse" x="0" y="0" width="108" height="108">
            <path className="draw draw-long" d={LONG_TRACK} pathLength={1} />
          </mask>
          <mask id="swell-tail" maskUnits="userSpaceOnUse" x="0" y="0" width="108" height="108">
            <path className="draw draw-tail" d={TAIL_TRACK} pathLength={1} />
          </mask>
        </defs>
        <path className="part long" d={LONG} mask="url(#swell-long)" />
        <path className="part tail" d={TAIL} mask="url(#swell-tail)" />
        <circle className="burst" cx={HANDLE_CENTRE.x} cy={HANDLE_CENTRE.y} r={7.5} />
        <circle className="burst burst-2" cx={HANDLE_CENTRE.x} cy={HANDLE_CENTRE.y} r={7.5} />
        <g className="handle-follow">
          <g className="handle">
            <path className="part" d={HANDLE} />
          </g>
        </g>
      </svg>
    )
  },
)
