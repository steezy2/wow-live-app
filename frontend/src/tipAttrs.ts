/** The attributes that give an element a hover box: its text, and a small line above it (a lane and time, say).
 * Spread them on any element; nothing is shown for an empty text. */
export function tip(text?: string, head?: string) {
  return text ? { 'data-tip': text, 'data-tip-head': head, 'aria-description': text } : {}
}

/** The attribute that makes an element's hover box say what using an item does (a flask, a potion), by item ID, with
 * text to show until Wowhead's answer comes. */
export function itemTip(item: number, text?: string) {
  return item ? { 'data-item-id': String(item), 'data-tip': text } : tip(text)
}

/** The attribute that makes an element's hover box say what an ability does (and its cooldown), by the name the combat
 * log gives it or a spell ID (a mob's spell). */
export function spellTip(spell: string | number, note?: string) {
  if (typeof spell === 'number') return { 'data-spell-id': String(spell), 'data-tip-note': note }
  return spell ? { 'data-spell': spell, 'data-tip-note': note } : {}
}
