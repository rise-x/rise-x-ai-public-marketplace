/*
 * Rise-X Kit — browser page. Vanilla JS, no build step. index.html is the
 * static shell; every region below a card header is rendered here.
 *
 * Every /api/* request carries header X-RiseX-Token: <token> (read from
 * <meta name="risex-token">, injected by the server).
 *
 * GET /api/overview -> 200
 *   { kitVersion, cli: { found, path?, version?,
 *       source?: "path"|"local-bin"|"desktop-bundle"|"windows-probe" } | null,
 *     marketplace?: { registered, installLocation?,
 *       autoUpdate?: bool,   // absent = key not set in settings.json
 *       autoUpdateMarketplace?: string, // absent = rise-x-public
 *       headStale?: bool,   // absent = GitHub unreachable, skipped
 *       settingsError?: bool, // true = settings.json is not valid JSON
 *       checkError?: string }, // set = the marketplace list could not be
 *                              // read, so registered says nothing
 *     plugins?: [{ name, description?, installed, enabled, localVersion?,
 *       publicVersion?, versionUnknown?, offline?, publicCheckError?,
 *       updateAvailable?,
 *       // set = the plugin list could not be read, so installed says nothing
 *       checkError?,
 *       // absent = not installed; "desktop" = synced by Claude Desktop, so
 *       // the CLI cannot install, update or remove that copy
 *       installSource?: "public"|"marketplace"|"desktop"|"organisation",
 *       sourceName? }],      // the marketplace installSource came from
 *     mcp?: { verdict: "connected"|"needs_auth"|"failed"|"pending"|"unknown"
 *               |"not_installed"|"managed", // managed = the Desktop app owns them
 *       servers?: [{ name, target, status }], configured?: [{ name, type?, url?, command?, args? }],
 *       stale?: [{ name, scope: "user"|"local"|"desktop", projectPath?, url, suggestedUrl }],
 *       message?: string,    // why the verdict is unknown, when it is
 *       raw?: string },      // full `claude mcp list` text, redacted
 *     reloadHint: bool }
 *
 * GET /api/doctor -> 200
 *   { checks: [{ id, status: "ok"|"warn"|"fail"|"skip", title, message,
 *                detail?, fix?, fixLabel?, fixTitle?, fixArgs? }] }
 *   fix is the action name to POST; detail is the newline-separated lines the
 *   fix would change, shown behind "Show lines". fixLabel replaces the
 *   button's "Fix" label and fixTitle is its tooltip. npmrc.clean has no
 *   fixArgs but the server demands {confirm:true}, so the page adds it.
 *
 * POST /api/actions/{name}  body: JSON (may be empty) -> 200 | 400 | 403 | 409 | 422
 *   Sync actions respond immediately:
 *     autoupdate.set {enabled, marketplace?} -> {backup}
 *     npmrc.clean {confirm:true} -> {updated: [string], backup}
 *     cli.rescan {} -> {found}  |  quit {} -> {ok: true}
 *     reload-hint.dismiss {} -> {ok: true}
 *   Job actions respond {jobId} and stream their log via GET /api/jobs/{id}:
 *     marketplace.add, marketplace.update, cli.install, node.install,
 *     plugin.install / plugin.uninstall {name},
 *     plugin.update {name, marketplace?},
 *     mcp.fix {name, scope, projectPath?} - or {} for every fixable connection
 *   400 = a bad Host, an unreadable body, or an argument the server refused.
 *   403 = a bad or missing token. 409 = a job is already running (the two sync
 *   file edits take the same slot). 422 = ~/.claude/settings.json is not valid
 *   JSON, so the switch cannot be changed; show the server's message.
 *   Every error body is {error: string}, the middlewares' included.
 *
 * GET /api/jobs/{id}?since=N -> 200
 *   { job: { id, action, status: "running"|"succeeded"|"failed", exitCode,
 *            error?, startedAt, finishedAt? },  // error may read "<action> timed out after 10m0s"
 *     log: [{ seq, text }] }
 */

/* This script is in <head>, so the theme lands before the body paints. */
const darkQuery = window.matchMedia("(prefers-color-scheme: dark)");
const applyTheme = () =>
  document.documentElement.classList.toggle("dark", darkQuery.matches);
applyTheme();
darkQuery.addEventListener("change", applyTheme);

const token = document.querySelector('meta[name="risex-token"]').content;

/* ------------------------------------------------------------------ helpers */

const $ = (id) => document.getElementById(id);

const ESCAPES = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
  "'": "&#39;",
};

/** esc makes any server-provided value safe to drop into HTML. */
function esc(value) {
  return String(value == null ? "" : value).replace(
    /[&<>"']/g,
    (c) => ESCAPES[c],
  );
}

function sentence(text) {
  const s = String(text || "");
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/* design-system class shorthands */

const BTN_VARIANT = {
  default: "bg-primary text-primary-foreground hover:bg-primary/85",
  outline: "border-border-strong bg-card hover:bg-muted dark:bg-transparent",
  ghost: "text-muted-foreground hover:bg-muted hover:text-foreground",
  destructive: "text-error-text hover:bg-error/10",
};

const BTN_SIZE = {
  sm: "h-control-sm rounded-md px-2.5",
  xs: "h-control-xs gap-1 rounded-md px-2 text-xs",
  "icon-sm": "size-control-sm rounded-md px-0",
  "icon-xs": "size-control-xs rounded-md px-0",
};

const BADGE = {
  default: "bg-fill-1 text-muted-foreground",
  success: "bg-success/13 text-success-text",
  warning: "bg-warning/15 text-warning-text",
  error: "bg-error/12 text-error-text",
  info: "bg-info/8 text-info-text",
  outline: "border border-border text-muted-foreground",
};

/** btnClass is separate so the <a> and <summary> that look like buttons share it. */
const btnClass = (variant, size) =>
  `ds-btn ${BTN_VARIANT[variant]} ${BTN_SIZE[size]}`;

const dataAttrs = (data) =>
  Object.entries(data || {})
    .map(([key, value]) => ` data-${key}="${esc(value)}"`)
    .join("");

/** btn derives the data-* annotation and the classes from one variant/size pair. */
function btn(label, o) {
  const variant = o.variant || "default";
  const size = o.size || "xs";
  return `<button type="button" data-act="${o.act}"${dataAttrs(o.data)}${o.aria ? ` aria-label="${esc(o.aria)}"` : ""}${o.title ? ` title="${esc(o.title)}"` : ""} data-slot="button" data-variant="${variant}" data-size="${size}" class="${btnClass(variant, size)}${o.cls ? ` ${o.cls}` : ""}"${o.disabled ? " disabled" : ""}>${label}</button>`;
}

/** badge does the same for badges: one key, one class, one annotation. */
function badge(variant, text, o = {}) {
  return `<span data-slot="badge" data-variant="${variant}" class="ds-badge ${BADGE[variant]}${o.cls ? ` ${o.cls}` : ""}"${o.title ? ` title="${esc(o.title)}"` : ""}>${esc(text)}</span>`;
}

const STROKE =
  'fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"';

const ICON = {
  refresh: `<svg viewBox="0 0 24 24" ${STROKE}><path d="M21 12a9 9 0 1 1-2.6-6.4"/><path d="M21 3v6h-6"/></svg>`,
  copy: `<svg viewBox="0 0 24 24" ${STROKE}><rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/></svg>`,
  check: `<svg viewBox="0 0 24 24" ${STROKE}><path d="M20 6 9 17l-5-5"/></svg>`,
  close: `<svg viewBox="0 0 24 24" ${STROKE} class="size-3"><path d="M18 6 6 18M6 6l12 12"/></svg>`,
  info: `<svg viewBox="0 0 24 24" ${STROKE} class="mt-0.5 size-3.5 shrink-0 text-info-text"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4M12 8h.01"/></svg>`,
  alert: `<svg viewBox="0 0 24 24" ${STROKE} class="mt-0.5 size-3.5 shrink-0 text-error-text"><circle cx="12" cy="12" r="10"/><path d="M12 8v5M12 16h.01"/></svg>`,
  terminal: `<svg viewBox="0 0 24 24" ${STROKE} class="size-[15px]"><path d="m9 9 3 3-3 3M14 15h4"/><rect x="2" y="4" width="20" height="16" rx="2"/></svg>`,
  external: `<svg viewBox="0 0 24 24" ${STROKE} class="size-3"><path d="M7 17 17 7M8 7h9v9"/></svg>`,
  chevronDown: '<path d="m6 9 6 6 6-6"/>',
  chevronUp: '<path d="m18 15-6-6-6 6"/>',
};

const SPINNER =
  '<span data-slot="spinner" role="status" aria-label="Working" class="kit-spinner-sm inline-block shrink-0 rounded-full border-border border-t-muted-foreground motion-safe:animate-spin"></span>';

const dot = (cls, title) =>
  `<span data-slot="status-dot" class="inline-block size-1.5 shrink-0 rounded-full ${cls}" title="${esc(title)}"></span>`;

/* copy */

/** Product copy for the two Rise-X skills; the catalog's own text wins. */
const SKILLS = {
  "rise-x-mcp": {
    label: "Rise-X",
    description:
      "Configure workflows, layouts and dashboards, query data, and manage work items on the Rise-X platform.",
  },
  "rise-x-apps": {
    label: "Rise-X Apps",
    description:
      "Design, build, and deploy federated apps for the Rise-X platform.",
  },
};

const skillLabel = (name) => (SKILLS[name] && SKILLS[name].label) || name;

/** Where Locate found the claude binary; see claudecli.Source* in Go. */
const CLI_SOURCE_LABELS = {
  path: "On your PATH",
  "local-bin": "Claude Code installer",
  "desktop-bundle": "Claude Desktop app",
  "windows-probe": "Found on this PC",
};

const CHECK_DOTS = {
  ok: ["bg-success", "Passed"],
  warn: ["bg-warning", "Needs attention"],
  fail: ["bg-error", "Not working"],
  skip: ["bg-fill-3", "Skipped"],
};

const CONNECTION = {
  connected: ["Connected", "success"],
  needs_auth: ["Sign-in needed", "warning"],
  failed: ["Not working", "error"],
  pending: ["Starting up", "default"],
  not_installed: ["Not installed", "outline"],
  managed: ["Managed in Claude Desktop", "info"],
  unknown: ["Unknown", "default"],
};

/** Where a skill came from, for the Status column's badge. */
const SOURCE_BADGE = {
  organisation: () => "Installed by your organisation",
  desktop: () => "Installed through Claude Desktop",
  marketplace: (plugin) =>
    `Installed from ${plugin.sourceName || "another marketplace"}`,
};

const SCOPE_LABELS = {
  user: "everywhere",
  local: "one project",
  desktop: "Claude Desktop",
};

/* api */

async function api(path, options) {
  const res = await fetch(path, {
    ...options,
    headers: { ...(options && options.headers), "X-RiseX-Token": token },
  });
  const body = await res.json().catch(() => null);
  if (!res.ok) {
    const err = new Error((body && body.error) || `HTTP ${res.status}`);
    err.status = res.status;
    throw err;
  }
  return body;
}

/* state */

let overview = null;
let checks = null;
let doctorError = ""; // set when /api/doctor itself failed, for the summary banner
const jobs = []; // newest first
let drawerOpen = false;

function notice(kind, text) {
  const info = kind === "info";
  $("kit-notice").innerHTML = `
    <div data-slot="alert" role="${info ? "status" : "alert"}" class="flex gap-[9px] rounded-lg border px-3 py-2.5 text-xs items-start ${info ? "border-info/25 bg-info/8" : "border-error/25 bg-error/8"}">
      ${info ? ICON.info : ICON.alert}
      <div data-slot="alert-description" class="min-w-0 flex-1 text-xs text-muted-foreground">${esc(text)}</div>
      ${btn(ICON.close, { act: "notice-dismiss", variant: "ghost", size: "icon-xs", aria: "Dismiss" })}
    </div>`;
}

const clearNotice = () => {
  $("kit-notice").innerHTML = "";
};

/* actions */

const JOB_TITLES = {
  "marketplace.add": () => "Add the Rise-X catalog",
  "marketplace.update": () => "Update the skill catalog",
  "plugin.install": (b) => `Install ${skillLabel(b.name)}`,
  "plugin.update": (b) => `Update ${skillLabel(b.name)}`,
  "plugin.uninstall": (b) => `Remove ${skillLabel(b.name)}`,
  "cli.install": () => "Install Claude Code",
  "node.install": () => "Install Node.js",
  "mcp.fix": (b) =>
    b.name ? `Update the address for ${b.name}` : "Update old Rise-X addresses",
};

/**
 * runAction posts one action. A job action opens the activity drawer and
 * starts polling; a sync action returns its result for the caller to report.
 */
async function runAction(name, body, button) {
  const payload = body || {};
  if (button) button.disabled = true;
  clearNotice();
  try {
    const result = await api(`/api/actions/${encodeURIComponent(name)}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    if (result && result.jobId) {
      jobs.unshift({
        id: result.jobId,
        action: name,
        title: (JOB_TITLES[name] || (() => name))(payload),
        subtitle: payload.name || "",
        lines: [],
        rendered: 0,
        since: 0,
        status: "running",
        startedAt: new Date().toISOString(),
      });
      drawerOpen = true;
      renderJobs();
      startPolling(jobs[0]);
    }
    return result;
  } catch (err) {
    notice(
      "error",
      err.status === 409 ? "Another change is still running." : err.message,
    );
    throw err;
  } finally {
    if (button) button.disabled = false;
  }
}

/**
 * startPolling follows one job to its end. The interval id lives in this
 * closure, so a late answer for one job can never stop another's polling, and
 * inFlight keeps one request outstanding at a time.
 */
function startPolling(job) {
  if (job.timer) return;
  let inFlight = false;
  job.timer = setInterval(async () => {
    if (inFlight) return;
    inFlight = true;
    try {
      const snap = await api(
        `/api/jobs/${encodeURIComponent(job.id)}?since=${job.since}`,
      );
      for (const line of snap.log || []) {
        job.lines.push(line.text);
        job.since = line.seq;
      }
      job.status = snap.job.status;
      job.startedAt = snap.job.startedAt;
      job.finishedAt = snap.job.finishedAt;
      job.error = snap.job.error || "";
      patchJob(job);
      if (job.status === "running") return; // still going: keep polling
      stopPolling(job);
      if (job.status === "failed")
        notice("error", job.error || `${job.title} did not finish.`);
      // A new Node.js lives somewhere the cached probe never looked, so this
      // one job re-probes the machine instead of reading the cache.
      refresh(job.action === "node.install");
    } catch (err) {
      stopPolling(job);
      job.status = "failed";
      job.error = err.message;
      patchJob(job);
      notice("error", err.message);
    } finally {
      inFlight = false;
    }
  }, 500);
}

function stopPolling(job) {
  clearInterval(job.timer);
  job.timer = null;
}

/* render */

function renderVersion() {
  const badge = $("kit-version");
  badge.textContent = overview.kitVersion || "";
  badge.hidden = !overview.kitVersion;
}

/** Alert colours per summary tone, with the role that announces it: a problem
 * interrupts the reader, a good result waits to be read. */
const SUMMARY_TONES = {
  neutral: ["border-border bg-muted/50", "status"],
  success: ["border-success/25 bg-success/8", "status"],
  warning: ["border-warning/30 bg-warning/10", "alert"],
  error: ["border-error/25 bg-error/8", "alert"],
};

/** cliCheckFailed reports whether a claude command the checks depend on could
 * not be read. Those rows are skips, so nothing in the counts below would
 * otherwise say the answer is unknown. */
function cliCheckFailed() {
  // The doctor can answer before the overview on a cached load.
  const ov = overview || {};
  return (
    !!(ov.marketplace || {}).checkError ||
    (ov.plugins || []).some((plugin) => plugin.checkError)
  );
}

/** checkLink turns one doctor row's title into a jump to that row. */
const checkLink = (check) =>
  `<a href="#check-${esc(check.id)}" data-act="jump" data-check="${esc(check.id)}" class="text-foreground underline decoration-border-strong underline-offset-2 hover:decoration-foreground">${esc(check.title)}</a>`;

/**
 * summaryState reduces the doctor's checks to the one line above the cards.
 * Failed rows come before warnings in the link list: a failure is what to deal
 * with first. body is a list of blocks, so the fail state can name a starting
 * point on its own line.
 */
function summaryState() {
  if (doctorError) {
    return {
      tone: "error",
      glyph: dot("bg-error mt-1.5", "Not working"),
      title: "Could not check your setup.",
      body: [esc(doctorError)],
    };
  }
  if (!checks) {
    return {
      tone: "neutral",
      glyph: `<span class="mt-0.5 inline-flex">${SPINNER}</span>`,
      title: "Checking your setup…",
      body: [],
    };
  }

  const fails = checks.filter((check) => check.status === "fail");
  const warns = checks.filter((check) => check.status === "warn");
  const links = [...fails, ...warns].map(checkLink).join(", ");

  if (fails.length) {
    return {
      tone: "error",
      glyph: dot("bg-error mt-1.5", "Not working"),
      title: "Rise-X is not ready yet",
      body: [`Start with: ${esc(fails[0].title)}.`, links],
    };
  }
  if (cliCheckFailed()) {
    return {
      tone: "warning",
      glyph: dot("bg-warning mt-1.5", "Needs attention"),
      title: "Some checks could not run",
      body: ["Try Run again.", links].filter(Boolean),
    };
  }
  if (warns.length) {
    return {
      tone: "warning",
      glyph: dot("bg-warning mt-1.5", "Needs attention"),
      title:
        warns.length === 1
          ? "One thing needs your attention"
          : `${warns.length} things need your attention`,
      body: [links],
    };
  }
  return {
    tone: "success",
    glyph: dot("bg-success mt-1.5", "Passed"),
    title: "Everything is set up",
    body: [
      "Your Rise-X skills and connection are ready to use in Claude Code Desktop.",
    ],
  };
}

function renderSummary() {
  const state = summaryState();
  const [tone, role] = SUMMARY_TONES[state.tone];
  $("kit-summary").innerHTML = `
    <div data-slot="alert" role="${role}" class="flex gap-[9px] rounded-lg border px-3 py-2.5 text-xs items-start ${tone}">
      ${state.glyph}
      <div class="min-w-0 flex-1">
        <div data-slot="alert-title" class="mb-px text-ui font-medium">${esc(state.title)}</div>
        ${
          state.body.length
            ? `<div data-slot="alert-description" class="text-xs text-muted-foreground">${state.body
                .map(
                  (block, i) =>
                    `<div${i ? ' class="mt-1"' : ""}>${block}</div>`,
                )
                .join("")}</div>`
            : ""
        }
      </div>
    </div>`;
}

/** jumped flashes the doctor row a summary link points at. The browser does
 * the scrolling from the link's own fragment; .kit-check keeps the sticky
 * header off it. */
function jumped(link) {
  const row = $(`check-${link.dataset.check}`);
  if (!row) return;
  row.classList.add("kit-check-flash");
  setTimeout(() => row.classList.remove("kit-check-flash"), 1500);
}

function renderBanner() {
  $("kit-banner").innerHTML = overview.reloadHint
    ? `<div data-slot="alert" role="status" class="flex gap-[9px] rounded-lg border px-3 py-2.5 text-xs border-info/25 bg-info/8 items-start">
        ${ICON.info}
        <div class="min-w-0 flex-1">
          <div data-slot="alert-title" class="mb-px text-ui font-medium">Your changes are ready to load</div>
          <div data-slot="alert-description" class="text-xs text-muted-foreground">
            Restart Claude Code Desktop, or type
            <kbd data-slot="kbd" class="inline-flex h-[18px] min-w-[18px] shrink-0 items-center justify-center rounded-sm bg-fill-2 px-1 font-sans text-[10px] font-medium text-current select-none">/reload-plugins</kbd>
            in a session, to load the changes.
          </div>
        </div>
        ${btn(ICON.close, { act: "banner-dismiss", variant: "ghost", size: "icon-xs", aria: "Dismiss" })}
      </div>`
    : "";
}

/**
 * listItem renders one row of a bordered list. The CLI facts, the connection
 * targets and the doctor checks all use it, so they cannot drift apart.
 */
function listItem({
  id,
  cls,
  dot,
  name,
  code,
  description,
  detail,
  detailKey,
  trailing,
  muted,
}) {
  return `
    <div data-slot="list-item"${id ? ` id="${esc(id)}"` : ""} class="flex items-center gap-2.5 border-t border-border-subtle px-3.5 py-2.5 first:border-t-0${cls ? ` ${cls}` : ""}">
      ${dot || ""}
      <div data-slot="list-main" class="min-w-0 flex-1">
        <span data-slot="list-name" class="flex items-center gap-2 text-ui${muted ? " text-muted-foreground" : ""}">${esc(name)}${code ? `<span class="text-micro text-subtle font-mono">${esc(code)}</span>` : ""}</span>
        ${description ? `<span data-slot="list-description" class="mt-0.5 block text-xs ${muted ? "text-subtle" : "text-muted-foreground"}">${esc(description)}</span>` : ""}
        ${detail ? lines(detail, detailKey) : ""}
      </div>
      ${trailing || ""}
    </div>`;
}

/** lines shows the exact entries a fix would change, collapsed by default.
 * key is stable across refreshes, so one the reader opened stays open. */
function lines(detail, key) {
  return `
    <details data-slot="collapsible" class="mt-1"${key ? ` data-detail="${esc(key)}"` : ""}>
      <summary data-slot="collapsible-trigger" class="${btnClass("ghost", "xs")} w-fit list-none px-1">
        Show lines
        <svg viewBox="0 0 24 24" ${STROKE}>${ICON.chevronDown}</svg>
      </summary>
      <pre data-slot="collapsible-content" class="kit-log mt-1.5 rounded-md bg-fill-0 px-3 py-2 text-muted-foreground">${esc(detail)}</pre>
    </details>`;
}

function renderCli() {
  const cli = overview.cli;

  if (cli && cli.found) {
    const sourceLabel = CLI_SOURCE_LABELS[cli.source];
    $("kit-cli-badge").innerHTML = badge("success", "Ready");
    $("kit-cli-body").innerHTML = `
      <div data-slot="list" class="flex flex-col rounded-lg border border-border-subtle">
        ${listItem({ name: "Location", trailing: `<span data-slot="list-value" class="shrink-0 text-ui text-muted-foreground font-mono text-xs">${esc(cli.path)}</span>` })}
        ${listItem({ name: "Version", trailing: `<span data-slot="list-value" class="shrink-0 text-ui tabular-nums">${esc(cli.version)}</span>` })}
        ${sourceLabel ? listItem({ name: "Found in", trailing: badge("default", sourceLabel) }) : ""}
      </div>`;
    return;
  }

  $("kit-cli-badge").innerHTML = badge("error", "Not found");
  $("kit-cli-body").innerHTML = `
    <div data-slot="empty-state" class="flex flex-col items-center gap-[3px] px-4 text-center py-14 rounded-lg bg-fill-0">
      <span data-slot="empty-state-glyph" class="mb-[5px] grid size-8 place-items-center rounded-lg bg-muted text-subtle">${ICON.terminal}</span>
      <div data-slot="empty-state-title" class="text-ui font-medium">Claude Code command-line tool not found</div>
      <div data-slot="empty-state-description" class="max-w-[42ch] text-xs text-muted-foreground">
        Kit needs it to manage your skills. Installing takes about a minute and does not change anything else on your machine.
      </div>
      <div data-slot="empty-state-actions" class="mt-3 flex items-center gap-2">
        ${btn("Install Claude Code", { act: "cli-install", size: "sm" })}
        ${btn("Rescan", { act: "cli-rescan", variant: "outline", size: "sm" })}
      </div>
    </div>`;
}

const CELL =
  "border-b border-border-subtle px-4 py-3.5 align-middle whitespace-nowrap";

function skillStatus(plugin) {
  const skillBtn = (label, op, variant, data) =>
    btn(label, {
      act: "skill",
      variant,
      data: { op, name: plugin.name, ...data },
    });
  const remove = skillBtn("Remove", "uninstall", "destructive");

  // The list that says whether this skill is installed could not be read, so
  // the row has nothing to offer: installing over an existing copy would
  // leave two.
  if (plugin.checkError) {
    return {
      badge: badge("default", "Not checked", { title: plugin.checkError }),
      actions: "",
    };
  }

  if (!plugin.installed) {
    return {
      badge: badge("outline", "Not installed"),
      actions: skillBtn("Install", "install", "default"),
    };
  }

  if (!plugin.enabled) {
    // Removing a copy that came from elsewhere is not this page's to offer.
    return {
      badge: badge("outline", "Turned off"),
      actions: plugin.installSource === "public" ? remove : "",
    };
  }

  // A copy from the organisation or another marketplace must not be installed,
  // updated or removed from the public one: that would leave two copies.
  const sourceLabel = SOURCE_BADGE[plugin.installSource];
  if (sourceLabel) {
    const behind = plugin.updateAvailable
      ? plugin.installSource === "organisation"
        ? `Your organisation's copy is ${plugin.localVersion}; public is ${plugin.publicVersion}`
        : `This copy is ${plugin.localVersion}; public is ${plugin.publicVersion}`
      : "";
    return {
      badge: badge("info", sourceLabel(plugin)),
      note: behind,
      actions:
        plugin.installSource === "marketplace" && plugin.updateAvailable
          ? skillBtn("Update", "update", "default", {
              marketplace: plugin.sourceName || "",
            })
          : "",
    };
  }

  if (plugin.versionUnknown || !plugin.localVersion) {
    return {
      badge: badge("default", "Version unknown"),
      actions: `${skillBtn("Reinstall", "update", "outline")}${remove}`,
    };
  }

  const canUpdate = !!plugin.updateAvailable;
  const actions = canUpdate
    ? `${skillBtn("Update", "update", "default")}${remove}`
    : remove;

  // The public version is unverified in both branches below, so neither may
  // claim "Up to date": the comparison only means something once it is.
  if (plugin.publicCheckError) {
    return {
      badge: badge("warning", "Check failed", {
        title: plugin.publicCheckError,
      }),
      actions,
    };
  }
  if (plugin.offline) {
    return { badge: badge("default", "Not checked"), actions };
  }
  if (canUpdate) {
    return { badge: badge("warning", "Update available"), actions };
  }
  return { badge: badge("success", "Up to date"), actions };
}

function renderSkills() {
  const plugins = overview.plugins || [];

  $("kit-skills-body").innerHTML = plugins.length
    ? plugins.map(skillRow).join("")
    : `<tr><td colspan="5" class="px-4 py-10 text-center text-xs text-muted-foreground">${
        overview.cli && overview.cli.found
          ? "No Rise-X skills in the catalog yet."
          : "Install Claude Code to see your Rise-X skills."
      }</td></tr>`;

  renderSkillsFooter();
}

function skillRow(plugin, i) {
  const { badge, note, actions } = skillStatus(plugin);
  const description =
    plugin.description ||
    (SKILLS[plugin.name] && SKILLS[plugin.name].description) ||
    "";
  const installed = !plugin.installed
    ? '<span class="text-subtle">&mdash;</span>'
    : plugin.localVersion
      ? `<span class="tabular-nums">${esc(plugin.localVersion)}</span>`
      : '<span class="text-subtle">Unknown</span>';
  const published = plugin.publicVersion
    ? `<span class="tabular-nums">${esc(plugin.publicVersion)}</span>${plugin.offline ? ' <span class="text-micro text-subtle">from your copy</span>' : ""}`
    : '<span class="text-subtle">&mdash;</span>';
  const descId = `skill-desc-${i}`;

  return `
    <tr data-slot="table-row" class="transition-colors duration-150 ease-decelerate hover:bg-fill-0">
      <td data-slot="table-cell" class="border-b border-border-subtle px-4 py-3.5 align-middle">
        <div data-slot="table-cell-stack" class="min-w-0">
          <div class="flex items-center gap-2 whitespace-nowrap">
            <span class="text-ui font-medium">${esc(skillLabel(plugin.name))}</span>
            ${
              description
                ? `<span class="kit-info-trigger" tabindex="0" role="img" aria-label="About this skill" aria-describedby="${descId}" data-tip="${esc(description)}"><svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4M12 8h.01"/></svg></span>
                   <span id="${descId}" class="sr-only">${esc(description)}</span>`
                : ""
            }
          </div>
          <span class="text-micro text-subtle font-mono whitespace-nowrap">${esc(plugin.name)}</span>
        </div>
      </td>
      <td data-slot="table-cell" class="${CELL} tabular-nums">${installed}</td>
      <td data-slot="table-cell" class="${CELL} tabular-nums">${published}</td>
      <td data-slot="table-cell" class="${CELL}">
        <div class="flex flex-col items-start gap-1">
          ${badge}
          ${note ? `<span class="kit-wrap max-w-[42ch] text-micro text-muted-foreground">${esc(note)}</span>` : ""}
        </div>
      </td>
      <td data-slot="table-cell" class="${CELL}"><div class="flex items-center gap-1.5 kit-end">${actions}</div></td>
    </tr>`;
}

function catalogStatus(marketplace) {
  const update = btn("Update catalog", {
    act: "catalog-update",
    variant: "outline",
  });

  if (marketplace.checkError) {
    return `<span class="inline-flex items-center gap-1.5" title="${esc(marketplace.checkError)}">${dot("bg-fill-3", "Not checked")} Kit could not check the catalog just now</span>`;
  }
  if (!marketplace.registered) {
    return `<span class="inline-flex items-center gap-2">
      <span class="inline-flex items-center gap-1.5">${dot("bg-error", "Not set up")} The Rise-X catalog is not set up yet</span>
      ${btn("Set it up", { act: "marketplace-add" })}
    </span>`;
  }
  if (marketplace.headStale === true) {
    return `<span class="inline-flex items-center gap-2">
      <span class="inline-flex items-center gap-1.5">${dot("bg-warning", "Behind")} A newer catalog is available</span>
      ${update}
    </span>`;
  }
  if (marketplace.headStale === false) {
    return `<span class="inline-flex items-center gap-1.5">${dot("bg-success", "Current")} Catalog: current</span>`;
  }
  return `<span class="inline-flex items-center gap-2">
    <span class="inline-flex items-center gap-1.5">${dot("bg-fill-3", "Not checked")} Kit could not reach GitHub to check the catalog</span>
    ${update}
  </span>`;
}

/** True once every catalog plugin comes from the organisation: nothing on this
 * machine reads the public marketplace's autoUpdate flag, so the checkbox
 * that controls it has nothing to do. Mirrors doctor.allFromOrg. */
function isOrgManaged(ov) {
  const plugins = ov.plugins || [];
  return (
    plugins.length > 0 &&
    plugins.every((p) => p.installSource === "organisation")
  );
}

const ORG_MANAGED_TIP =
  "Your organisation installs and updates Rise-X skills through its own marketplace, so this setting is not used on this machine.";

const SETTINGS_ERROR_TIP =
  "Could not read ~/.claude/settings.json, so this setting cannot be checked or changed. Fix the file first.";

function renderSkillsFooter() {
  const marketplace = overview.marketplace || {};
  // Every catalog button needs the CLI, and "not set up yet" would be a guess
  // without one: with no CLI the footer is the auto-update switch alone.
  const cliFound = !!(overview.cli && overview.cli.found);
  const on = marketplace.autoUpdate === true;
  const orgManaged = isOrgManaged(overview);
  const settingsError = marketplace.settingsError === true;
  const disabled = orgManaged || settingsError;
  const tip = orgManaged ? ORG_MANAGED_TIP : SETTINGS_ERROR_TIP;
  const caption = orgManaged
    ? "Managed by your organisation."
    : settingsError
      ? "Fix ~/.claude/settings.json to check this setting."
      : "Claude Code refreshes the catalog and updates installed skills after each session starts.";

  $("kit-skills-footer").innerHTML = `
    <div class="min-w-0 flex-1">${cliFound ? catalogStatus(marketplace) : ""}</div>
    <div data-slot="choice-row" class="flex items-start gap-2.5 shrink-0"${disabled ? ` data-tip="${esc(tip)}" title="${esc(tip)}" tabindex="0"` : ""}>
      <button
        type="button" role="checkbox" data-act="autoupdate" id="kit-autoupdate"
        aria-checked="${on}" data-state="${on ? "checked" : "unchecked"}" ${on ? "data-checked" : ""}
        ${disabled ? 'disabled data-disabled="" aria-disabled="true" aria-describedby="autoupdate-tip"' : ""}
        data-slot="checkbox"
        class="peer size-[15px] mt-0.5 shrink-0 rounded-[4px] border border-input bg-card transition-colors duration-100 outline-none focus-visible:ring-3 focus-visible:ring-ring-control/35 disabled:cursor-not-allowed disabled:opacity-45 data-checked:border-primary data-checked:bg-primary data-checked:text-primary-foreground flex items-center justify-center"
      >
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3.5" stroke-linecap="round" stroke-linejoin="round" class="size-3" aria-hidden="true" ${on ? "" : "hidden"}><path d="M20 6 9 17l-5-5"/></svg>
      </button>
      <div class="min-w-0 peer-disabled:cursor-not-allowed peer-disabled:opacity-50">
        <div class="flex items-center gap-1.5">
          <label for="kit-autoupdate" data-slot="label" class="text-ui font-medium text-foreground select-none">Keep Rise-X skills up to date automatically</label>
          ${disabled ? `<span id="autoupdate-tip" class="sr-only">${esc(tip)}</span>` : ""}
        </div>
        <span class="mt-0.5 block text-micro text-subtle">${caption}</span>
      </div>
    </div>`;
}

/** MOVED_NOTE explains a connector that predates the mcp.rise-x.io addresses. */
const MOVED_NOTE =
  'Connectors added in Claude Desktop before the move to mcp.rise-x.io still point at the old address; remove them in <strong class="font-medium text-foreground">Customize</strong> &rsaquo; <strong class="font-medium text-foreground">Connectors</strong> and add the new one.';

const GUIDE_STEPS = [
  'Open <strong class="font-medium text-foreground">Claude Desktop</strong> &rsaquo; <strong class="font-medium text-foreground">Customize</strong> &rsaquo; <strong class="font-medium text-foreground">Connectors</strong>, then press <strong class="font-medium text-foreground">Add</strong>.',
  "Paste the name and address from the rows above. Leave the headers empty.",
  'Sign in when the browser opens, then come back here and press <strong class="font-medium text-foreground">Recheck</strong>.',
];

function connectionRows(mcp) {
  const configured = mcp.configured || [];
  const rows = configured.length
    ? configured.map((s) => ({ name: s.name, target: s.url || s.command }))
    : (mcp.servers || []).map((s) => ({ name: s.name, target: s.target }));

  return rows
    .map((row) =>
      listItem({
        name: row.name,
        description: /test/i.test(row.name)
          ? "A safe copy for trying things out"
          : "Your live Rise-X environment",
        trailing:
          `<span class="shrink-0 font-mono text-xs text-muted-foreground">${esc(row.target)}</span>` +
          btn(ICON.copy, {
            act: "copy",
            variant: "ghost",
            size: "icon-sm",
            data: { copy: row.target },
            aria: `Copy ${row.target}`,
          }),
      }),
    )
    .join("");
}

/** staleRows lists each connection still on an old Rise-X address. */
function staleRows(stale) {
  return stale
    .map((entry) =>
      listItem({
        dot: dot("bg-warning", "Old address"),
        name: entry.name,
        code: SCOPE_LABELS[entry.scope] || entry.scope,
        description: `${entry.url} → ${entry.suggestedUrl}`,
        trailing:
          entry.scope === "desktop"
            ? '<span class="kit-wrap max-w-[42ch] shrink-0 text-right text-xs text-muted-foreground">Update this one in Claude Desktop &rsaquo; Customize &rsaquo; Connectors.</span>'
            : btn("Fix", {
                act: "mcp-fix",
                variant: "outline",
                cls: "shrink-0",
                data: {
                  name: entry.name,
                  scope: entry.scope,
                  project: entry.projectPath || "",
                },
              }),
      }),
    )
    .join("");
}

function renderConnection() {
  const mcp = overview.mcp || { verdict: "unknown" };
  const [label, variant] = CONNECTION[mcp.verdict] || CONNECTION.unknown;
  $("kit-conn-badge").innerHTML = badge(variant, label);

  const rows = connectionRows(mcp);
  const managed = mcp.verdict === "managed";

  const guide = managed
    ? `<div class="rounded-lg bg-fill-0 p-3.5">
         <div class="text-xs font-medium text-foreground">Managed in Claude Desktop</div>
         <div class="mt-1 text-xs text-muted-foreground">
           Claude Desktop set these up from your account, so Kit cannot check them from here.
           Open <strong class="font-medium text-foreground">Customize</strong> &rsaquo; <strong class="font-medium text-foreground">Connectors</strong> to see whether they are signed in.
         </div>
       </div>`
    : `<div class="rounded-lg bg-fill-0 p-3.5">
         <div class="text-xs font-medium text-foreground">Add it in Claude Desktop</div>
         <ol class="mt-2.5 flex flex-col gap-2">${GUIDE_STEPS.map(
           (step, i) => `<li class="flex items-start gap-2.5">
             <span class="mt-0.5 grid size-5 shrink-0 place-items-center rounded-full bg-fill-2 text-micro font-medium text-muted-foreground tabular-nums">${i + 1}</span>
             <span class="text-xs text-muted-foreground">${step}</span>
           </li>`,
         ).join("")}</ol>
         <div class="mt-2.5 text-xs text-muted-foreground">${MOVED_NOTE}</div>
       </div>`;

  const stale = mcp.stale || [];
  const staleBlock = stale.length
    ? `<div data-slot="list" class="flex flex-col rounded-lg border border-warning/30">
         <div class="border-b border-border-subtle px-3.5 py-2.5 text-xs font-medium text-foreground">Old Rise-X addresses</div>
         ${staleRows(stale)}
       </div>`
    : "";

  // A check that could not run leaves the badge on "Unknown"; the server says
  // why in plain words.
  const note = mcp.message
    ? `<div data-slot="alert" role="status" class="flex gap-[9px] rounded-lg border px-3 py-2.5 text-xs items-start border-info/25 bg-info/8">
         ${ICON.info}
         <div data-slot="alert-description" class="min-w-0 flex-1 text-xs text-muted-foreground">${esc(mcp.message)}</div>
       </div>`
    : "";

  const body = rows
    ? `${note}
       <div data-slot="list" class="flex flex-col rounded-lg border border-border-subtle">${rows}</div>
       ${staleBlock}
       ${guide}`
    : `${note}
       ${staleBlock}
       <div class="rounded-lg bg-fill-0 px-3.5 py-3 text-xs text-muted-foreground">
         Install the Rise-X skill above, then its connection details appear here.
       </div>`;

  $("kit-conn-body").innerHTML = `
    ${body}
    <div class="flex items-center gap-2">
      ${btn(`${ICON.refresh}Recheck`, { act: "recheck", variant: "outline", size: "sm" })}
    </div>
    ${
      mcp.raw
        ? `<details data-slot="collapsible" data-detail="mcp-raw">
             <summary data-slot="collapsible-trigger" class="${btnClass("ghost", "sm")} w-fit list-none">
               Show raw check output
               <svg viewBox="0 0 24 24" ${STROKE}>${ICON.chevronDown}</svg>
             </summary>
             <div data-slot="collapsible-content" class="mt-2 rounded-lg bg-fill-0 px-3.5 py-3">
               <pre class="kit-log text-muted-foreground">${esc(mcp.raw.trim())}</pre>
             </div>
           </details>`
        : ""
    }`;
}

function checkAction(check) {
  if (check.fix) {
    return btn(check.fixLabel || "Fix", {
      act: "fix",
      variant: "outline",
      cls: "shrink-0",
      title: check.fixTitle || "",
      data: {
        fix: check.fix,
        args: JSON.stringify(check.fixArgs || {}),
        detail: check.detail || "",
      },
    });
  }
  if (check.id === "node" && check.status !== "ok") {
    return `<a href="https://nodejs.org/en/download" target="_blank" rel="noreferrer noopener" data-slot="button" data-variant="ghost" data-size="xs" class="${btnClass("ghost", "xs")} shrink-0">How to install${ICON.external}</a>`;
  }
  return "";
}

function renderDoctor() {
  if (!checks) {
    $("kit-doctor-desc").textContent = doctorError
      ? "Could not check your setup."
      : "Checking your setup…";
    $("kit-doctor-list").innerHTML = doctorError
      ? `<div class="px-3.5 py-2.5 text-xs text-muted-foreground">${esc(doctorError)}</div>`
      : "";
    return;
  }
  $("kit-doctor-desc").textContent =
    `${checks.length} checks, checked just now. Anything that needs you has a button next to it.`;

  $("kit-doctor-list").innerHTML = checks
    .map((check) => {
      const [dotClass, dotTitle] = CHECK_DOTS[check.status] || CHECK_DOTS.skip;
      const isSkill = check.id.startsWith("plugin.");
      return listItem({
        id: `check-${check.id}`,
        cls: "kit-check",
        dot: dot(dotClass, dotTitle),
        name: check.title,
        code: isSkill ? check.id.slice("plugin.".length) : "",
        description: sentence(check.message),
        detail: check.detail,
        detailKey: check.id,
        trailing: checkAction(check),
        muted: check.status === "skip",
      });
    })
    .join("");
}

function seconds(job) {
  if (!job.startedAt || !job.finishedAt) return "";
  const ms = new Date(job.finishedAt) - new Date(job.startedAt);
  return `${Math.max(1, Math.round(ms / 1000))} s`;
}

const jobGlyph = (job) =>
  job.status === "running"
    ? SPINNER
    : dot(
        job.status === "failed" ? "bg-error" : "bg-success",
        job.status === "failed" ? "Failed" : "Done",
      );

const jobResult = (job) =>
  job.status === "running"
    ? ""
    : job.status === "failed"
      ? '<span class="text-xs text-error-text">Failed</span>'
      : `<span class="text-xs text-muted-foreground">Done in <span class="tabular-nums">${seconds(job)}</span></span>`;

const jobError = (job) => (job.status === "failed" && job.error) || "";

/**
 * jobItem renders one drawer entry with every part present even when empty,
 * so patchJob can update it in place. job.rendered tracks how many log lines
 * are already in the DOM.
 */
function jobItem(job) {
  job.rendered = job.lines.length;
  const error = jobError(job);
  return `
    <div data-slot="item" id="kit-job-${esc(job.id)}" class="flex flex-col rounded-lg border border-border-subtle p-3.5">
      <div class="flex items-center gap-2.5">
        <span data-job-glyph class="inline-flex shrink-0">${jobGlyph(job)}</span>
        <span data-slot="item-title" class="text-ui font-medium">${esc(job.title)}</span>
        ${job.subtitle ? `<span class="text-micro text-subtle font-mono">${esc(job.subtitle)}</span>` : ""}
        <span class="flex-1"></span>
        <span data-job-result>${jobResult(job)}</span>
      </div>
      <div data-job-error class="mt-1.5 text-xs text-error-text"${error ? "" : " hidden"}>${esc(error)}</div>
      <pre data-job-log class="kit-log mt-2.5 rounded-md bg-fill-0 px-3 py-2 text-muted-foreground"${job.lines.length ? "" : " hidden"}>${esc(job.lines.join("\n"))}</pre>
    </div>`;
}

/** renderJobsHeader draws the collapsed bar: it is the same whether or not the
 * drawer is open. */
function renderJobsHeader() {
  const running = jobs.find((job) => job.status === "running");
  const finished = jobs.length - (running ? 1 : 0);

  $("kit-jobs-glyph").innerHTML = running
    ? SPINNER
    : dot(finished ? "bg-success" : "bg-fill-3", finished ? "Done" : "Idle");
  $("kit-jobs-summary").textContent = running
    ? `${running.title}…`
    : finished
      ? `${finished} finished`
      : "Nothing yet";

  const badge = $("kit-jobs-badge");
  badge.hidden = !running || !finished;
  badge.textContent = String(finished);

  $("kit-jobs-toggle").setAttribute("aria-expanded", String(drawerOpen));
  $("kit-jobs-chevron").innerHTML = drawerOpen
    ? ICON.chevronDown
    : ICON.chevronUp;
  $("kit-jobs-panel").hidden = !drawerOpen;
}

/** renderJobs rebuilds the drawer. Only opening it, or starting a job, calls
 * this; a poll patches the entry it is about. */
function renderJobs() {
  renderJobsHeader();
  if (!drawerOpen) return;

  $("kit-jobs-panel").innerHTML = jobs.length
    ? jobs.map(jobItem).join("")
    : '<div class="rounded-lg border border-border-subtle px-3.5 py-2.5 text-xs text-muted-foreground">Nothing has run yet in this session.</div>';

  const running = jobs.find((job) => job.status === "running");
  if (running) scrollLog(running);
}

/**
 * patchJob updates one entry in place: its glyph, its result, and the log
 * lines that arrived since the last poll. Rebuilding the drawer twice a second
 * instead would drop the reader's text selection and re-collapse the page.
 */
function patchJob(job) {
  renderJobsHeader();
  if (!drawerOpen) return;
  const block = $(`kit-job-${job.id}`);
  if (!block) {
    renderJobs(); // the entry is new to the drawer
    return;
  }
  block.querySelector("[data-job-glyph]").innerHTML = jobGlyph(job);
  block.querySelector("[data-job-result]").innerHTML = jobResult(job);

  const error = block.querySelector("[data-job-error]");
  error.textContent = jobError(job);
  error.hidden = !jobError(job);

  if (job.lines.length > job.rendered) {
    const log = block.querySelector("[data-job-log]");
    const fresh = job.lines.slice(job.rendered).join("\n");
    log.textContent += job.rendered ? `\n${fresh}` : fresh;
    job.rendered = job.lines.length;
    log.hidden = false;
    log.scrollTop = log.scrollHeight;
  }
}

function scrollLog(job) {
  const block = $(`kit-job-${job.id}`);
  const log = block && block.querySelector("[data-job-log]");
  if (log) log.scrollTop = log.scrollHeight;
}

/* skeletons */

/**
 * The first paint, before /api/overview answers. Each block mirrors the row
 * count and height of the card it stands in for, so the real content lands in
 * the same place and nothing jumps. Shown once, on startup: a later refresh
 * keeps what is already on screen and reports itself through the pressed
 * control's spinner instead.
 */
const bar = (size) =>
  `<span class="kit-skeleton kit-skeleton-${size} bg-fill-2" aria-hidden="true"></span>`;

const skelRow = (left, right) => `
  <div class="flex items-center gap-2.5 border-t border-border-subtle px-3.5 py-2.5 first:border-t-0">
    <div class="min-w-0 flex-1">${bar(left)}</div>
    ${right ? bar(right) : ""}
  </div>`;

const skelList = (...rows) =>
  `<div class="flex flex-col rounded-lg border border-border-subtle">${rows.join("")}</div>`;

const skelSkillRow = () => `
  <tr>
    <td class="border-b border-border-subtle px-4 py-3.5 align-middle">${bar("md")}</td>
    <td class="${CELL}">${bar("xs")}</td>
    <td class="${CELL}">${bar("xs")}</td>
    <td class="${CELL}">${bar("sm")}</td>
    <td class="${CELL}"><div class="flex kit-end">${bar("sm")}</div></td>
  </tr>`;

/** BUSY_REGIONS are the containers a skeleton fills. aria-busy tells a screen
 * reader the content is still coming; the summary banner says so in words. */
const BUSY_REGIONS = [
  "kit-cli-badge",
  "kit-cli-body",
  "kit-conn-badge",
  "kit-conn-body",
  "kit-skills-body",
  "kit-doctor-list",
];

function renderSkeletons() {
  const fill = (id, html) => {
    const el = $(id);
    el.setAttribute("aria-busy", "true");
    el.innerHTML = html;
  };
  fill("kit-cli-badge", bar("badge"));
  fill(
    "kit-cli-body",
    skelList(skelRow("sm", "xl"), skelRow("sm", "xs"), skelRow("sm", "md")),
  );
  fill("kit-conn-badge", bar("badge"));
  fill("kit-conn-body", skelList(skelRow("md", "lg"), skelRow("md", "lg")));
  fill("kit-skills-body", skelSkillRow() + skelSkillRow());
  fill(
    "kit-doctor-list",
    Array.from({ length: 6 }, () => skelRow("lg", "xs")).join(""),
  );
}

function clearBusy() {
  for (const id of BUSY_REGIONS) $(id).removeAttribute("aria-busy");
}

/* load */

async function loadOverview(fresh) {
  overview = await api(fresh ? "/api/overview?fresh=1" : "/api/overview");
  renderVersion();
  renderBanner();
  renderCli();
  renderSkills();
  renderConnection();
  syncRefreshControls();
}

/** doctorLoading paints the "still working" state a fresh re-probe needs,
 * since that takes seconds and the old answer must not stand meanwhile. */
function doctorLoading() {
  checks = null;
  doctorError = "";
  renderSummary();
  renderDoctor();
}

async function loadDoctor() {
  try {
    checks = (await api("/api/doctor")).checks || [];
    doctorError = "";
  } catch (err) {
    // The rows below would otherwise keep showing the previous run's verdicts
    // under a banner saying the check failed.
    checks = null;
    doctorError = err.message;
    renderSummary();
    renderDoctor();
    throw err;
  }
  renderSummary();
  renderDoctor();
}

/** openDetails/restoreDetails keep a disclosure the reader opened open across
 * a refresh; each one carries a key that is stable between renders. */
function openDetails() {
  return new Set(
    Array.from(
      document.querySelectorAll("details[data-detail][open]"),
      (el) => el.dataset.detail,
    ),
  );
}

function restoreDetails(keys) {
  for (const el of document.querySelectorAll("details[data-detail]")) {
    if (keys.has(el.dataset.detail)) el.open = true;
  }
}

/** focusKey identifies the control the reader is on by what it does, so a
 * refresh that replaces the element can put focus back on its successor. */
function focusKey(el) {
  const data = el && el.dataset;
  if (!data || !data.act) return "";
  return [
    data.act,
    data.op || "",
    data.name || data.server || data.fix || data.check || "",
  ].join("|");
}

/** restoreFocus puts focus back only where the re-render took it away: an
 * element still in the document, or one the reader moved to meanwhile, keeps
 * it. A fresh gather takes seconds, and stealing the caret back from wherever
 * they went next is worse than losing it. */
function restoreFocus(key, was) {
  if (!key) return;
  const active = document.activeElement;
  if (active && active !== document.body && active !== was) return;
  if (was && was.isConnected) return;
  for (const el of document.querySelectorAll("[data-act]")) {
    if (focusKey(el) === key) {
      el.focus();
      return;
    }
  }
}

/**
 * Refresh, Recheck and Run again all run the same gather, so while one is in
 * flight every one of them is disabled: a second ?fresh=1 only drops the probe
 * caches again and re-spawns claude for an answer the first is already
 * fetching. refreshingAct names the one that was pressed, so the spinner lands
 * on it.
 */
const REFRESH_ACTS = ["refresh", "recheck", "doctor-again"];
let refreshing = false;
let refreshingAct = "";
let queued = false;
let queuedFresh = false;

/** syncRefreshControls applies that lock to whatever is in the document now.
 * It is re-applied after each render, because renderConnection replaces the
 * Recheck button while the gather that disabled it is still running. */
function syncRefreshControls() {
  for (const el of document.querySelectorAll("[data-act]")) {
    if (!REFRESH_ACTS.includes(el.dataset.act)) continue;
    el.disabled = refreshing;
    const spin = refreshing && el.dataset.act === refreshingAct;
    if (spin && el.dataset.idleLabel === undefined) {
      el.dataset.idleLabel = el.innerHTML;
      el.innerHTML = `${SPINNER}Checking…`;
    } else if (!spin && el.dataset.idleLabel !== undefined) {
      el.innerHTML = el.dataset.idleLabel;
      delete el.dataset.idleLabel;
    }
  }
}

async function refresh(fresh, act) {
  if (refreshing) {
    // The gather in flight may have started before whatever prompted this, so
    // run one more afterwards rather than dropping the request.
    queued = true;
    queuedFresh = queuedFresh || !!fresh;
    return;
  }
  refreshing = true;
  refreshingAct = act || "";
  syncRefreshControls();

  const open = openDetails();
  const focused = focusKey(document.activeElement);
  const wasFocused = document.activeElement;
  try {
    if (fresh) {
      // Sequential on purpose: only the overview's ?fresh=1 drops the caches,
      // and the doctor has to read what that gather refilled rather than race
      // the invalidation and be handed the answer it was meant to replace.
      doctorLoading();
      await loadOverview(true);
      await loadDoctor();
    } else {
      await Promise.all([loadOverview(), loadDoctor()]);
    }
    renderSummary();
  } catch (err) {
    notice("error", `Could not read this machine: ${err.message}`);
  } finally {
    refreshing = false;
    refreshingAct = "";
    clearBusy();
    syncRefreshControls();
    restoreDetails(open);
    restoreFocus(focused, wasFocused);
  }

  if (queued) {
    const again = queuedFresh;
    queued = false;
    queuedFresh = false;
    await refresh(again);
  }
}

/* clipboard */

async function copyText(text) {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    const area = document.createElement("textarea");
    area.value = text;
    area.setAttribute("readonly", "");
    area.style.position = "fixed";
    area.style.opacity = "0";
    document.body.appendChild(area);
    area.select();
    const copied = document.execCommand("copy");
    area.remove();
    return copied;
  }
}

async function flashCopied(button, text) {
  if (!(await copyText(text))) {
    notice("error", "Could not copy. Select the address and copy it by hand.");
    return;
  }
  const original = button.innerHTML;
  button.innerHTML = ICON.check;
  setTimeout(() => {
    button.innerHTML = original;
  }, 1200);
}

/* interactions */

/** backupNote names the backup only when the server made one. */
const backupNote = (backup) =>
  backup ? ` Your previous file is saved at ${backup}.` : "";

function paintCheckbox(button, on) {
  button.setAttribute("aria-checked", String(on));
  button.dataset.state = on ? "checked" : "unchecked";
  button.toggleAttribute("data-checked", on);
  // An SVGElement has no "hidden" property, so this must be the attribute.
  button.querySelector("svg").toggleAttribute("hidden", !on);
}

function toggleAutoUpdate(button) {
  const next = button.getAttribute("aria-checked") !== "true";
  paintCheckbox(button, next);
  button.disabled = true;
  const marketplace = (overview.marketplace || {}).autoUpdateMarketplace;
  return runAction("autoupdate.set", { enabled: next, marketplace })
    .then((result) => {
      notice(
        "info",
        `Automatic updates are ${next ? "on" : "off"}.${backupNote(result.backup)}`,
      );
      return refresh();
    })
    .catch(() => paintCheckbox(button, !next))
    .finally(() => {
      button.disabled = false;
    });
}

function stopped() {
  jobs.forEach(stopPolling);
  document.querySelector("header").hidden = true;
  $("kit-activity").hidden = true;
  $("kit-main").innerHTML = `
    <div data-slot="empty-state" class="flex flex-col items-center gap-[3px] px-4 text-center py-14 rounded-xl bg-card shadow-card">
      <div data-slot="empty-state-title" class="text-ui font-medium">Rise-X Kit has stopped</div>
      <div data-slot="empty-state-description" class="text-xs text-muted-foreground">You can close this tab.</div>
    </div>`;
}

function runFix(button) {
  const name = button.dataset.fix;
  const args = JSON.parse(button.dataset.args || "{}");
  if (name === "npmrc.clean") {
    const detail = button.dataset.detail;
    const question = detail
      ? `Point @rise-x at the public npm registry in ~/.npmrc?\n\n${detail}`
      : "Point @rise-x at the public npm registry in ~/.npmrc?";
    if (!confirm(question)) return undefined;
    args.confirm = true;
  } else if (name === "node.install") {
    if (
      !confirm(
        "Install Node.js? This downloads the current LTS release and may take a few minutes.",
      )
    ) {
      return undefined;
    }
  } else if (name === "mcp.fix") {
    const detail = button.dataset.detail;
    const question = detail
      ? `Update these Rise-X connections to their current address?\n\n${detail}`
      : "Update old Rise-X addresses to their current address?";
    if (!confirm(question)) return undefined;
  }
  return runAction(name, args, button).then((result) => {
    if (!result || result.jobId) return undefined;
    if (name === "autoupdate.set") {
      notice("info", `Automatic updates are on.${backupNote(result.backup)}`);
    } else if (name === "npmrc.clean") {
      const updated = result.updated || [];
      notice(
        "info",
        `Updated ${updated.length} line(s) in ~/.npmrc.${backupNote(result.backup)}`,
      );
      // npmrc.clean is synchronous (no jobId), so the drawer entry is built
      // here instead of via startPolling, to show the masked updated lines.
      jobs.unshift({
        id: `local-${Date.now()}`,
        title: "Clean up ~/.npmrc",
        subtitle: "",
        lines: updated.length ? ["Updated:", ...updated] : [],
        rendered: 0,
        since: 0,
        status: "succeeded",
        startedAt: new Date().toISOString(),
        finishedAt: new Date().toISOString(),
      });
      drawerOpen = true;
      renderJobs();
    }
    return refresh();
  });
}

/** simple wraps an action that posts nothing but an empty body. */
const simple = (action) => (button) => runAction(action, {}, button);

const ACTIONS = {
  refresh: () => refresh(true, "refresh"),
  recheck: () => refresh(true, "recheck"),
  "doctor-again": () => refresh(true, "doctor-again"),
  "notice-dismiss": () => clearNotice(),
  "banner-dismiss": () => {
    overview.reloadHint = false;
    renderBanner();
    // Forget it server-side too, or the next refresh brings it back.
    return runAction("reload-hint.dismiss", {});
  },
  "jobs-toggle": () => {
    drawerOpen = !drawerOpen;
    renderJobs();
  },
  quit: (button) => runAction("quit", {}, button).then(stopped),
  "cli-install": simple("cli.install"),
  "cli-rescan": (button) =>
    runAction("cli.rescan", {}, button).then((result) => {
      notice(
        "info",
        result.found
          ? "Found Claude Code."
          : "Still cannot find Claude Code on this machine.",
      );
      return refresh();
    }),
  "catalog-update": simple("marketplace.update"),
  "marketplace-add": simple("marketplace.add"),
  "mcp-fix": (button) => {
    const { name, scope, project } = button.dataset;
    return runAction(
      "mcp.fix",
      { name, scope, projectPath: project || undefined },
      button,
    );
  },
  autoupdate: (button) => toggleAutoUpdate(button),
  copy: (button) => flashCopied(button, button.dataset.copy),
  skill: (button) => {
    const { op, name, marketplace } = button.dataset;
    if (
      op === "uninstall" &&
      !confirm(
        `Remove ${skillLabel(name)}? Claude Code will no longer have its skills.`,
      )
    ) {
      return undefined;
    }
    return runAction(`plugin.${op}`, { name, marketplace }, button);
  },
  fix: runFix,
  jump: jumped,
};

function onClick(event) {
  const button = event.target.closest("[data-act]");
  if (!button) return;
  const handler = ACTIONS[button.dataset.act];
  if (!handler) return;
  const result = handler(button);
  // runAction rethrows so callers can chain; the notice is already shown.
  if (result && result.catch) result.catch(() => {});
}

document.addEventListener("DOMContentLoaded", () => {
  document.addEventListener("click", onClick);
  renderSummary();
  renderSkeletons();
  renderJobs();
  refresh();
});
