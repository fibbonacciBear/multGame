import type { ClientInputMessage } from "./types";

const MOUSE_MAX_DIST = 400;

export class InputController {
  private readonly canvas: HTMLCanvasElement;
  private mouseX = 0;
  private mouseY = 0;
  private shooting = false;
  private keyboardShoot = false;
  private readonly cleanupCallbacks: Array<() => void> = [];

  constructor(canvas: HTMLCanvasElement) {
    this.canvas = canvas;

    const handlePointerMove = (event: PointerEvent) => {
      const rect = this.canvas.getBoundingClientRect();
      this.mouseX = event.clientX - rect.left;
      this.mouseY = event.clientY - rect.top;
    };

    const handlePointerDown = () => {
      this.shooting = true;
    };

    const handlePointerUp = () => {
      this.shooting = false;
    };

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.code === "Space") {
        this.keyboardShoot = true;
      }
    };

    const handleKeyUp = (event: KeyboardEvent) => {
      if (event.code === "Space") {
        this.keyboardShoot = false;
      }
    };

    this.canvas.addEventListener("pointermove", handlePointerMove);
    this.canvas.addEventListener("pointerdown", handlePointerDown);
    window.addEventListener("pointerup", handlePointerUp);
    window.addEventListener("keydown", handleKeyDown);
    window.addEventListener("keyup", handleKeyUp);

    this.cleanupCallbacks.push(() => this.canvas.removeEventListener("pointermove", handlePointerMove));
    this.cleanupCallbacks.push(() => this.canvas.removeEventListener("pointerdown", handlePointerDown));
    this.cleanupCallbacks.push(() => window.removeEventListener("pointerup", handlePointerUp));
    this.cleanupCallbacks.push(() => window.removeEventListener("keydown", handleKeyDown));
    this.cleanupCallbacks.push(() => window.removeEventListener("keyup", handleKeyUp));
  }

  getState(): ClientInputMessage {
    const rect = this.canvas.getBoundingClientRect();
    const dx = this.mouseX - rect.width / 2;
    const dy = this.mouseY - rect.height / 2;
    const distance = Math.hypot(dx, dy);

    return {
      type: "input",
      angle: distance > 0.001 ? Math.atan2(dy, dx) : 0,
      strength: Math.min(distance / MOUSE_MAX_DIST, 1),
      shoot: this.shooting || this.keyboardShoot,
    };
  }

  dispose() {
    for (const callback of this.cleanupCallbacks) {
      callback();
    }
  }
}
