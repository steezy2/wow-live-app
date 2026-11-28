import { useState } from 'react'
import type { KeyView } from './api'
import { chatRows, chatRun, type ChatChannel } from './format'
import { tip } from './tipAttrs'

/** How long a copy button says Copied. */
const COPIED_MS = 1500

/** A finished key's DPS and HPS rows (chatRows) for WoW's chat, on the key page and the overlay: the chat to send
 * them to (Party, or Say) and a button each copying them as a /run that sends one message a row there (chatRun). */
export function ChatCopy({ keyView }: { keyView: KeyView }) {
  const [channel, setChannel] = useState<ChatChannel>('PARTY')
  return (
    <span className="chat-copy">
      <select
        value={channel}
        onChange={(e) => setChannel(e.target.value as ChatChannel)}
        aria-label="Chat to send to"
        {...tip('WoW can block a script\'s party chat during a key: Say still goes through', 'Chat to send to')}
      >
        <option value="PARTY">Party</option>
        <option value="SAY">Say</option>
      </select>
      {(['dps', 'hps'] as const).map((what) => (
        <CopyButton key={what} keyView={keyView} what={what} channel={channel} />
      ))}
    </span>
  )
}

/** A button copying the key's DPS or HPS /run for the channel, saying Copied for a moment; its hover shows the rows. */
function CopyButton(props: { keyView: KeyView; what: 'dps' | 'hps'; channel: ChatChannel }) {
  const [copied, setCopied] = useState(false)
  const rows = chatRows(props.keyView, props.what)
  const label = `Copy ${props.what.toUpperCase()} for chat`
  const copy = () =>
    void navigator.clipboard.writeText(chatRun(rows, props.channel)).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), COPIED_MS)
    }, () => undefined)
  return (
    <button onClick={copy} {...tip(rows.join(' / '), label)}>
      {copied ? 'Copied' : label}
    </button>
  )
}
