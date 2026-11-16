// The overlay window is transparent: it gets no splash. Loaded by index.html (an inline script is not allowed by the CSP).
if (location.pathname === '/overlay') document.documentElement.className = 'is-overlay'
