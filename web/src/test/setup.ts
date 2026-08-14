// TPL-1 milestone-3/task-3: Vitest setup file (vitest.config.ts's
// test.setupFiles) — extends vitest's `expect` with jest-dom's DOM
// matchers (toBeInTheDocument, etc.), run once before every test file.
import "@testing-library/jest-dom/vitest";

// jsdom implements neither the Pointer Events capture methods nor
// scrollIntoView — real browser APIs Radix's <Select>/<Popover> primitives
// call when opening (hasPointerCapture/setPointerCapture/
// releasePointerCapture) or when moving keyboard focus onto an item
// (scrollIntoView). Without these, any test that opens a Radix select via
// a real click throws `target.hasPointerCapture is not a function` before
// React even finishes the event — not a test bug, a jsdom gap every
// Radix-based project hits. No-op stubs are enough: this milestone's own
// tests only need the open/close DOM state, never real capture/scroll
// behavior.
if (!Element.prototype.hasPointerCapture) {
  Element.prototype.hasPointerCapture = () => false;
}
if (!Element.prototype.setPointerCapture) {
  Element.prototype.setPointerCapture = () => {};
}
if (!Element.prototype.releasePointerCapture) {
  Element.prototype.releasePointerCapture = () => {};
}
if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => {};
}
