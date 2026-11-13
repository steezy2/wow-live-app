/** The attributes that give an element a hover box: its text, and a small line above it (a lane and time, say).
 * Spread them on any element; nothing is shown for an empty text. */
export function tip(text?: string, head?: string) {
  return text ? { 'data-tip': text, 'data-tip-head': head, 'aria-description': text } : {}
}

/** The attribute that makes an element's hover box say what an ability does (and its cooldown), by the name the combat
 * log gives it. */
export function spellTip(name: string) {
  return name ? { 'data-spell': name } : {}
}
