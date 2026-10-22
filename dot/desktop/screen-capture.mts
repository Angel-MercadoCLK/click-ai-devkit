import { CAPTURE_MAX_WIDTH, type CaptureFn } from "./capture.mts";

type ImageLike = { isEmpty(): boolean; getSize(): { width: number; height: number }; toPNG(): Buffer };
type DisplayLike = { id?: number; size: { width: number; height: number }; scaleFactor: number };

/** The slice of Electron's `screen` and `desktopCapturer` this module uses; the real ones satisfy it. */
export type ScreenDeps = {
  screen: { getPrimaryDisplay(): DisplayLike };
  desktopCapturer: {
    getSources(options: { types: Array<"screen">; thumbnailSize: { width: number; height: number } }): Promise<Array<{ display_id: string; thumbnail: ImageLike }>>;
  };
};

/** The primary display in device pixels, scaled down to at most `maxWidth` wide to bound memory. */
export function thumbnailSizeFor(display: Pick<DisplayLike, "size" | "scaleFactor">, maxWidth = CAPTURE_MAX_WIDTH) {
  const factor = display.scaleFactor > 0 ? display.scaleFactor : 1;
  const width = display.size.width * factor;
  const height = display.size.height * factor;
  const scale = width > maxWidth ? maxWidth / width : 1;
  return { width: Math.max(1, Math.round(width * scale)), height: Math.max(1, Math.round(height * scale)) };
}

/** Takes one PNG frame of the primary display. Returns undefined when the system gives back nothing usable. */
export function createScreenCapture({ screen, desktopCapturer }: ScreenDeps, maxWidth = CAPTURE_MAX_WIDTH): CaptureFn {
  return async () => {
    const display = screen.getPrimaryDisplay();
    const sources = await desktopCapturer.getSources({ types: ["screen"], thumbnailSize: thumbnailSizeFor(display, maxWidth) });
    const source = sources.find((candidate) => display.id !== undefined && candidate.display_id === String(display.id)) ?? sources[0];
    if (!source || source.thumbnail.isEmpty()) return undefined;
    const { width, height } = source.thumbnail.getSize();
    return { image: source.thumbnail.toPNG(), width, height };
  };
}
