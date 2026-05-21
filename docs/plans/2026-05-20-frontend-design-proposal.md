# Pic Gallery Frontend Design Proposal (2026-05-20)

## 1. Overview
This document outlines the "Cinematic User + Clinical Admin" design direction for the Pic Gallery project. It builds upon the existing `frontend-design-spec.md` and `frontend-visual-directions.md` to provide a concrete implementation strategy.

## 2. User Side: Luminous Vault (Cinematic)

### 2.1 Visual Atmosphere
- **Background**: Deep obsidian (`#05070d`) with a dynamic radial "light leak" (low-opacity gradient) that provides depth.
- **Materiality**: Extensive use of `backdrop-filter: blur()` and layered transparencies. Borders use thin, semi-transparent strokes (`rgba(255, 255, 255, 0.08)`).
- **Accents**: High-saturation "Jewel Tones" (Gold, Coral, Emerald, Violet) used sparingly for active states and critical feedback.

### 2.2 Layout: Workbench (Left Params + Right Canvas)
- **Left Sidebar (Navigation)**: Narrow (`108px`), dark, with monochromatic icons that "ignite" on hover.
- **Control Panel (Left)**: A semi-transparent "glass" pane containing sliders, toggles, and dropdowns. 
- **Canvas (Right)**: A large, centered display area for generated assets, with a subtle outer glow when a task is active.
- **TopBar**: Global utilities (Avatar, Balance, Notifications) in a minimalist, floating strip.

### 2.3 Typography
- **Display**: `Cormorant Garamond` (Light/Regular) for artistic titles, large numbers, and luxury feel.
- **Functional**: `Manrope` (Variable) for all technical labels and inputs, ensuring high legibility in dark mode.

## 3. Admin Side: Soft Grid Ops (Clinical)

### 3.1 Visual Atmosphere
- **Background**: Soft, neutral gray (`#eef2f4`). 
- **Structure**: A strict 8px grid system. No shadows; depth is achieved through layered whites and faint borders.
- **Status Colors**: Muted, "architectural" palette (Soft Blue for info, Sage Green for success, Amber for warnings).

### 3.2 Layout: Configuration Center
- **Sidebar**: Standard width (`216px`) for clear menu text.
- **TopBar + Status Band**: A secondary "info strip" below the TopBar providing real-time system context (Env: Prod, Cluster: Alpha, etc.).
- **Content Area**: Primarily high-density data tables and focused configuration forms. Rows are compact with subtle hover states.

### 3.3 Typography
- **Display**: `Fraunces` (Soft Serif) for a professional, authoritative tone in headers.
- **Functional**: `Manrope` for all data and form fields, optimized for scanning.

## 4. Implementation Priorities
1. **Shared Tokens**: Update `web/shared/tokens.css` with the new grid and spacing variables.
2. **User Mother Template**: Implement the "Luminous" shell with the dynamic lighting effect.
3. **Admin Mother Template**: Implement the "Clinical" shell with the status band.
4. **Core Pages**: Re-skin the "Create" page (User) and "Config" page (Admin).
