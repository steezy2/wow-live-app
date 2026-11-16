/** A panel of numbered things to work on: a key's or pull's suggestions (attention), or a player's advice. */
export function FixNext({ title, items, attention = true }: { title: string; items: string[]; attention?: boolean }) {
  if (items.length === 0) return null
  return (
    <section className={attention ? 'panel attention' : 'panel'}>
      <h2>{title}</h2>
      <ol className="fix-next">
        {items.map((s) => (
          <li key={s}>{s}</li>
        ))}
      </ol>
    </section>
  )
}
