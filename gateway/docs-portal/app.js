// Shared state is initialized before content factories. Only public preferences are persisted.
const endpointOptions = [
  "https://api.psydo.top",
  "https://cn2.ppxcode.com",
  "https://cf.ppxcode.com",
  "https://api.ppx-ai.com",
  "https://us.psydo.top",
];
function readPreference(key) {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}
function savePreference(key, value) {
  try {
    localStorage.setItem(key, value);
  } catch {}
}
const savedEndpoint = readPreference("ppx-docs-endpoint");
const state = {
  baseURL: endpointOptions.includes(savedEndpoint)
    ? savedEndpoint
    : location.hostname === "doc.ppxcode.com"
      ? "https://cn2.ppxcode.com"
      : "https://api.psydo.top",
  os: readPreference("ppx-docs-os") === "unix" ? "unix" : "windows",
};
