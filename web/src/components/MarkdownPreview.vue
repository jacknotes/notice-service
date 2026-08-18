<template>
  <div class="md-preview" v-html="html"></div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { marked } from 'marked'

const props = defineProps<{ content: string }>()

/**
 * Highlight template variables as `<span class="var">` before rendering,
 * so `{{变量}}` placeholders are visibly emphasized in the preview.
 * HTML is escaped first (& < >) to neutralise any injected markup.
 */
function highlight(raw: string): string {
  const esc = (s: string) =>
    s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')

  const safe = esc(raw ?? '')
  return safe.replace(/\{\{([^{}]+)\}\}/g, (_, name: string) => {
    return `<span class="var">{{${esc(name)}}}</span>`
  })
}

const html = computed<string>(() => marked.parse(highlight(props.content)) as string)
</script>

<style scoped>
/* ── Rendered Markdown, dressed in Signal Relay tokens ─────────────── */
.md-preview {
  font-size: var(--text-base);
  line-height: var(--leading-normal);
  color: var(--text-primary);
  word-break: break-word;
  overflow-wrap: anywhere;
}

.md-preview :deep(h1),
.md-preview :deep(h2),
.md-preview :deep(h3),
.md-preview :deep(h4) {
  font-family: var(--font-display);
  font-weight: 600;
  color: var(--text-primary);
  line-height: var(--leading-tight);
  margin: 0.9em 0 0.45em;
}
.md-preview :deep(h1) { font-size: 1.5em; }
.md-preview :deep(h2) { font-size: 1.3em; }
.md-preview :deep(h3) { font-size: 1.15em; }
.md-preview :deep(h4) { font-size: 1em; }

.md-preview :deep(p) { margin: 0.5em 0; }

.md-preview :deep(a) {
  color: var(--indigo-400);
  text-decoration: underline;
  text-underline-offset: 3px;
}
.md-preview :deep(a):hover { color: var(--violet-400); }

.md-preview :deep(strong) { color: var(--text-primary); font-weight: 700; }
.md-preview :deep(em) { color: var(--text-secondary); }

.md-preview :deep(ul),
.md-preview :deep(ol) {
  margin: 0.5em 0;
  padding-left: 1.5em;
}
.md-preview :deep(li) { margin: 0.25em 0; }

.md-preview :deep(blockquote) {
  margin: 0.6em 0;
  padding: 0.4em 0.9em;
  border-left: 3px solid var(--indigo-500);
  border-radius: var(--radius-xs);
  background: var(--grad-primary-soft);
  color: var(--text-secondary);
}

.md-preview :deep(code) {
  font-family: var(--font-mono);
  font-size: 0.88em;
  padding: 0.12em 0.4em;
  border-radius: var(--radius-xs);
  background: rgba(148, 163, 184, 0.12);
  color: var(--violet-400);
}

.md-preview :deep(pre) {
  margin: 0.6em 0;
  padding: 0.9em 1em;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border);
  background: rgba(11, 17, 32, 0.72);
  overflow-x: auto;
}
.md-preview :deep(pre code) {
  padding: 0;
  background: transparent;
  color: var(--text-primary);
}

.md-preview :deep(hr) {
  border: none;
  height: 1px;
  margin: 0.9em 0;
  background: var(--border);
}

.md-preview :deep(table) {
  width: 100%;
  border-collapse: collapse;
  margin: 0.6em 0;
  font-size: var(--text-sm);
}
.md-preview :deep(th),
.md-preview :deep(td) {
  border: 1px solid var(--border);
  padding: 0.35em 0.6em;
  text-align: left;
}
.md-preview :deep(th) {
  background: rgba(148, 163, 184, 0.08);
  color: var(--text-secondary);
  font-weight: 600;
}

/* ── Template variable placeholder ─────────────────────────────────── */
.md-preview :deep(.var) {
  display: inline-block;
  font-family: var(--font-mono);
  font-size: 0.88em;
  letter-spacing: 0.01em;
  padding: 0 0.35em;
  border-radius: var(--radius-xs);
  color: var(--violet-400);
  background: rgba(139, 92, 246, 0.14);
  border: 1px solid rgba(139, 92, 246, 0.35);
  box-shadow: 0 0 12px rgba(139, 92, 246, 0.18);
}
</style>
