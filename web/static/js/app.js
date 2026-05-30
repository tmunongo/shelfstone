/**
 * Shelfstone — app.js
 * Alpine.js component factories + progress sync utilities.
 * All Alpine components are registered globally so Templ can reference them by name.
 */

/* ============================================================
   Player component
   ============================================================ */
function playerApp({ bookID, audioSrc, startPos, chapters, bookAuthor }) {
  return {
    // State
    playing: false,
    currentTime: 0,
    duration: 0,
    speed: 1,
    currentChapterIdx: -1,
    currentChapterTitle: '',
    book: { author: bookAuthor },
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
    },

    // ---- Playback controls ----

    togglePlay() {
      const audio = this.$refs.audio;
      if (!audio) return;
      if (this.playing) {
        audio.pause();
        this._saveProgress();
        this.playing = false;
      } else {
        const promise = audio.play();
        if (promise !== undefined) {
          promise.then(() => { this.playing = true; }).catch(err => {
            console.error('Playback error:', err);
            this.playing = false;
          });
        } else {
          this.playing = true;
        }
      }
    },

    seek(value) {
      const audio = this.$refs.audio;
      if (!audio) return;
      audio.currentTime = parseFloat(value);
      this.currentTime = parseFloat(value);
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
    },

    // ---- Audio element events ----

    onLoaded() {
      const audio = this.$refs.audio;
      this.duration = audio.duration || 0;
      // Seek to saved position once metadata is loaded.
      if (this.currentTime > 0) {
        audio.currentTime = this.currentTime;
      }
    },

    onTimeUpdate() {
      const audio = this.$refs.audio;
      if (!audio) return;
      this.currentTime = audio.currentTime;
      this._updateCurrentChapter();
    },

    onEnded() {
      this.playing = false;
      this._saveProgress(true); // mark completed
    },

    // ---- Chapter tracking ----

    _updateCurrentChapter() {
      if (!this.chapters.length) return;
      for (let i = this.chapters.length - 1; i >= 0; i--) {
        if (this.currentTime >= this.chapters[i].start) {
          if (this.currentChapterIdx !== i) {
            this.currentChapterIdx = i;
            this.currentChapterTitle = this.chapters[i].title;
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
