import { Mark } from './Mark.tsx'
import { Knob } from './theme/Knob.tsx'
import { Arrival } from './arrival/Arrival.tsx'
import { Problem } from './sections/Problem.tsx'
import { Answer } from './sections/Answer.tsx'
import { WorksWith } from './sections/WorksWith.tsx'
import { Wrist } from './sections/Wrist.tsx'
import { Reel } from './sections/Reel.tsx'
import { Traveller } from './traveller/Traveller.tsx'
import './App.css'

const REPO = 'https://github.com/Kushal-MR/CueSeek'

/**
 * The page's bones: every section from the approved storyboard, in order, each saying which
 * phase fills it. Deliberately plain — M6.2 proves the site builds, deploys and keeps its
 * promise to load nothing but itself; the showpieces arrive one phase at a time.
 */
const SECTIONS = [
  { id: 'why', n: '06', title: "Why it's built this way.", phase: 'M6.10' },
  { id: 'try', n: '07', title: 'Break a server. Nothing will happen.', phase: 'M6.11' },
  { id: 'download', n: '08', title: 'Install it.', phase: 'M6.12' },
] as const

export default function App() {
  return (
    <>
      <header className="nav">
        <a className="brand" href="#top">
          <Mark size={20} />
          CueSeek
        </a>
        <nav aria-label="Sections">
          <a href="#answer">How it works</a>
          <a href="#works-with">Works with</a>
          <a href="#download">Download</a>
          <a href={REPO}>GitHub</a>
        </nav>
        <Knob />
      </header>

      <main id="top">
        <Arrival />
        <Problem />
        <Answer />
        <WorksWith />
        <Wrist />
        <Reel />

        {SECTIONS.map((s) => (
          <section key={s.id} id={s.id} className="stub" aria-labelledby={`${s.id}-title`}>
            <span className="hud">
              Section {s.n} — arrives in {s.phase}
            </span>
            <h2 id={`${s.id}-title`}>{s.title}</h2>
          </section>
        ))}
      </main>
      <Traveller />

      <footer className="foot">
        <span>
          <Mark size={14} /> CueSeek · Apache-2.0 · this page loads nothing but itself
        </span>
        <a href={REPO}>Source on GitHub</a>
      </footer>
    </>
  )
}
