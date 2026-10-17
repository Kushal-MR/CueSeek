import mark from '@brand/cueseek-mark.svg?raw'

/**
 * The Swell's three pieces as SVG path data, split from the file docs/brand/icon.py
 * generates: the long arc, the short tail, and the handle — the dot. Coordinates are the
 * mark's own (viewBox 24 26 60 56).
 */
const d = mark.match(/ d="([^"]+)"/)![1]
// Three subpaths, each starting with its own `M`.
export const [LONG, TAIL, HANDLE] = d.split(/ (?=M )/)

/** Centre of the handle — the dot — in the mark's own coordinates (DESIGN.md §13). */
export const HANDLE_CENTRE = { x: 71.97, y: 40.53 }
