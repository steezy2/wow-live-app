import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import { Overlay } from './Overlay'
import { TipHost } from './Tip'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <TipHost />
    {location.pathname === '/overlay' ? <Overlay /> : <App />}
  </StrictMode>,
)
