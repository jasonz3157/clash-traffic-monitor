# README And Homepage Positioning Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refresh the project README and homepage copy so the project presents stronger metadata, clearly explains its lightweight positioning next to `foru17/neko-master`, and updates homepage time range presets to 1/7/15/30 days plus custom.

**Architecture:** Keep the change lightweight and documentation-first. Update README badges and positioning copy in `README.md`, add a concise comparison/info banner in `web/index.html`, and update the existing embedded-page test coverage in `main_test.go` so the homepage copy and time-range presets are locked in.

**Tech Stack:** Go, embedded HTML/CSS/JS, Go test

---

### Task 1: Lock In Homepage Copy Requirements

**Files:**
- Modify: `main_test.go`
- Reference: `web/index.html`

- [ ] **Step 1: Write the failing test**

Add assertions that the embedded homepage:
- exposes `1 天` / `7 天` / `15 天` / `30 天` / `自定义` range options
- no longer shows the old `最近 1 小时` and `最近 24 小时` presets
- includes a concise `neko-master` positioning mention

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestEmbeddedIndexDisablesPeriodicAutoRefresh -timeout 15s`
Expected: FAIL because the current embedded HTML still has the old range options and no comparison copy.

- [ ] **Step 3: Write minimal implementation**

Update the homepage markup and copy in `web/index.html` until the new assertions pass.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestEmbeddedIndexDisablesPeriodicAutoRefresh -timeout 15s`
Expected: PASS

### Task 2: Refresh README Presentation

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Write the content changes**

Add a centered badge row, tighten the intro, and add a dedicated positioning section that directly names `foru17/neko-master` while framing this project as the lighter, simpler, Mihomo-focused option.

- [ ] **Step 2: Keep claims grounded in the repo**

Ensure every advantage is based on verifiable repo facts such as Go single-binary deployment, SQLite local storage, embedded frontend, fewer runtime dependencies, and OpenWrt-oriented lightweight use.

- [ ] **Step 3: Review README for scanability**

Check headings, bullets, and badges for readability without turning the README into a long marketing page.

### Task 3: Full Verification

**Files:**
- Verify: `README.md`
- Verify: `web/index.html`
- Verify: `main_test.go`

- [ ] **Step 1: Run focused homepage test**

Run: `go test ./... -run TestEmbeddedIndexDisablesPeriodicAutoRefresh -timeout 15s`
Expected: PASS

- [ ] **Step 2: Run full test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 3: Summarize final changes**

Report README badge/copy updates, homepage comparison banner, and the new time range presets with verification evidence.
