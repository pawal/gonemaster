// Recent-tests history in localStorage: one versioned key, a JSON array of
// entries newest first, capped. Mutations load-modify-save and return the
// resulting list; storage errors are swallowed.

export const STORAGE_KEY = "gonemaster.public.history.v1";
export const MAX_ENTRIES = 20;

/** True when a stored value has the shape of a history entry. */
function isEntry(e) {
  return (
    e !== null &&
    typeof e === "object" &&
    typeof e.id === "string" &&
    e.id !== "" &&
    typeof e.domain === "string" &&
    e.domain !== "" &&
    (e.finishedAt === null || typeof e.finishedAt === "string") &&
    (e.grade === undefined || typeof e.grade === "string")
  );
}

/** Read and validate the stored list; anything unusable yields []. */
function read() {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(isEntry).slice(0, MAX_ENTRIES);
  } catch (_) {
    return [];
  }
}

/** Persist the list, ignoring storage failures, and return it. */
function write(entries) {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(entries));
  } catch (_) {}
  return entries;
}

/** Return the stored entries, newest first. */
export function loadEntries() {
  return read();
}

/**
 * Add an entry to the front of the list. Only known fields are persisted;
 * an existing entry with the same id is replaced. Invalid input is ignored.
 */
export function addEntry(entry) {
  const id = entry && typeof entry.id === "string" ? entry.id : "";
  const domain = entry && typeof entry.domain === "string" ? entry.domain : "";
  if (id === "" || domain === "") return read();
  const e = {
    id,
    domain,
    finishedAt: typeof entry.finishedAt === "string" ? entry.finishedAt : null,
  };
  if (typeof entry.grade === "string" && entry.grade !== "") e.grade = entry.grade;
  const rest = read().filter((x) => x.id !== id);
  return write([e, ...rest].slice(0, MAX_ENTRIES));
}

/** Set the grade of an existing entry. Never creates entries. */
export function setGrade(id, grade) {
  const entries = read();
  if (typeof grade !== "string" || grade === "") return entries;
  const idx = entries.findIndex((e) => e.id === id);
  if (idx === -1) return entries;
  entries[idx] = { ...entries[idx], grade };
  return write(entries);
}

/** Remove the entry with the given id, if present. */
export function removeEntry(id) {
  return write(read().filter((e) => e.id !== id));
}

/** Remove all history. */
export function clearEntries() {
  try {
    window.localStorage.removeItem(STORAGE_KEY);
  } catch (_) {}
  return [];
}
