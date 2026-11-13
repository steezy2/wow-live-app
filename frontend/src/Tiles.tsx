/** Big numbers in boxes, each with its label under it. */
export function Tiles({ tiles }: { tiles: [string, string][] }) {
  return (
    <div className="recap-tiles">
      {tiles.map(([value, label]) => (
        <div key={label} className="card small">
          <b>{value}</b>
          <span className="muted small">{label}</span>
        </div>
      ))}
    </div>
  )
}
