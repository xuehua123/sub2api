(() => {
  let value;
  try {
    value = localStorage.getItem("ppx-theme");
  } catch {}
  document.documentElement.dataset.theme = ["light", "dark"].includes(value)
    ? value
    : "dark";
})();
