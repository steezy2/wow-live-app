import { findFights, reportPages, type ReportLink } from './api'
import { NoteLine } from './NoteLine'
import { track, useNote } from './note'
import { tip } from './tipAttrs'

/** The tag on a key or raid pull that is a Warcraft Logs fight (imported, or matched to a report): it opens it there. */
export function ReportTag({ link, imported }: { link: ReportLink; imported?: boolean }) {
  const title = imported ? 'Imported from Warcraft Logs: open the fight' : 'On Warcraft Logs: open the fight'
  return (
    <a className="tag" href={reportPages(link).warcraftLogs} target="_blank" rel="noreferrer" {...tip(title)}>
      Warcraft Logs
    </a>
  )
}

/** A finished key's or raid pull's (fight: its start) fight on Warcraft Logs and WoWAnalyzer once a report has it;
 * until then, Find my report, which looks through the character's recent reports. */
export function ReportLinks({ fight, link }: { fight: number; link?: ReportLink }) {
  const [note, setNote] = useNote(`find-report-${fight}`)
  if (link) {
    const pages = reportPages(link)
    return (
      <p className="links">
        <a href={pages.warcraftLogs} target="_blank" rel="noreferrer">
          Warcraft Logs
        </a>{' '}
        ·{' '}
        <a href={pages.wowAnalyzer} target="_blank" rel="noreferrer">
          WoWAnalyzer
        </a>
      </p>
    )
  }
  const find = () =>
    track(
      findFights(),
      setNote,
      'Looking through your recent Warcraft Logs reports…',
      () => 'Looking in the background: the links show here once a report has this fight.',
    ).catch(() => {})
  return (
    <>
      <p className="links">
        <button onClick={find} disabled={!!note?.busy}>
          Find my report
        </button>
      </p>
      <NoteLine note={note} />
    </>
  )
}
