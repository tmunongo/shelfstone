/**
 * Shelfstone — app.js
 * Alpine.js component factories + progress sync utilities.
 * All Alpine components are registered globally so Templ can reference them by name.
 */

/* ============================================================
   Player component
   ============================================================ */
function playerApp({ bookID, audioSrc, startPos, chapters, bookAuthor, bookTitle, coverPath }) {
  return {
    // State
    playing: false,
    currentTime: 0,
    duration: 0,
    speed: 1,
    currentChapterIdx: -1,
    currentChapterTitle: '',
    book: { author: bookAuthor, title: bookTitle, coverPath: coverPath },
    audioSrc: '',        // resolved in init()
    chapters,

    // Sync housekeeping
    _syncTimer: null,
    _localStorageKey: `shelfstone_pos_${bookID}`,

    async init() {
      // Resolve the actual audio file URL.
      // For M4B/single-file: audioSrc ends with '/', we just need the first audio file.
      this.audioSrc = await this._resolveAudioSrc(audioSrc);

      // Restore position: LocalStorage vs server; pick the newest.
      const localData = this._readLocal();
      if (localData) {
        const serverData = await this._fetchServerProgress(localData.updatedAt);
        if (serverData && !serverData.use_local) {
          this.currentTime = serverData.position_sec || 0;
        } else {
          this.currentTime = localData.position_sec || startPos;
        }
      } else {
        this.currentTime = startPos;
      }

      // Start periodic sync every 10 seconds while playing.
      this._syncTimer = setInterval(() => {
        if (this.playing) this._saveProgress();
      }, 10_000);

      // Save on page hide (tab close, navigation, etc.)
      document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'hidden') this._saveProgress();
      });

      // Media Session registration
      this._setupMediaSessionActions();
      this._updateMediaSession();
    },

    // ---- Playback controls ----

    togglePlay() {
      const audio = this.$refs.audio;
      if (!audio) return;
      if (this.playing) {
        audio.pause();
        this._saveProgress();
        this.playing = false;
        this._updateMediaSession();
      } else {
        const promise = audio.play();
        if (promise !== undefined) {
          promise.then(() => {
            this.playing = true;
            this._updateMediaSession();
          }).catch(err => {
            console.error('Playback error:', err);
            this.playing = false;
            this._updateMediaSession();
          });
        } else {
          this.playing = true;
          this._updateMediaSession();
        }
      }
    },

    seek(value) {
      const audio = this.$refs.audio;
      if (!audio) return;
      audio.currentTime = parseFloat(value);
      this.currentTime = parseFloat(value);
      this._updateMediaSession();
    },

    seekToChapter(startSec) {
      this.seek(startSec);
      if (!this.playing) this.togglePlay();
    },

    skipForward() {
      this.seek(Math.min(this.currentTime + 30, this.duration));
    },

    skipBack() {
      this.seek(Math.max(this.currentTime - 30, 0));
    },

    setSpeed(s) {
      const audio = this.$refs.audio;
      this.speed = s;
      if (audio) audio.playbackRate = s;
      this._updateMediaSession();
    },

    // ---- Audio element events ----

    onLoaded() {
      const audio = this.$refs.audio;
      this.duration = audio.duration || 0;
      // Seek to saved position once metadata is loaded.
      if (this.currentTime > 0) {
        audio.currentTime = this.currentTime;
      }
      this._updateMediaSession();
    },

    onTimeUpdate() {
      const audio = this.$refs.audio;
      if (!audio) return;
      this.currentTime = audio.currentTime;
      this._updateCurrentChapter();

      // Update position timeline on the OS lockscreen dynamically
      if ('mediaSession' in navigator && 'setPositionState' in navigator.mediaSession) {
        if (this.duration > 0) {
          try {
            navigator.mediaSession.setPositionState({
              duration: this.duration,
              playbackRate: this.speed || 1,
              position: this.currentTime
            });
          } catch (e) {
            console.warn('Error setting Media Session position:', e);
          }
        }
      }
    },

    onEnded() {
      this.playing = false;
      this._saveProgress(true); // mark completed
      this._updateMediaSession();
    },

    // ---- Chapter tracking ----

    _updateCurrentChapter() {
      if (!this.chapters.length) return;
      for (let i = this.chapters.length - 1; i >= 0; i--) {
        if (this.currentTime >= this.chapters[i].start) {
          if (this.currentChapterIdx !== i) {
            this.currentChapterIdx = i;
            this.currentChapterTitle = this.chapters[i].title;
            this._updateMediaSession(); // Trigger lockscreen metadata title update!
          }
          return;
        }
      }
    },

    // ---- Progress persistence ----

    _readLocal() {
      try {
        const raw = localStorage.getItem(this._localStorageKey);
        return raw ? JSON.parse(raw) : null;
      } catch { return null; }
    },

    _writeLocal(positionSec) {
      try {
        localStorage.setItem(this._localStorageKey, JSON.stringify({
          position_sec: positionSec,
          updatedAt: new Date().toISOString(),
        }));
      } catch { /* storage quota */ }
    },

    async _fetchServerProgress(localUpdatedAt) {
      try {
        const url = `/api/progress/${bookID}?client_time=${encodeURIComponent(localUpdatedAt || '')}`;
        const resp = await fetch(url);
        if (!resp.ok) return null;
        return await resp.json();
      } catch { return null; }
    },

    async _saveProgress(completed = false) {
      const pos = this.currentTime;
      this._writeLocal(pos);

      try {
        await fetch(`/api/progress/${bookID}`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            position_sec: pos,
            completed,
            client_time: new Date().toISOString(),
          }),
        });
      } catch { /* offline — LocalStorage is the fallback */ }
    },

    // ---- Helpers ----

    fmtTime(secs) {
      if (!secs || isNaN(secs)) return '0:00';
      const h = Math.floor(secs / 3600);
      const m = Math.floor((secs % 3600) / 60);
      const s = Math.floor(secs % 60);
      if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
      return `${m}:${String(s).padStart(2, '0')}`;
    },

    async _resolveAudioSrc(baseUrl) {
      // If the URL ends with '/', resolve via the book-files API.
      if (!baseUrl.endsWith('/')) return baseUrl;

      try {
        const resp = await fetch(`/api/book-files/${bookID}`);
        if (resp.ok) {
          const data = await resp.json();
          if (data.files && data.files.length > 0) {
            // Encode each path segment so spaces/parens in filenames work.
            const encoded = data.files[0]
              .split('/')
              .map(seg => encodeURIComponent(seg))
              .join('/');
            return `/media/${encoded}`;
          }
        }
      } catch { /* fall through */ }

      return baseUrl;
    },

    _updateMediaSession() {
      if (!('mediaSession' in navigator)) return;

      const artworkUrl = this.book.coverPath 
        ? `/media/${this.book.coverPath}` 
        : '/static/images/logo.png';

      try {
        navigator.mediaSession.metadata = new MediaMetadata({
          title: this.currentChapterTitle || this.book.title || 'Listening...',
          artist: this.book.author || 'Shelfstone',
          album: this.book.title || 'Audiobook',
          artwork: [
            { src: artworkUrl, sizes: '512x512', type: 'image/png' }
          ]
        });

        navigator.mediaSession.playbackState = this.playing ? 'playing' : 'paused';
      } catch (e) {
        console.warn('Error updating Media Session metadata:', e);
      }
    },

    _setupMediaSessionActions() {
      if (!('mediaSession' in navigator)) return;

      const actionHandlers = [
        ['play', () => this.togglePlay()],
        ['pause', () => this.togglePlay()],
        ['seekbackward', () => this.skipBack()],
        ['seekforward', () => this.skipForward()],
        ['previoustrack', () => {
          // If we are more than 5s into a chapter, restart it; otherwise go to previous chapter
          const activeCh = this.chapters[this.currentChapterIdx];
          const activeChStart = activeCh ? activeCh.start : 0;
          if (this.currentTime - activeChStart > 5) {
            this.seek(activeChStart);
          } else if (this.currentChapterIdx > 0) {
            this.seekToChapter(this.chapters[this.currentChapterIdx - 1].start);
          } else {
            this.seek(0);
          }
        }],
        ['nexttrack', () => {
          if (this.currentChapterIdx >= 0 && this.currentChapterIdx < this.chapters.length - 1) {
            this.seekToChapter(this.chapters[this.currentChapterIdx + 1].start);
          }
        }]
      ];

      for (const [action, handler] of actionHandlers) {
        try {
          navigator.mediaSession.setActionHandler(action, handler);
        } catch (error) {
          console.warn(`Media Session action "${action}" not supported:`, error);
        }
      }
    },
  };
}

/* ============================================================
   Note editor component
   ============================================================ */
function noteEditor(bookID) {
  return {
    newNote: '',

    async saveNote() {
      const body = this.newNote.trim();
      if (!body) return;

      const resp = await fetch('/api/notes', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ audiobook_id: bookID, body }),
      });

      if (resp.ok) {
        // Simple page reload to show the new note.
        // A future version can inject the note DOM without a reload.
        this.newNote = '';
        window.location.reload();
      }
    },
  };
}

async function deleteNote(noteID) {
  if (!confirm('Delete this note?')) return;
  const resp = await fetch(`/api/notes/${noteID}`, { method: 'DELETE' });
  if (resp.ok) {
    const el = document.getElementById(`note-${noteID}`);
    if (el) el.remove();
  }
}

/* ============================================================
   Tag editor component
   ============================================================ */
function tagEditor({ bookID, initialTags }) {
  return {
    tags: initialTags || [],
    newTag: '',

    init() {
      // Tags are seeded directly from Templ parameters.
    },

    addTag() {
      const name = this.newTag.trim().toLowerCase();
      if (!name || this.tags.includes(name)) return;
      this.tags.push(name);
      this.newTag = '';
      this._sync();
    },

    removeTag(name) {
      this.tags = this.tags.filter(t => t !== name);
      this._sync();
    },

    async _sync() {
      await fetch(`/api/book/${bookID}/tags`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ tags: this.tags }),
      });
    },
  };
}

/* ============================================================
   PWA Install component
   ============================================================ */
function pwaInstall() {
  return {
    deferredPrompt: null,
    showInstallBtn: false,

    init() {
      window.addEventListener('beforeinstallprompt', (e) => {
        // Prevent Chrome 67 and earlier from automatically showing the prompt
        e.preventDefault();
        // Stash the event so it can be triggered later.
        this.deferredPrompt = e;
        // Update UI notify the user they can install the PWA
        this.showInstallBtn = true;
      });

      window.addEventListener('appinstalled', () => {
        this.showInstallBtn = false;
        this.deferredPrompt = null;
        console.log('Shelfstone was installed successfully!');
      });
      
      // Handle already installed app detection or display stand-alone mode checks
      if (window.matchMedia('(display-mode: standalone)').matches || window.navigator.standalone === true) {
        this.showInstallBtn = false;
      }
    },

    async installApp() {
      if (!this.deferredPrompt) return;
      this.deferredPrompt.prompt();
      const { outcome } = await this.deferredPrompt.userChoice;
      console.log(`User response to the install prompt: ${outcome}`);
      this.deferredPrompt = null;
      this.showInstallBtn = false;
    }
  };
}

/* ============================================================
   Offline Book Manager component
   ============================================================ */
function offlineBook(bookID) {
  return {
    bookID,
    downloaded: false,
    downloading: false,
    progress: 0,
    filesCount: 0,
    cachedFilesCount: 0,

    async init() {
      await this.checkCacheStatus();
    },

    async checkCacheStatus() {
      try {
        const files = await this._getBookFiles();
        if (!files || files.length === 0) {
          this.downloaded = false;
          return;
        }
        
        this.filesCount = files.length;
        const cache = await caches.open('shelfstone-static-v1');
        let cachedCount = 0;
        
        for (const file of files) {
          const match = await cache.match(file);
          if (match) cachedCount++;
        }
        
        this.cachedFilesCount = cachedCount;
        this.downloaded = (cachedCount === files.length && files.length > 0);
      } catch (e) {
        console.error('Error checking offline cache status:', e);
      }
    },

    async download() {
      if (this.downloaded || this.downloading) return;
      this.downloading = true;
      this.progress = 0;

      try {
        const files = await this._getBookFiles();
        if (!files || files.length === 0) {
          alert('Could not find any files to download for this audiobook.');
          this.downloading = false;
          return;
        }

        const cache = await caches.open('shelfstone-static-v1');
        let downloadedCount = 0;

        for (const url of files) {
          try {
            const resp = await fetch(url);
            if (resp.ok) {
              await cache.put(url, resp);
              downloadedCount++;
              this.progress = Math.round((downloadedCount / files.length) * 100);
            } else {
              throw new Error(`Failed to fetch file: ${resp.statusText}`);
            }
          } catch (fetchErr) {
            console.error(`Error downloading fragment: ${url}`, fetchErr);
          }
        }

        await this.checkCacheStatus();
      } catch (err) {
        console.error('Offline download failed:', err);
        alert('An error occurred while downloading the audiobook.');
      } finally {
        this.downloading = false;
      }
    },

    async removeOffline() {
      if (!confirm('Are you sure you want to remove the downloaded files for this audiobook? This will free up storage space.')) return;
      
      try {
        const files = await this._getBookFiles();
        const cache = await caches.open('shelfstone-static-v1');
        
        for (const url of files) {
          await cache.delete(url);
        }
        
        this.downloaded = false;
        this.progress = 0;
        this.cachedFilesCount = 0;
      } catch (err) {
        console.error('Failed to remove cached files:', err);
      }
    },

    // ---- Private Helpers ----

    async _getBookFiles() {
      try {
        const resp = await fetch(`/api/book-files/${this.bookID}`);
        if (!resp.ok) return [];
        const data = await resp.json();
        if (!data.files || data.files.length === 0) return [];
        
        return data.files.map(file => {
          // Exactly matches app.js _resolveAudioSrc encoder logic
          const encoded = file.split('/')
            .map(seg => encodeURIComponent(seg))
            .join('/');
          return `/media/${encoded}`;
        });
      } catch (e) {
        console.error('Error in _getBookFiles:', e);
        return [];
      }
    }
  };
}
