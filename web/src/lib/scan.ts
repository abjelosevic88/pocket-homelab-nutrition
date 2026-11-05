// The browser's own barcode reader, where there is one (Android, desktop
// Chrome), for food barcodes (EAN, UPC); elsewhere the server reads a frame.

interface Detector { detect: (src: CanvasImageSource) => Promise<{ rawValue: string; format: string }[]> }
let detector: Detector | null | undefined
const FORMATS = ['ean_13', 'ean_8', 'upc_a', 'upc_e']

export async function browserDetector(): Promise<Detector | null> {
  if (detector !== undefined) return detector
  const BD = (window as unknown as { BarcodeDetector?: { new(o: { formats: string[] }): Detector; getSupportedFormats: () => Promise<string[]> } }).BarcodeDetector
  if (!BD) return (detector = null)
  try {
    const have = await BD.getSupportedFormats()
    const formats = FORMATS.filter(f => have.includes(f))
    detector = formats.length ? new BD({ formats }) : null
  } catch { detector = null }
  return detector
}

export async function detectIn(src: CanvasImageSource): Promise<{ format: string; value: string } | null> {
  const d = await browserDetector()
  if (!d) return null
  try {
    const found = await d.detect(src)
    const f = found.find(x => FORMATS.includes(x.format) && x.rawValue)
    return f ? { format: f.format, value: f.rawValue } : null
  } catch { return null }
}

/** A photo made smaller (JPEG), for sending. */
export async function shrinkImage(file: Blob, max = 1600, quality = 0.86): Promise<Blob> {
  const url = URL.createObjectURL(file)
  try {
    const img = await new Promise<HTMLImageElement>((ok, bad) => { const i = new Image(); i.onload = () => ok(i); i.onerror = () => bad(new Error('That photo cannot be opened')); i.src = url })
    const scale = Math.min(1, max / Math.max(img.naturalWidth, img.naturalHeight))
    const w = Math.round(img.naturalWidth * scale), h = Math.round(img.naturalHeight * scale)
    const cv = document.createElement('canvas')
    cv.width = w; cv.height = h
    cv.getContext('2d')!.drawImage(img, 0, 0, w, h)
    return await new Promise<Blob>((ok, bad) => cv.toBlob(b => b ? ok(b) : bad(new Error('That photo cannot be read')), 'image/jpeg', quality))
  } finally { URL.revokeObjectURL(url) }
}
