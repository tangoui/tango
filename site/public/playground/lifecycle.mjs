// A cached page keeps its Go runtime; real navigation releases the session.
export function previewLifecycle({ pause, resume, dispose }) {
  let cached = false;
  let disposed = false;
  return {
    hide(event) {
      if (disposed) return;
      if (event.persisted) { cached = true; pause(); }
      else { disposed = true; dispose(); }
    },
    show(event) {
      if (!disposed && cached && event.persisted) { cached = false; resume(); }
    },
  };
}
