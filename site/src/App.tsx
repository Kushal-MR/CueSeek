import { Mark } from './Mark.tsx'
import { Knob } from './theme/Knob.tsx'
import { Arrival } from './arrival/Arrival.tsx'
import { Problem } from './sections/Problem.tsx'
import { Answer } from './sections/Answer.tsx'
import { WorksWith } from './sections/WorksWith.tsx'
import { Wrist } from './sections/Wrist.tsx'
import { Reel } from './sections/Reel.tsx'
import { Why } from './sections/Why.tsx'
import { Try } from './sections/Try.tsx'
import { Download } from './sections/Download.tsx'
import { Traveller } from './traveller/Traveller.tsx'
import './App.css'

const REPO = 'https://github.com/Kushal-MR/CueSeek'
const LINKEDIN = 'https://www.linkedin.com/in/kushal-mr/'

/**
 * The page, in the order of the approved storyboard: the arrival, the problem and the answer,
 * what it works with, the watch, the reel, why it is built this way, the demo, and the
 * download. The travelling dot is rendered once, over all of them.
 */
export default function App() {
  return (
    <>
      <header className="nav">
        <a className="brand" href="#top">
          <Mark size={20} />
          CueSeek
        </a>
        <a className="demo-cta" href="./demo">
          <span aria-hidden="true" />
          Try the demo
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
        <Why />
        <Try />
        <Download />

      </main>
      <Traveller />

      <footer className="foot">
        <span>
          <Mark size={14} /> CueSeek · Apache-2.0 · this page loads nothing but itself
        </span>
        <span>
          Built by Kushal M R · <a href={REPO}>GitHub</a> · <a href={LINKEDIN}>LinkedIn</a>
        </span>
      </footer>
    </>
  )
}
