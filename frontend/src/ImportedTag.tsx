/** The tag on a key or raid pull imported from Warcraft Logs, linking to its fight on Warcraft Logs. */
export function ImportedTag({ url }: { url: string }) {
  return (
    <a className="tag" href={url} target="_blank" rel="noreferrer" title="Imported from Warcraft Logs: open the fight">
      Warcraft Logs
    </a>
  )
}
