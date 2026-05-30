/**
 * Shelfstone — sw.js
 * Service Worker for offline shell caching, stale-while-revalidate for assets,
 * and elegant offline pages.
 */

const CACHE_NAME = 'shelfstone-static-v1';
const DYNAMIC_CACHE_NAME = 'shelfstone-dynamic-v1';

// Assets to cache immediately on SW install
const STATIC_ASSETS = [
  '/',
  '/offline',
  '/static/css/main.css',
  '/static/js/app.js',
  '/static/js/alpine.min.js',
  '/static/images/logo.png',
  '/static/images/icon-192.png',
  '/static/images/icon-512.png',
  '/static/images/apple-touch-icon.png',
  '/static/images/favicon-32.png',
  '/static/images/favicon-16.png',
  'https://fonts.googleapis.com/css2?family=Outfit:wght@300;400;500;600;700;800&display=swap',
  'https://fonts.gstatic.com/s/outfit/v11/QId5dD5E65SkV2r7K3A7.woff2' // Cache core Outfit font file if possible
];

// Install Event - cache core app shell
self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => {
      console.log('[Service Worker] Pre-caching app shell');
      return cache.addAll(STATIC_ASSETS);
    }).then(() => self.skipWaiting())
  );
});

// Activate Event - clean up old caches
self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) => {
      return Promise.all(
        keys.map((key) => {
          if (key !== CACHE_NAME && key !== DYNAMIC_CACHE_NAME && !key.startsWith('shelfstone-audio-')) {
            console.log('[Service Worker] Removing old cache:', key);
            return caches.delete(key);
          }
        })
      );
    }).then(() => self.clients.claim())
  );
});

// Helper to serve cached media responses in ranges (required for audio seeking/scrubbing compatibility)
function serveRangeRequest(request, cachedResponse) {
  const rangeHeader = request.headers.get('Range');
  if (!rangeHeader) {
    return cachedResponse;
  }

  return cachedResponse.arrayBuffer().then((arrayBuffer) => {
    const bytes = rangeHeader.match(/^bytes=(\d+)-(\d+)?$/);
    if (!bytes) {
      return new Response(arrayBuffer, {
        status: 206,
        statusText: 'Partial Content',
        headers: cachedResponse.headers
      });
    }

    const totalLength = arrayBuffer.byteLength;
    const start = parseInt(bytes[1], 10);
    const end = bytes[2] ? parseInt(bytes[2], 10) : totalLength - 1;

    // Slice the buffer according to the range requested
    const slicedBuffer = arrayBuffer.slice(start, end + 1);
    
    // Create correct headers for 206 Partial Content response
    const headers = new Headers(cachedResponse.headers);
    headers.set('Content-Range', `bytes ${start}-${end}/${totalLength}`);
    headers.set('Content-Length', slicedBuffer.byteLength);

    return new Response(slicedBuffer, {
      status: 206,
      statusText: 'Partial Content',
      headers: headers
    });
  });
}

// Fetch Event - routing strategies
self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url);

  // Skip non-GET requests or requests to APIs/auth
  if (event.request.method !== 'GET' || url.pathname.startsWith('/api/') || url.pathname === '/login' || url.pathname === '/logout') {
    return;
  }

  // Handle media requests (Range requests) - served from cache if offline downloaded
  if (url.pathname.startsWith('/media/')) {
    event.respondWith(
      caches.match(event.request).then((cachedResponse) => {
        if (cachedResponse) {
          console.log('[Service Worker] Serving cached media range request:', url.pathname);
          return serveRangeRequest(event.request, cachedResponse);
        }
        // If not cached, fetch from network normally
        return fetch(event.request);
      })
    );
    return;
  }

  // Page Navigations (HTML) - Network-first, fallback to cached offline page
  if (event.request.mode === 'navigate') {
    event.respondWith(
      fetch(event.request)
        .then((response) => {
          // Put page in dynamic cache for offline browsing
          const copy = response.clone();
          caches.open(DYNAMIC_CACHE_NAME).then((cache) => cache.put(event.request, copy));
          return response;
        })
        .catch(() => {
          // If offline, try dynamic cache first, otherwise serve the offline template
          return caches.match(event.request).then((cachedResponse) => {
            return cachedResponse || caches.match('/offline');
          });
        })
    );
    return;
  }

  // Static Assets (CSS, JS, Fonts, Images) - Stale-while-revalidate
  event.respondWith(
    caches.match(event.request).then((cachedResponse) => {
      if (cachedResponse) {
        // Fetch fresh in background and update cache
        fetch(event.request)
          .then((networkResponse) => {
            if (networkResponse.status === 200) {
              caches.open(CACHE_NAME).then((cache) => cache.put(event.request, networkResponse));
            }
          })
          .catch(() => {/* ignore background fetch failure */});
        return cachedResponse;
      }

      // If not in cache, fetch from network
      return fetch(event.request).then((networkResponse) => {
        if (networkResponse.status === 200) {
          const copy = networkResponse.clone();
          caches.open(CACHE_NAME).then((cache) => cache.put(event.request, copy));
        }
        return networkResponse;
      });
    })
  );
});
