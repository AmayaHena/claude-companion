// claude-companion site. Two behaviours, nothing else:
// 1. the terminal rows get an index so CSS can stagger their arrival;
// 2. the install command copies to the clipboard with visible feedback;
// No scroll listeners, no libraries, reduced motion handled in CSS.
(() => {
  "use strict";
  const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  // 1. arrival order for the live render
  document.querySelectorAll(".term .row").forEach((row, i) => row.style.setProperty("--i", String(i)));
  const term = document.querySelector(".term");
  if (term && !reduce) {
    term.classList.add("live");
    // keep the newest row in view while the rows arrive, like the tool does
    const rows = term.querySelectorAll(".row");
    const last = rows[rows.length - 1];
    if (last) {
      const total = 250 + rows.length * 55 + 420;
      const tick = setInterval(() => { term.scrollTop = term.scrollHeight; }, 120);
      setTimeout(() => clearInterval(tick), total);
    }
  }

  // 2. copy the install command
  const btn = document.querySelector("[data-copy]");
  if (btn) {
    const text = document.querySelector(btn.dataset.copy)?.textContent.trim() ?? "";
    const label = btn.textContent;
    let timer = 0;
    btn.addEventListener("click", async () => {
      try {
        await navigator.clipboard.writeText(text);
        btn.dataset.state = "copied";
        btn.textContent = "Copied";
      } catch {
        btn.dataset.state = "failed";
        btn.textContent = "Select it";
      }
      clearTimeout(timer);
      timer = setTimeout(() => { delete btn.dataset.state; btn.textContent = label; }, 1600);
    });
  }
})();
