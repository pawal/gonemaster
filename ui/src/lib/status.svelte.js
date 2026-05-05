let message = $state("");
let tone = $state("");
let dismissTimer = null;

export const status = {
  get message() { return message; },
  get tone() { return tone; },
};

export function setStatus(nextMessage, nextTone = "") {
  message = nextMessage;
  tone = nextTone;
  if (dismissTimer) {
    clearTimeout(dismissTimer);
    dismissTimer = null;
  }
  if (!nextMessage) return;
  const delay = nextTone === "ok" ? 5000 : 8000;
  dismissTimer = setTimeout(() => {
    message = "";
    tone = "";
    dismissTimer = null;
  }, delay);
}

export function clearStatus() {
  message = "";
  tone = "";
  if (dismissTimer) {
    clearTimeout(dismissTimer);
    dismissTimer = null;
  }
}
