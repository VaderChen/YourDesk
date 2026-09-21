'use strict';
// Bounded browser-side metadata and 4 KiB reads; native staging owns drag-out.
const YourDeskFiles = (() => {
  const MAX_ITEMS = 64, MAX_BYTES = 2 * 1024 ** 3, MAX_DEPTH = 16, CHUNK = 4096;
  class FileError extends Error { constructor(code) { super(code); this.code = code; } }
  const utf8 = new TextEncoder();
  function component(name) {
    if (typeof name !== 'string' || !name || name === '.' || name === '..' || /[\\/:<>"|?*\x00-\x1f\x7f]/.test(name) || /[. ]$/.test(name) || utf8.encode(name).length > 255 || /^\.yourdesk-transfer-/i.test(name) || /^(CON|PRN|AUX|NUL|COM[0-9]|LPT[0-9])(?:\.|$)/i.test(name)) throw new FileError('invalidName');
    return name;
  }
  function relative(path) {
    if (path === '' || path === '.') return '';
    if (typeof path !== 'string' || utf8.encode(path).length > 1024) throw new FileError('invalidName');
    return path.split('/').map(component).join('/');
  }
  const join = (parent, name) => relative([relative(parent), component(name)].filter(Boolean).join('/'));
  async function collect(items, files, destination, capacity = MAX_ITEMS, remaining = MAX_BYTES) {
    const out = [], paths = new Set(); let bytes = 0;
    const add = (name, directory, file) => {
      const path = relative(name);
      if (out.length >= capacity || out.length >= MAX_ITEMS) throw new FileError('tooMany');
      if (paths.has(path)) throw new FileError('duplicate');
      if (!directory && (!Number.isSafeInteger(file.size) || file.size < 0 || file.size > MAX_BYTES || bytes + file.size > remaining)) throw new FileError('tooLarge');
      paths.add(path); bytes += directory ? 0 : file.size; out.push({path, directory, file, size: directory ? 0 : file.size});
    };
    async function walk(entry, parent, depth) {
      if (depth > MAX_DEPTH) throw new FileError('tooDeep');
      const path = join(parent, entry.name);
      if (entry.isFile) {
        const file = await new Promise((resolve, reject) => entry.file(resolve, reject));
        add(path, false, file);
      } else if (entry.isDirectory && typeof entry.createReader === 'function') {
        add(path, true, null);
        const reader = entry.createReader();
        for (;;) {
          const batch = await new Promise((resolve, reject) => reader.readEntries(resolve, reject));
          if (!batch.length) break;
          if (batch.length > capacity - out.length) throw new FileError('tooMany');
          for (const child of batch) await walk(child, path, depth + 1);
        }
      } else throw new FileError('unsupportedDirectory');
    }
    if (items && items.length > MAX_ITEMS) throw new FileError('tooMany');
    const usable = items ? Array.from(items).filter(item => item.kind === 'file') : [];
    if (usable.length) {
      if (usable.length > capacity) throw new FileError('tooMany');
      // Capture entries during the drop event, before its data store is revoked.
      const roots = usable.map(item => {
        if (typeof item.webkitGetAsEntry === 'function') {
          const entry = item.webkitGetAsEntry();
          if (!entry) throw new FileError('unsupportedDirectory');
          return {entry};
        }
        const file = item.getAsFile();
        if (!file) throw new FileError('unsupportedDirectory');
        return {file};
      });
      for (const root of roots) {
        if (root.entry) await walk(root.entry, destination, 1);
        else { await root.file.slice(0, 1).arrayBuffer(); add(join(destination, root.file.name), false, root.file); }
      }
    } else {
      if (!files || files.length > capacity) throw new FileError('tooMany');
      for (const file of files) {
        // Folder pickers are intentionally unsupported here; never flatten paths.
        if (file.webkitRelativePath) throw new FileError('unsupportedDirectory');
        await file.slice(0, 1).arrayBuffer(); add(join(destination, file.name), false, file);
      }
    }
    return out;
  }

  function transfer(item) {
    return item.transfer ||= {id:'', token:'', offset:0, started:false, acknowledged:false, complete:false};
  }
  function confirmed(out, size) {
    if (!out || !Number.isSafeInteger(out.nextOffset) || out.nextOffset < 0 || out.nextOffset > size ||
        !['uploading','complete'].includes(out.state) || (out.state === 'complete' && out.nextOffset !== size)) throw new FileError('invalidReply');
    return out;
  }
  function newToken() {
    const bytes = new Uint8Array(32); crypto.getRandomValues(bytes);
    return Array.from(bytes,value => value.toString(16).padStart(2,'0')).join('');
  }
  async function upload(item, call, progress, control) {
    const record = transfer(item);
    const check = () => {
      if (control.cancelled) throw new FileError('cancelled');
      if (control.interrupted) throw new FileError('interrupted');
      if (control.paused) throw new FileError('paused');
    };
    check();
    if (record.complete) { progress(item.size,item.size); return; }
    if (item.directory) {
      // mkdir has no ownership receipt. An uncertain reply must not merge into
      // an existing folder on retry, nor release its dependent file uploads.
      if (record.started) throw new FileError('unknownDirectory');
      record.started = true;
      let out;
      try { out = await call('mkdir',{path:item.path}); }
      catch { throw new FileError('unknownDirectory'); }
      if (out.ok !== true) throw new FileError('unknownDirectory');
      record.complete = true; return;
    }
    if (record.acknowledged) {
      const out = confirmed(await call('resume',{id:record.id,token:record.token,path:item.path,size:item.size}),item.size);
      record.offset = out.nextOffset; progress(record.offset,item.size);
      if (out.state === 'complete') {record.complete = true; return;}
      check();
    } else {
      if (!record.started) {record.token = newToken(); record.id = record.token.slice(0,32);}
      record.started = true;
      const out = confirmed(await call('begin',{path:item.path,size:item.size,token:record.token}),item.size);
      if (out.id !== record.id || out.resumeToken !== record.token) throw new FileError('invalidReply');
      record.acknowledged = true;
      record.offset = out.nextOffset; progress(record.offset,item.size);
      if (out.state === 'complete') {record.complete = true; return;}
      check();
    }
    while (record.offset < item.size) {
      check();
      const offset = record.offset;
      const bytes = new Uint8Array(await item.file.slice(offset,Math.min(offset + CHUNK,item.size)).arrayBuffer());
      check();
      if (!bytes.length || bytes.length > CHUNK || offset + bytes.length > item.size) throw new FileError('readFailed');
      const out = await call('write',{id:record.id,offset,data:btoa(String.fromCharCode(...bytes))});
      if (out.nextOffset !== offset + bytes.length) throw new FileError('invalidReply');
      record.offset = out.nextOffset; progress(record.offset,item.size);
    }
    check();
    const out = await call('commit',{id:record.id});
    if (out.ok !== true) throw new FileError('invalidReply');
    record.complete = true; progress(item.size,item.size);
  }
  async function cancelUpload(item, call) {
    const record = transfer(item);
    if (record.complete) return {complete:true};
    if (!record.id) return {complete:false,unknown:record.started && !item.directory};
    // Cancellation proves ownership without requiring the partial file or its
    // destination to remain resumable. Completed receipts make ACK-loss safe.
    const out = await call('cancel',{id:record.id,token:record.token,path:item.path,size:item.size});
    if (out.ok !== true || !['complete','cancelled'].includes(out.state)) throw new FileError('invalidReply');
    if (out.state === 'complete') {record.complete = true;record.offset = item.size;}
    return {complete:record.complete};
  }
  const contains = (parent, child) => child === parent || child.startsWith(parent + '/');
  const percent = (received,total,complete=false) => total > 0 ? Math.min(100,Math.floor(received / total * 100)) : complete ? 100 : 0;
  return {MAX_ITEMS, MAX_BYTES, MAX_DEPTH, CHUNK, FileError, component, relative, join, collect, transfer, upload, cancelUpload, contains, percent};
})();
if (typeof module !== 'undefined' && module.exports) module.exports = YourDeskFiles;
