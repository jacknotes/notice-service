import { ref } from 'vue'

/**
 * Day / Night theme singleton.
 *
 * Lightweight shared module (no Pinia needed): reads `localStorage.theme`
 * (default `'dark'`), applies `document.documentElement.setAttribute('data-theme', …)`
 * on init and on every toggle, and persists the choice.
 *
 * `index.html` also sets the attribute inline (before any paint) to avoid a
 * flash of the wrong theme on reload.
 */

export type Theme = 'dark' | 'light'

const STORAGE_KEY = 'theme'

function resolveInitialTheme(): Theme {
  try {
    return localStorage.getItem(STORAGE_KEY) === 'light' ? 'light' : 'dark'
  } catch {
    return 'dark'
  }
}

/** Current theme — reactive, so components reflect it immediately. */
export const theme = ref<Theme>(resolveInitialTheme())

export function applyTheme(next: Theme) {
  theme.value = next
  document.documentElement.setAttribute('data-theme', next)
  try {
    localStorage.setItem(STORAGE_KEY, next)
  } catch {
    /* private mode / quota — non-fatal, theme just won't persist */
  }
}

export function setTheme(next: Theme) {
  applyTheme(next)
}

export function toggleTheme() {
  applyTheme(theme.value === 'dark' ? 'light' : 'dark')
}

// Apply synchronously at module load so the attribute is set before the app
// mounts (belt-and-braces on top of the inline script in index.html).
applyTheme(theme.value)
