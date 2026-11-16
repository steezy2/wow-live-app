import { addsDrawing, PULL_COLORS } from './routes'
import { tip } from './tipAttrs'

/** What a press on the route map does: Select picks enemies as before, the others draw or erase. */
export type DrawTool = 'select' | 'line' | 'marker' | 'note' | 'erase'

const TOOLS: { tool: DrawTool; label: string; hint: string }[] = [
  { tool: 'select', label: 'Select', hint: 'Click enemies to put them in the selected pull; drag to move the map' },
  { tool: 'line', label: 'Line', hint: 'Drag to draw a line' },
  { tool: 'marker', label: 'Marker', hint: 'Click to place a marker' },
  { tool: 'note', label: 'Note', hint: 'Type the note, then click where it goes' },
  { tool: 'erase', label: 'Erase', hint: 'Click a drawing to erase it' },
]

/** The route map's drawing toolbar: the tools, the color the next drawing gets (the pull colors, picked while a tool
 * that adds one is on, always shown so the map doesn't jump), and the Note tool's text. With `full` (the route has
 * its most drawings) only Select and Erase work. */
export function DrawTools(props: {
  tool: DrawTool
  onTool: (tool: DrawTool) => void
  color: string
  onColor: (color: string) => void
  text: string
  onText: (text: string) => void
  maxText: number
  full: boolean
}) {
  const { tool, onTool, color, onColor, text, onText, maxText, full } = props
  const adding = addsDrawing(tool)
  return (
    <div className="draw-tools" role="toolbar" aria-label="drawing tools">
      {TOOLS.map((t) => (
        <button
          key={t.tool}
          aria-pressed={t.tool === tool}
          disabled={addsDrawing(t.tool) && full}
          {...tip(addsDrawing(t.tool) && full ? 'The route has as many drawings as it can: erase one first' : t.hint)}
          onClick={() => onTool(t.tool)}
        >
          {t.label}
        </button>
      ))}
      {PULL_COLORS.map((c) => (
        <button
          key={c}
          className="route-swatch"
          style={{ background: `#${c}` }}
          aria-pressed={c === color}
          aria-label={`draw in #${c}`}
          disabled={!adding}
          onClick={() => onColor(c)}
        />
      ))}
      {tool === 'note' && (
        <input
          value={text}
          maxLength={maxText === Infinity ? undefined : maxText}
          placeholder="Note text"
          aria-label="note text"
          onChange={(e) => onText(e.target.value)}
        />
      )}
    </div>
  )
}
