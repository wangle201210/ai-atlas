import { ref, watch } from "vue";

// Only restore known values, so stale or invalid preferences cannot reach the API.
export function rememberedChoice(
  key: string,
  choices: readonly string[],
  fallback: string,
) {
  const storageKey = `ai-atlas:${key}`;
  let initial = fallback;
  try {
    const saved = localStorage.getItem(storageKey);
    if (saved !== null && choices.includes(saved)) initial = saved;
  } catch {
    // Storage may be unavailable; in-memory preferences still work.
  }
  const value = ref(initial);
  watch(
    value,
    (next) => {
      if (!choices.includes(next)) return;
      try {
        localStorage.setItem(storageKey, next);
      } catch {
        // A preference write must not interrupt navigation or sorting.
      }
    },
    { flush: "sync" },
  );
  return value;
}
