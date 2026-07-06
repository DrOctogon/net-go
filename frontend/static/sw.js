/* global self */
// Minimal service worker for PWA install support.
// This enables the browser's "Add to Home Screen" / "Install App" prompt.
// No offline caching — VoiceWatch runs on a local network.
self.addEventListener('install', event => event.waitUntil(self.skipWaiting()));
self.addEventListener('activate', event => event.waitUntil(self.clients.claim()));
