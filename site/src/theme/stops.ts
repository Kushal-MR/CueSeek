/**
 * The knob's five stops — the only place the site's colours are written down.
 *
 * Night and Day are the apps' own two themes (docs/DESIGN.md §3), so the site and the app
 * agree at both ends. The three between them are where the green is strongest: Sage puts
 * the page on the brand green itself, with cards in pale paper.
 *
 * Every pair a reader depends on is checked by `npm run check:contrast` (CI runs it), so a
 * value changed here cannot quietly drop below WCAG AA.
 */

export const TOKENS = [
  'bg', // the page
  'card', // a raised surface
  'card-text', // text on a card
  'text', // text on the page
  'muted', // secondary text on the page
  'accent', // the mark at small sizes, focus rings, lit LEDs
  'hi', // the mark and the wordmark at hero size
  'line', // hairlines and borders
  'ok', // status colours — always read on a card
  'warn',
  'bad',
  'glow', // decorative halo; never carries text
] as const

export type Token = (typeof TOKENS)[number]

export interface Stop {
  id: 'night' | 'forest' | 'sage' | 'mist' | 'day'
  name: string
  /** For the browser's own controls and scrollbars. */
  scheme: 'dark' | 'light'
  colors: Record<Token, string>
}

export const STOPS: readonly Stop[] = [
  {
    id: 'night',
    name: 'Night',
    scheme: 'dark',
    colors: {
      bg: '#0e1210', card: '#191e18', 'card-text': '#e2e7de', text: '#e2e7de', muted: '#b4bcb1',
      accent: '#a8c4a6', hi: '#cde8cf', line: '#333a32',
      ok: '#a8c4a6', warn: '#e5be84', bad: '#e0a79e', glow: '#5f8f66',
    },
  },
  {
    id: 'forest',
    name: 'Forest',
    scheme: 'dark',
    colors: {
      bg: '#1d3722', card: '#263f2a', 'card-text': '#e8f0e6', text: '#e8f0e6', muted: '#b9d2ba',
      accent: '#cde8cf', hi: '#e3f4e2', line: '#344e38',
      ok: '#b8d8b6', warn: '#e8c48c', bad: '#e8b0a8', glow: '#7fb487',
    },
  },
  {
    id: 'sage',
    name: 'Sage',
    scheme: 'dark',
    colors: {
      bg: '#4c6650', card: '#f1f4ee', 'card-text': '#191d18', text: '#ffffff', muted: '#e3ebe0',
      accent: '#f1f4ee', hi: '#ffffff', line: '#6e8c70',
      ok: '#3f5e46', warn: '#7a5215', bad: '#8c3a31', glow: '#cfe6cf',
    },
  },
  {
    id: 'mist',
    name: 'Mist',
    scheme: 'light',
    colors: {
      bg: '#d3e3d2', card: '#f1f4ee', 'card-text': '#191d18', text: '#0a2010', muted: '#2e4432',
      accent: '#344e38', hi: '#0a2010', line: '#b1cdb2',
      ok: '#3f5e46', warn: '#7a5215', bad: '#8c3a31', glow: '#4c6650',
    },
  },
  {
    id: 'day',
    name: 'Day',
    scheme: 'light',
    colors: {
      bg: '#f1f4ee', card: '#fcfdfa', 'card-text': '#191d18', text: '#191d18', muted: '#434840',
      accent: '#4c6650', hi: '#344e38', line: '#c3c8be',
      ok: '#3f5e46', warn: '#7a5215', bad: '#8c3a31', glow: '#6e8c70',
    },
  },
]

export const NIGHT = 0
export const DAY = STOPS.length - 1

/** `--bg: #0e1210; …` for one stop — used for the first paint and by the knob. */
export function declarations(stop: Stop): string {
  return TOKENS.map((t) => `--${t}:${stop.colors[t]}`).join(';') + `;color-scheme:${stop.scheme}`
}
