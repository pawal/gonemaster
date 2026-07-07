// Shared unsaved-changes guard. The active editable panel registers whether it
// has unsaved edits; navigation calls confirmLeave() before leaving.

let dirty = $state(false);
let confirmMessage = $state("");

export const dirtyGuard = {
  get dirty() { return dirty; },
  register(isDirty, message) {
    dirty = !!isDirty;
    confirmMessage = message || "";
  },
  clear() {
    dirty = false;
    confirmMessage = "";
  },
  // Returns true if navigation may proceed: not dirty, or the user confirms
  // discarding. Clears the guard once the user accepts.
  confirmLeave() {
    if (!dirty) return true;
    const ok = typeof window === "undefined" ? true : window.confirm(confirmMessage);
    if (ok) this.clear();
    return ok;
  },
};
