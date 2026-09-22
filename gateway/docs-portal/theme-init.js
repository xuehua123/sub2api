// Run before the stylesheet paints. Only a public appearance preference is read.
(() => {
  document.documentElement.dataset.enhanced = "true";
  let stored;
  try {
    stored = localStorage.getItem("ppx-theme");
  } catch {}
  document.documentElement.dataset.theme = ["dark", "light"].includes(stored)
    ? stored
    : matchMedia("(prefers-color-scheme: dark)").matches
      ? "dark"
      : "light";
})();
