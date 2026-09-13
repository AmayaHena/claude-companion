// claude-companion site. Three behaviours, nothing else:
// 1. the terminal rows get an index so CSS can stagger their arrival;
// 2. the install command copies to the clipboard with visible feedback;
// 3. sections and cells reveal once when they enter the viewport.
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

  // 3. reveal once. The hidden state is armed here, never in CSS alone, and a
  //    timer releases anything the observer has not reached, so no element
  //    can stay invisible.
  const targets = document.querySelectorAll(".reveal");
  if (reduce || !("IntersectionObserver" in window)) return;
  targets.forEach((el) => el.classList.add("pre"));
  const show = (el) => el.classList.add("in");
  const io = new IntersectionObserver((entries) => {
    for (const e of entries) {
      if (e.isIntersecting) { show(e.target); io.unobserve(e.target); }
    }
  }, { rootMargin: "0px 0px -8% 0px", threshold: 0.15 });
  targets.forEach((el) => io.observe(el));
  setTimeout(() => { targets.forEach(show); io.disconnect(); }, 2500);
})();
