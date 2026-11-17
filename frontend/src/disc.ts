/** Draws a circle at (x, y) on the canvas: filled with fill (none when null), then outlined in stroke. */
export function disc(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  r: number,
  fill: string | null,
  stroke: string,
  width: number,
): void {
  ctx.beginPath()
  ctx.arc(x, y, r, 0, 2 * Math.PI)
  if (fill !== null) {
    ctx.fillStyle = fill
    ctx.fill()
  }
  ctx.strokeStyle = stroke
  ctx.lineWidth = width
  ctx.stroke()
}
