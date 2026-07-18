// Caches the app shell (not file data) so installing Nuage as a PWA gets
// an instant-loading icon on the home screen instead of a browser tab.
// Bump CACHE_NAME whenever the shell files change meaningfully, since
// there's no build-time cache-busting here — stale-while-revalidate below
// limits the damage of forgetting to (next load always re-fetches and
// updates the cache in the background).
const CACHE_NAME = "nuage-shell-v1";
const SHELL_ASSETS = ["/", "/app.js", "/style.css", "/manifest.webmanifest"];

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => cache.addAll(SHELL_ASSETS))
  );
  self.skipWaiting();
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k)))
    )
  );
  self.clients.claim();
});

self.addEventListener("fetch", (event) => {
  const url = new URL(event.request.url);

  // Never cache the API — file listings, uploads, downloads, and auth
  // must always hit the network.
  if (url.pathname.startsWith("/api/")) return;
  if (event.request.method !== "GET") return;

  event.respondWith(
    caches.open(CACHE_NAME).then(async (cache) => {
      const cached = await cache.match(event.request);
      const network = fetch(event.request)
        .then((res) => {
          if (res.ok) cache.put(event.request, res.clone());
          return res;
        })
        .catch(() => cached);
      return cached || network;
    })
  );
});
