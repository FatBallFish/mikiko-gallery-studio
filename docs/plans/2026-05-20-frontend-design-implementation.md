# Frontend Design Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the "Cinematic User + Clinical Admin" design across the `web/user` and `web/admin` applications.

**Architecture:** 
- Centralize all design tokens in `web/shared/tokens.css`.
- Use CSS variables and `backdrop-filter` for the User "Luminous Vault" theme.
- Use a strict grid and minimal borders for the Admin "Soft Grid Ops" theme.
- Implement "Mother Templates" in the main shell components to ensure consistency.

**Tech Stack:** React (TypeScript), Vanilla CSS.

---

### Task 1: Refine Global Tokens

**Files:**
- Modify: `web/shared/tokens.css`

**Step 1: Update tokens.css with new grid and shadow variables**

```css
:root {
  /* Spacing - 8px base grid */
  --pg-space-1: 4px;
  --pg-space-2: 8px;
  --pg-space-3: 12px;
  --pg-space-4: 16px;
  --pg-space-5: 20px;
  --pg-space-6: 24px;
  --pg-space-8: 32px;
  --pg-space-12: 48px;

  /* Radius */
  --pg-radius-sm: 8px;
  --pg-radius-md: 12px;
  --pg-radius-lg: 20px;
  --pg-radius-full: 9999px;

  /* Dimensions */
  --pg-pill-height: 42px;
  --pg-topbar-height: 72px;
  --pg-sidebar-user-width: 100px;
  --pg-sidebar-admin-width: 240px;

  /* Shadows - Minimal for Admin, Glows for User */
  --pg-shadow-sm: 0 1px 2px rgba(0,0,0,0.05);
  --pg-shadow-md: 0 4px 6px -1px rgba(0,0,0,0.1);
  --pg-glow-gold: 0 0 20px rgba(212, 157, 94, 0.2);
}
```

**Step 2: Commit**

```bash
git add web/shared/tokens.css
git commit -m "style: refine global design tokens"
```

---

### Task 2: Implement User "Luminous Vault" Base

**Files:**
- Modify: `web/shared/user-theme.css`

**Step 1: Update user-theme.css with cinematic background and glass effects**

```css
@import url('https://fonts.googleapis.com/css2?family=Cormorant+Garamond:wght@300;400;500&family=Manrope:wght@300;400;500;600&display=swap');
@import './tokens.css';

:root {
  --bg-deep: #05070d;
  --bg-surface: rgba(255, 255, 255, 0.03);
  --bg-glass: rgba(10, 12, 20, 0.6);
  
  --text-main: #f6efe4;
  --text-dim: rgba(246, 239, 228, 0.5);
  
  --border-thin: rgba(255, 255, 255, 0.08);
  --border-bright: rgba(255, 255, 255, 0.15);
}

body {
  background-color: var(--bg-deep);
  background-image: 
    radial-gradient(circle at 50% -20%, rgba(212, 157, 94, 0.08), transparent 50%),
    radial-gradient(circle at 0% 100%, rgba(131, 118, 255, 0.05), transparent 40%);
  color: var(--text-main);
  min-height: 100vh;
  margin: 0;
  -webkit-font-smoothing: antialiased;
}

.glass-panel {
  background: var(--bg-glass);
  backdrop-filter: blur(20px);
  border: 1px solid var(--border-thin);
  border-radius: var(--pg-radius-md);
}
```

**Step 2: Commit**

```bash
git add web/shared/user-theme.css
git commit -m "style: implement Luminous Vault base theme"
```

---

### Task 3: Implement Admin "Soft Grid Ops" Base

**Files:**
- Modify: `web/shared/admin-theme.css`

**Step 1: Update admin-theme.css with clinical and high-density rules**

```css
@import url('https://fonts.googleapis.com/css2?family=Fraunces:wght@400;500&family=Manrope:wght@400;500;600&display=swap');
@import './tokens.css';

:root {
  --bg-app: #eef2f4;
  --bg-card: #ffffff;
  --bg-subtle: #f8fafb;
  
  --text-main: #1a2532;
  --text-dim: #68788b;
  
  --border-base: rgba(26, 37, 50, 0.08);
  --border-focus: rgba(87, 117, 185, 0.3);
  
  --color-primary: #5775b9;
}

body {
  background-color: var(--bg-app);
  color: var(--text-main);
  margin: 0;
  font-size: 14px; /* Slightly smaller for density */
}

.admin-grid-card {
  background: var(--bg-card);
  border: 1px solid var(--border-base);
  border-radius: var(--pg-radius-sm);
  box-shadow: var(--pg-shadow-sm);
}
```

**Step 2: Commit**

```bash
git add web/shared/admin-theme.css
git commit -m "style: implement Soft Grid Ops base theme"
```

---

### Task 4: Re-skin User Workbench (Create Page)

**Files:**
- Modify: `web/user/src/styles.css`
- Modify: `web/user/src/pages.tsx` (ReferencePage or TextPage)

**Step 1: Apply cinematic styles to the workbench layout**
- Update `.user-shell` to use the new sidebar width.
- Implement the `.glass-panel` for the left parameter area.
- Add a large, focused frame for the `.canvas-area`.

**Step 2: Commit**

```bash
git add web/user/src/styles.css web/user/src/pages.tsx
git commit -m "style: re-skin user workbench with cinematic direction"
```

---

### Task 5: Re-skin Admin Configuration Center

**Files:**
- Modify: `web/admin/src/styles.css`
- Modify: `web/admin/src/pages.tsx` (ConfigPage)

**Step 1: Apply clinical styles to the config layout**
- Implement the "Status Band" below the TopBar.
- Use `.admin-grid-card` for configuration blocks.
- Compact the data table rows and typography.

**Step 2: Commit**

```bash
git add web/admin/src/styles.css web/admin/src/pages.tsx
git commit -m "style: re-skin admin config center with clinical direction"
```
