import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import { Overlay } from './Overlay'
import { ReviewPage } from './ReviewPage'

const review = /^\/review\/(mplus|raid)\/(\d+)$/.exec(location.pathname) // a finished key's or pull's full review

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    {review ? (
      <ReviewPage kind={review[1] as 'mplus' | 'raid'} start={Number(review[2])} />
    ) : location.pathname === '/overlay' ? (
      <Overlay />
    ) : (
      <App />
    )}
  </StrictMode>,
)
