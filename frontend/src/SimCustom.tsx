/** The custom simc lines card, collapsed until opened: extra simc options and an action list the sim adds after the
 * character. The server checks every line against its allowlist and names the one it refuses (`error`). */
export function CustomCard(props: { value: string; onChange: (v: string) => void; error?: string | null }) {
  const { value, onChange, error } = props
  return (
    <details className="panel sim-custom" open={!!value.trim() || !!error}>
      <summary>Custom SimC lines and action list</summary>
      <p className="muted small">
        One per line: an action list (<code>actions=</code>, <code>actions+=</code>,{' '}
        <code>actions.&lt;list&gt;+=</code>) or a listed option such as <code>optimal_raid=0</code> or{' '}
        <code>override.bloodlust=0</code>. A custom action list is not what other simmers get, so its numbers won&apos;t
        match theirs.
      </p>
      <label>
        Lines for SimulationCraft
        <textarea rows={6} value={value} spellCheck={false} onChange={(e) => onChange(e.target.value)} />
      </label>
      {error && <p className="warning">{error}</p>}
    </details>
  )
}
