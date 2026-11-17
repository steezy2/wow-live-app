/** What shows in place of something a fight was saved without: a key or pull saved before the app kept `what`. */
export function SavedBefore({ what }: { what: string }) {
  return (
    <p className="muted">
      Saved before {what} were kept: <i>Read old logs</i> in Settings fills it in from the logs still in the game's
      folder.
    </p>
  )
}
